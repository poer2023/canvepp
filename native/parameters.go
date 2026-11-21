package main

import (
	"math"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

const (
	creationParameterHeight    = 37
	creationStackedPixelHeight = 57
)

func (a *desktop) creationPixelsStacked() bool {
	return a.canvasW < 284+24 // The canvas reserves 12 pt at each edge of the panel.
}

func creationFieldColor(t *ui.Theme) ui.Color {
	if t.Dark {
		return t.Background
	}
	return t.Surface
}

func (a *desktop) creationParameters(c *ui.Context) {
	t := c.Theme()
	rule := t.Border.Mix(creationFieldColor(t), .32)
	ui.Column(c.Key("creation-parameters")).FillWidth().Gap(0).Children(func() {
		ui.Row(c.Key("ratio-row")).FillWidth().Height(creationParameterHeight).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(rule).Children(func() {
			ui.Text(c, "比例").FontSize(12).TextColor(t.TextMuted)
			ui.Spacer(c)
			a.creationRatioPicker(c)
		})
		a.creationPixelRow(c, rule)
		ui.Row(c.Key("output-count")).FillWidth().Height(creationParameterHeight).AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Text(c, "张数").FontSize(12).TextColor(t.TextMuted)
			ui.Spacer(c)
			segments := ui.Row(c.Key("output-count-options")).Width(141).Height(29).MinWidth(0).Padding(2).Gap(2).Radius(7).Background(creationFieldColor(t))
			index := segments.AnimateWith("selected-count", float32(max(1, min(4, a.count))-1), selectionDuration, ui.EaseOut)
			segments.Draw(func(p *ui.Painter, r ui.Rect) {
				w := (r.W - 10) / 4
				p.Fill(ui.Rect{X: r.X + 2 + index*(w+2), Y: r.Y + 2, W: w, H: r.H - 4}, t.SurfacePressed, 5)
			}).Children(func() {
				for i := 1; i <= 4; i++ {
					count, label := strconv.Itoa(i), "生成 "+strconv.Itoa(i)+" 张"
					b := ui.Button(c.Key(i), count).Label(label).Selected(a.count == i).FontFeatures("tnum").Grow(1).MinWidth(0).
						FillHeight().Padding(0).FontSize(12).Radius(5).Border(0, ui.Transparent)
					fg := t.TextMuted
					if a.count == i {
						fg = t.Text
						b.FontWeight(600)
					}
					feedback(c, commandHint(c, b, label), ui.Transparent, fg).OnClick(func() { a.count = i; a.updateDraft() })
				}
			})
		})
		if a.count > 1 {
			ui.Row(c.Key("parallel-generation")).FillWidth().Height(creationParameterHeight).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(rule).Children(func() {
				ui.Text(c, "并行生成").FontSize(12).TextColor(t.TextMuted)
				ui.Spacer(c)
				parallel := a.parallelGeneration
				sw := commandHint(c, flatSwitch(c, &parallel, "多张图片并行生成"), "同时生成多张图片，关闭时逐张生成")
				if sw.Changed() {
					if a.store != nil {
						if _, err := a.store.DB.Exec("INSERT OR REPLACE INTO native_meta VALUES('parallel_generation',?)", parallel); err != nil {
							a.message("保存生成方式失败：" + err.Error())
							return
						}
					}
					a.parallelGeneration = parallel
				}
			})
		}
	})
}

func (a *desktop) creationRatioPicker(c *ui.Context) {
	t := c.Theme()
	value := "自由像素"
	if a.ratio >= 0 && a.ratio < len(ratios) {
		value = ratios[a.ratio].label
	}
	chevron := "chevron-down"
	if a.ratioMenuOpen {
		chevron = "chevron-up"
	}
	anchor := ui.ButtonBase(c.Key("creation-ratio")).Label("选择比例 · "+value).Expanded(a.ratioMenuOpen).
		Height(30).Padding(0, 7).Gap(5).AlignItems(ui.Center).Radius(6)
	feedback(c, commandHint(c, anchor, "选择比例，实际像素以生成结果为准"), ui.Transparent, t.TextMuted).Children(func() {
		ui.Text(c, value).FontFeatures("tnum").FontSize(12)
		uiIcon(c, chevron, 13)
	}).OnClick(func() { a.ratioMenuOpen = !a.ratioMenuOpen })
	ui.PopoverBase(c, anchor, &a.ratioMenuOpen, func(panel ui.Element) {
		background := t.Background
		if t.Dark {
			background = t.Surface
		}
		panel.Label("画幅比例").AttachTo(anchor, ui.AnchorBottomEnd, ui.AnchorTopEnd).Margin(4, 0, 0, 0).
			Width(272).Padding(8).Radius(9).Background(background).Border(1, t.Border).TextColor(t.Text)
		paneMotion(c, panel, 0, 5)
		ui.Column(c).FillWidth().Gap(5).Children(func() {
			ui.Row(c).FillWidth().Height(25).AlignItems(ui.Center).Children(func() {
				sectionTitle(c, "画幅")
				ui.Spacer(c)
				iconButton(c, "close", "关闭比例选择", func() { a.ratioMenuOpen = false }).Size(25, 25).Padding(4)
			})
			ui.Grid(c.Key("creation-ratio-options")).FillWidth().Columns(4).Gap(4).Children(func() {
				for i := range ratios {
					a.creationRatioChoice(c, i)
				}
			})
			a.creationRatioChoice(c, -1).FillWidth()
		})
	})
}

