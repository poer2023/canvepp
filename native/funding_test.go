package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestFundingSettingsRouteIndependently(t *testing.T) {
	s := testStore(t)
	a, err := newDesktop(s)
	if err != nil {
		t.Fatal(err)
	}
	if a.funding.Image != codexFunding || a.funding.Summary != codexFunding {
		t.Fatal("unexpected fresh defaults")
	}
	tt := ui.NewTester(a.settingsView, 850, 760)
	for _, pair := range [][2]fundingSource{{apiFunding, codexFunding}, {apiFunding, apiFunding}, {codexFunding, apiFunding}, {codexFunding, codexFunding}} {
		for i, title := range []string{"生图额度", "摘要额度"} {
			if err := tt.Click(title + "来源"); err != nil {
				t.Fatal(err)
			}
			key := ui.KeyUp
			if pair[i] == apiFunding {
				key = ui.KeyDown
			}
			tt.Key(0, key)
			tt.Key(0, ui.KeyEnter)
			tt.Frame()
		}
		restored, err := newDesktop(s)
		if err != nil || restored.funding.Image != pair[0] || restored.funding.Summary != pair[1] {
			t.Fatal(pair, err, restored.funding)
		}
	}
}

func TestFundingOfficialLoginFixture(t *testing.T) {
	fixtureCodex(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, denied := range []string{"", "1"} {
		t.Setenv("CANVEPP_LOGIN_DENIED", denied)
		c, err := startCodex(ctx)
		if err != nil {
			t.Fatal(err)
		}
		opened := false
		err = c.loginChatGPT(ctx, func(login chatLogin) error {
			u, e := url.Parse(login.URL)
			opened = e == nil && u.Host == "auth.openai.com"
			return nil
		})
		if !opened || (err != nil) != (denied == "1") {
			t.Fatal("incorrect login receipt or authorization origin", err)
		}
		if denied == "" {
			account, err := c.readChatGPTAccount(ctx)
			if err != nil || account.Email != "studio@example.com" || account.Plan != "pro" {
				t.Fatal("account read must retain the identity and plan after login", err)
			}
		}
		c.Close()
	}
}

func TestFundingAPIImagesAndSummary(t *testing.T) {
	s := testStore(t)
	calls := fixtureCodex(t)
	c := s.openAI
	c.secrets = &pinterestMemorySecrets{}
	if err := c.secrets.Set(c.secretID, []byte("test-only")); err != nil {
		t.Fatal(err)
	}
	var edits, creates, summaries atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-only" {
			t.Error("authentication missing")
		}
		switch r.URL.Path {
		case "/models":
			io.WriteString(w, `{"object":"list","data":[]}`)
			return
		case "/responses":
			var v struct {
				Model, Input string
				Store        *bool
				Reasoning    map[string]string
			}
			if json.NewDecoder(r.Body).Decode(&v) != nil || v.Model != textModel || v.Reasoning["effort"] != defaultEffort || v.Input != "events" || v.Store == nil || *v.Store {
				t.Error("wrong summary body")
			}
			summaries.Add(1)
			io.WriteString(w, `{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"摘要"}]}]}`)
			return
		case "/images/generations":
			var v map[string]string
			if json.NewDecoder(r.Body).Decode(&v) != nil || v["model"] != apiImageModel || v["size"] != "1024x1024" || v["quality"] != "high" {
				t.Error("wrong image request")
			}
			creates.Add(1)
		case "/images/edits":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
				return
			}
			if len(r.MultipartForm.File["image[]"]) != 2 {
				t.Error("missing reference")
			}
			f, h, err := r.FormFile("image[]")
			if err != nil {
				t.Error(err)
				return
			}
			defer f.Close()
			data, err := io.ReadAll(f)
			if err != nil || !bytes.Equal(data, tinyPNG()) || h.Header.Get("Content-Type") != "image/png" || r.FormValue("model") != apiImageModel {
				t.Error("original image was not forwarded")
			}
			edits.Add(1)
		default:
			t.Error("unexpected endpoint", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]string{"b64_json": base64.StdEncoding.EncodeToString(tinyPNG())}}})
	}))
	defer server.Close()
	c.base = server.URL
	if _, err := c.verify(t.Context(), "test-only"); err != nil {
		t.Fatal(err)
	}
	reply, err := summarizeWithFunding(t.Context(), s, apiFunding, modelChoice{}, "events")
	if err != nil || reply.Text != "摘要" {
		t.Fatal(reply, err)
	}
	var refs []Asset
	for i := 0; i < 2; i++ {
		r := NativeRun{ID: uuid(), NodeID: uuid(), ProjectID: "p", Status: "queued", Sources: refs, Prompt: "test", Params: defaults(), Funding: apiFunding, Model: apiImageModel}
		if err := s.QueueRuns([]NativeRun{r}); err != nil {
			t.Fatal(err)
		}
		var got NativeRun
		var result *Asset
		executeNative(t.Context(), s, r, false, func(o NativeRun, a *Asset) { got = o; result = a })
		if got.Status != "succeeded" || result == nil || result.Source != "api" || result.SHA != digest(tinyPNG()) || (i == 1 && (len(result.Parents) != 2 || result.Parents[0] != refs[0].ID || result.Parents[1] != refs[1].ID)) {
			t.Fatal(got, result)
		}
		if i == 0 {
			second, err := s.ImportImage(tinyPNG(), "p", uuid(), "extra.png", "import", "", nil)
			if err != nil {
				t.Fatal(err)
			}
			refs = []Asset{*result, second}
		}
		for _, recover := range []bool{false, true} {
			executeNative(t.Context(), s, r, recover, func(o NativeRun, _ *Asset) { got = o })
			if got.Status != "succeeded" {
				t.Fatalf("lost successful receipt: %s", got.Status)
			}
		}
	}
	if edits.Load() != 1 || creates.Load() != 1 || summaries.Load() != 1 {
		t.Fatal("duplicated or missing request")
	}
	if _, err := os.Stat(calls); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("API request contacted Codex")
	}
}

