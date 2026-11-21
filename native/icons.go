package main

import "github.com/egoist/mygo/ui"

// Lucide 0.468 paths share a 24-unit grid. See assets/lucide-LICENSE.
func glyph(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` + shapes + `</svg>`))
}

var icons = map[string]*ui.SVG{
	"more":          glyph(`<circle cx="5" cy="12" r="1" fill="currentColor"/><circle cx="12" cy="12" r="1" fill="currentColor"/><circle cx="19" cy="12" r="1" fill="currentColor"/>`),
	"hourglass":     glyph(`<path d="M5 3h14M5 21h14M6 3v3c0 3 3 4 6 6-3 2-6 3-6 6v3m12-18v3c0 3-3 4-6 6 3 2 6 3 6 6v3"/>`),
	"search-empty":  glyph(`<circle cx="10" cy="10" r="7"/><path d="m15.5 15.5 5 5M7.5 10h5"/>`),
	"warning":       glyph(`<path d="m10.3 3.8-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.7-3.2l-8-14a2 2 0 0 0-3.4 0Z"/><path d="M12 8v5m0 3h.01"/>`),
	"sidebar-close": glyph(`<path d="M5 4v16m12-13-5 5 5 5"/>`),
	"sidebar-open":  glyph(`<path d="M5 4v16m7-13 5 5-5 5"/>`),
	"chat":          glyph(`<path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"></path>`),
	"code":          glyph(`<path d="m18 16 4-4-4-4M6 8l-4 4 4 4"></path>`),
	"user":          glyph(`<circle cx="12" cy="8" r="4"/><path d="M4 21v-2a4 4 0 0 1 4-4h8a4 4 0 0 1 4 4v2"/>`),
	"picture":       glyph(`<rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="9" cy="9" r="2"/><path d="m21 15-3.1-3.1a2 2 0 0 0-2.8 0L6 21"/>`),
	"text-lines":    glyph(`<path d="M4 6h16M4 10h12M4 14h16M4 18h10"/>`),
	"type":          glyph(`<path d="M4 7V4h16v3M12 4v16m-4 0h8"/>`),
	"subscription":  glyph(`<path d="M3.85 8.62a4 4 0 0 1 4.78-4.78 4 4 0 0 1 6.74 0 4 4 0 0 1 4.78 4.78 4 4 0 0 1 0 6.76 4 4 0 0 1-4.78 4.78 4 4 0 0 1-6.74 0 4 4 0 0 1-4.78-4.78 4 4 0 0 1 0-6.76Z"/><path d="m9 12 2 2 4-4"/>`),
	"plug":          glyph(`<path d="m14 7 3-3m0 6 3-3M7 17l-2 2m1-5 8-8 4 4-8 8H7Z"/>`),
	"upload":        glyph(`<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path><polyline points="17 8 12 3 7 8"></polyline><line x1="12" x2="12" y1="3" y2="15"></line>`),
	"image-up":      glyph(`<path d="M10.3 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2v10l-3.1-3.1a2 2 0 0 0-2.814.014L6 21"/><path d="m14 19.5 3-3 3 3M17 22v-5.5"/><circle cx="9" cy="9" r="2"/>`),
	"login":         glyph(`<path d="M15 3h4a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2h-4"></path><polyline points="10 17 15 12 10 7"></polyline><line x1="15" x2="3" y1="12" y2="12"></line>`),
	"unlink":        glyph(`<path d="M15 7h2a5 5 0 0 1 0 10h-2m-6 0H7A5 5 0 0 1 7 7h2"></path>`),
	"keyboard":      glyph(`<path d="M10 8h.01"></path><path d="M12 12h.01"></path><path d="M14 8h.01"></path><path d="M16 12h.01"></path><path d="M18 8h.01"></path><path d="M6 8h.01"></path><path d="M7 16h10"></path><path d="M8 12h.01"></path><rect width="20" height="16" x="2" y="4" rx="2"></rect>`),
	"database":      glyph(`<ellipse cx="12" cy="5" rx="9" ry="3"></ellipse><path d="M3 5V19A9 3 0 0 0 21 19V5"></path><path d="M3 12A9 3 0 0 0 21 12"></path>`),
	"shield":        glyph(`<path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"></path><path d="m9 12 2 2 4-4"></path>`),
	"attach":        glyph(`<path d="M13.234 20.252 21 12.3"></path><path d="m16 6-8.414 8.586a2 2 0 0 0 0 2.828 2 2 0 0 0 2.828 0l8.414-8.586a4 4 0 0 0 0-5.656 4 4 0 0 0-5.656 0l-8.415 8.585a6 6 0 1 0 8.486 8.486"></path>`),
	"file":          glyph(`<path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z"></path><path d="M14 2v4a2 2 0 0 0 2 2h4"></path><path d="M10 9H8"></path><path d="M16 13H8"></path><path d="M16 17H8"></path>`),
	"library":       glyph(`<path d="M18 22H4a2 2 0 0 1-2-2V6"></path><path d="m22 13-1.296-1.296a2.41 2.41 0 0 0-3.408 0L11 18"></path><circle cx="12" cy="8" r="2"></circle><rect width="16" height="16" x="6" y="2" rx="2"></rect>`),
	"send":          glyph(`<path d="m5 12 7-7 7 7"></path><path d="M12 19V5"></path>`),
	"log":           glyph(`<path d="M3 12h.01"></path><path d="M3 18h.01"></path><path d="M3 6h.01"></path><path d="M8 12h13"></path><path d="M8 18h13"></path><path d="M8 6h13"></path>`),
	"lock":          glyph(`<rect x="4" y="11" width="16" height="10" rx="2"></rect><path d="M8 11V7a4 4 0 0 1 8 0v4"></path>`),
	"unlock":        glyph(`<rect x="4" y="11" width="16" height="10" rx="2"></rect><path d="M8 11V7a4 4 0 0 1 7.7-1.5"></path>`),
	"settings":      glyph(`<path d="M20 7h-9"></path><path d="M14 17H5"></path><circle cx="17" cy="17" r="3"></circle><circle cx="7" cy="7" r="3"></circle>`),
	"search":        glyph(`<circle cx="11" cy="11" r="8"></circle><path d="m21 21-4.3-4.3"></path>`),
	"sort":          glyph(`<path d="m3 16 4 4 4-4"></path><path d="M7 20V4"></path><path d="M11 4h10"></path><path d="M11 8h7"></path><path d="M11 12h4"></path>`),
	"canvas":        glyph(`<rect width="18" height="18" x="3" y="3" rx="2"></rect><path d="M3 9h18"></path><path d="M9 21V9"></path>`),
	"calendar":      glyph(`<path d="M8 2v4"></path><path d="M16 2v4"></path><rect width="18" height="18" x="3" y="4" rx="2"></rect><path d="M3 10h18"></path><path d="M8 14h.01"></path><path d="M12 14h.01"></path><path d="M16 14h.01"></path><path d="M8 18h.01"></path><path d="M12 18h.01"></path><path d="M16 18h.01"></path>`),
	"refresh":       glyph(`<path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"></path><path d="M21 3v5h-5"></path><path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"></path><path d="M8 16H3v5"></path>`),
	"history":       glyph(`<path d="M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8"></path><path d="M3 3v5h5"></path><path d="M12 7v5l4 2"></path>`),
	"return":        glyph(`<polyline points="9 14 4 9 9 4"></polyline><path d="M20 20v-7a4 4 0 0 0-4-4H4"></path>`),
	"sliders":       glyph(`<line x1="21" x2="14" y1="4" y2="4"></line><line x1="10" x2="3" y1="4" y2="4"></line><line x1="21" x2="12" y1="12" y2="12"></line><line x1="8" x2="3" y1="12" y2="12"></line><line x1="21" x2="16" y1="20" y2="20"></line><line x1="12" x2="3" y1="20" y2="20"></line><line x1="14" x2="14" y1="2" y2="6"></line><line x1="8" x2="8" y1="10" y2="14"></line><line x1="16" x2="16" y1="18" y2="22"></line>`),
	"grip":          glyph(`<circle cx="9" cy="12" r="1"></circle><circle cx="9" cy="5" r="1"></circle><circle cx="9" cy="19" r="1"></circle><circle cx="15" cy="12" r="1"></circle><circle cx="15" cy="5" r="1"></circle><circle cx="15" cy="19" r="1"></circle>`),
	"close":         glyph(`<path d="M18 6 6 18"></path><path d="m6 6 12 12"></path>`),
	"minus":         glyph(`<path d="M5 12h14"></path>`),
	"plus":          glyph(`<path d="M5 12h14"></path><path d="M12 5v14"></path>`),
	"square-plus":   glyph(`<rect x="3" y="3" width="18" height="18" rx="2"/><path d="M8 12h8M12 8v8"/>`),
	"expand":        glyph(`<path d="M8 3H5a2 2 0 0 0-2 2v3"></path><path d="M21 8V5a2 2 0 0 0-2-2h-3"></path><path d="M3 16v3a2 2 0 0 0 2 2h3"></path><path d="M16 21h3a2 2 0 0 0 2-2v-3"></path>`),
	"download":      glyph(`<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path><polyline points="7 10 12 15 17 10"></polyline><line x1="12" x2="12" y1="15" y2="3"></line>`),
	"copy":          glyph(`<rect width="14" height="14" x="8" y="8" rx="2" ry="2"></rect><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"></path>`),
	"trash":         glyph(`<path d="M3 6h18"></path><path d="M19 6v14c0 1-1 2-2 2H7c-1 0-2-1-2-2V6"></path><path d="M8 6V4c0-1 1-2 2-2h4c1 0 2 1 2 2v2"></path><line x1="10" x2="10" y1="11" y2="17"></line><line x1="14" x2="14" y1="11" y2="17"></line>`),
	"undo":          glyph(`<path d="M9 14 4 9l5-5"></path><path d="M4 9h10.5a5.5 5.5 0 0 1 5.5 5.5a5.5 5.5 0 0 1-5.5 5.5H11"></path>`),
	"redo":          glyph(`<path d="m15 14 5-5-5-5"></path><path d="M20 9H9.5A5.5 5.5 0 0 0 4 14.5A5.5 5.5 0 0 0 9.5 20H13"></path>`),
	"back":          glyph(`<path d="m12 19-7-7 7-7"></path><path d="M19 12H5"></path>`),
	"forward":       glyph(`<path d="M5 12h14"></path><path d="m12 5 7 7-7 7"></path>`),
	"external":      glyph(`<path d="M15 3h6v6"></path><path d="M10 14 21 3"></path><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"></path>`),
	"link":          glyph(`<path d="M9 17H7A5 5 0 0 1 7 7h2"></path><path d="M15 7h2a5 5 0 1 1 0 10h-2"></path><line x1="8" x2="16" y1="12" y2="12"></line>`),
	"spark":         glyph(`<path d="M9.937 15.5A2 2 0 0 0 8.5 14.063l-6.135-1.582a.5.5 0 0 1 0-.962L8.5 9.936A2 2 0 0 0 9.937 8.5l1.582-6.135a.5.5 0 0 1 .963 0L14.063 8.5A2 2 0 0 0 15.5 9.937l6.135 1.581a.5.5 0 0 1 0 .964L15.5 14.063a2 2 0 0 0-1.437 1.437l-1.582 6.135a.5.5 0 0 1-.963 0z"></path><path d="M20 3v4"></path><path d="M22 5h-4"></path><path d="M4 17v2"></path><path d="M5 18H3"></path>`),
	"branch":        glyph(`<line x1="6" x2="6" y1="3" y2="15"></line><circle cx="18" cy="6" r="3"></circle><circle cx="6" cy="18" r="3"></circle><path d="M18 9a9 9 0 0 1-9 9"></path>`),
	"chevron-down":  glyph(`<path d="m6 9 6 6 6-6"></path>`),
	"chevron-up":    glyph(`<path d="m18 15-6-6-6 6"></path>`),
	"check":         glyph(`<path d="M20 6 9 17l-5-5"></path>`),
	"reuse":         glyph(`<polyline points="15 14 20 9 15 4"></polyline><path d="M4 20v-7a4 4 0 0 1 4-4h12"></path>`),
	"edit":          glyph(`<path d="M21.174 6.812a1 1 0 0 0-3.986-3.987L3.842 16.174a2 2 0 0 0-.5.83l-1.321 4.352a.5.5 0 0 0 .623.622l4.353-1.32a2 2 0 0 0 .83-.497z"></path><path d="m15 5 4 4"></path>`),
	"system":        glyph(`<rect width="20" height="14" x="2" y="3" rx="2"></rect><line x1="8" x2="16" y1="21" y2="21"></line><line x1="12" x2="12" y1="17" y2="21"></line>`),
	"layers":        glyph(`<path d="m12 2 10 5-10 5L2 7z"></path><path d="m2 12 10 5 10-5M2 17l10 5 10-5"></path>`),
	"key":           glyph(`<circle cx="7.5" cy="15.5" r="5.5"></circle><path d="m11.5 11.5 9-9m-4 4L20 10l3-3-3.5-3.5"></path>`),
	"light":         glyph(`<circle cx="12" cy="12" r="4"></circle><path d="M12 2v2"></path><path d="M12 20v2"></path><path d="m4.93 4.93 1.41 1.41"></path><path d="m17.66 17.66 1.41 1.41"></path><path d="M2 12h2"></path><path d="M20 12h2"></path><path d="m6.34 17.66-1.41 1.41"></path><path d="m19.07 4.93-1.41 1.41"></path>`),
	"dark":          glyph(`<path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z"></path>`),
	"group":         glyph(`<path d="M20 10a1 1 0 0 0 1-1V6a1 1 0 0 0-1-1h-2.5a1 1 0 0 1-.8-.4l-.9-1.2A1 1 0 0 0 15 3h-2a1 1 0 0 0-1 1v5a1 1 0 0 0 1 1Z"></path><path d="M20 21a1 1 0 0 0 1-1v-3a1 1 0 0 0-1-1h-2.9a1 1 0 0 1-.88-.55l-.42-.85a1 1 0 0 0-.92-.6H13a1 1 0 0 0-1 1v5a1 1 0 0 0 1 1Z"></path><path d="M3 5a2 2 0 0 0 2 2h3"></path><path d="M3 3v13a2 2 0 0 0 2 2h3"></path>`),
	"canvases":      glyph(`<rect x="7" y="6" width="14" height="14" rx="2"/><path d="M16 3H5a2 2 0 0 0-2 2v11"/>`),
	"board":         glyph(`<line x1="22" x2="2" y1="6" y2="6"></line><line x1="22" x2="2" y1="18" y2="18"></line><line x1="6" x2="6" y1="2" y2="22"></line><line x1="18" x2="18" y1="2" y2="22"></line>`),
}

