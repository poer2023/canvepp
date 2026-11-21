package main

import (
	"net/url"
	"testing"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

func TestWebImportRequiresUserClickAndPinterestSources(t *testing.T) {
	q := url.Values{"image": {"https://i.pinimg.com/originals/a.png"}, "pin": {"https://www.pinterest.com/pin/123/"}, "name": {"茶杯"}}
	raw := "canvepp-pinterest://import?" + q.Encode()
	if p, ok := pinFromNavigation(pinterestHome, raw, true); !ok || p.Name != "茶杯" {
		t.Fatal("valid click rejected", p)
	}
	for _, tc := range []struct {
		page, raw string
		click     bool
	}{
		{pinterestHome, raw, false}, {"https://evil.test/", raw, true},
		{pinterestHome, raw + "&image=https://i.pinimg.com/b.jpg", true},
		{pinterestHome, "canvepp-pinterest://user@import?" + q.Encode(), true},
		{pinterestHome, raw + "#hidden", true},
	} {
		if _, ok := pinFromNavigation(tc.page, tc.raw, tc.click); ok {
			t.Fatal("unsafe import accepted", tc)
		}
	}
	for _, invalid := range []string{"https://www.pinterest.com/me/", "https://www.pinterest.com/pin/", "https://www.pinterest.com/pin/123/other", "https://www.pinterest.com/pin/script", "https://pinterest.com.evil.test/pin/123/"} {
		if validPinPage(invalid) {
			t.Fatal("invalid pin", invalid)
		}
	}
}

func TestPinterestLoginHandoffTargets(t *testing.T) {
	for _, tc := range []struct {
		page, target string
		popup, login bool
	}{
		{pinterestHome, "https://accounts.google.com/o/oauth2/auth?state=website", true, true},
		{pinterestLoginURL, "https://accounts.google.com/gsi/select", false, true},
		{pinterestLoginURL, "about:blank", true, true},
		{pinterestHome, pinterestHome + "connect/google/", false, true},
		{pinterestHome, "https://accounts.google.com.evil.test/", true, false},
		{pinterestHome, "https://accounts.google.com@evil.test/", false, false},
		{"https://evil.test/", "https://accounts.google.com/", true, false},
		{pinterestHome, pinterestHome + "pin/123/", true, false},
		{pinterestHome, pinterestLoginURL, false, false},
	} {
		if got := pinterestLoginNavigation(tc.page, tc.target, tc.popup); got != tc.login {
			t.Errorf("handoff %s: got %t", tc.target, got)
		}
	}
}

func TestPinterestAuthorizedViewSurvivesNavigationAndRestart(t *testing.T) {
	s := testStore(t)
	a := testDesktop()
	a.store = s
	a.returnToPinterestCollections()
	if !a.dockVisible() || !a.materials.showNativePins {
		t.Fatal("did not return to original canvas dock")
	}
	a.materials.pin = &pinterestState{ready: true, connected: true}
	a.materials.connected = true
	a.materials.eagle = &eagleClient{Name: "Eagle 测试库"}
	tt := ui.NewTester(a.view, 1200, 800)
	for _, label := range []string{"添加侧栏页面", "Eagle", "侧栏：Pinterest", "关闭当前侧栏页面", "添加侧栏页面", "Pinterest"} {
		var err error
		if label == dockPinterest || label == dockEagle {
			err = tt.ChooseMenuItem(label)
		} else {
			err = tt.Click(label)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if !a.materials.showNativePins {
		t.Fatal("switching away discarded connected view")
	}
	reopened := &desktop{store: s}
	reopened.prepareMaterialBrowser()
	if !reopened.materials.showNativePins {
		t.Fatal("startup ignored saved collection preference")
	}
	reopened.setPinterestView(false)
	website := &desktop{store: s}
	website.prepareMaterialBrowser()
	if website.materials.showNativePins || !website.materials.pinViewChosen {
		t.Fatal("explicit switch to website was lost")
	}
}

func TestPinterestGoogleClickWithoutClientOpensDockSetup(t *testing.T) {
	a := testDesktop()
	a.store = testStore(t)
	a.openDock(dockPinterest)
	p := &pinterestState{ready: true}
	a.materials.pin = p
	a.requestPinterestLogin(pinterestLoginToolbar)
	a.requestPinterestLogin(pinterestLoginPrompt)
	if a.materials.web == nil || a.materials.showNativePins {
		t.Fatal("website login was lost")
	}
	a.requestPinterestLogin(pinterestGoogleClick)
	if !a.dockVisible() || !p.configure || !p.connectAfterConfig || !a.materials.showNativePins {
		t.Fatal("Google login did not open the collection setup in the canvas")
	}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if _, ok := tt.Find("Pinterest App ID"); !ok {
		t.Fatal("missing app configuration")
	}
}

func TestPinterestBrowserDragRepresentations(t *testing.T) {
	const imageURL = "https://i.pinimg.com/736x/aa/bb/sample.jpg"
	const pinURL = "https://www.pinterest.com/pin/123456/"
	fragment := `<a href="` + pinURL + `"><img alt="茶杯" src="` + imageURL + `"></a>`
	for _, tc := range []struct {
		reps []transfer.Representation
		want webPin
	}{
		{[]transfer.Representation{transfer.Bytes(transfer.HTML, []byte(fragment))}, webPin{Pin: pinURL, Image: imageURL, Name: "茶杯"}},
		{[]transfer.Representation{transfer.Bytes(transfer.URIList, []byte("# source\r\n"+pinURL+"\r\n"))}, webPin{Pin: pinURL}},
		{[]transfer.Representation{transfer.Bytes(transfer.Text, []byte(imageURL))}, webPin{Image: imageURL}},
		{[]transfer.Representation{transfer.Bytes(transfer.HTML, []byte(`<a href="https://pinterest.com.evil.test/pin/123/"><img src="http://127.0.0.1/secret"></a>`))}, webPin{}},
	} {
		if got := browserPin(transfer.New(transfer.NewItem(tc.reps...))); got != tc.want {
			t.Errorf("drop: got %#v, want %#v", got, tc.want)
		}
	}
}
