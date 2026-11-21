package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

func pinImageURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Hostname() == "i.pinimg.com" && u.Port() == "" && u.User == nil
}
func pinterestURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Port() != "" || u.User != nil {
		return false
	}
	switch u.Hostname() {
	case "pinterest.com", "www.pinterest.com", "www.pinterest.co.uk", "www.pinterest.jp", "www.pinterest.ca", "www.pinterest.de", "www.pinterest.fr":
		return true
	}
	return false
}

// Repeated drops of the same CDN URL share its fresh HTTP response within
// this material panel. Keep just one original, bounded by maxImageBytes.
type pinImageCache struct {
	mu    sync.Mutex
	url   string
	until time.Time
	data  []byte
}

func pinFreshness(h http.Header) time.Duration {
	if h.Get("Vary") != "" && !strings.EqualFold(h.Get("Vary"), "Accept-Encoding") {
		return 0
	}
	var ttl int64
	for _, part := range strings.Split(strings.ToLower(strings.Join(h.Values("Cache-Control"), ",")), ",") {
		key, value, _ := strings.Cut(strings.TrimSpace(part), "=")
		switch key {
		case "no-store", "no-cache", "private":
			return 0
		case "max-age":
			ttl, _ = strconv.ParseInt(strings.Trim(value, "\""), 10, 32)
		}
	}
	age, _ := strconv.ParseInt(h.Get("Age"), 10, 32)
	return time.Duration(max(0, min(ttl-max(age, 0), 300))) * time.Second
}

func downloadPin(ctx context.Context, raw string) ([]byte, error) {
	return (*pinImageCache)(nil).download(ctx, raw)
}

func (cache *pinImageCache) download(ctx context.Context, raw string) ([]byte, error) {
	if !pinImageURL(raw) {
		return nil, errors.New("请选择 Pinterest 的图片")
	}
	if cache != nil {
		cache.mu.Lock()
		data := cache.data
		fresh := cache.url == raw && time.Now().Before(cache.until)
		cache.mu.Unlock()
		if fresh {
			return data, nil
		}
	}
	client := http.Client{Timeout: 30 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 4 || !pinImageURL(req.URL.String()) {
			return errors.New("图片跳转地址无效")
		}
		return nil
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	r, err := client.Do(req)
	if err != nil {
		return nil, errors.New("图片下载失败，请检查网络后重试")
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return nil, fmt.Errorf("图片暂时无法下载（%d）", r.StatusCode)
	}
	if r.ContentLength > maxImageBytes {
		return nil, errors.New("图片超过 20 MB")
	}
	b, err := io.ReadAll(io.LimitReader(r.Body, maxImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxImageBytes {
		return nil, errors.New("图片超过 20 MB")
	}
	if cache != nil && r.Request.URL.String() == raw {
		if ttl := pinFreshness(r.Header); ttl > 0 {
			cache.mu.Lock()
			cache.url, cache.data, cache.until = raw, b, time.Now().Add(ttl)
			cache.mu.Unlock()
		}
	}
	return b, nil
}

// A truncated or invalid response must be retried over the network.
func (cache *pinImageCache) discard(b []byte) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if len(b) > 0 && len(cache.data) > 0 && &b[0] == &cache.data[0] {
		cache.url, cache.data = "", nil
	}
}

type pendingMaterial struct {
	editor *Editor
	at     Point
	source string
}

func (a *desktop) materialImporting(source, key string) bool {
	if a.editor == nil {
		return false
	}
	_, busy := a.materials.importing[a.editor.Project.ID+":"+source+":"+key]
	return busy
}

// Capture the destination editor before asynchronous reads. A later canvas
// switch must never redirect an in-flight import.
func (a *desktop) importMaterialInto(e *Editor, at Point, source, key, name, sourceURL string, read func(context.Context) ([]byte, error)) {
	m := a.materials
	if e == nil || m == nil || a.closing {
		return
	}
	busyKey := e.Project.ID + ":" + source + ":" + key
	if _, busy := m.importing[busyKey]; busy {
		return
	}
	if len(e.Project.Graph.Nodes) >= e.Project.nodeLimit() {
		m.status = "当前画布已满"
		a.invalidateMaterials()
		return
	}
	m.importing[busyKey] = pendingMaterial{editor: e, at: at, source: source}
	m.status = "正在导入…"
	a.invalidateMaterials()
	n := newNode(e.Project.ID, at)
	projectID := e.Project.ID
	canvasName := e.Project.Name
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		b, err := read(ctx)
		var asset Asset
		var decoded image.Image
		if err == nil {
			asset, err = a.store.importImage(b, projectID, n.ID, name, source, "", nil, &decoded, assetOrigin{URL: sourceURL, ID: key})
			if err != nil && source == "pinterest" {
				m.pinDownloads.discard(b)
			}
		}
		a.publish(func() {
			delete(m.importing, busyKey)
			if err != nil {
				m.status = "导入失败：" + err.Error()
				a.message(m.status)
			} else {
				if a.images != nil && a.editor == e {
					size := 512
					if e.View.Zoom > 2 {
						size = 1024
					}
					a.images.request(asset, size, decoded)
				}
				n.Name, n.Asset = asset.Name, asset.ID
				e.Assets[asset.ID] = asset
				e.AddNamed([]Node{n}, nil, "从 "+sourceLabel(source)+" 导入")
				a.draftKey = ""
				m.status = "已导入「" + canvasName + "」"
				a.message(m.status)
			}
			a.invalidateMaterials()
		})
	}()
}
func (a *desktop) importPin(raw, pin, name string) {
	a.importPinInto(a.editor, a.center(), raw, pin, name)
}
func (a *desktop) importPinInto(e *Editor, at Point, raw, pin, name string) {
	if !pinImageURL(raw) || !validPinPage(pin) {
		a.materials.status = "未找到可导入的 Pinterest 图片"
		a.invalidateMaterials()
		return
	}
	u, _ := url.Parse(pin)
	if !strings.HasPrefix(u.Path, "/pin/") {
		a.materials.status = "请先打开一张 Pin 图片"
		a.invalidateMaterials()
		return
	}
	if strings.TrimSpace(name) == "" {
		name = "Pinterest 图片"
	}
	if len([]rune(name)) > 140 {
		name = string([]rune(name)[:140])
	}
	ext := filepath.Ext(strings.Split(raw, "?")[0])
	if ext != ".png" && ext != ".webp" && ext != ".jpg" && ext != ".jpeg" {
		ext = ".jpg"
	}
	downloads := &a.materials.pinDownloads
	a.importMaterialInto(e, at, "pinterest", strings.Trim(u.Path, "/"), name+ext, pin, func(ctx context.Context) ([]byte, error) { return downloads.download(ctx, raw) })
}
