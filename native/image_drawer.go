package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *desktop) syncImageDrawer() {
	n, asset, hasImage := a.selectedImage()
	context := asset.ID
	if !hasImage {
		context = "selection"
	}
	if a.drawerContext == context {
		return
	}
	a.drawerContext = context
	a.creationList.ScrollTo(0, ui.Start)
	a.reusePending, a.creationRecordID = nil, ""
	a.imageDrawer, a.creationEditing = "creation", true
	if hasImage && !a.hasCreationDraft(n, asset) {
		// Selection is read-only. Legacy config can contain the original
		// generation prompt; keep it in Source, not in a new request.
		a.prompt = ""
	}
}

func (a *desktop) creationVisible() bool {
	n, asset, hasImage := a.selectedImage()
	return !hasImage || a.creationEditing || a.hasCreationDraft(n, asset) || len(a.subsequentRecords(asset)) > 0
}

func (a *desktop) openCreation() {
	n, asset, hasImage := a.selectedImage()
	if hasImage && !a.hasCreationDraft(n, asset) {
		a.prompt = ""
	}
	a.imageDrawer, a.creationEditing, a.creationRecordID = "creation", true, ""
	a.panel.Collapsed = false
	a.updateDraft()
}

func (a *desktop) reuseRecord(r imageRecord) {
	n, asset, hasImage := a.selectedImage()
	changed := a.prompt != r.Prompt || (r.Params.Size != "" && fmt.Sprintf("%.0fx%.0f", a.width, a.height) != r.Params.Size)
	if changed && strings.TrimSpace(a.prompt) != "" && (!hasImage || a.hasCreationDraft(n, asset)) {
		a.reusePending = &r
		return
	}
	a.applyRecord(r)
}

func (a *desktop) applyRecord(r imageRecord) {
	a.prompt = r.Prompt
	if r.Params.Size != "" {
		fmt.Sscanf(r.Params.Size, "%fx%f", &a.width, &a.height)
		a.ratio = ratioFor(a.width, a.height)
	}
	// Each click starts a new draft using the currently selected image(s).
	// It never resubmits a historical request or changes its original references.
	a.imageDrawer, a.creationEditing, a.creationRecordID, a.reusePending = "creation", true, "", nil
	a.updateDraft()
}

func sourceLabel(mode string) string {
	switch mode {
	case "import":
		return "本地上传"
	case "pinterest":
		return "Pinterest 收藏"
	case "photoshop":
		return "Adobe Photoshop"
	case "illustrator":
		return "Adobe Illustrator"
	case "eagle":
		return "Eagle 素材库"
	case "mock":
		return "示例图片"
	case "manual":
		return "外部生成"
	case "codex", "codex-native":
		return "Codex 生成"
	case "api", "openai":
		return "API 生成"
	default:
		return "图片来源"
	}
}

func imageTime(raw string) string {
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return "时间未记录"
	}
	return t.Local().Format("2006.01.02 · 15:04")
}

func (a *desktop) inspector(c *ui.Context) {
	if a.editor == nil || len(a.draftSelection) == 0 {
		ui.Column(c).Grow(1).Center().Gap(10).Children(func() {
			uiIcon(c, "canvas", 25).TextColor(c.Theme().TextMuted)
			ui.Text(c, "未选中图片").FontSize(13)
		})
		return
	}
	_, asset, hasImage := a.selectedImage()
	ui.Column(c.Key("image-journey")).Grow(1).FillWidth().Clip().Children(func() {
		if hasImage {
			a.drawerHeading(c, "source", "来源", sourceLabel(asset.Source), &asset, func() { a.imageDrawer = "source" })
			if a.imageDrawer == "source" {
				a.drawerBody(c, "source", func() { a.sourceContent(c, asset) })
			}
		}
		if a.imageDrawer != "source" && a.creationVisible() {
			if hasImage || len(a.draftSelection) > 1 {
				description := ""
				if len(a.draftSelection) > 1 {
					references := 0
					for _, id := range a.draftSelection {
						if n := a.editor.Node(id); n != nil && n.Asset != "" {
							references++
						}
					}
					if references > 0 {
						description = fmt.Sprintf("%d 张参考图", references)
					} else {
						description = fmt.Sprintf("%d 个空占位", len(a.draftSelection))
					}
				}
				a.drawerHeading(c, "creation", "创作", description, nil, func() {
					a.imageDrawer = "creation"
					if !hasImage || len(a.subsequentRecords(asset)) == 0 {
						a.creationEditing = true
					}
				})
			}
			if a.imageDrawer == "creation" {
				a.drawerBody(c, "creation", func() {
					if a.creationEditing {
						a.creationForm(c)
					} else {
						a.creationContent(c, asset)
					}
				})
			}
		}
		if a.reusePending != nil {
			ui.Row(c).Padding(8, 14).Gap(5).Background(c.Theme().Surface).Children(func() {
				ui.Text(c, "替换当前草稿？").FontSize(11).Grow(1)
				iconButton(c, "close", "保留当前草稿", func() { a.reusePending = nil })
				iconButton(c, "check", "确认沿用并替换草稿", func() { a.applyRecord(*a.reusePending) })
			})
		}

	})
}

