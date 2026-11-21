package main

import (
	"fmt"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *desktop) pinterestView(c *ui.Context) {
	p := a.materials.pin
	if p == nil {
		return
	}
	if p.previewOpen {
		a.pinterestPreviewView(c)
		return
	}
	if p.configure {
		a.pinterestConfigView(c)
		return
	}
	if !p.ready || !p.connected || p.auth != nil {
		ui.Column(c).Grow(1).FillWidth().Center().Padding(24).Gap(18).Children(func() {
			if p.ready && p.auth == nil {
				emptyIcon(c, "unlink")
			} else {
				emptyIcon(c, "hourglass")
			}
			title, icon, button, tip := "Pinterest 未连接", "link", "连接 Pinterest", "连接 Pinterest · 在浏览器中授权访问图板"
			needsConfig := p.ready && p.auth == nil && p.config.validate() != nil
			if needsConfig {
				title, icon, button, tip = "Pinterest 收藏", "login", "Pinterest 网页登录", "在此用邮箱或 Pinterest 二维码登录"
			}
			if !p.ready {
				title = "正在读取连接…"
			}
			if p.auth != nil {
				title, icon, button, tip = "等待浏览器授权", "close", "取消授权", "取消 Pinterest 授权"
			}
			sectionTitle(c, title)
			if p.status != "" {
				ui.Text(c, p.status).MaxWidth(480).FontSize(13).TextColor(c.Theme().TextMuted)
			} else if needsConfig {
				ui.Text(c, "可在素材库内扫码或用邮箱登录。").FontSize(12).TextColor(c.Theme().TextMuted).MaxWidth(340)
			}
			ui.Row(c).Gap(8).Center().Children(func() {
				primaryIcon(c, icon, button, tip, !p.ready || p.working, func() {
					if p.auth != nil {
						a.cancelPinterestAuth()
					} else if needsConfig {
						a.pinterestSiteLogin()
					} else {
						a.connectPinterest()
					}
				})
				if p.auth == nil {
					disable(iconButton(c, "settings", "Pinterest 连接设置", a.openPinterestConfig, "Pinterest 连接设置 · 首次连接需配置应用"), !p.ready || p.working)
				}
			})
		})
		return
	}
	layout := ui.Row(c).Grow(1).FillWidth()
	dock := a.dockVisible()
	if dock {
		layout.Column()
	}
	layout.Children(func() {
		w, _ := c.Size()
		side := float32(208)
		if w < 850 {
			side = 176
		}
		boards := ui.Column(c).Width(side).FillHeight().Padding(14, 12, 12, 16).Gap(12).Background(c.Theme().Background)
		if dock {
			boards.FillWidth().Height(164).Padding(4, 12).Gap(2)
		}
		boards.Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				sectionTitle(c, "图板").Grow(1)
				activityIcon(c, "refresh", "刷新 Pinterest 图板", "正在读取图板", p.boardsLoading, false, func() { a.loadPinterestBoards(false) })
			})
			ui.Row(c).Height(32).FillWidth().Children(func() {
				quietSearch(c, &p.query, &p.search, "搜索 Pinterest 图板", "筛选图板", side-28)
			})
			list := thinScroll(c, ui.Scroll(c.Key("pinterest-boards")), nil).Grow(1).Gap(3)
			resultsMotion(c, list, resultsMotionKey{query: p.query, count: len(p.boards)}).Children(func() {
				matches := 0
				filter := parseLibraryQuery(p.query)
				for _, board := range p.boards {
					if !filter.matches(board.Name) {
						continue
					}
					matches++
					bg := ui.Transparent
					if p.board.ID == board.ID {
						bg = c.Theme().SurfacePressed
					}
					button := ui.Button(c.Key(board.ID), "").Label("图板："+board.Name).Selected(p.board.ID == board.ID).FillWidth().Height(30).Padding(0, 8).Border(0, ui.Transparent).Radius(8)
					feedback(c, commandHint(c, button, "打开图板 · "+board.Name), selectionColor(c, button, bg), menuText(c, p.board.ID == board.ID)).Children(func() {
						ui.Text(c, board.Name).FontSize(12).FontWeight(400).Grow(1).SingleLine()
						ui.Textf(c, "%d", board.Count).FontSize(11).TextColor(c.Theme().TextMuted)
					}).OnClick(func() { a.selectPinterestBoard(board) })
				}
				if matches == 0 {
					text := "没有匹配的图板"
					if len(p.boards) == 0 {
						text = "还没有图板"
					}
					if p.boardsLoading {
						text = "正在读取图板…"
					}
					ui.Text(c, text).FontSize(12).TextColor(c.Theme().TextMuted).Padding(8)
				}
				if p.boardBookmark != "" {
					disable(iconButton(c, "chevron-down", "加载更多图板", func() { a.loadPinterestBoards(true) }), p.boardsLoading)
				}
			})
			ui.Row(c).Gap(4).Children(func() {
				disable(iconButton(c, "settings", "Pinterest 连接设置", a.openPinterestConfig, "Pinterest 连接设置 · 首次连接需配置应用"), p.working)
				ui.Spacer(c)
				disable(iconButton(c, "unlink", "断开 Pinterest 连接", a.disconnectPinterest), p.working)
			})
		})
		pins := ui.Column(c.Key("pin-results")).Grow(1).MinHeight(0)
		if dock {
			pins.FillWidth()
		} else {
			pins.FillHeight()
		}
		key := resultsMotionKey{group: p.board.ID, count: len(p.pictures)}
		if p.loading {
			key.mode = 1
		}
		resultsMotion(c, pins, key).Children(func() {
			ui.Row(c).Height(54).Padding(10, 18).AlignItems(ui.Center).Gap(8).Children(func() {
				title := p.board.Name
				if title == "" {
					title = "收藏"
				}
				fittingLabel(c, title, 13, 500).Grow(1)
				if p.board.ID != "" {
					disable(iconButton(c, "refresh", "刷新当前图板", func() {
						if len(p.pages) > 0 {
							a.loadPinterestPins(p.pages[len(p.pages)-1])
						}
					}), p.loading)

				}
			})
			if len(p.pictures) == 0 && p.loading {
				pendingMaterialGrid(c, 172, 228, 14, 18, 0)
			} else if len(p.pictures) == 0 {
				ui.Column(c).Grow(1).FillWidth().Center().Gap(12).Padding(24).Children(func() {
					text := "这个图板暂无可导入的静态图片"
					if p.board.ID == "" {
						text = "未选中图板"
					}
					emptyIcon(c, "library")
					ui.Text(c, text).TextColor(c.Theme().TextMuted)
				})
			} else {
				p.grid.Key = func(i int) any { return p.pictures[i].Key }
				p.grid.Label = func(i int) string { return p.pictures[i].Name }
				thinScroll(c, ui.GridView(c.Key("pinterest-pictures"), &p.grid, len(p.pictures), 172, 228, func(i int) {
					pic := p.pictures[i]
					hint := "查看大图"
					if p.thumbErrors[pic.Key] {
						hint = "缩略图不可用 · 查看原图"
					}
					ui.Column(c).Fill().Gap(4).Children(func() {
						b := imageCommand(c, ui.Button(c, "").Label("预览 Pinterest 图片："+pic.Name), hint).Grow(1).FillWidth().Padding(0).Border(0, ui.Transparent).Background(ui.Transparent).OnClick(func() { a.previewPinterest(pic) }).Draw(func(painter *ui.Painter, r ui.Rect) {
							if im := p.thumbs[pic.Key]; im != nil {
								painter.Image(im, r, ui.Contain)
							} else {
								pendingThumbnail(painter, r, p.thumbErrors[pic.Key])
							}
						})
						if dock {
							b.Drag(materialDrag{func(e *Editor, at Point) { a.importPinterestPictureAt(e, at, pic) }}).Cursor(ui.CursorGrab)
						}
						ui.Row(c).Height(30).AlignItems(ui.Center).Gap(3).Children(func() {
							fittingLabel(c, pic.Name, 12, 400).Grow(1).TextColor(c.Theme().TextMuted)
							activityIcon(c, "plus", "导入 Pinterest 图片："+pic.Name, "正在导入 "+pic.Name, a.materialImporting("pinterest", pic.Key), a.editor == nil, func() { a.importPinterestPicture(pic) }, "导入当前画布")
						})
					})
				}), nil).Grow(1).Padding(0, 18, 10, 18).Gap(14)
			}
			if p.status != "" {
				ui.Row(c).Padding(4, 18).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, p.status).FontSize(12).TextColor(c.Theme().TextMuted).Grow(1)
					disable(iconButton(c, "login", "重新授权", a.connectPinterest), p.working)
				})
			}
			if len(p.pages) > 1 || p.bookmark != "" || len(p.batch) > 48 {
				ui.Row(c).Height(42).Padding(4, 18).Gap(8).Center().Children(func() {
					disable(iconButton(c, "back", "Pinterest 上一页", func() {
						if p.part > 0 {
							a.showPinterestPart(p.part - 1)
						} else {
							p.pages = p.pages[:len(p.pages)-1]
							a.loadPinterestPins(p.pages[len(p.pages)-1], -1)
						}
					}), p.loading || (len(p.pages) < 2 && p.part == 0))

					disable(iconButton(c, "forward", "Pinterest 下一页", func() {
						if (p.part+1)*48 < len(p.batch) {
							a.showPinterestPart(p.part + 1)
						} else {
							p.pages = append(p.pages, p.bookmark)
							a.loadPinterestPins(p.bookmark)
						}
					}), p.loading || (p.bookmark == "" && (p.part+1)*48 >= len(p.batch)))

				})
			}
		})
	})
}

