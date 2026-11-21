package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
)

const workspaceFont = "Unica77LL, Eina03"
const contentInset = 10

// Fonts stay local: use an installed reference app or a user supplied font
// folder, never redistribute its commercial font files in Canvepp's bundle.
func registerSpatialFonts() {
	dirs := []string{os.Getenv("CANVEPP_FONT_DIR"), "/Applications/Spatial.app/Contents/Resources"}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		for _, name := range []string{"Unica77LL.otf", "Eina03-Regular.ttf", "Eina03-SemiBold.ttf", "Eina03-Bold.ttf", "Volkhov-Regular.ttf", "Volkhov-Bold.ttf"} {
			if data, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
				family := strings.Split(strings.TrimSuffix(name, filepath.Ext(name)), "-")[0]
				_ = ui.RegisterFont(data, family)
			}
		}
	}
}

// Empty mode follows the window's system appearance.
func workspaceTheme(c *ui.Context, mode string) *ui.Theme {
	dark := c.Theme().Dark
	if mode != "" {
		dark = mode == "dark"
	}
	t := ui.LightTheme()
	if dark {
		t = ui.DarkTheme()
	}
	if dark {
		t.Background, t.Surface = ui.Hex("#202020"), ui.Hex("#292929")
		t.SurfaceHover, t.SurfacePressed = ui.Hex("#353535"), ui.Hex("#454545")
		t.Text, t.TextMuted, t.Border = ui.Hex("#f3f3f3"), ui.Hex("#b4b4b4"), ui.Hex("#424242")
		t.Accent, t.AccentHover, t.AccentPressed, t.AccentText = ui.Hex("#eeeeee"), ui.Hex("#ffffff"), ui.Hex("#cecece"), ui.Hex("#171717")
	} else {
		t.Background, t.Surface = ui.Hex("#ffffff"), ui.Hex("#f6f6f6")
		t.SurfaceHover, t.SurfacePressed = ui.Hex("#eaeaea"), ui.Hex("#dedede")
		t.Text, t.TextMuted, t.Border = ui.Hex("#222222"), ui.Hex("#636363"), ui.Hex("#dfdfdf")
		t.Accent, t.AccentHover, t.AccentPressed, t.AccentText = ui.Hex("#272727"), ui.Hex("#414141"), ui.Hex("#151515"), ui.Hex("#ffffff")
	}
	t.ScrollbarWidth = thumbWidth
	t.Scrollbar = t.TextMuted.Alpha(.48)
	t.Font, t.Spacing, t.FontSize, t.Radius = workspaceFont, 3, 12, 6
	t.Focus, t.Inverse, t.InverseText = t.Text.Alpha(.8), t.Text, t.Background
	t.Selection = t.Text.Alpha(.16)
	if c.Preferences().HighContrast {
		t.Border, t.TextMuted, t.Focus = t.Text.Mix(t.Background, .35), t.Text, t.Text
	}
	return t
}

func menuText(c *ui.Context, selected bool) ui.Color {
	t := c.Theme()
	if c.Preferences().HighContrast {
		return t.Text
	}
	if selected {
		return t.TextMuted.Mix(t.Text, .22)
	}
	return t.TextMuted.Mix(t.Surface, .08)
}

// Opaque tool surfaces keep their contrast over any artwork.
func toolSurface(c *ui.Context, e ui.Element, radius float32) ui.Element {
	e.Radius(radius).Background(c.Theme().Surface).Border(0, ui.Transparent)
	if c.Preferences().HighContrast {
		e.Border(1, c.Theme().Border)
	}
	return e
}