// Headings use a single property row; details live inside the chosen drawer.
func (a *desktop) drawerHeading(c *ui.Context, id, title, subtitle string, asset *Asset, action func()) {
	fg := c.Theme().TextMuted
	if a.imageDrawer == id {
		fg = c.Theme().Text
	}
	ui.Row(c.Key("drawer-heading-"+id)).Height(imageDrawerHeadingHeight).Padding(2, 10).Gap(2).Children(func() {
		b := commandHint(c, ui.Button(c.Key("open-"+id), "").Label("展开"+title+"抽屉"), "展开"+title).Grow(1).FillHeight().Padding(4, 6).Gap(8).Radius(6).Border(0, ui.Transparent)
		feedback(c, b, ui.Transparent, fg).Children(func() {
			sectionTitle(c, title)
			ui.Spacer(c)
			if subtitle != "" {
				ui.Text(c, subtitle).FontSize(12).TextColor(c.Theme().TextMuted).SingleLine()
			}
			if asset != nil {
				a.drawerThumbnail(c, *asset, 24)
			}
			icon := "chevron-down"
			if id == "creation" {
				icon = "chevron-up"
			}
			uiIcon(c, icon, 13).TextColor(c.Theme().TextMuted)
		}).OnClick(action)
		if id == "creation" {
			if _, image, ok := a.selectedImage(); ok && len(a.subsequentRecords(image)) > 0 {
				if a.creationEditing {
					iconButton(c, "history", "查看创作记录", func() { a.imageDrawer, a.creationEditing = "creation", false }, fmt.Sprintf("%d 次创作", len(a.subsequentRecords(image))))
				} else {
					iconButton(c, "edit", "继续创作草稿", func() { a.imageDrawer, a.creationEditing = "creation", true })
				}
			}
		}
	})
}

