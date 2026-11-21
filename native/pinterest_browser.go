package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/egoist/mygo/ui"
)

const pinterestSavedPage = "https://www.pinterest.com/me/pins/"

// A one-use local page identifies the one browser window we opened before
// sending it to Pinterest. Existing personal browser windows never get mounted.
func openPinterestLanding(ctx context.Context, login bool, launch func(string) error, mount func(string) error) error {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	id := hex.EncodeToString(nonce[:])
	title := "Canvepp " + id
	path := "/" + id
	host := listener.Addr().String()
	mounted := make(chan struct{})
	served := make(chan struct{}, 1)
	next := pinterestSavedPage
	if login {
		next = pinterestLoginURL + "?next=%2Fme%2Fpins%2F"
	}
	server := &http.Server{ReadHeaderTimeout: 3 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if r.Method != http.MethodGet || r.Host != host {
			http.NotFound(w, r)
			return
		}
		switch r.URL.Path {
		case path:
			w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'nonce-"+id+"'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>%s</title><script nonce="%s">fetch("%s/ready").then(r => { if (r.ok) location.replace("%s") })</script>`, title, id, path, next)
		case path + "/ready":
			select {
			case <-mounted:
				w.WriteHeader(http.StatusNoContent)
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
				select {
				case served <- struct{}{}:
				default:
				}
			case <-ctx.Done():
				http.Error(w, "Window closed", http.StatusGone)
			}
		default:
			http.NotFound(w, r)
		}
	})}
	go server.Serve(listener)
	defer server.Close()
	if err := launch("http://" + host + path); err != nil {
		return err
	}
	if err := mount(title); err != nil {
		return err
	}
	close(mounted)
	select {
	case <-served:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *desktop) wantsPinterestBrowser() bool {
	m := a.materials
	return a.win != nil && a.dockVisible() && m != nil && a.dock.active == dockPinterest && !m.showNativePins && m.web != nil && pinterestBrowserAvailable()
}

func (a *desktop) pinterestBrowserContent(c *ui.Context) {
	w := a.materials.web
	if w.browser == nil {
		w.browser = a.newPinterestBrowser(false)
	}
	b := w.browser
	open := func(login bool) { b.close(); w.browser = a.newPinterestBrowser(login); a.invalidateMaterials() }
	ui.Row(c).Height(36).Padding(2, 8).AlignItems(ui.Center).Gap(3).Children(func() {
		iconButton(c, "back", "Pinterest 后退", func() { b.command(1) })
		iconButton(c, "forward", "Pinterest 前进", func() { b.command(2) })
		iconButton(c, "refresh", "刷新 Pinterest 网页", func() {
			if b.owns() {
				b.command(3)
			} else {
				open(false)
			}
		})
	})
	ui.Row(c.Key("pinterest-viewport")).Grow(1).FillWidth().Children(func() {
		ui.Box(c.Key("pinterest-browser-slot")).Label("Pinterest 网页内容").Grow(1).MinWidth(0).FillHeight().Center().Draw(func(p *ui.Painter, r ui.Rect) { b.place(r, p.Scale()) }).Children(func() {
			if b.message != "" {
				ui.Column(c).Center().Gap(6).Children(func() {
					ui.Text(c, b.message).FontSize(12).TextColor(c.Theme().TextMuted)
					iconButton(c, "refresh", "重新打开 Pinterest 网页", func() { open(false) })
				})
			}
		})
		b.scrollbar(c)
	})
}
