package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type eagleClient struct {
	Base          string
	HTTP          *http.Client
	V2            bool
	Library, Name string
}
type eagleItem struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Ext     string   `json:"ext"`
	URL     string   `json:"url"`
	Tags    []string `json:"tags"`
	Deleted bool     `json:"isDeleted"`
}
type eaglePage struct {
	Items  []eagleItem
	Offset int
	More   bool
}
type eagleHTTPError int

func (e eagleHTTPError) Error() string { return fmt.Sprintf("Eagle 接口返回 %d", e) }
func newEagleClient() *eagleClient {
	return &eagleClient{Base: "http://127.0.0.1:41595", HTTP: &http.Client{Timeout: 8 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (e *eagleClient) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.Base+path, nil)
	if err != nil {
		return err
	}
	r, err := e.HTTP.Do(req)
	if err != nil {
		return errors.New("未连接 Eagle，请先打开本机 Eagle 和素材库")
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return eagleHTTPError(r.StatusCode)
	}
	var envelope struct {
		Status  string
		Data    json.RawMessage
		Message string
	}
	b, err := io.ReadAll(io.LimitReader(r.Body, 4<<20+1))
	if err != nil {
		return err
	}
	if len(b) > 4<<20 {
		return errors.New("Eagle 返回内容过大")
	}
	if err = json.Unmarshal(b, &envelope); err != nil {
		return errors.New("Eagle 返回了无法识别的数据")
	}
	if envelope.Status != "success" {
		return errors.New("Eagle 尚未就绪，请打开素材库后重试")
	}
	return json.Unmarshal(envelope.Data, out)
}
func (e *eagleClient) Connect(ctx context.Context) error {
	var info struct{ Name, Path, LibraryPath string }
	err := e.get(ctx, "/api/v2/library/info", &info)
	if err == nil {
		e.V2 = true
	} else {
		// Older Eagle versions expose only /api/library/info.
		var status eagleHTTPError
		if !errors.As(err, &status) || (status != 404 && status != 400) {
			return err
		}
		if err = e.get(ctx, "/api/library/info", &info); err != nil {
			return err
		}
	}
	e.Library = info.Path
	if e.Library == "" {
		e.Library = info.LibraryPath
	}
	e.Name = info.Name
	if e.Name == "" && e.Library != "" {
		e.Name = strings.TrimSuffix(filepath.Base(e.Library), ".library")
	}
	if e.Name == "" {
		e.Name = "当前 Eagle 素材库"
	}
	return nil
}
func (e *eagleClient) List(ctx context.Context, keyword string, offset int) (eaglePage, error) {
	const limit = 48
	q := url.Values{"limit": {strconv.Itoa(limit)}, "offset": {strconv.Itoa(max(0, offset))}}
	var all []eagleItem
	more := false
	if e.V2 {
		q.Set("keywords", strings.TrimSpace(keyword))
		q.Set("orderBy", "-CREATEDATE")
		var data struct {
			Data  []eagleItem
			Total int
		}
		if err := e.get(ctx, "/api/v2/item/get?"+q.Encode(), &data); err != nil {
			return eaglePage{}, err
		}
		all = data.Data
		more = offset+len(all) < data.Total
	} else {
		q.Set("keyword", strings.TrimSpace(keyword))
		q.Set("orderBy", "-CREATEDATE")
		if err := e.get(ctx, "/api/item/list?"+q.Encode(), &all); err != nil {
			return eaglePage{}, err
		}
		more = len(all) == limit
	}
	page := eaglePage{Offset: offset + len(all), More: more}
	for _, item := range all {
		if !item.Deleted && eagleImage(item.Ext) {
			page.Items = append(page.Items, item)
		}
	}
	return page, nil
}
func eagleImage(ext string) bool {
	switch strings.ToLower(strings.TrimPrefix(ext, ".")) {
	case "png", "jpg", "jpeg", "webp":
		return true
	}
	return false
}
func eaglePart(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, "/\\\x00")
}
func containedFile(root, path string) (string, error) {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(realRoot, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("图片路径超出素材库")
	}
	st, err := os.Stat(real)
	if err != nil || !st.Mode().IsRegular() {
		return "", errors.New("原始图片不存在")
	}
	return real, nil
}
func (e *eagleClient) itemDirectory(ctx context.Context, item eagleItem) (string, string, error) {
	if !eaglePart(item.ID) || !eaglePart(item.Name) || !eagleImage(item.Ext) {
		return "", "", errors.New("Eagle 图片信息无效")
	}
	root := e.Library
	if root == "" {
		var thumbnail string
		if err := e.get(ctx, "/api/item/thumbnail?id="+url.QueryEscape(item.ID), &thumbnail); err != nil {
			return "", "", err
		}
		itemDir := filepath.Dir(thumbnail)
		images := filepath.Dir(itemDir)
		root = filepath.Dir(images)
		if !filepath.IsAbs(thumbnail) || filepath.Base(itemDir) != item.ID+".info" || filepath.Base(images) != "images" || !strings.HasSuffix(strings.ToLower(root), ".library") {
			return "", "", errors.New("Eagle 未提供有效的本地素材路径")
		}
	}
	if !filepath.IsAbs(root) {
		return "", "", errors.New("Eagle 素材库不在本机")
	}
	return root, filepath.Join(root, "images", item.ID+".info"), nil
}
func (e *eagleClient) Original(ctx context.Context, item eagleItem) (string, error) {
	root, dir, err := e.itemDirectory(ctx, item)
	if err != nil {
		return "", err
	}
	// Eagle's public library layout: images/{id}.info/{name}.{ext}.
	return containedFile(root, filepath.Join(dir, item.Name+"."+strings.TrimPrefix(item.Ext, ".")))
}
