package main

import (
	"fmt"
	"log"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
)

// Enabled only with --receipt. This observes real input; it neither injects
// events nor changes scheduling. Timings stop at command painting, before
// Metal presentation, the window compositor and any remote desktop stream.
type motionTrace struct {
	active, kind         string
	lastInput, lastPaint time.Time
	pending              int
	ending               bool
	inputs, paints, ages []float64
	strokes              []map[string]any
	pointerPhases        []map[string]any
}

func (m *motionTrace) input(ev ui.InputEvent, gesture, target string) {
	if (ev.Kind == ui.InputPointerDown || ev.Kind == ui.InputPointerUp) && len(m.pointerPhases) < 100 {
		entry := map[string]any{"phase": fmt.Sprint(ev.Kind), "x": ev.X, "y": ev.Y, "button": ev.Button, "gesture": gesture, "target": target}
		m.pointerPhases = append(m.pointerPhases, entry)
		log.Printf("canvepp: pointer %s", encode(entry))
	}
	switch ev.Kind {
	case ui.InputPointerDown:
		if m.ending {
			m.finish()
		}
		m.active, m.kind = target, target
		m.lastInput, m.lastPaint = time.Time{}, time.Time{}
		m.pending = 0
		m.inputs, m.paints, m.ages = nil, nil, nil
	case ui.InputPointerMove:
		if m.active != target || gesture == "" {
			return
		}
		m.kind = gesture
		now := time.Now()
		if !m.lastInput.IsZero() && len(m.inputs) < 4096 {
			m.inputs = append(m.inputs, float64(now.Sub(m.lastInput))/1e6)
		}
		m.lastInput = now
		m.pending++
	case ui.InputPointerUp:
		if m.active != target {
			return
		}
		m.ending = true
	}
}

func (m *motionTrace) paint(now time.Time) {
	if m.pending > 0 && len(m.ages) < 4096 {
		m.ages = append(m.ages, float64(now.Sub(m.lastInput))/1e6)
		if !m.lastPaint.IsZero() {
			m.paints = append(m.paints, float64(now.Sub(m.lastPaint))/1e6)
		}
		m.pending, m.lastPaint = 0, now
	}
	if m.ending {
		m.finish()
	}
}

func (m *motionTrace) finish() {
	if len(m.ages) > 0 {
		r := map[string]any{"kind": m.kind, "inputInterval": motionDistribution(m.inputs), "paintInterval": motionDistribution(m.paints), "latestInputToPaint": motionDistribution(m.ages)}
		if len(m.strokes) == 64 {
			m.strokes = m.strokes[1:]
		}
		m.strokes = append(m.strokes, r)
		log.Printf("canvepp: motion %s", encode(r))
	}
	m.active, m.ending = "", false
}

func motionDistribution(values []float64) map[string]any {
	s := slices.Clone(values)
	slices.Sort(s)
	r := map[string]any{"samples": len(s)}
	if len(s) > 0 {
		r["median_ms"], r["p95_ms"], r["max_ms"] = s[(len(s)-1)/2], s[int(float64(len(s)-1)*.95)], s[len(s)-1]
	}
	return r
}

func (m *motionTrace) receipt() any {
	if m == nil {
		return nil
	}
	return map[string]any{"scope": "Observed input arrival to painting commands, excluding GPU/compositor/remote transport. Pauses while holding the pointer are included in interval samples.", "strokes": m.strokes, "pointerPhases": m.pointerPhases}
}
