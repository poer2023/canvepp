package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/egoist/mygo/transfer"
	"golang.org/x/net/html"
)

func browserPin(data transfer.Data) webPin {
	var pin webPin
	addURL := func(raw string) {
		raw = strings.TrimSpace(raw)
		if validPinPage(raw) && pin.Pin == "" {
			pin.Pin = raw
		}
		if pinImageURL(raw) && pin.Image == "" {
			pin.Image = raw
		}
	}
	for _, format := range []transfer.Format{transfer.URIList, transfer.Text} {
		b, err := data.Read(format)
		if err != nil || len(b) > 8192 {
			continue
		}
		for _, line := range strings.Split(string(b), "\n") {
			addURL(line)
		}
	}
	if b, err := data.Read(transfer.HTML); err == nil && len(b) <= 1<<20 {
		t := html.NewTokenizer(bytes.NewReader(b))
		for {
			k := t.Next()
			if k == html.ErrorToken {
				break
			}
			if k != html.StartTagToken && k != html.SelfClosingTagToken {
				continue
			}
			tok := t.Token()
			for _, a := range tok.Attr {
				switch {
				case tok.Data == "a" && a.Key == "href", tok.Data == "img" && a.Key == "src":
					addURL(a.Val)
				case tok.Data == "img" && a.Key == "alt" && pin.Name == "":
					pin.Name = a.Val
				}
			}
		}
	}
	return pin
}

func (a *desktop) importBrowserPin(e *Editor, at Point, p webPin) {
	if p.Image != "" && p.Pin != "" {
		a.importPinInto(e, at, p.Image, p.Pin, p.Name)
		return
	}
	source := p.Pin
	if source == "" {
		source = p.Image
	}
	if source == "" {
		a.message("请拖入 Pinterest 图片或 Pin 链接")
		return
	}
	name := strings.TrimSpace(p.Name)
	if name == "" {
		name = "Pinterest 图片"
	}
	if n := []rune(name); len(n) > 140 {
		name = string(n[:140])
	}
	downloads := &a.materials.pinDownloads
	a.importMaterialInto(e, at, "pinterest", digest([]byte(source)), name, source, func(ctx context.Context) ([]byte, error) {
		raw := p.Image
		if raw == "" {
			var err error
			raw, err = linkedPinImage(ctx, p.Pin)
			if err != nil {
				return nil, err
			}
		}
		return downloads.download(ctx, raw)
	})
}

// Chrome can offer only the Pin link when the user grabs its surrounding
// anchor. Resolve that single public Pin; protected images can be dragged
// directly from the open Pin. No browser credentials are copied to the app.
func linkedPinImage(ctx context.Context, link string) (string, error) {
	if !validPinPage(link) {
		return "", errors.New("无效的 Pin 链接")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return "", err
	}
	client := http.Client{CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 3 || !validPinPage(r.URL.String()) {
			return http.ErrUseLastResponse
		}
		return nil
	}}
	resp, err := client.Do(req)
	if err != nil {
		return "", errors.New("无法读取 Pin，请重试")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t := html.NewTokenizer(io.LimitReader(resp.Body, 2<<20))
		for {
			k := t.Next()
			if k == html.ErrorToken {
				break
			}
			if k != html.StartTagToken && k != html.SelfClosingTagToken {
				continue
			}
			tok := t.Token()
			if tok.Data == "body" {
				break
			}
			if tok.Data != "meta" {
				continue
			}
			isImage, raw := false, ""
			for _, attr := range tok.Attr {
				if attr.Key == "property" && attr.Val == "og:image" {
					isImage = true
				}
				if attr.Key == "content" {
					raw = attr.Val
				}
			}
			if isImage && pinImageURL(raw) {
				return raw, nil
			}
		}
	}
	return "", errors.New("请打开这张 Pin，再拖动原图")
}
