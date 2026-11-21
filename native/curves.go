package main

import (
	"math"

	"github.com/egoist/mygo/ui"
)

type connectionCurve [4][2]float32

// Route from the facing horizontal sides. If cards overlap horizontally,
// sweep around their outside instead of doubling back through the pictures.
// Values are screen-space DIPs, keeping the gap and curvature useful at 5–400%.
func imageConnection(source, target ui.Rect) connectionCurve {
	sy, ty := source.Y+source.H/2, target.Y+target.H/2
	gap := target.X - (source.X + source.W)
	reverse := source.X - (target.X + target.W)
	if gap >= 0 || reverse >= 0 {
		dir, sx, tx := float32(1), source.X+source.W, target.X
		if reverse > gap {
			dir, sx, tx, gap = -1, source.X, target.X+target.W, reverse
		}
		inset := min(float32(6), gap/4)
		sx, tx = sx+dir*inset, tx-dir*inset
		distance := float32(math.Abs(float64(tx - sx)))
		bend := distance * .5
		return connectionCurve{{sx, sy}, {sx + dir*bend, sy}, {tx - dir*bend, ty}, {tx, ty}}
	}
	// A C-shaped route has horizontal tangents at both ends, including when
	// one image is directly above the other. There are no top/bottom handles.
	sx, tx := source.X+source.W+6, target.X+target.W+6
	outside := max(sx, tx) + min(float32(120), max(32, float32(math.Abs(float64(ty-sy)))*.22))
	if target.X+target.W/2 < source.X+source.W/2 {
		sx, tx = source.X-6, target.X-6
		outside = min(sx, tx) - min(float32(120), max(32, float32(math.Abs(float64(ty-sy)))*.22))
	}
	return connectionCurve{{sx, sy}, {outside, sy}, {outside, ty}, {tx, ty}}
}

func (r connectionCurve) points(dst [][2]float32) [][2]float32 {
	return flattenConnection(append(dst, r[0]), r[0], r[1], r[2], r[3], 0)
}

func (r connectionCurve) visible(view ui.Rect) bool {
	left, right, top, bottom := r[0][0], r[0][0], r[0][1], r[0][1]
	for _, p := range r[1:] {
		left, right = min(left, p[0]), max(right, p[0])
		top, bottom = min(top, p[1]), max(bottom, p[1])
	}
	return left <= view.X+view.W && right >= view.X && top <= view.Y+view.H && bottom >= view.Y
}

// Flatten in screen space to a subpixel error, then let the GPU draw narrow
// segments. StrokePath rasterizes a rectangular alpha mask spanning an entire
// curve: long links at large zoom allocated tens of MB during one wheel event.
// The bounded reusable buffer instead costs O(curve length), not O(mask area).
func connectionPoints(dst [][2]float32, sx, sy, tx, ty, bend float32) [][2]float32 {
	start := [2]float32{sx, sy}
	dst = append(dst, start)
	return flattenConnection(dst, start, [2]float32{sx + bend, sy}, [2]float32{tx - bend, ty}, [2]float32{tx, ty}, 0)
}

func flattenConnection(dst [][2]float32, a, b, c, d [2]float32, depth int) [][2]float32 {
	// The second difference bounds deviation from the chord even when all
	// four controls are collinear but turn back on themselves.
	err := float32(0)
	for i := 0; i < 2; i++ {
		err = max(err, float32(math.Abs(float64(a[i]-2*b[i]+c[i]))), float32(math.Abs(float64(b[i]-2*c[i]+d[i]))))
	}
	if err <= .4 || depth == 8 {
		return append(dst, d)
	}
	mid := func(x, y [2]float32) [2]float32 { return [2]float32{(x[0] + y[0]) / 2, (x[1] + y[1]) / 2} }
	ab, bc, cd := mid(a, b), mid(b, c), mid(c, d)
	abc, bcd := mid(ab, bc), mid(bc, cd)
	m := mid(abc, bcd)
	dst = flattenConnection(dst, a, ab, abc, m, depth+1)
	return flattenConnection(dst, m, bcd, cd, d, depth+1)
}
