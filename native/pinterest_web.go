package main

import (
	"context"
	_ "embed"
	"fmt"
	"net/url"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const pinterestHome = "https://www.pinterest.com/"
const pinterestLoginURL = pinterestHome + "login/"

//go:embed assets/pinterest-import.js
var pinterestImportTemplate string

//go:embed assets/web-scrollbars.js
var pinterestRails string

var pinterestImportScript = strings.NewReplacer("__CANVEPP_RAILS__", pinterestRails, "__CANVEPP_SCROLL__",
	fmt.Sprintf("{width:%g, edge:%g, idle:%d}", thumbWidth, thumbEdge, scrollIdle.Milliseconds())).Replace(pinterestImportTemplate)

type pinterestWeb struct {
	browser    *pinterestBrowser
	view       *mygo.WebView
	loadError  string
	readingPin bool
}

type webPin struct {
	Image string `json:"image"`
	Pin   string `json:"pin"`
	Name  string `json:"name"`
}

func validPinPage(raw string) bool {
	if !pinterestURL(raw) {
		return false
	}
	u, _ := url.Parse(raw)
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 2 || parts[0] != "pin" || parts[1] == "" {
		return false
	}
	for _, ch := range parts[1] {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

// Third-party pages cannot call Go. Only a user click on a Pinterest page
// can request importing a validated CDN image through this narrow navigation.
func pinFromNavigation(current, raw string, userInitiated bool) (webPin, bool) {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 8192 || !userInitiated || !pinterestURL(current) || u.Scheme != "canvepp-pinterest" || u.Host != "import" || u.User != nil || u.Fragment != "" || u.Path != "" {
		return webPin{}, false
	}
	q := u.Query()
	if len(q["image"]) != 1 || len(q["pin"]) != 1 || len(q["name"]) > 1 {
		return webPin{}, false
	}
	pin := webPin{Image: q.Get("image"), Pin: q.Get("pin"), Name: q.Get("name")}
	return pin, pinImageURL(pin.Image) && validPinPage(pin.Pin)
}

func safeExternalPage(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.Port() == ""
}

func pinterestLoginNavigation(current, target string, popup bool) bool {
	if !pinterestURL(current) {
		return false
	}
	if popup && (target == "" || target == "about:blank") {
		u, _ := url.Parse(current)
		return strings.Trim(u.Path, "/") == "login"
	}
	if !safeExternalPage(target) {
		return false
	}
	u, _ := url.Parse(target)
	if strings.EqualFold(u.Hostname(), "accounts.google.com") {
		return true
	}
	if pinterestURL(target) {
		path := strings.TrimRight(u.Path, "/")
		return path == "/connect/google" || strings.HasPrefix(path, "/connect/google/")
	}
	return false
}

func (a *desktop) handlePinterestGoogleNavigation(userInitiated bool) {
	request := pinterestLoginPrompt
	if userInitiated {
		request = pinterestGoogleClick
	}
	a.publish(func() { a.requestPinterestLogin(request) })
}

func (a *desktop) pinterestSiteLogin() {
	a.setPinterestView(false)
	a.initPinterestWeb()
	b := a.materials.web
	if b.view != nil {
		if onPinterestLoginPage(b.view.Page().URL()) {
			a.invalidateMaterials()
			return
		}
		if err := b.view.Page().LoadURL(pinterestLoginURL); err != nil {
			b.loadError = "Pinterest 登录页未能打开，请刷新重试"
		}
	}
	a.invalidateMaterials()
}

func onPinterestLoginPage(raw string) bool {
	if !pinterestURL(raw) {
		return false
	}
	u, _ := url.Parse(raw)
	return strings.Trim(u.Path, "/") == "login"
}

func (a *desktop) initPinterestWeb() {
	m := a.materials
	if m == nil {
		return
	}
	if m.web == nil {
		m.web = &pinterestWeb{}
	}
	b := m.web
	if b.view != nil || a.win == nil || a.closing || a.wantsPinterestBrowser() {
		return
	}
	opts := mygo.PageOptions{
		Untrusted: true, Session: mygo.NewSession("canvepp-pinterest"), PreloadScript: pinterestImportScript,
		SwipeNavigation: true, PinchZoom: true,
	}
	if runtime.GOOS == "darwin" {
		opts.UserAgentSuffix = "Version/18.0 Safari/605.1.15"
	}
	v, err := a.win.NewWebView(mygo.WebViewOptions{URL: "about:blank", BackgroundColor: "#fafafa", Page: opts})
	if err != nil {
		b.loadError = "网页未能打开，请重试"
		return
	}
	b.view = v
	page := v.Page()
	page.OnPageTitleUpdated(func(e *mygo.TitleEvent) { e.PreventDefault() })
	page.OnWillNavigate(func(e *mygo.NavigateEvent) {
		if pinterestLoginNavigation(page.URL(), e.URL, false) {
			e.PreventDefault()
			a.handlePinterestGoogleNavigation(e.UserInitiated)
			return
		}
		if pinterestURL(e.URL) {
			b.loadError = ""
			a.invalidateMaterials()
			return
		}
		e.PreventDefault()
		if pin, ok := pinFromNavigation(page.URL(), e.URL, e.UserInitiated); ok {
			if a.dockVisible() && a.dock.active == dockPinterest && !m.showNativePins {
				a.importPin(pin.Image, pin.Pin, pin.Name)
			}
			return
		}
		if e.UserInitiated && safeExternalPage(e.URL) {
			_ = mygo.Shell.OpenExternal(e.URL)
		}
	})
	page.SetWindowOpenHandler(func(req mygo.WindowOpenRequest) *mygo.WindowOptions {
		if pinterestLoginNavigation(page.URL(), req.URL, true) {
			a.handlePinterestGoogleNavigation(req.UserInitiated)
			return nil
		}
		if req.UserInitiated {
			if pinterestURL(req.URL) {
				a.publish(func() { _ = page.LoadURL(req.URL) })
			} else if safeExternalPage(req.URL) {
				_ = mygo.Shell.OpenExternal(req.URL)
			}
		}
		return nil
	})
	page.OnDidNavigate(func(raw string) {
		b.loadError = ""

		a.invalidateMaterials()
	})
	page.OnDidFinishLoad(func() { a.invalidateMaterials() })
	page.OnDidFailLoad(func(err *mygo.LoadError) {
		if err.Code != -999 { // A prevented import/link navigation is not a load failure.
			b.loadError = "Pinterest 暂时无法加载，请刷新重试"
			a.invalidateMaterials()
		}
	})
	page.OnRenderProcessGone(func(string) { b.loadError = "Pinterest 网页已暂停"; a.invalidateMaterials() })
	page.OnContextMenu(func(e *mygo.ContextMenuEvent) {
		pin := e.LinkURL
		if !validPinPage(pin) {
			pin = page.URL()
		}
		if !pinterestURL(page.URL()) || !pinImageURL(e.ImageURL) || !validPinPage(pin) || e.Menu == nil {
			return
		}
		image, name := e.ImageURL, e.LinkText
		e.Menu.Append(mygo.Separator(), &mygo.MenuItem{Label: "导入当前画布", Click: func(*mygo.MenuItem, *mygo.Window) { a.importPin(image, pin, name) }})
	})
	_ = page.LoadURL(pinterestHome)
}

func (a *desktop) pinterestWebView(c *ui.Context) {
	if a.wantsPinterestBrowser() {
		a.pinterestBrowserContent(c)
		return
	}
	m := a.materials
	b := m.web
	ui.Row(c).Height(36).Padding(2, 8).Gap(4).AlignItems(ui.Center).Children(func() {
		ready := b != nil && b.view != nil
		back, forward := false, false
		if ready {
			back, forward = b.view.Page().CanGoBack(), b.view.Page().CanGoForward()
		}
		disable(iconButton(c, "back", "Pinterest 后退", func() { b.view.Page().GoBack() }), !back)
		disable(iconButton(c, "forward", "Pinterest 前进", func() { b.view.Page().GoForward() }), !forward)
		iconButton(c, "refresh", "刷新 Pinterest 网页", func() {
			if ready {
				b.loadError = ""
				b.view.Page().Reload()
			} else {
				a.initPinterestWeb()
			}
		})
		ui.Spacer(c)
		disable(iconButton(c, "plus", "导入当前 Pin 图片", a.importCurrentWebPin, "导入当前 Pin，或在图片上选择 ＋"), !ready || b.readingPin || a.editor == nil)
		iconButton(c, "external", "在本机浏览器打开 Pinterest", func() {
			raw := pinterestHome
			if ready && pinterestURL(b.view.Page().URL()) {
				raw = b.view.Page().URL()
			}
			_ = mygo.Shell.OpenExternal(raw)
		})
	})
	if b != nil && b.loadError != "" {
		ui.Text(c, b.loadError).FontSize(12).TextColor(c.Theme().TextMuted).Padding(6, 22)
	}
	if b != nil && b.view != nil {
		ui.WebView(c.Key("pinterest-web-content"), b.view).Label("Pinterest 网页内容").Grow(1).FillWidth()
	} else {
		ui.Box(c).Label("Pinterest 网页内容").Grow(1).FillWidth().Center().Children(func() {
			ui.Text(c, "正在打开 Pinterest…").TextColor(c.Theme().TextMuted)
		})
	}
}

func (a *desktop) importCurrentWebPin() {
	m := a.materials
	if m == nil || m.web == nil || m.web.view == nil || m.web.readingPin {
		return
	}
	b := m.web
	page := b.view.Page()
	if !validPinPage(page.URL()) {
		m.status = "当前页面没有可导入的 Pin 图片"
		return
	}
	b.readingPin = true
	destination, at := a.editor, a.center()
	// Read public page metadata only after the native import button is clicked.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		value, err := page.EvalContext(ctx, `({image:document.querySelector('meta[property="og:image"]')?.content || '',pin:location.href,name:document.querySelector('meta[property="og:title"]')?.content || document.title})`)
		a.publish(func() {
			b.readingPin = false
			data, ok := value.(map[string]any)
			if err != nil || !ok {
				m.status = "未能读取当前 Pin 图片"
				a.invalidateMaterials()
				return
			}
			image, _ := data["image"].(string)
			pin, _ := data["pin"].(string)
			name, _ := data["name"].(string)
			a.importPinInto(destination, at, image, pin, name)
		})
	}()
}
