package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type pinterestMemorySecrets struct {
	mu   sync.Mutex
	data map[string][]byte
	fail bool
}

func (s *pinterestMemorySecrets) Get(key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.data[key]
	if !ok {
		return nil, mygo.ErrSecretNotFound
	}
	return append([]byte{}, b...), nil
}
func (s *pinterestMemorySecrets) Set(key string, b []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return errors.New("vault unavailable")
	}
	if s.data == nil {
		s.data = map[string][]byte{}
	}
	s.data[key] = append([]byte{}, b...)
	return nil
}
func (s *pinterestMemorySecrets) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	return nil
}
func testPinterestVault() *pinterestVault {
	return &pinterestVault{store: &pinterestMemorySecrets{}, key: "isolated-test"}
}
func testPinterestConfig() pinterestConfig {
	return pinterestConfig{ClientID: "12345", ClientSecret: "local-test-secret", RedirectURI: pinterestRedirect, IncludeSecret: true}
}

func TestPinterestConfigurationBoundaries(t *testing.T) {
	c := testPinterestConfig()
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"https://remote.test/callback", "http://0.0.0.0:47843/cb", "http://localhost:80/cb", "http://localhost:47843", "http://localhost:47843/cb?code=x", "http://user@localhost:47843/cb", "http://localhost:47843/cb#f"} {
		c.RedirectURI = raw
		if c.validate() == nil {
			t.Fatal("accepted unsafe callback", raw)
		}
	}
	c = testPinterestConfig()
	v := testPinterestVault()
	if err := v.save(pinterestSession{Config: c}); err != nil {
		t.Fatal(err)
	}
	s, err := v.load()
	if err != nil || s.Config != c {
		t.Fatal("configuration lost", err)
	}
}
func newTestPinterestAuth(t *testing.T, ctx context.Context) (*pinterestAuthorization, string) {
	t.Helper()
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	c := testPinterestConfig()
	c.RedirectURI = "http://" + l.Addr().String() + "/oauth/pinterest/callback"
	a, err := listenPinterestAuthorization(ctx, c, "https://www.pinterest.com/oauth/", l)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Cancel)
	return a, c.RedirectURI
}
func TestPinterestAuthorizationCallback(t *testing.T) {
	a, callback := newTestPinterestAuth(t, context.Background())
	u, _ := url.Parse(a.URL)
	q := u.Query()
	state := q.Get("state")
	if len(state) < 40 || q.Get("client_secret") != "" || !strings.Contains(q.Get("scope"), "pins:read_secret") || q.Get("redirect_uri") != callback {
		t.Fatal("incorrect authorization URL")
	}
	for _, suffix := range []string{"?code=bad&state=wrong", "?code=a&code=b&state=" + state, "?code=a&state=" + state + "&state=" + state} {
		r, err := http.Get(callback + suffix)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode != 400 {
			t.Fatal("accepted mismatched/ambiguous callback", r.StatusCode)
		}
	}
	select {
	case <-a.Done:
		t.Fatal("invalid request consumed authorization")
	default:
	}
	r, err := http.Get(callback + "?code=one-use-code&state=" + state)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if r.StatusCode != 200 || r.Header.Get("Location") != "" || r.Header.Get("Cache-Control") != "no-store" || strings.Contains(string(b), state) || strings.Contains(string(b), "one-use-code") {
		t.Fatal("callback leaked credentials or redirected")
	}
	select {
	case result := <-a.Done:
		if result.Err != nil || result.Code != "one-use-code" {
			t.Fatal(result)
		}
	case <-time.After(time.Second):
		t.Fatal("callback not delivered")
	}
	// Replay cannot yield a second exchange, including when the listener has
	// already closed after returning the success page.
	client := &http.Client{Timeout: time.Second}
	r, err = client.Get(callback + "?code=replay&state=" + state)
	if err == nil {
		r.Body.Close()
		if r.StatusCode == 200 {
			t.Fatal("replayed callback")
		}
	}
	select {
	case <-a.Done:
		t.Fatal("callback delivered twice")
	case <-time.After(20 * time.Millisecond):
	}
}
func TestPinterestDeniedAndCancelled(t *testing.T) {
	a, callback := newTestPinterestAuth(t, context.Background())
	u, _ := url.Parse(a.URL)
	r, err := http.Get(callback + "?error=access_denied&state=" + u.Query().Get("state"))
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if result := <-a.Done; result.Err == nil || result.Code != "" {
		t.Fatal("denial became a code")
	}
	a, _ = newTestPinterestAuth(t, context.Background())
	a.Cancel()
	select {
	case result := <-a.Done:
		if result.Err == nil {
			t.Fatal("missing cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("cancel hung")
	}
}
func TestPinterestReadOnlyPaginationRefreshAndDisconnect(t *testing.T) {
	var exchanges, refreshes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/oauth/token" {
			id, secret, ok := r.BasicAuth()
			if !ok || id != "12345" || secret != "local-test-secret" {
				t.Error("missing token credentials")
			}
			r.ParseForm()
			if r.Form.Get("grant_type") == "authorization_code" {
				exchanges.Add(1)
				if r.Form.Get("code") != "only-once" || r.Form.Get("redirect_uri") != pinterestRedirect {
					t.Error("wrong code exchange")
				}
			} else {
				refreshes.Add(1)
				if r.Form.Get("refresh_token") != "rotating-1" {
					t.Error("wrong refresh token")
				}
			}
			json.NewEncoder(w).Encode(map[string]any{"access_token": "good", "refresh_token": "rotating-2", "expires_in": 3600, "token_type": "bearer", "scope": "boards:read pins:read"})
			return
		}
		if r.Method != "GET" || r.Header.Get("Authorization") != "Bearer good" {
			t.Error("not an authorized read")
		}
		if r.URL.Query().Get("page_size") != "48" {
			t.Error("unbounded page")
		}
		switch r.URL.Path {
		case "/boards":
			if r.URL.Query().Get("bookmark") != "opaque &?+" {
				t.Error("cursor not escaped")
			}
			io.WriteString(w, `{"items":[{"id":"12","name":"私密灵感","pin_count":2,"privacy":"SECRET"}],"bookmark":"next"}`)
		case "/boards/12/pins":
			io.WriteString(w, `{"items":[{"id":"34","title":"收藏","media":{"media_type":"image","images":{"1200x":{"url":"https://i.pinimg.com/1200x/a.jpg","width":1200,"height":1600}}}}],"bookmark":null}`)
		default:
			t.Error("unexpected endpoint", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	v := testPinterestVault()
	c := newPinterestClient(pinterestSession{Config: testPinterestConfig()}, v)
	c.base = server.URL
	c.http = server.Client()
	if err := c.Exchange(context.Background(), "only-once"); err != nil {
		t.Fatal(err)
	}
	if exchanges.Load() != 1 {
		t.Fatal("code exchanged more than once")
	}
	c.token = pinterestToken{Access: "expired", Refresh: "rotating-1", ExpiresAt: 1}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			page, err := c.Boards(context.Background(), "opaque &?+")
			if err != nil || len(page.Items) != 1 || page.Bookmark != "next" {
				t.Error(page, err)
			}
		}()
	}
	wg.Wait()
	if refreshes.Load() != 1 {
		t.Fatal("concurrent requests duplicated refresh", refreshes.Load())
	}
	saved, _ := v.load()
	if saved.Token.Refresh != "rotating-2" {
		t.Fatal("rotation not persisted")
	}
	page, err := c.Pins(context.Background(), "12", "")
	if err != nil || len(pinterestPictures(page.Items)) != 1 || page.Bookmark != "" {
		t.Fatal(page, err)
	}
	if _, err := c.Pins(context.Background(), "../private", ""); err == nil {
		t.Fatal("unsafe path accepted")
	}
	if err := c.Disconnect(); err != nil {
		t.Fatal(err)
	}
	saved, _ = v.load()
	if saved.Token.Access != "" || saved.Token.Refresh != "" || saved.Config.ClientID == "" {
		t.Fatal("disconnect removed config or retained tokens")
	}
	if _, err := c.access(context.Background(), ""); !errors.Is(err, context.Canceled) {
		t.Fatal("retired client restored connection", err)
	}
}
func TestPinterestMultiImageAndUnsafeCDN(t *testing.T) {
	var pins []pinterestPin
	err := json.Unmarshal([]byte(`[{"id":"12","title":"多张","media":{"media_type":"multiple_images","items":[{"images":{"600x":{"url":"https://i.pinimg.com/a.jpg","width":600,"height":800},"1200x":{"url":"https://i.pinimg.com/b.jpg","width":1200,"height":1600}}},{"images":{"1200x":{"url":"https://i.pinimg.com/c.jpg","width":1200,"height":800}}}]}},{"id":"13","media":{"media_type":"image","images":{"1200x":{"url":"https://evil.test/a.jpg","width":1200,"height":1600}}}}]`), &pins)
	if err != nil {
		t.Fatal(err)
	}
	got := pinterestPictures(pins)
	if len(got) != 2 || got[0].Key == got[1].Key || got[0].Thumb.URL != "https://i.pinimg.com/a.jpg" || got[0].Original.URL != "https://i.pinimg.com/b.jpg" || got[1].WebURL() != "https://www.pinterest.com/pin/12/" {
		t.Fatal(got)
	}
}
func TestPinterestNativeLayouts(t *testing.T) {
	for _, size := range [][2]int{{1040, 740}, {700, 460}} {
		a := testDesktop()
		p := &pinterestState{ready: true, config: testPinterestConfig(), thumbs: map[string]*ui.Bitmap{}}
		a.materials = &materialsPanel{pin: p, importing: map[string]pendingMaterial{}, showNativePins: true}
		tt := ui.NewTester(a.pinterestView, size[0], size[1])
		tt.Frame()
		for _, name := range []string{"连接 Pinterest", "Pinterest 连接设置"} {
			if r, ok := tt.Find(name); !ok || r.X < 0 || r.Y < 0 || r.X+r.W > float32(size[0]) || r.Y+r.H > float32(size[1]) {
				t.Fatalf("landing %s clipped %v", name, r)
			}
		}
		p.connected = true
		p.boards = []pinterestBoard{{ID: "123", Name: "生活灵感", Count: 2}}
		p.board = p.boards[0]
		p.pages = []string{""}
		p.pictures = []pinterestPicture{{Key: "1:0", PinID: "1", Name: "收藏图片"}}
		tt.Frame()
		for _, name := range []string{"图板：生活灵感", "预览 Pinterest 图片：收藏图片", "导入 Pinterest 图片：收藏图片", "Pinterest 连接设置"} {
			if r, ok := tt.Find(name); !ok || r.X < 0 || r.Y < 0 || r.X+r.W > float32(size[0])+1 || r.Y+r.H > float32(size[1])+1 {
				t.Fatalf("grid %s clipped %v", name, r)
			}
		}
		p.configure = true
		if r, ok := tt.Find("搜索 Pinterest 图板"); !ok || r.H > 36 {
			t.Fatal("search became a tall card", r)
		}
		p.draft = p.config
		p.draft.ClientSecret = ""
		tt.Frame()
		if _, ok := tt.Find("Pinterest App Secret"); !ok {
			t.Fatal("missing secure configuration")
		}
	}
}
