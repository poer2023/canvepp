package main

import (
	"image"
	"math"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

const (
	assistantASCIIInterval = time.Second / 12
	assistantASCIICycle    = 4500 * time.Millisecond
	assistantASCIIRamp     = " .:-=+*#%@"
	assistantGlyphW        = 7
	assistantGlyphH        = 13
)

type assistantRelief struct {
	opening uint64
	start   time.Time
}

// One alpha mask per character keeps the background monochrome even when
// the Windows text engine uses subpixel smoothing on readable UI text.
var assistantGlyphAtlas = sync.OnceValue(func() *ui.Bitmap {
	pixels := image.NewRGBA(image.Rect(0, 0, (len(assistantASCIIRamp)-1)*assistantGlyphW, assistantGlyphH))
	drawer := font.Drawer{Dst: pixels, Src: image.White, Face: basicfont.Face7x13}
	for i := 1; i < len(assistantASCIIRamp); i++ {
		drawer.Dot = fixed.P((i-1)*assistantGlyphW, basicfont.Face7x13.Ascent)
		drawer.DrawString(assistantASCIIRamp[i : i+1])
	}
	return ui.NewBitmap(pixels)
})

func assistantReliefSurface(c *ui.Context, panel ui.Element, bar *assistantBar, draftWidth float32, inputHeight int) {
	relief := ui.Local(panel, "assistant-relief", func() assistantRelief { return assistantRelief{} })
	if relief.start.IsZero() || relief.opening != bar.opening {
		relief.opening, relief.start = bar.opening, c.Now()
	}
	ink := c.Theme().Accent
	opacity := 1.0
	if !c.Theme().Dark {
		opacity = .61
	}
	if c.Preferences().HighContrast {
		opacity *= .35
	}
	reduced := c.Preferences().ReduceMotion
	panel.Draw(func(p *ui.Painter, r ui.Rect) {
		phase := 0.0
		if !reduced && bar.visible && (bar.win == nil || bar.win.IsFocused()) {
			elapsed := p.Now().Sub(relief.start) % assistantASCIICycle
			phase = 2 * math.Pi * float64(elapsed) / float64(assistantASCIICycle)
			p.After(assistantASCIIInterval)
		}
		relief.paint(p, r, ink, opacity, phase, draftWidth, inputHeight)
	})
}

func reliefClamp(x float64) float64 { return max(0, min(1, x)) }

func reliefSmooth(x float64) float64 {
	x = reliefClamp(x)
	return x * x * (3 - 2*x)
}

// Sample two overlapping curved surfaces on a fixed glyph grid. Phase changes
// the surface normals and silhouettes, which determines each cell's glyph and ink.
func (a *assistantRelief) paint(p *ui.Painter, r ui.Rect, ink ui.Color, opacity, phase float64, draftWidth float32, inputHeight int) {
	const cellWidth, cellHeight = float32(6), float32(7.6)
	cols, rows := int(r.W/cellWidth), int(r.H/cellHeight)
	if cols == 0 || rows == 0 {
		return
	}
	left := r.X + (r.W-float32(cols)*cellWidth)/2
	top := r.Y + (r.H-float32(rows)*cellHeight)/2
	atlas := assistantGlyphAtlas()

	// Soften the relief around the draft, with room for wider or taller text.
	const compactHeight = assistantChromeH + assistantTextMinH
	extra := float64(inputHeight-assistantTextMinH) / 2
	textBandMid := (.47*compactHeight + extra) / float64(r.H)
	textBandHeight := (.23*compactHeight + extra) / float64(r.H)
	textBandWidth := max(.35, float64(draftWidth+assistantInset)/float64(r.W)-.20)
	light := math.Sin(phase)
	for col := 0; col < cols; col++ {
		u := (float64(col) + .5) / float64(cols)
		x := left + (float32(col)+.5)*cellWidth
		ends := math.Sin(math.Pi * u)
		center := .54 - .19*math.Sin(2*math.Pi*(u-.08)) + .11*math.Sin(phase+4.3*u)*ends
		radius := .27 + .075*math.Cos(2*math.Pi*u+.72) + .046*math.Cos(phase-3.4*u)*ends
		lampY := .23 + .29*math.Sin(phase+4*u)
		foldAngle := 3.5*u + .85 + .54*light
		backCenter := .69 + .18*math.Cos(3.7*u+.35) + .055*math.Cos(phase-3*u)*ends
		backRadius := .18 + .051*math.Sin(4.2*u)
		backAngle := 5.8*u + .38*light
		textFadeX := math.Pow((u-.24)/textBandWidth, 6)
		for row := 0; row < rows; row++ {
			v := (float64(row) + .5) / float64(rows)
			q := (v - center) / radius
			coverage := reliefSmooth((1.06 - math.Abs(q)) / .41)
			var front float64
			if coverage > 0 {
				ny := max(-1, min(1, q))
				nz := math.Sqrt(1 - ny*ny)
				reflection := math.Pow(reliefClamp(.82*nz+lampY*ny), 3.2)
				fold := .47 + .53*math.Pow(math.Abs(math.Cos(foldAngle+1.1*v)), .64)
				front = coverage * (.20 + .77*reflection) * fold
			}
			behind := (v - backCenter) / backRadius
			back := reliefSmooth((1.04-math.Abs(behind))/.62) * (.22 + .24*math.Cos(backAngle+1.9*v))
			intensity := reliefClamp(max(front, back*(1-.64*coverage)) * 1.36)
			index := int(reliefClamp((intensity-.12)/.88)*float64(len(assistantASCIIRamp)-1) + .5)
			if index == 0 {
				continue
			}
			underText := math.Exp(-textFadeX - math.Pow((v-textBandMid)/textBandHeight, 6))
			alpha := (.18 + .64*math.Pow(intensity, 1.18)) * (1 - .72*underText) * opacity
			y := top + (float32(row)+.5)*cellHeight
			const height = float32(assistantGlyphH) * cellWidth / assistantGlyphW
			src := ui.Rect{X: float32(index-1) * assistantGlyphW, W: assistantGlyphW, H: assistantGlyphH}
			dst := ui.Rect{X: x - cellWidth/2, Y: y - height/2, W: cellWidth, H: height}
			p.ImageRegion(atlas, src, dst, ui.ImageOptions{Tint: ink.Alpha(float32(alpha))})
		}
	}
}