func TestFundingAPIFailureNoFallback(t *testing.T) {
	s := testStore(t)
	calls := fixtureCodex(t)
	c := s.openAI
	c.secrets = &pinterestMemorySecrets{}
	if _, err := summarizeWithFunding(t.Context(), s, apiFunding, modelChoice{}, "events"); err == nil {
		t.Fatal("missing key accepted")
	}
	c.secrets.Set(c.secretID, []byte("test-only"))
	for _, code := range []int{401, 403, 429, 503} {
		count := atomic.Int32{}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "POST" {
				count.Add(1)
			}
			w.WriteHeader(code)
		}))
		c.base = srv.URL
		_, err := c.verify(t.Context(), "test-only")
		if err == nil {
			t.Fatal("model list failure misclassified", code)
		}
		r := NativeRun{ID: uuid(), ProjectID: "p", NodeID: uuid(), Funding: apiFunding, Status: "queued", Params: defaults()}
		if err := s.QueueRuns([]NativeRun{r}); err != nil {
			t.Fatal(err)
		}
		var got NativeRun
		for _, recover := range []bool{false, true, false} {
			executeNative(t.Context(), s, r, recover, func(o NativeRun, _ *Asset) { got = o })
		}
		if count.Load() != 1 || (code == 503 && got.Status != "unknown") || (code != 503 && got.Status != "failed") {
			t.Fatal(code, count.Load(), got)
		}
		srv.Close()
	}
	if _, err := os.Stat(calls); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed API request contacted Codex")
	}
}

func TestBundledCodexCleanAccount(t *testing.T) {
	path := os.Getenv("CANVEPP_TEST_BUNDLED_CODEX")
	if path == "" {
		t.Skip("release smoke requires packaged runtime")
	}
	t.Setenv("CANVEPP_CODEX_BIN", path)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("cli_auth_credentials_store = \"file\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	// Windows may finish releasing Codex's SQLite handles just after the
	// subprocess exits. Wait only for this test's isolated profile.
	t.Cleanup(func() {
		deadline := time.Now().Add(5 * time.Second)
		for {
			err := os.RemoveAll(dir)
			if err == nil {
				return
			}
			if time.Now().After(deadline) {
				t.Error(err)
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	})
	t.Setenv("CODEX_HOME", dir)
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("CODEX_API_KEY", "")
	if sys := os.Getenv("SystemRoot"); sys != "" {
		t.Setenv("PATH", filepath.Join(sys, "System32"))
	} else {
		t.Setenv("PATH", "/usr/bin:/bin")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	c, err := startCodex(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err = c.Account(ctx); err == nil || !strings.Contains(err.Error(), "请登录") {
		t.Fatalf("clean profile: %v", err)
	}
	login, err := c.beginChatLogin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if login.UserCode != "" {
		t.Log("official device login ready")
	} else {
		t.Log("official loopback login ready")
	}
	if err = c.Call(ctx, "account/login/cancel", map[string]string{"loginId": login.LoginID}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestOfficialAPIConnectivity(t *testing.T) {
	if os.Getenv("CANVEPP_TEST_OFFICIAL_API") != "1" {
		t.Skip("external network smoke")
	}
	c := newOpenAIClient(t.TempDir())
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	_, err := c.verify(ctx, "canvepp-connectivity-test-invalid")
	if !errors.Is(err, apiFailure(401)) {
		t.Fatalf("expected official authentication challenge: %v", err)
	}
}

func TestFundingRestrictedVerification(t *testing.T) {
	c := newOpenAIClient(t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		io.WriteString(w, `{"error":{"code":"insufficient_permissions"}}`)
	}))
	defer server.Close()
	c.base = server.URL
	status, err := c.verify(t.Context(), "test-restricted")
	if err != nil || !strings.Contains(status, "无权限") {
		t.Fatal(status, err)
	}
}

func TestFundingLoginReservedPort(t *testing.T) {
	fixtureCodex(t)
	t.Setenv("CANVEPP_LOGIN_PORT_BUSY", "1")
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	c, err := startCodex(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	opened := false
	err = c.loginChatGPT(ctx, func(login chatLogin) error {
		opened = login.UserCode == "EXAMPLE" && login.URL == "https://auth.openai.com/codex/device"
		return nil
	})
	if !opened || err != nil {
		t.Fatal("device fallback did not complete", err)
	}
}

func TestFundingAPIInterruptedImport(t *testing.T) {
	s := testStore(t)
	p := Project{ID: uuid(), Name: "恢复", Graph: Graph{Nodes: []Node{{ID: "n"}}}}
	if err := s.Save(p, nil); err != nil {
		t.Fatal(err)
	}
	r := NativeRun{ID: uuid(), ProjectID: p.ID, NodeID: "n", Status: "submitted", Funding: apiFunding}
	if err := s.SaveRun(r); err != nil {
		t.Fatal(err)
	}
	img, err := s.ImportImage(tinyPNG(), p.ID, r.NodeID, "test.png", "api", r.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	pending := r
	pending.ID = uuid()
	if err := s.SaveRun(pending); err != nil {
		t.Fatal(err)
	}
	app, err := newDesktop(s)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.NativeRun(r.ID)
	if err != nil || got.Status != "succeeded" || app.editor.Node("n").Asset != img.ID {
		t.Fatal("durable API image not reattached on first open", err)
	}
	got, err = s.NativeRun(pending.ID)
	if err != nil || got.Status != "unknown" {
		t.Fatal("ambiguous API request not preserved", err)
	}
}