func (a *desktop) pinterestConfigView(c *ui.Context) {
	p := a.materials.pin
	ui.Row(c).Height(50).Padding(8, 20).Gap(10).AlignItems(ui.Center).Children(func() {
		disable(iconButton(c, "back", "返回 Pinterest 收藏", func() { p.configure = false; p.draft.ClientSecret = "" }), p.working)
		sectionTitle(c, "Pinterest 连接设置")
	})
	thinScroll(c, ui.Scroll(c), nil).Grow(1).Padding(8, 28, 24, 28).Children(func() {
		ui.Column(c).MaxWidth(560).FillWidth().Gap(16).Children(func() {
			if p.connectAfterConfig {
				ui.Text(c, "连接 Google 登录的收藏需要先配置 Canvepp 的 Pinterest 应用。").FontSize(12).TextColor(c.Theme().TextMuted)
			}
			ui.Column(c).FillWidth().Gap(5).Children(func() {
				ui.Text(c, "App ID").FontSize(12).TextColor(c.Theme().TextMuted)
				editHint(c, ui.TextInput(c, &p.draft.ClientID).Label("Pinterest App ID"), "Pinterest 应用 ID").FillWidth().Height(38)
			})
			ui.Column(c).FillWidth().Gap(5).Children(func() {
				ui.Text(c, "App Secret").FontSize(12).TextColor(c.Theme().TextMuted)
				hint := ""
				if p.config.ClientSecret != "" {
					hint = "已保存"
				}
				editHint(c, ui.TextInput(c, &p.draft.ClientSecret).Password().Placeholder(hint).Label("Pinterest App Secret"), "密钥保存在系统凭据库中，留空沿用已保存的值").FillWidth().Height(38)
			})
			ui.Column(c).FillWidth().Gap(5).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "回调地址").FontSize(12).TextColor(c.Theme().TextMuted).Grow(1)
					iconButton(c, "copy", "复制 Pinterest 回调地址", func() { mygo.Clipboard.WriteText(p.draft.RedirectURI) })
				})
				editHint(c, ui.TextInput(c, &p.draft.RedirectURI).Label("Pinterest 回调地址"), "须与 Pinterest 开发者后台登记的 Redirect URI 一致").FillWidth().Height(38)
			})
			check := ui.Checkbox(c, &p.draft.IncludeSecret, "包含私密图板（只读）").Label("包含 Pinterest 私密图板")
			commandHint(c, check, "切换私密图板的只读授权")
			if check.Hovered() {
				check.Background(c.Theme().SurfaceHover)
			}
			if p.configError != "" {
				ui.Text(c, p.configError).TextColor(c.Theme().TextMuted).FontSize(12)
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				primaryIcon(c, "check", "保存 Pinterest 连接设置", "保存连接设置", p.working, a.savePinterestConfig)
				iconButton(c, "external", "开发者后台 ↗", func() { _ = mygo.Shell.OpenExternal("https://developers.pinterest.com/apps/") })
			})
		})
	})
}

func (a *desktop) pinterestPreviewView(c *ui.Context) {
	p := a.materials.pin
	pic := p.preview
	ui.Column(c.Key("pinterest-preview")).Label("Pinterest 大图内容区").Grow(1).FillWidth().Children(func() {
		ui.Row(c).Height(46).Padding(6, 14).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, fmt.Sprintf("%d × %d", pic.Original.Width, pic.Original.Height)).FontSize(12).TextColor(c.Theme().TextMuted).Grow(1)
			iconButton(c, "return", "在 Pinterest 查看原图", func() { _ = mygo.Shell.OpenExternal(pic.WebURL()) })
			disable(iconButton(c, "plus", "导入预览图片到画布", func() { a.importPinterestPicture(pic) }), a.editor == nil)
			iconButton(c, "close", "关闭 Pinterest 大图", func() { p.previewOpen = false })
		})
		ui.Box(c).Grow(1).FillWidth().Draw(func(painter *ui.Painter, r ui.Rect) {
			if p.previewBitmap != nil {
				painter.Image(p.previewBitmap, r, ui.Contain)
			}
		})
		if p.previewError != "" {
			ui.Text(c, p.previewError).Padding(8, 16).FontSize(12).TextColor(c.Theme().TextMuted)
		}
	})
}
