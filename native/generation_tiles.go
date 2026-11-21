package main

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"
)

const imageRevealDuration = 560 * time.Millisecond

type imageReveal struct {
	project, asset string
	start, expires time.Time
}

func (a *desktop) beginImageReveal(e *Editor, node, asset string) {
	if a.editor != e || a.left != "画布" || a.images == nil || e.Node(node) == nil {
		return
	}
	if a.imageReveals == nil {
		a.imageReveals = make(map[string]imageReveal)
	}
	a.imageReveals[node] = imageReveal{project: e.Project.ID, asset: asset}
}

func (a *desktop) syncImageReveals(c *ui.Context, view Rect, motion bool) {
	if !motion {
		clear(a.imageReveals)
		return
	}
	e, now := a.editor, c.Now()
	for id, reveal := range a.imageReveals {
		n := e.Node(id)
		if reveal.project != e.Project.ID || n == nil || n.Asset != reveal.asset || !view.Intersects(nodeRect(*n, e.Assets)) {
			delete(a.imageReveals, id)
			continue
		}
		if reveal.expires.IsZero() {
			reveal.expires = now.Add(3 * time.Second)
		}
		if reveal.start.IsZero() && a.canvasBitmap(e.Assets[reveal.asset], e.View.Zoom) != nil {
			reveal.start = now
			reveal.expires = now.Add(imageRevealDuration)
		}
		if left := reveal.expires.Sub(now); left > 0 {
			a.imageReveals[id] = reveal
			c.After(left)
		} else {
			delete(a.imageReveals, id)
		}
	}
}

func (a *desktop) generationMotion(c *ui.Context) bool {
	return !c.Preferences().ReduceMotion && (a.win == nil || a.win.IsVisible() && !a.win.IsMinimized())
}

// The mosaic takes the existing viewport clip and its own rounded corners.
// During reveal one thumbnail supplies both the tile colors and the final image.
var generationTileShader = shader.MustCompile(`package main
var Grid vec2
var Surface vec4
var Ink vec4
var Phase float
var Reveal float

func Fragment(dst vec4, pos vec2) vec4 {
	size := rectSize()
	gridPos := pos / size * Grid
	cell := floor(gridPos)
	local := fract(gridPos) - vec2(0.5)
	seed := fract(sin(dot(cell, vec2(12.9898, 78.233))) * 43758.5453)
	wave := 0.5 + 0.5*sin(Phase + seed*6.28318 + cell.x*0.11)
	color := mix(Surface.rgb, Ink.rgb, 0.15 + 0.36*smoothstep(0.06, 0.94, wave))
	expansion := 0.0
	opacity := 1.0
	if Reveal >= 0.0 {
		t := clamp((Reveal - 0.14*seed) / 0.86, 0.0, 1.0)
		expansion = smoothstep(0.22, 0.72, t)
		src := image0Size()
		scale := min(size.x/src.x, size.y/src.y)
		p := (cell + vec2(0.5)) / Grid * size
		sample := image0Sample((p - (size-src*scale)*0.5) / scale)
		color = mix(color, sample.rgb + Surface.rgb*(1.0-sample.a), smoothstep(0.0, 0.44, t))
		opacity = 1.0 - smoothstep(0.66, 1.0, Reveal)
	}
	halfSide := mix(0.29, 0.53, expansion)
	radius := mix(0.072, 0.0, expansion)
	q := abs(local) - vec2(halfSide-radius)
	distance := length(max(q, vec2(0.0))) + min(max(q.x, q.y), 0.0) - radius
	pixel := Grid / size / pixelScale()
	aa := min(0.11, max(pixel.x, pixel.y)*0.67)
	mask := 1.0 - smoothstep(-aa, aa, distance)
	rgb := mix(Surface.rgb, color, mask)
	return vec4(rgb*opacity, opacity)
}
`)

func paintImageTiles(p *ui.Painter, rect ui.Rect, radius float32, bitmap *ui.Bitmap, reveal float32, motion bool) {
	if min(rect.W, rect.H) < 8 {
		if bitmap == nil {
			p.Fill(rect, p.Theme().Surface, radius)
		}
		return
	}
	phase := float32(0)
	if motion {
		const cycle = 1750 * time.Millisecond
		phase = 2 * math.Pi * float32(p.Now().UnixNano()%int64(cycle)) / float32(cycle)
		if bitmap != nil {
			p.After(time.Second / 45)
		} else {
			p.After(time.Second / 26)
		}
	}
	cell := max(float32(8), min(rect.W, rect.H)/13)
	count := func(size float32) float32 { return max(1, min(24, float32(math.Round(float64(size/cell))))) }
	t := p.Theme()
	ink := t.TextMuted.Mix(ui.Hex("#9aacc5"), .48)
	generationTileShader.Draw(p, rect, shader.Options{
		Images: [4]*ui.Bitmap{bitmap}, Radius: radius,
		Uniforms: map[string]any{"Grid": [2]float32{count(rect.W), count(rect.H)}, "Surface": t.Surface, "Ink": ink, "Phase": phase, "Reveal": reveal},
	})
}

func paintGenerationMiniGrid(p *ui.Painter, box ui.Rect, motion bool) {
	paintImageTiles(p, box, 0, nil, -1, motion)
}
