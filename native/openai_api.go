package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
)

const apiUncertain = "API 已提交，结果未确认。请检查 OpenAI 用量记录；此任务不会自动重发。"

var errAPIKeyMissing = errors.New("未配置 API Key")

type openAIClient struct {
	base     string
	http     *http.Client
	secrets  secretStore
	secretID string
}

func newOpenAIClient(dir string) *openAIClient {
	return &openAIClient{base: "https://api.openai.com/v1", http: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, secrets: mygo.Secrets.Service("app.canvepp.native"), secretID: "openai.api." + digest([]byte(dir))[:16]}
}
func (c *openAIClient) key() (string, error) {
	b, err := c.secrets.Get(c.secretID)
	if errors.Is(err, mygo.ErrSecretNotFound) || (err == nil && len(b) == 0) {
		return "", errAPIKeyMissing
	}
	if err != nil {
		return "", errors.New("无法读取系统凭据库")
	}
	return string(b), nil
}

var errModelListPermission = errors.New("API Key 缺少模型列表读取权限")

type apiFailure int

func (e apiFailure) Error() string {
	switch int(e) {
	case 400:
		return "OpenAI 拒绝了请求，请检查模型、图片尺寸或输入内容（400）"
	case 401:
		return "API Key 无效或已撤销（401）"
	case 403:
		return "当前 API 项目缺少权限，请检查密钥权限和组织验证（403）"
	case 404:
		return "当前 API 项目无法使用该模型或接口（404）"
	case 429:
		return "OpenAI API 额度不足或达到速率限制（429）"
	}
	return fmt.Sprintf("OpenAI API 请求失败（HTTP %d）", int(e))
}
func (c *openAIClient) request(ctx context.Context, key, method, path, kind string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	if kind != "" {
		req.Header.Set("Content-Type", kind)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, errors.New("无法连接 OpenAI API，请检查网络")
	}
	if resp.StatusCode/100 != 2 {
		defer resp.Body.Close()
		if path == "/models" && (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized) {
			var failure struct {
				Error struct{ Code, Message string }
			}
			if json.NewDecoder(io.LimitReader(resp.Body, 8192)).Decode(&failure) == nil {
				msg := strings.ToLower(failure.Error.Message)
				if failure.Error.Code == "insufficient_permissions" || strings.Contains(msg, "missing scopes:") {
					return nil, errModelListPermission
				}
			}
		}
		return nil, apiFailure(resp.StatusCode)
	}
	return resp, nil
}
func (c *openAIClient) verify(ctx context.Context, key string) (string, error) {
	if key == "" || len(key) > 2048 || strings.ContainsAny(key, " \t\r\n") {
		return "", errors.New("API Key 格式无效")
	}
	resp, err := c.request(ctx, key, "GET", "/models", "", nil)
	// Restricted keys can call Images or Responses without models.list access.
	if errors.Is(err, errModelListPermission) {
		return "已配置 · 模型列表无权限", nil
	}
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var result struct{ Object string }
	if err = json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&result); err != nil || result.Object != "list" {
		return "", errors.New("OpenAI 未返回有效的模型列表")
	}
	return "已验证", nil
}
func (c *openAIClient) summary(ctx context.Context, model modelChoice, events string) (textReply, error) {
	key, err := c.key()
	if err != nil {
		return textReply{}, err
	}
	payload := encode(map[string]any{"model": model.Model, "reasoning": map[string]string{"effort": model.Effort}, "instructions": logInstructions, "input": events, "store": false})
	resp, err := c.request(ctx, key, "POST", "/responses", "application/json", bytes.NewReader(payload))
	if err != nil {
		return textReply{}, err
	}
	defer resp.Body.Close()
	var result struct {
		Status string
		Output []struct {
			Type    string
			Content []struct{ Type, Text string }
		}
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&result) != nil || result.Status != "completed" {
		return textReply{}, errors.New("API 未返回完整摘要")
	}
	var parts []string
	for _, o := range result.Output {
		if o.Type == "message" {
			for _, p := range o.Content {
				if p.Type == "output_text" {
					parts = append(parts, p.Text)
				}
			}
		}
	}
	out := strings.TrimSpace(strings.Join(parts, "\n"))
	if out == "" {
		return textReply{}, errors.New("API 摘要为空")
	}
	return textReply{Text: out}, nil
}