func (a *desktop) creationRatioChoice(c *ui.Context, index int) ui.Element {
	t := c.Theme()
	label := "自由像素"
	if index >= 0 {
		label = ratios[index].label
	}
	selected := a.ratio == index
	b := ui.ButtonBase(c.Key(index)).Label(label).Selected(selected).Height(29).Padding(0, 3).
		Gap(4).AlignItems(ui.Center).Justify(ui.Center).Radius(5)
	bg, fg := creationFieldColor(t), t.TextMuted
	if selected {
		bg, fg = t.SurfacePressed, t.Text
		b.AutoFocus()
	}
	return feedback(c, commandHint(c, b, label), bg, fg).Children(func() {
		if index < 0 {
			uiIcon(c, "expand", 12)
		} else {
			r := ratios[index]
			ui.Box(c).Size(14, 14).Shrink(0).Draw(func(p *ui.Painter, box ui.Rect) {
				w, h := float32(r.w), float32(r.h)
				scale := 12 / max(w, h)
				w, h = w*scale, h*scale
				p.Stroke(ui.Rect{X: box.X + (box.W-w)/2, Y: box.Y + (box.H-h)/2, W: w, H: h}, fg, 1, 1)
			})
		}
		ui.Text(c, label).FontFeatures("tnum").FontSize(11)
	}).OnClick(func() {
		a.ratio, a.ratioMenuOpen = index, false
		if index >= 0 {
			a.setPixelDimension(true, a.width)
		}
		a.updateDraft()
	})
}

func (a *desktop) creationPixelRow(c *ui.Context, rule ui.Color) {
	t := c.Theme()
	label := func() {
		flatTip(c, ui.Text(c, "目标像素").FontSize(12).TextColor(t.TextMuted), "实际像素以生成结果为准")
	}
	fields := func() {
		ui.Row(c.Key("pixel-fields")).Width(205).MaxWidthPercent(100).MinWidth(0).Gap(5).AlignItems(ui.Center).Children(func() {
			a.pixelField(c, true)
			ui.Text(c, "×").FontSize(11).TextColor(t.TextMuted)
			a.pixelField(c, false)
			ui.Text(c, "px").FontSize(11).TextColor(t.TextMuted)
		})
	}
	if a.creationPixelsStacked() {
		ui.Column(c.Key("target-pixels")).FillWidth().Height(creationStackedPixelHeight).Padding(2, 0, 4, 0).Gap(3).
			BorderWidth(0, 0, 1, 0).BorderColor(rule).Children(func() { label(); fields() })
		return
	}
	ui.Row(c.Key("target-pixels")).FillWidth().Height(creationParameterHeight).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(rule).Children(func() { label(); ui.Spacer(c); fields() })
}

type pixelInputState struct {
	Text  string
	Value float64
}

// Preserve incomplete text while typing; only validated dimensions enter the
// draft. Linked fields, ratio changes and selection changes refresh the value.
func (a *desktop) pixelField(c *ui.Context, width bool) {
	name, value := "高", &a.height
	label := "高度像素"
	if width {
		name, value, label = "宽", &a.width, "宽度像素"
	}
	row := ui.Row(c.Key(label)).Grow(1).MinWidth(0).Height(30).Padding(0, 7).Gap(4).AlignItems(ui.Center).
		Radius(6).Background(creationFieldColor(c.Theme()))
	state := ui.Local(row, "number", func() pixelInputState { return pixelInputState{Text: pixelText(*value), Value: *value} })
	if state.Value != *value {
		state.Text, state.Value = pixelText(*value), *value
	}
	row.Children(func() {
		ui.Text(c, name).FontSize(12).TextColor(c.Theme().TextMuted)
		input := editHint(c, ui.TextInput(c.Key("value"), &state.Text).Label(label), label).
			Grow(1).MinWidth(0).Height(28).Padding(0).FontFeatures("tnum").FontSize(12).Background(ui.Transparent).Border(0, ui.Transparent)
		if input.Changed() {
			if v, err := strconv.Atoi(strings.TrimSpace(state.Text)); err == nil && v >= 64 && v <= 8192 {
				a.setPixelDimension(width, float64(v))
				a.updateDraft()
			}
			state.Value = *value
		}
		if !input.Focused() || input.Submitted() {
			state.Text = pixelText(*value)
		}
		if input.Focused() {
			row.Border(1, c.Theme().Focus)
		}
	})
}

func pixelText(v float64) string { return strconv.FormatFloat(v, 'f', 0, 64) }

func (a *desktop) setPixelDimension(width bool, value float64) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return
	}
	value = math.Round(max(64, min(8192, value)))
	if a.ratio >= 0 && a.ratio < len(ratios) {
		r := ratios[a.ratio]
		factor := float64(r.h) / float64(r.w)
		if !width {
			factor = 1 / factor
		}
		value = max(math.Ceil(64/factor), min(math.Floor(8192/factor), value))
		if width {
			a.width, a.height = value, math.Round(value*factor)
		} else {
			a.height, a.width = value, math.Round(value*factor)
		}
	} else if width {
		a.width = value
	} else {
		a.height = value
	}
}
