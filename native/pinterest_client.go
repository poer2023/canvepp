package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

var errPinterestLogin = errors.New("Pinterest 授权已过期，请重新连接")

type pinterestToken struct {
	Access    string `json:"access_token"`
	Refresh   string `json:"refresh_token"`
	ExpiresAt int64  `json:"expires_at"`
	Scope     string `json:"scope"`
}
type pinterestClient struct {
	mu              sync.Mutex
	config          pinterestConfig
	token           pinterestToken
	vault           *pinterestVault
	closed          bool
	base, authorize string
	http            *http.Client
}

func newPinterestClient(s pinterestSession, vault *pinterestVault) *pinterestClient {
	return &pinterestClient{config: s.Config, token: s.Token, vault: vault, base: "https://api.pinterest.com/v5", authorize: "https://www.pinterest.com/oauth/", http: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (c *pinterestClient) tokenRequest(ctx context.Context, values url.Values) (pinterestToken, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", c.base+"/oauth/token", strings.NewReader(values.Encode()))
	if err != nil {
		return pinterestToken{}, errors.New("无法创建授权请求")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(c.config.ClientID, c.config.ClientSecret)
	r, err := c.http.Do(req)
	if err != nil {
		return pinterestToken{}, errors.New("无法连接 Pinterest，请检查网络后重试")
	}
	defer r.Body.Close()
	if r.StatusCode != 200 && r.StatusCode != 201 {
		if r.StatusCode == 400 || r.StatusCode == 401 {
			return pinterestToken{}, errors.New("授权兑换失败，请检查应用凭据及回调地址，再重新连接")
		}
		return pinterestToken{}, pinterestHTTPError(r.StatusCode)
	}
	var response struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
		Expires int64  `json:"expires_in"`
		Scope   string `json:"scope"`
		Type    string `json:"token_type"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 32768)).Decode(&response); err != nil || response.Access == "" || response.Expires <= 0 || !strings.EqualFold(response.Type, "bearer") {
		return pinterestToken{}, errors.New("Pinterest 返回的授权信息不完整，请重新连接")
	}
	return pinterestToken{Access: response.Access, Refresh: response.Refresh, ExpiresAt: time.Now().Unix() + response.Expires, Scope: response.Scope}, nil
}
func (c *pinterestClient) Exchange(ctx context.Context, code string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return context.Canceled
	}
	token, err := c.tokenRequest(ctx, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {c.config.RedirectURI}, "continuous_refresh": {"true"}})
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := c.vault.save(pinterestSession{Config: c.config, Token: token}); err != nil {
		return err
	}
	c.token = token
	return nil
}
func (c *pinterestClient) access(ctx context.Context, rejected string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return "", context.Canceled
	}
	if c.token.Access != "" && c.token.ExpiresAt > time.Now().Unix()+60 && (rejected == "" || c.token.Access != rejected) {
		return c.token.Access, nil
	}
	if c.token.Refresh == "" {
		return "", errPinterestLogin
	}
	token, err := c.tokenRequest(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {c.token.Refresh}, "continuous_refresh": {"true"}})
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if token.Refresh == "" {
		token.Refresh = c.token.Refresh
	}
	if token.Scope == "" {
		token.Scope = c.token.Scope
	}
	if err := c.vault.save(pinterestSession{Config: c.config, Token: token}); err != nil {
		return "", err
	}
	c.token = token
	return token.Access, nil
}
func (c *pinterestClient) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Serialize with refresh so a late response cannot restore a cleared token.
	if err := c.vault.save(pinterestSession{Config: c.config}); err != nil {
		return err
	}
	c.closed = true
	c.token = pinterestToken{}
	return nil
}
func (c *pinterestClient) replaceConfig(config pinterestConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.vault.save(pinterestSession{Config: config}); err != nil {
		return err
	}
	c.closed = true
	return nil
}
func pinterestHTTPError(code int) error {
	switch code {
	case 401:
		return errPinterestLogin
	case 403:
		return errors.New("Pinterest 未允许读取这些收藏，请检查应用权限后重新授权")
	case 404:
		return errors.New("这个 Pinterest 图板暂时无法访问")
	case 429:
		return errors.New("Pinterest 请求较多，请稍后刷新")
	default:
		return fmt.Errorf("Pinterest 暂时无法读取（%d），请稍后重试", code)
	}
}
func (c *pinterestClient) get(ctx context.Context, path, bookmark string, result any) error {
	access, err := c.access(ctx, "")
	if err != nil {
		return err
	}
	q := url.Values{"page_size": {"48"}}
	if bookmark != "" {
		q.Set("bookmark", bookmark)
	}
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "GET", c.base+path+"?"+q.Encode(), nil)
		if err != nil {
			return errors.New("Pinterest 请求无效")
		}
		req.Header.Set("Authorization", "Bearer "+access)
		r, err := c.http.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return errors.New("无法读取 Pinterest，请检查网络后刷新")
		}
		if r.StatusCode == 401 && attempt == 0 {
			r.Body.Close()
			access, err = c.access(ctx, access)
			if err != nil {
				return err
			}
			continue
		}
		if r.StatusCode != 200 {
			r.Body.Close()
			return pinterestHTTPError(r.StatusCode)
		}
		err = json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(result)
		r.Body.Close()
		if err != nil {
			return errors.New("Pinterest 返回的收藏数据无法读取")
		}
		return nil
	}
	return errPinterestLogin
}

type pinterestPage[T any] struct {
	Items    []T    `json:"items"`
	Bookmark string `json:"bookmark"`
}
type pinterestBoard struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Count   int    `json:"pin_count"`
	Privacy string `json:"privacy"`
}
type pinterestImage struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}
type pinterestPin struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Alt   string `json:"alt_text"`
	Media struct {
		Type   string                    `json:"media_type"`
		Images map[string]pinterestImage `json:"images"`
		Items  []struct {
			ItemType string                    `json:"item_type"`
			Images   map[string]pinterestImage `json:"images"`
		} `json:"items"`
	} `json:"media"`
}
type pinterestPicture struct {
	Key, PinID, Name string
	Thumb, Original  pinterestImage
}

func (p pinterestPicture) WebURL() string { return "https://www.pinterest.com/pin/" + p.PinID + "/" }
func pinterestID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
func (c *pinterestClient) Boards(ctx context.Context, bookmark string) (pinterestPage[pinterestBoard], error) {
	var page pinterestPage[pinterestBoard]
	err := c.get(ctx, "/boards", bookmark, &page)
	return page, err
}
func (c *pinterestClient) Pins(ctx context.Context, board, bookmark string) (pinterestPage[pinterestPin], error) {
	var page pinterestPage[pinterestPin]
	if !pinterestID(board) {
		return page, errors.New("图板编号无效")
	}
	err := c.get(ctx, "/boards/"+board+"/pins", bookmark, &page)
	return page, err
}

// Keep every still image in a multi-image Pin. Video cover art is not silently
// treated as a saved photograph. Only Pinterest's image CDN is downloaded.
func pinterestPictures(pins []pinterestPin) []pinterestPicture {
	var out []pinterestPicture
	for _, p := range pins {
		if !pinterestID(p.ID) || (p.Media.Type != "image" && p.Media.Type != "multiple_images" && p.Media.Type != "multiple_mixed") {
			continue
		}
		sets := []map[string]pinterestImage{p.Media.Images}
		if p.Media.Type == "multiple_images" || p.Media.Type == "multiple_mixed" {
			sets = nil
			for _, item := range p.Media.Items {
				if p.Media.Type == "multiple_mixed" && item.ItemType != "image" {
					continue
				}
				sets = append(sets, item.Images)
			}
		}
		for index, images := range sets {
			var variants []pinterestImage
			for _, im := range images {
				if pinImageURL(im.URL) && im.Width > 0 && im.Height > 0 {
					variants = append(variants, im)
				}
			}
			sort.Slice(variants, func(i, j int) bool {
				ai, aj := int64(variants[i].Width)*int64(variants[i].Height), int64(variants[j].Width)*int64(variants[j].Height)
				if ai == aj {
					return variants[i].URL < variants[j].URL
				}
				return ai < aj
			})
			if len(variants) == 0 {
				continue
			}
			large := variants[len(variants)-1]
			thumb := large
			for _, im := range variants {
				if max(im.Width, im.Height) >= 512 {
					thumb = im
					break
				}
			}
			name := strings.TrimSpace(p.Title)
			if name == "" {
				name = strings.TrimSpace(p.Alt)
			}
			if name == "" {
				name = "Pinterest 图片"
			}
			if len([]rune(name)) > 100 {
				name = string([]rune(name)[:100])
			}
			if len(sets) > 1 {
				name = fmt.Sprintf("%s · %d", name, index+1)
			}
			out = append(out, pinterestPicture{Key: fmt.Sprintf("%s:%d", p.ID, index), PinID: p.ID, Name: name, Thumb: thumb, Original: large})
		}
	}
	return out
}