// Actions belong to the scroll content, including source/history views.
func (a *desktop) imageActions(c *ui.Context, asset Asset, generate bool) {
	t := c.Theme()
	source := !generate && a.imageDrawer == "source"
	row := ui.Row(c.Key("image-actions")).FillWidth().Height(36).Gap(4).AlignItems(ui.Center)
	if generate || source {
		row.Height(creationActionsHeight).Padding(8, 0, 0, 0).BorderWidth(1, 0, 0, 0).
			BorderColor(t.Border.Mix(creationFieldColor(t), .32))
	}
	row.Children(func() {
		if generate || source {
			iconButton(c, "trash", "移入回收站", a.moveSelectionToTrash)
		} else {
			iconButton(c, "spark", "开始新创作", a.openCreation)
		}
		ui.Spacer(c)
		if asset.ID != "" {
			iconButton(c, "download", "下载当前图片", func() { a.exportImage(asset) })
			iconButton(c, "expand", "放大当前图片", func() { a.preview(asset) })
		}
		if !generate {
			if !source {
				iconButton(c, "trash", "移入回收站", a.moveSelectionToTrash)
				return
			}
			if len(a.subsequentRecords(asset)) > 0 {
				iconButton(c, "history", "查看创作记录", func() { a.imageDrawer, a.creationEditing = "creation", false })
			}
			if a.canvasW < 295 {
				iconButton(c, "spark", "开始新创作", a.openCreation)
				return
			}
			button := ui.ButtonBase(c.Key("source-create")).Label("开始新创作").Height(32).Padding(0, 9).
				Gap(5).Radius(7).Shrink(0).AlignItems(ui.Center)
			feedback(c, commandHint(c, button, "开始新创作"), t.Surface, t.Text).Children(func() {
				uiIcon(c, "spark", 15)
				ui.Text(c, "创作图片").FontSize(12).FontWeight(500).SingleLine()
			}).OnClick(a.openCreation)
			return
		}
		generation := a.selectionGeneration()
		if generation.pending {
			caption := statusLabel(generation.status)
			ui.Row(c.Key("generate-progress")).Role(ui.RoleStatus).Label(caption+" · 当前选区").
				Height(32).Padding(0, 10).Gap(6).AlignItems(ui.Center).Radius(7).Shrink(0).Background(t.Surface).Children(func() {
				if generation.animates() {
					motion := a.generationMotion(c)
					ui.Box(c).Size(24, 16).Draw(func(p *ui.Painter, r ui.Rect) { paintGenerationMiniGrid(p, r, motion) })
				}
				ui.Text(c, caption).FontSize(12).TextColor(t.Text)
			})
			return
		}
		caption := fmt.Sprintf("生成 %d 张", a.count)
		label := caption + "图片"
		disabled := strings.TrimSpace(a.prompt) == ""
		bg, fg := t.Accent, t.AccentText
		if disabled {
			bg, fg = t.SurfaceHover, t.TextMuted
		}
		button := ui.ButtonBase(c.Key("generate-images")).Label(label).Height(32).Padding(0, 10).
			Gap(6).AlignItems(ui.Center).Radius(7).Shrink(0)
		disable(button, disabled)
		feedback(c, commandHint(c, button, label), bg, fg).Children(func() {
			uiIcon(c, "spark", 15)
			ui.Text(c, caption).FontSize(12).FontWeight(600)
		}).OnClick(a.generate)
	})
}

func (a *desktop) drawerBody(c *ui.Context, side string, children func()) {
	body := ui.Column(c.Key("drawer-body-" + side)).Grow(1).FillWidth().Clip()
	y := float32(-6)
	if side == "creation" {
		y = 6
	}
	paneMotion(c, body, 0, y).Children(children)
}

func (a *desktop) drawerThumbnail(c *ui.Context, asset Asset, size float32) {
	header := a.panelHandle.Bounds(c)
	a.imageThumbnail(c, asset, size, header.Y, header.Y+a.panelHeight()-44)
}

func (a *desktop) imageThumbnail(c *ui.Context, asset Asset, size, top, bottom float32) {
	ui.Box(c.Key("thumb-"+asset.ID)).Size(size, size).Draw(func(p *ui.Painter, r ui.Rect) {
		fitted := r
		if asset.Width > 0 && asset.Height > 0 {
			scale := min(r.W/float32(asset.Width), r.H/float32(asset.Height))
			fitted.W, fitted.H = float32(asset.Width)*scale, float32(asset.Height)*scale
			fitted.X, fitted.Y = r.X+(r.W-fitted.W)/2, r.Y+(r.H-fitted.H)/2
		}
		p.Clip(fitted, 3, func() { drawCheckers(p, fitted, 6) })
		if a.images != nil {
			// List rows are measured beyond the viewport. Only visible thumbnails
			// enter the decode queue, including during the first estimate pass.
			if r.Y+r.H > top && r.Y < bottom {
				a.images.Request(asset, 512)
			}
			if b := a.images.Get(asset, 512); b != nil {
				p.Clip(fitted, 3, func() { p.Image(b, fitted, ui.Contain) })
			}
		}
	})
}

