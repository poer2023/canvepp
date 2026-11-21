package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
)

const pinterestRedirect = "http://localhost:47843/oauth/pinterest/callback"

type pinterestConfig struct {
	ClientID      string `json:"client_id"`
	ClientSecret  string `json:"client_secret"`
	RedirectURI   string `json:"redirect_uri"`
	IncludeSecret bool   `json:"include_secret"`
}

func (c pinterestConfig) validate() error {
	if strings.TrimSpace(c.ClientID) == "" || strings.TrimSpace(c.ClientSecret) == "" {
		return errors.New("请填写 Pinterest App ID 和 App Secret")
	}
	if len(c.ClientID) > 200 || len(c.ClientSecret) > 500 || strings.ContainsAny(c.ClientID+c.ClientSecret, "\r\n") {
		return errors.New("应用凭据格式不正确")
	}
	u, err := url.Parse(c.RedirectURI)
	if err != nil || u.Scheme != "http" || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path == "" || len(c.RedirectURI) > 500 {
		return errors.New("回调地址须为本机 localhost 或 127.0.0.1 的 HTTP 地址，包含端口和路径")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1024 || port > 65535 {
		return errors.New("回调端口须介于 1024 和 65535")
	}
	return nil
}

type secretStore interface {
	Get(string) ([]byte, error)
	Set(string, []byte) error
	Delete(string) error
}

// One encrypted record keeps credentials and rotated tokens consistent. It is
// outside the canvas database and its backups. The data-directory key also
// prevents acceptance builds from borrowing the user's connection.
type pinterestSession struct {
	Config pinterestConfig `json:"config"`
	Token  pinterestToken  `json:"token"`
}
type pinterestVault struct {
	mu    sync.Mutex
	store secretStore
	key   string
}

func newPinterestVault(dir string) *pinterestVault {
	return &pinterestVault{store: mygo.Secrets, key: pinterestVaultKey(dir)}
}
func (v *pinterestVault) load() (pinterestSession, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	var s pinterestSession
	b, err := v.store.Get(v.key)
	if errors.Is(err, mygo.ErrSecretNotFound) {
		return s, nil
	}
	if err != nil {
		return s, errors.New("无法读取系统钥匙串，请解锁后重试")
	}
	if json.Unmarshal(b, &s) != nil {
		return s, errors.New("Pinterest 连接配置无法读取，请重新配置")
	}
	return s, nil
}
func (v *pinterestVault) save(s pinterestSession) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if err := v.store.Set(v.key, encode(s)); err != nil {
		return errors.New("无法保存到系统钥匙串，连接尚未保存")
	}
	return nil
}

type pinterestAuthResult struct {
	Code string
	Err  error
}
type pinterestAuthorization struct {
	URL           string
	callbackTitle string
	Done          chan pinterestAuthResult
	cancel        context.CancelFunc
}

func (a *pinterestAuthorization) Cancel() { a.cancel() }

func startPinterestAuthorization(parent context.Context, config pinterestConfig, authorizeURL string) (*pinterestAuthorization, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	u, _ := url.Parse(config.RedirectURI)
	l, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", u.Port()))
	if err != nil {
		return nil, errors.New("本机授权回调端口被占用，请关闭其他授权窗口后重试")
	}
	return listenPinterestAuthorization(parent, config, authorizeURL, l)
}

func listenPinterestAuthorization(parent context.Context, config pinterestConfig, authorizeURL string, listener net.Listener) (*pinterestAuthorization, error) {
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		listener.Close()
		return nil, errors.New("无法创建安全授权请求")
	}
	state := base64.RawURLEncoding.EncodeToString(nonce[:])
	u, err := url.Parse(authorizeURL)
	if err != nil {
		listener.Close()
		return nil, errors.New("授权地址无效")
	}
	scope := "boards:read,pins:read"
	if config.IncludeSecret {
		scope += ",boards:read_secret,pins:read_secret"
	}
	q := u.Query()
	q.Set("client_id", config.ClientID)
	q.Set("redirect_uri", config.RedirectURI)
	q.Set("response_type", "code")
	q.Set("scope", scope)
	q.Set("state", state)
	u.RawQuery = q.Encode()
	ctx, cancel := context.WithTimeout(parent, 5*time.Minute)
	a := &pinterestAuthorization{URL: u.String(), callbackTitle: "Canvepp · Pinterest · " + digest(nonce[:])[:8], Done: make(chan pinterestAuthResult, 1), cancel: cancel}
	callback, _ := url.Parse(config.RedirectURI)
	var accepted atomic.Bool
	var once sync.Once
	finish := func(result pinterestAuthResult) { once.Do(func() { a.Done <- result; cancel() }) }
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 5 * time.Second, MaxHeaderBytes: 8192}
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'; base-uri 'none'")
		if r.Method != "GET" || r.Host != callback.Host || r.URL.Path != callback.Path || len(r.URL.RawQuery) > 8192 {
			http.Error(w, "Invalid callback", http.StatusBadRequest)
			return
		}
		values, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || len(values["state"]) != 1 || subtle.ConstantTimeCompare([]byte(values.Get("state")), []byte(state)) != 1 {
			http.Error(w, "Authorization does not match. Return to Canvepp and try again.", http.StatusBadRequest)
			return
		}
		result := pinterestAuthResult{}
		if len(values["error"]) == 1 && values.Get("error") != "" {
			result.Err = errors.New("未完成 Pinterest 授权，可重新连接")
		} else if len(values["code"]) == 1 && values.Get("code") != "" && len(values["error"]) == 0 {
			result.Code = values.Get("code")
		} else {
			http.Error(w, "Missing authorization code", http.StatusBadRequest)
			return
		}
		if !accepted.CompareAndSwap(false, true) {
			http.Error(w, "Authorization already received", http.StatusConflict)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		title := "授权已收到"
		if result.Err != nil {
			title = "授权未完成"
		}
		// A 200 document, not a second redirect: Pinterest requires this. No
		// token, state or authorization code is reflected in the response.
		fmt.Fprintf(w, `<!doctype html><html lang="zh"><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>%s</title><style>body{font:16px system-ui;background:#f7f7f8;color:#25262a;display:grid;place-content:center;min-height:90vh;text-align:center}h1{font-size:28px}a{color:inherit;display:inline-block;padding:12px 20px;background:#fff;border-radius:12px;text-decoration:none}p{color:#76767c;margin-bottom:28px}</style><h1>%s</h1><p>正在返回 Canvepp。此页面可以关闭。</p><a href="canvepp://materials/pinterest">返回 Canvepp</a></html>`, a.callbackTitle, title)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		finish(result)
	})
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			finish(pinterestAuthResult{Err: errors.New("本机授权回调已停止，请重试")})
		}
	}()
	go func() {
		<-ctx.Done()
		message := "已取消授权"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			message = "授权等待已超时，请重新连接"
		}
		finish(pinterestAuthResult{Err: errors.New(message)})
		shutdown, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		_ = server.Shutdown(shutdown)
	}()
	return a, nil
}
