//go:build pinterestqa

package main

// This loopback provider exists only in an explicitly tagged acceptance build.
// The normal release cannot redirect Pinterest OAuth or CDN traffic to fixtures.
import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

var pinterestQA struct {
	once                        sync.Once
	mu                          sync.Mutex
	server                      *httptest.Server
	images                      []string
	codes                       map[string]bool
	exchanges, reads, downloads int
}

const pinterestAppName = "Canvepp Pinterest QA"

func pinterestVaultKey(dir string) string { return "fixture.pinterest." + digest([]byte(dir))[:16] }

func pinterestAcceptanceDataDir(dir string) error {
	base, _ := os.UserConfigDir()
	if filepath.Clean(dir) == filepath.Join(base, "CanveppNative") {
		return fmt.Errorf("验收版必须使用显式的隔离数据目录")
	}
	return nil
}

func pinterestQAReceipt() {
	if path := os.Getenv("CANVEPP_PINTEREST_QA_RECEIPT"); path != "" {
		_ = os.WriteFile(path, encode(map[string]any{"fixture": true, "exchanges": pinterestQA.exchanges, "apiReads": pinterestQA.reads, "imageReads": pinterestQA.downloads}), 0600)
	}
}
func pinterestAcceptanceSetup(p *pinterestState) {
	pinterestQA.once.Do(func() {
		pinterestQA.codes = map[string]bool{}
		_ = json.Unmarshal([]byte(os.Getenv("CANVEPP_PINTEREST_QA_IMAGES")), &pinterestQA.images)
		pinterestQA.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			pinterestQA.mu.Lock()
			defer pinterestQA.mu.Unlock()
			defer pinterestQAReceipt()
			switch r.URL.Path {
			case "/oauth/":
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				const page = `<!doctype html><html lang="zh"><meta charset="utf-8"><title>Canvepp 本机授权验收</title><style>body{font:16px system-ui;background:#f5f5f6;color:#25262a;margin:12vh auto;max-width:540px}main{background:#fff;padding:40px;border-radius:20px}h1{font-size:26px}p{line-height:1.8;color:#686970}button{background:#242529;color:white;border:0;border-radius:9px;padding:12px 20px;margin-right:8px;font:inherit}.deny{background:#eee;color:#444}</style><main><h1>Canvepp 本机授权验收</h1><p>这是隔离的模拟授权服务，验证浏览器返回、图板浏览和导入。不会连接或验证真实 Pinterest 账号。</p><form action="/decision" method="get"><input type="hidden" name="redirect_uri" value="{{.Redirect}}"><input type="hidden" name="state" value="{{.State}}"><button name="allow" value="yes">授权测试只读收藏</button><button class="deny" name="allow" value="no">取消授权</button></form></main></html>`
				_ = template.Must(template.New("auth").Parse(page)).Execute(w, struct{ Redirect, State string }{r.URL.Query().Get("redirect_uri"), r.URL.Query().Get("state")})
			case "/decision":
				u, err := url.Parse(r.URL.Query().Get("redirect_uri"))
				if err != nil || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1") {
					http.Error(w, "bad callback", 400)
					return
				}
				q := url.Values{"state": {r.URL.Query().Get("state")}}
				if r.URL.Query().Get("allow") == "yes" {
					code := fmt.Sprintf("fixture-code-%d", len(pinterestQA.codes))
					pinterestQA.codes[code] = true
					q.Set("code", code)
				} else {
					q.Set("error", "access_denied")
				}
				u.RawQuery = q.Encode()
				http.Redirect(w, r, u.String(), http.StatusSeeOther)
			case "/oauth/token":
				r.ParseForm()
				code := r.Form.Get("code")
				if !pinterestQA.codes[code] {
					http.Error(w, "invalid code", 400)
					return
				}
				pinterestQA.codes[code] = false
				pinterestQA.exchanges++
				json.NewEncoder(w).Encode(map[string]any{"access_token": "fixture-access", "refresh_token": "fixture-refresh", "expires_in": 3600, "token_type": "bearer"})
			case "/boards":
				pinterestQA.reads++
				if r.Header.Get("Authorization") != "Bearer fixture-access" {
					http.Error(w, "unauthorized", 401)
					return
				}
				if r.URL.Query().Get("bookmark") != "" {
					json.NewEncoder(w).Encode(pinterestPage[pinterestBoard]{Items: []pinterestBoard{{ID: "103", Name: "更多灵感 · 验收", Count: 1}}})
					return
				}
				json.NewEncoder(w).Encode(pinterestPage[pinterestBoard]{Items: []pinterestBoard{{ID: "101", Name: "光影与构图 · 验收", Count: 8}, {ID: "102", Name: "空图板 · 验收", Count: 0}}, Bookmark: "boards-next"})
			case "/boards/101/pins", "/boards/102/pins", "/boards/103/pins":
				pinterestQA.reads++
				if r.Header.Get("Authorization") != "Bearer fixture-access" {
					http.Error(w, "unauthorized", 401)
					return
				}
				var pins []pinterestPin
				bookmark := ""
				if r.URL.Path != "/boards/102/pins" {
					for i := 0; i < len(pinterestQA.images); i++ {
						pic := pinterestPin{ID: strconv.Itoa(1000 + i), Title: fmt.Sprintf("灵感收藏 %d", i+1)}
						pic.Media.Type = "image"
						pic.Media.Images = map[string]pinterestImage{"1200x": {URL: fmt.Sprintf("https://i.pinimg.com/canvepp-local-qa/%d.png", i), Width: 1200, Height: 1200}}
						pins = append(pins, pic)
					}
					bookmark = "pins-next"
					if r.URL.Query().Get("bookmark") != "" {
						bookmark = ""
						pins = pins[:min(1, len(pins))]
					}
				}
				json.NewEncoder(w).Encode(pinterestPage[pinterestPin]{Items: pins, Bookmark: bookmark})
			default:
				http.NotFound(w, r)
			}
		}))
	})
	if p.config.ClientID == "" {
		p.config = pinterestConfig{ClientID: "12345", ClientSecret: "fixture-secret", RedirectURI: "http://localhost:47844/oauth/pinterest/callback"}
	}
	p.client.config = p.config
	p.client.base = pinterestQA.server.URL
	p.client.authorize = pinterestQA.server.URL + "/oauth/"
}
func downloadPinterestPicture(ctx context.Context, raw string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	prefix := "https://i.pinimg.com/canvepp-local-qa/"
	if !strings.HasPrefix(raw, prefix) {
		return nil, fmt.Errorf("验收版仅加载本机测试图片")
	}
	n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(raw, prefix), ".png"))
	if err != nil || n < 0 || n >= len(pinterestQA.images) {
		return nil, fmt.Errorf("测试图片不存在")
	}
	pinterestQA.mu.Lock()
	pinterestQA.downloads++
	pinterestQAReceipt()
	pinterestQA.mu.Unlock()
	return readBounded(pinterestQA.images[n], maxImageBytes)
}