func (a *desktop) sourceContent(c *ui.Context, asset Asset) {
	r, hasRecord := a.sourceRecord(asset)
	thinScroll(c, ui.Scroll(c.Key("source-scroll")), nil).Grow(1).Padding(8, 13, 12, 13).Gap(12).Children(func() {
		ui.Column(c).Gap(6).Children(func() {
			flatTip(c, ui.Text(c, imageTitle(asset)).FontSize(12).SingleLine(), asset.Name)
			ui.Text(c, imageTime(asset.CreatedAt)).FontSize(10).TextColor(c.Theme().TextMuted)
		})
		if n, _, ok := a.selectedImage(); ok {
			a.imageVersions(c, n)
		}
		if n := a.editor.Node(asset.NodeID); n != nil && n.Adobe != nil && n.Adobe.Group != "" {
			ui.Text(c, "图层组  "+n.Adobe.Group).FontSize(11).TextColor(c.Theme().TextMuted)
		}
		if hasRecord || (asset.Source != "photoshop" && asset.Source != "illustrator" && asset.Source != "import" && asset.Source != "eagle" && asset.Source != "pinterest") {
			a.recordPrompt(c, r, "原始提示词")
		}
		if len(r.Sources) > 0 {
			a.referenceStrip(c, r.Sources)
		}
		ui.Column(c).Gap(7).Children(func() {
			ui.Textf(c, "%d × %d px · %s", asset.Width, asset.Height, imageByteSize(asset.Bytes)).FontSize(11).TextColor(c.Theme().TextMuted)
			if r.Params.Size != "" {
				ui.Text(c, "生成目标  "+strings.ReplaceAll(r.Params.Size, "x", " × ")).FontSize(10).TextColor(c.Theme().TextMuted)
			}
		})
		if asset.SourceURL != "" && (strings.HasPrefix(asset.SourceURL, "https://") || strings.HasPrefix(asset.SourceURL, "http://")) {
			iconButton(c, "external", "打开图片来源", func() { _ = mygo.Shell.OpenExternal(asset.SourceURL) })
		}
		a.imageActions(c, asset, false)
	})
}

func (a *desktop) imageVersions(c *ui.Context, n Node) {
	versions := a.canvasImageVersions(n)
	if len(versions) < 2 {
		return
	}
	ui.Column(c.Key("image-versions")).Gap(6).Children(func() {
		ui.Text(c, "图片版本").FontSize(10).TextColor(c.Theme().TextMuted)
		ui.Row(c).Wrap().Gap(6).Children(func() {
			for i, asset := range versions {
				label := fmt.Sprintf("切换到图片版本 %d", i+1)
				selected := asset.ID == n.Asset
				stroke := c.Theme().Border
				if selected {
					stroke = c.Theme().Text
				}
				imageCommand(c, ui.ButtonBase(c.Key(asset.ID)).Label(label).Selected(selected), label).
					Size(48, 48).Center().Radius(5).Background(ui.Transparent).Border(1, stroke).
					Children(func() { a.drawerThumbnail(c, asset, 42) }).
					OnClick(func() {
						if current := a.editor.Node(n.ID); current != nil && current.Asset != asset.ID {
							next := *current
							next.Asset = asset.ID
							a.editor.Edit(next)
							a.drawerContext = asset.ID
						}
					})
			}
		})
	})
}

func (a *desktop) recordPrompt(c *ui.Context, r imageRecord, label string) {
	ui.Column(c.Key("prompt-record-" + r.ID)).Gap(9).Children(func() {
		ui.Row(c).Children(func() {
			ui.Text(c, label).FontSize(10).TextColor(c.Theme().TextMuted).Grow(1)
			if r.Prompt != "" {
				iconButton(c, "copy", "复制"+label, func() { mygo.Clipboard.WriteText(r.Prompt) })
				iconButton(c, "reuse", "沿用"+label, func() { a.reuseRecord(r) })
			}
		})
		prompt := r.Prompt
		if prompt == "" {
			ui.Text(c, "原始提示词未记录").FontSize(12).TextColor(c.Theme().TextMuted)
		} else {
			editHint(c, ui.Text(c, prompt).FillWidth().FontSize(14).Selectable(), "选择或复制提示词")
		}
	})
}

func (a *desktop) referenceStrip(c *ui.Context, ids []string) {
	ui.Column(c).Gap(8).Children(func() {
		ui.Textf(c, "参考图片 · %d", len(ids)).FontSize(10).TextColor(c.Theme().TextMuted)
		ui.Row(c).Wrap().Gap(7).Children(func() {
			for i, id := range ids {
				asset, ok := a.imageHistory().assets[id]
				if !ok {
					flatTip(c, ui.Box(c.Key(id)).Size(56, 56).Background(c.Theme().Surface).Radius(4).Center(), "参考图片未保存在本机").Children(func() { uiIcon(c, "canvas", 18).TextColor(c.Theme().TextMuted) })
					continue
				}
				imageCommand(c, ui.Button(c.Key(id), "").Label(fmt.Sprintf("查看参考图片 %d", i+1)), asset.Name).Size(56, 56).Padding(0).Border(0, ui.Transparent).Background(ui.Transparent).Children(func() {
					a.drawerThumbnail(c, asset, 56)
				}).OnClick(func() { a.preview(asset) })
			}
		})
	})
}