func uiIcon(c *ui.Context, icon string, size float32) ui.Element {
	return ui.Icon(c, icons[icon]).Size(size, size).Shrink(0).AlignSelf(ui.Center)
}

func iconControl(c *ui.Context, name, label, tip string, base, fg ui.Color, disabled bool) ui.Element {
	b := ui.ButtonBase(c.Key(label)).Label(label).Size(30, 30).Padding(6).Radius(6).Border(0, ui.Transparent)
	feedback(c, b, base, fg).Children(func() { uiIcon(c, name, 16) })
	return disable(commandHint(c, b, tip), disabled)
}

func iconButton(c *ui.Context, name, label string, action func(), tip ...string) ui.Element {
	text := label
	if len(tip) > 0 {
		text = tip[0]
	}
	return iconControl(c, name, label, text, ui.Transparent, c.Theme().Text, false).OnClick(action)
}

func primaryIcon(c *ui.Context, name, label, tip string, disabled bool, action func()) ui.Element {
	return iconControl(c, name, label, tip, c.Theme().Accent, c.Theme().AccentText, disabled).Size(36, 32).OnClick(action)
}

func choiceIcon(c *ui.Context, name, label string, selected bool, action func()) ui.Element {
	bg, fg := ui.Transparent, c.Theme().TextMuted
	if selected {
		bg, fg = c.Theme().SurfacePressed, c.Theme().Text
	}
	b := iconControl(c, name, label, label, bg, fg, false).Selected(selected)
	// MyGo sets accent colors with Selected. Apply the neutral palette after
	// setting that accessibility state.
	return feedback(c, b, selectionColor(c, b, bg), fg).OnClick(action)
}
