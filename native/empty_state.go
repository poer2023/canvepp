package main

import "github.com/egoist/mygo/ui"

func (a *desktop) emptyCanvasActions(c *ui.Context, canvas ui.Element) {
	actions := ui.Box(c.Key("empty-canvas")).Absolute().Fill().Padding(0, 8, 24).Center().PassThrough()
	// Drops over a button use the same insertion point as the file picker.
	// The gap lets input and file drops reach the canvas underneath.
	if files := actions.DroppedFiles(); len(files) > 0 {
		a.importFiles(files, a.center())
	}
	if actions.FileDragOver() {
		canvas.Border(2, c.Theme().Text)
	}
	actions.Children(func() {
		ui.Row(c).FillWidth().MaxWidth(280).Height(106).Gap(12).PassThrough().Children(func() {
			emptyCanvasAction(c, "image-up", "导入图片", "导入图片 · ⌘ O，也可拖入画布", a.chooseImages)
			emptyCanvasAction(c, "square-plus", "新建图片", "新建图片 · ⌘ N", func() { a.addNode(a.center()) })
		})
	})
}

func emptyCanvasAction(c *ui.Context, icon, label, tip string, action func()) {
	t := c.Theme()
	b := ui.ButtonBase(c.Key(label)).Role(ui.RoleButton).Label(label).Column().Center().
		Grow(1).Basis(0).MinWidth(0).FillHeight().Padding(0).Gap(11)
	b = toolSurface(c, commandHint(c, b, tip), 11)
	feedback(c, b, t.Surface, t.Text).Children(func() {
		uiIcon(c, icon, 32)
		ui.Text(c, label).FontSize(14).FontWeight(500).FixedLineHeight(20).SingleLine()
	}).OnClick(action)
}

// A miniature workspace and one identifying object. All marks use the two
// neutral surfaces of the current theme and are decorative, never hit targets.
func emptyIcon(c *ui.Context, icon string) {
	t := c.Theme()
	ui.Box(c).Size(76, 50).AlignSelf(ui.Center).IgnoreInput(true).Children(func() {
		card := ui.Box(c).Absolute().Top(4).Left(8).Size(54, 36).Radius(6).Background(t.SurfaceHover)
		card.Children(func() {
			ui.Box(c).Absolute().Left(5).Top(6).Bottom(6).Width(6).Radius(2).Background(t.SurfacePressed)
			ui.Box(c).Absolute().Left(15).Top(6).Right(5).Bottom(6).Radius(3).Background(t.Background)
		})
		ui.Box(c).Absolute().Right(3).Bottom(1).Size(25, 25).Radius(6).Background(t.Surface).Center().Children(func() {
			uiIcon(c, icon, 17).TextColor(t.TextMuted)
		})
	})
}