func (a *desktop) creationContent(c *ui.Context, asset Asset) {
	ui.Column(c).Grow(1).FillWidth().Children(func() {
		ids := a.subsequentRecords(asset)
		a.creationList.Key = func(i int) any {
			if i == len(ids) {
				return "image-actions-footer"
			}
			return ids[i]
		}
		a.creationList.Label = func(i int) string {
			if i == len(ids) {
				return "图片操作"
			}
			return a.imageHistory().records[ids[i]].Prompt
		}
		thinScroll(c, ui.List(c.Key("creation-history-list"), &a.creationList, len(ids)+1, func(i int) {
			if i == len(ids) {
				a.imageActions(c, asset, false)
				return
			}
			id := ids[i]
			r := a.imageHistory().records[id]
			ui.Column(c.Key("creation-"+id)).FillWidth().Gap(3).Padding(0, 0, 6, 0).BorderWidth(0, 0, 1, 0).BorderColor(c.Theme().Border).Children(func() {
				ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
					if len(r.Results) > 0 {
						if image, ok := a.imageHistory().assets[r.Results[0]]; ok {
							a.creationResultThumbnail(c, image)
						}
					}
					ui.Column(c).Grow(1).Gap(5).Children(func() {
						ui.Text(c, imageTime(r.CreatedAt)).FontSize(9).TextColor(c.Theme().TextMuted).SingleLine()
						prompt := r.Prompt
						if prompt == "" {
							prompt = "提示词未记录"
						}
						feedback(c, commandHint(c, ui.Button(c.Key("details-"+id), "").Label("查看创作记录："+id), "查看完整创作记录").FillWidth().Padding(0).Border(0, ui.Transparent).Justify(ui.Start), ui.Transparent, c.Theme().Text).Children(func() {
							ui.Text(c, prompt).FontSize(12).SingleLine().Grow(1)
							uiIcon(c, "chevron-down", 13)
						}).OnClick(func() {
							if a.creationRecordID == id {
								a.creationRecordID = ""
							} else {
								a.creationRecordID = id
							}
						})
						ui.Row(c).Gap(3).Children(func() {
							label := statusLabel(r.Status)
							if r.Status == "succeeded" {
								label = fmt.Sprintf("%d 张结果", len(r.Results))
							}
							ui.Text(c, label).FontSize(10).TextColor(c.Theme().TextMuted).Grow(1)
							if len(r.Results) > 0 {
								if image, ok := a.imageHistory().assets[r.Results[0]]; ok {
									a.creationResultLocation(c, image)
								}
							}
						})
					})
				})
				if len(r.Results) > 1 {
					ui.Row(c).Wrap().Gap(7).Children(func() {
						for _, result := range r.Results[1:] {
							if image, ok := a.imageHistory().assets[result]; ok {
								ui.Column(c.Key(result)).Gap(2).Children(func() { a.creationResultThumbnail(c, image); a.creationResultLocation(c, image) })
							}
						}
					})
				}
				if a.creationRecordID == id {
					a.recordPrompt(c, r, "创作提示词")
					if r.Params.Size != "" {
						ui.Text(c, strings.ReplaceAll(r.Params.Size, "x", " × ")+" px").FontSize(10).TextColor(c.Theme().TextMuted)
					}
					if len(r.Sources) > 1 {
						a.referenceStrip(c, r.Sources)
					}
				}
			})
		}), nil).Grow(1).Padding(4, 16, 16, 16).Gap(12)
	})
}

func (a *desktop) creationResultThumbnail(c *ui.Context, image Asset) {
	imageCommand(c, ui.Button(c.Key("result-"+image.ID), "").Label("预览创作结果："+image.ID), "查看大图").Size(76, 76).Padding(0).Border(0, ui.Transparent).Background(ui.Transparent).Children(func() { a.drawerThumbnail(c, image, 76) }).OnClick(func() { a.preview(image) })
}
func (a *desktop) creationResultLocation(c *ui.Context, image Asset) {
	iconButton(c, "return", "定位创作结果："+image.ID, func() { a.returnToAsset(libraryEntry{projectID: image.ProjectID, asset: &image}) }, "回到画布图片")
}