// Preserve the canvas dimensions as intent for auto sizing when an exact
// request lies outside the model's supported sizes.
func apiSize(raw string) string {
	x, y, ok := strings.Cut(raw, "x")
	w, e1 := strconv.Atoi(x)
	h, e2 := strconv.Atoi(y)
	if !ok || e1 != nil || e2 != nil || w <= 0 || h <= 0 || w > 3840 || h > 3840 || w%16 != 0 || h%16 != 0 || w*h < 655360 || w*h > 8294400 || max(w, h) > 3*min(w, h) {
		return "auto"
	}
	return raw
}
func prepareAPIImage(s *Store, r NativeRun) (path, kind string, body io.Reader, cleanup func(), err error) {
	prompt := r.Prompt
	if r.Params.Size != "" && r.Params.Size != "auto" {
		prompt += "\n目标画幅：" + r.Params.Size + " px。"
	}
	size := apiSize(r.Params.Size)
	model := r.Model
	if model == "" {
		model = apiImageModel
	}
	fields := map[string]string{"model": model, "prompt": prompt, "size": size, "quality": r.Params.Quality}
	if fields["quality"] == "" {
		fields["quality"] = "auto"
	}
	if len(r.Sources) == 0 {
		return "/images/generations", "application/json", bytes.NewReader(encode(fields)), func() {}, nil
	}
	// Spool one multipart body to an owner-only temporary file. This bounds
	// memory and validates every original before submitting a billable request.
	f, err := os.CreateTemp("", "canvepp-edit-")
	if err != nil {
		return "", "", nil, nil, err
	}
	remove := func() { f.Close(); os.Remove(f.Name()) }
	defer func() {
		if err != nil {
			remove()
		}
	}()
	m := multipart.NewWriter(f)
	for k, v := range fields {
		if err = m.WriteField(k, v); err != nil {
			return
		}
	}
	for _, asset := range r.Sources {
		var src string
		src, err = s.AssetPath(asset)
		if err != nil {
			return
		}
		var data []byte
		data, err = readBounded(src, maxImageBytes)
		if err != nil {
			return
		}
		if digest(data) != asset.SHA {
			err = errors.New("参考图片校验失败")
			return
		}
		var part io.Writer
		part, err = m.CreatePart(textproto.MIMEHeader{
			"Content-Disposition": {mime.FormatMediaType("form-data", map[string]string{"name": "image[]", "filename": filepath.Base(src)})},
			"Content-Type":        {asset.Mime},
		})
		if err != nil {
			return
		}
		if _, err = part.Write(data); err != nil {
			return
		}
	}
	if err = m.Close(); err != nil {
		return
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return
	}
	return "/images/edits", m.FormDataContentType(), f, remove, nil
}
func executeAPIImage(ctx context.Context, s *Store, r NativeRun, notify func(NativeRun, *Asset), update func(string, string, *Asset)) {
	key, err := s.openAI.key()
	if err != nil {
		update("failed", err.Error(), nil)
		return
	}
	path, kind, body, cleanup, err := prepareAPIImage(s, r)
	if err != nil {
		update("failed", err.Error(), nil)
		return
	}
	defer cleanup()
	current, claimed, err := s.advanceRun(r.ID, "preparing", "submitted")
	if err != nil {
		update("failed", err.Error(), nil)
		return
	}
	if current.Cancelled {
		update("cancelled", "", nil)
		return
	}
	if !claimed {
		notify(current, nil)
		return
	}
	notify(current, nil)
	resp, err := s.openAI.request(ctx, key, "POST", path, kind, body)
	if err != nil {
		var status apiFailure
		if errors.As(err, &status) && status >= 400 && status < 500 {
			update("failed", err.Error(), nil)
		} else {
			update("unknown", apiUncertain, nil)
		}
		return
	}
	defer resp.Body.Close()
	var result struct {
		Data []struct {
			Image string `json:"b64_json"`
		}
	}
	if json.NewDecoder(io.LimitReader(resp.Body, (maxImageBytes+2)/3*4+65536)).Decode(&result) != nil || len(result.Data) != 1 {
		update("unknown", apiUncertain, nil)
		return
	}
	raw, err := base64.StdEncoding.DecodeString(result.Data[0].Image)
	if err != nil || len(raw) == 0 {
		update("unknown", apiUncertain, nil)
		return
	}
	var parents []string
	for _, a := range r.Sources {
		parents = append(parents, a.ID)
	}
	img, err := s.ImportImage(raw, r.ProjectID, r.NodeID, "生成结果.png", "api", r.ID, parents)
	if err != nil {
		update("unknown", "API 已返回图片，保存失败："+err.Error(), nil)
		return
	}
	update("succeeded", "", &img)
}
