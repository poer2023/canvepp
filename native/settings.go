package main

import (
	"runtime"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
)

const appVersion = "0.6.12"

const settingsSidebarWidth float32 = 166

func (a *desktop) settingsView(c *ui.Context) {
	defer beginHints(c)()
	t := workspaceTheme(c, a.appearance)
	c.SetTheme(t)
	c.Root().Background(a.workspaceSurface(c, t.Surface, .65))
	c.OnShortcut(0, ui.KeyEscape, a.closeSettings)
	c.OnShortcut(ui.Cmd, ui.KeyComma, a.closeSettings)
	ui.Column(c).Fill().Children(func() {
		titleBarRow(c, "settings-titlebar").Children(func() {
			ui.Box(c).Grow(1).FillHeight().DragWindow().Label("拖动设置窗口").Cursor(ui.CursorGrab)
		})
		ui.Row(c).Grow(1).MinHeight(0).FillWidth().Children(func() {
			a.settingsSidebar(c)
			ui.Box(c).Grow(1).MinWidth(0).FillHeight().Padding(0, canvasInset, canvasInset, 0).Children(func() {
				ui.Box(c).Fill().Radius(canvasCorner).Clip().Background(t.Background).Children(func() {
					scroll := thinScroll(c, ui.Scroll(c.Key(a.settingsTab)), nil).Fill().Padding(20, pageInset, 26).AlignItems(ui.Center)
					paneMotion(c, scroll, 0, 0).Children(func() {
						ui.Column(c).Label("设置正文").FillWidth().MaxWidth(680).Gap(23).Children(func() {
							switch a.settingsTab {
							case 0:
								a.generalSettings(c)
							case 1:
								a.dataSettings(c)
							case 2:
								a.shortcutSettings(c)
							}
						})
					})
				})
			})
		})
	})
}

func (a *desktop) settingsSidebar(c *ui.Context) {
	t := c.Theme()
	// Clickable descendants take the pointer first. Every unused part of
	// the menu works like the native title bar, without a separate handle.
	ui.Column(c.Key("settings-sidebar")).Width(settingsSidebarWidth).Shrink(0).
		FillHeight().DragWindow().Label("设置侧栏").Cursor(ui.CursorGrab).Children(func() {
		ui.Row(c).FillWidth().Height(workspaceHeaderHeight).Padding(0, 16).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "设置").SingleLine().FontSize(12).FontWeight(400).TextColor(t.TextMuted)
		})
		ui.Column(c).FillWidth().Padding(2, 9, 6, 9).Gap(1).Children(func() {
			for i, tab := range []struct{ name, icon string }{
				{"通用", "sliders"}, {"数据", "database"}, {"快捷键", "keyboard"},
			} {
				selected := a.settingsTab == i
				button := sidebarNavRow(c, tab.icon, tab.name, "设置："+tab.name, "", selected, false, func() {
					a.settingsTab = i
					if i != 0 {
						a.funding.closeKeyEditor()
					}
				})
				if selected {
					button.AutoFocus()
				}
			}
		})
	})
}

func (a *desktop) generalSettings(c *ui.Context) {
	settingsGroup(c, "外观", func() {
		settingsRow(c, "主题", "system", func() { a.themeChoices(c) })
		settingsRow(c, "磨砂半透明", "layers", func() {
			enabled := a.frosted
			sw := flatSwitch(c, &enabled, "磨砂半透明")
			if registerHint(c, sw, "应用于侧栏、菜单和工具栏", false, true).Changed() {
				a.setFrosted(enabled)
			}
		})
	})
	settingsGroup(c, "账户与助手", func() {
		a.accountSettings(c)
		settingsRow(c, "助手输入条", "chat", func() {
			settingsButton(c, "打开助手", a.showAssistant)
		})
		settingsRow(c, "三击空格", "keyboard", func() {
			switch a.hotkeyStatus {
			case "全局监听已开启":
				settingsStatus(c, "全局监听开启", settingsReady)
			case "正在启动监听":
				settingsStatus(c, "正在启动监听", settingsNeutral)
			case "监听启动失败":
				settingsStatus(c, a.hotkeyStatus, settingsWarning)
			}
			enabled := a.hotkeyEnabled
			sw := flatSwitch(c, &enabled, "全局三击空格")
			hint := "连按三次空格打开助手输入条"
			if runtime.GOOS == "windows" {
				hint += "，已识别的游戏窗口跳过"
			}
			if registerHint(c, sw, hint, false, true).Changed() {
				a.setHotkey(enabled)
			}
		})
		if runtime.GOOS == "darwin" && a.hotkeyEnabled &&
			(a.hotkeyStatus == "需要输入监控权限" || a.hotkeyStatus == "监听已停止") {
			settingsRow(c, "输入监控", "shield", func() {
				if a.hotkeyStatus == "监听已停止" {
					settingsStatus(c, a.hotkeyStatus, settingsWarning)
				}
				settingsButton(c, "检查权限", func() { a.refreshHotkey(true) })
				button := settingsButton(c, "系统设置", func() {
					_ = mygo.Shell.OpenExternal("x-apple.systempreferences:com.apple.preference.security?Privacy_ListenEvent")
				})
				registerHint(c, button, "授权后可在其他应用中使用三击空格", false, true)
			})
		}
	})
	settingsGroup(c, "额度", func() {
		a.fundingRow(c, "生图额度", false)
		a.fundingRow(c, "摘要额度", true)
		a.apiSettings(c)
	})
	settingsGroup(c, "模型", func() {
		a.modelRows(c, true)
		a.modelRows(c, false)
	})
}

func (a *desktop) themeChoices(c *ui.Context) {
	t := c.Theme()
	mode := a.appearance
	day, night := workspaceTheme(c, "light"), workspaceTheme(c, "dark")
	ui.Row(c.Key("theme-choices")).Gap(6).AlignItems(ui.Center).
		Role(ui.RoleRadioGroup).Label("选择主题").FocusGroup(ui.Horizontal).Children(func() {
		for _, option := range []struct{ mode, label string }{
			{"", "跟随系统"}, {"light", "浅色模式"}, {"dark", "深色模式"},
		} {
			selected := mode == option.mode
			tile := ui.RadioBase(c.Key("theme-"+option.mode), &mode, option.mode).
				Label(option.label).Size(100, 73).Radius(7).Center()
			bg, edge := ui.Transparent, ui.Transparent
			if selected {
				bg, edge = t.Surface, t.TextMuted
			}
			feedback(c, tile, bg, t.Text).Border(1, edge).Children(func() {
				ui.Column(c).Gap(4).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Size(86, 42).Draw(func(p *ui.Painter, r ui.Rect) {
						p.Clip(r, 5, func() {
							switch option.mode {
							case "light":
								paintThemeThumbnail(p, r, day)
							case "dark":
								paintThemeThumbnail(p, r, night)
							default:
								left := ui.Rect{X: r.X, Y: r.Y, W: r.W / 2, H: r.H}
								right := ui.Rect{X: r.X + left.W, Y: r.Y, W: r.W - left.W, H: r.H}
								paintThemeThumbnail(p, left, day)
								paintThemeThumbnail(p, right, night)
							}
						})
						p.Stroke(r, t.Border, 5, 1)
					})
					ui.Row(c).Height(16).Gap(3).AlignItems(ui.Center).Children(func() {
						ui.Text(c, option.label).SingleLine().FontSize(11)
						if selected {
							uiIcon(c, "check", 11)
						} else {
							ui.Box(c).Width(11)
						}
					})
				})
			})
			if tile.Changed() {
				a.setAppearance(mode)
			}
		}
	})
}

func paintThemeThumbnail(p *ui.Painter, r ui.Rect, theme *ui.Theme) {
	p.Fill(r, theme.Surface, 0)
	rail := r.W * .23
	pane := ui.Rect{X: r.X + rail + 3, Y: r.Y + 4, W: r.W - rail - 6, H: r.H - 8}
	p.Fill(pane, theme.Background, 3)
	for i, width := range []float32{.68, .53, .62} {
		p.Fill(ui.Rect{X: r.X + 4, Y: r.Y + 9 + float32(i)*6, W: rail * width, H: 2}, theme.SurfacePressed, 1)
	}
	cardW := pane.W * .18
	for i := range 2 {
		x := pane.X + pane.W/2 - cardW - 1.5 + float32(i)*(cardW+3)
		p.Stroke(ui.Rect{X: x, Y: pane.Y + pane.H/2 - 3, W: cardW, H: 6}, theme.SurfacePressed, 2, 1)
	}
}

func (a *desktop) accountSettings(c *ui.Context) {
	settingsRow(c, "ChatGPT 账户", "user", func() {
		switch {
		case a.cancelLogin != nil:
			settingsStatus(c, "等待授权", settingsNeutral)
			settingsButton(c, "取消登录", a.cancelLogin)
		case a.accountBusy:
			settingsStatus(c, "正在读取账户…", settingsNeutral)
		case a.chatGPTAccount != nil:
			label := strings.TrimSpace(a.chatGPTAccount.Email)
			if label == "" {
				label = strings.TrimSpace(a.chatGPTAccount.WorkspaceName)
			}
			if label == "" {
				label = "已登录"
			}
			fittingLabel(c, label, 12, 400).MaxWidth(252).TextColor(c.Theme().TextMuted)
		default:
			settingsButton(c, "登录", func() { a.checkAccount(true) })
		}
		if a.loginError != "" && !a.accountBusy {
			iconButton(c, "refresh", "重新读取账户", func() { a.checkAccount(false) })
		}
	})
	if a.loginCode != "" {
		settingsRow(c, "浏览器授权设备码", "key", func() {
			settingsKeycaps(c, a.loginCode)
			button := settingsButton(c, "复制设备码", func() { mygo.Clipboard.WriteText(a.loginCode) })
			registerHint(c, button, "在浏览器授权页面输入此设备码", false, true)
		})
	}
	settingsMessage(c, a.loginError, settingsError)
}

func (a *desktop) dataSettings(c *ui.Context) {
	settingsGroup(c, "备份与恢复", func() {
		settingsTaskRow(c, "完整备份", "download", "包含画布、原图、历史记录、回收站和本地助手附件",
			"导出备份", "正在导出…", a.backupSettings, func() { a.backup(a.settingsWin) })
		settingsTaskRow(c, "导入到当前资料库", "upload", "导入新画布，跳过已有画布",
			"导入画布或备份", "正在导入…", a.importSettings, a.importLegacy)
		settingsTaskRow(c, "恢复备份", "database", "将备份恢复到新的资料库并打开",
			"恢复到新资料库", "正在恢复…", a.restoreSettings, a.restoreBackup)
		settingsTaskRow(c, "已恢复的资料库", "canvas", "选择之前恢复的资料库目录",
			"打开资料库", "正在打开…", a.openLibrarySettings, a.openRestoredLibrary)
	})
	settingsGroup(c, "应用", func() {
		settingsRow(c, "版本", "canvas", func() {
			ui.Text(c, "Canvepp "+appVersion).FontSize(12).FontFeatures("tnum").TextColor(c.Theme().TextMuted)
		})
	})
}

type settingsTask struct {
	busy    bool
	message string
	failed  bool
}

func settingsTaskRow(c *ui.Context, title, icon, tip, label, progress string, task settingsTask, action func()) {
	settingsRow(c, title, icon, func() {
		if task.busy {
			label = progress
		}
		button := disable(settingsButton(c, label, action), task.busy)
		registerHint(c, button, tip, false, true)
	})
	tone := settingsReady
	if task.failed {
		tone = settingsError
	}
	if !task.busy {
		settingsMessage(c, task.message, tone)
	}
}

func (a *desktop) shortcutSettings(c *ui.Context) {
	a.explorerPreviewSettings(c)
	settingsGroup(c, "Adobe 快照", func() {
		settingsRow(c, "导入当前 Photoshop / Illustrator 画布", "", func() {
			settingsKeycaps(c, adobeShortcutLabel())
		})
		settingsMessage(c, a.adobeShortcutError, settingsError)
	})
	settingsGroup(c, "画布操作", func() {
		windowsKeys := strings.NewReplacer("⌘ ", "Ctrl+", "⇧ ", "Shift+", "⌫", "Delete")
		for _, item := range [][2]string{
			{"新建图片", "⌘ N"}, {"导入图片", "⌘ O"}, {"复制 / 粘贴", "⌘ C / ⌘ V"},
			{"撤销 / 重做", "⌘ Z / ⇧ ⌘ Z"}, {"全选图片", "⌘ A"}, {"移入回收站", "⌫"},
			{"打开设置", "⌘ ,"}, {"助手输入条", "⌘ ⇧ Space / 三击空格"},
		} {
			keys := item[1]
			if runtime.GOOS != "darwin" {
				keys = windowsKeys.Replace(keys)
			}
			settingsRow(c, item[0], "", func() { settingsKeycaps(c, keys) })
		}
	})
	settingsGroup(c, "手势", func() {
		for _, item := range [][2]string{
			{"缩放画布", "滚轮 / 双指捏合"}, {"移动画布", "空格拖动 / 鼠标中键"},
			{"多选图片", "框选 / Shift 点击"}, {"吸附图片面板", "拖动标题，靠边松开"},
		} {
			settingsRow(c, item[0], "", func() {
				ui.Text(c, item[1]).SingleLine().FontSize(12).TextColor(c.Theme().TextMuted)
			})
		}
	})
}

func settingsKeycaps(c *ui.Context, keys string) {
	t := c.Theme()
	for i, chord := range strings.Split(keys, " / ") {
		if i > 0 {
			ui.Text(c, "/").FontSize(11).TextColor(t.TextMuted)
		}
		toolSurface(c, ui.Text(c, chord).SingleLine().Padding(3, 7), 4).
			FontFeatures("tnum").FontSize(12).TextColor(t.Text)
	}
}

func settingsGroup(c *ui.Context, title string, body func()) {
	t := c.Theme()
	line := t.Border
	if !c.Preferences().HighContrast {
		line = line.Mix(t.Background, .4)
	}
	ui.Column(c.Key(title)).FillWidth().Gap(5).Children(func() {
		sectionTitle(c, title).FontSize(14).FontWeight(600).Role(ui.RoleHeading).Level(2)
		// Only the spaces between rows expose the divider color.
		ui.Column(c).FillWidth().Gap(1).Background(line).Children(body)
	})
}

func settingsLabel(c *ui.Context, title, icon string) {
	ui.Row(c).Grow(1).MinWidth(0).Gap(8).AlignItems(ui.Center).Children(func() {
		if icon != "" {
			uiIcon(c, icon, 16).TextColor(c.Theme().TextMuted)
		}
		ui.Text(c, title).MinWidth(0).FontSize(13).FontWeight(400)
	})
}

func settingsRow(c *ui.Context, title, icon string, controls func()) {
	ui.Row(c).FillWidth().MinHeight(43).Padding(5, 0).Gap(12).AlignItems(ui.Center).Background(c.Theme().Background).Children(func() {
		settingsLabel(c, title, icon)
		ui.Row(c).Shrink(0).Gap(7).AlignItems(ui.Center).Children(controls)
	})
}

func settingsButton(c *ui.Context, label string, action func()) ui.Element {
	t := c.Theme()
	b := ui.Button(c.Key(label), label).Label(label).Height(30).Padding(0, 10).FontSize(12).Shrink(0)
	return feedback(c, toolSurface(c, b, 6), t.Surface, t.Text).OnClick(action)
}

type settingsOption[T comparable] struct {
	value       T
	label, icon string
}

func settingsSelect[T comparable](c *ui.Context, label string, selected *T, options []settingsOption[T], fallback string) ui.Element {
	t := c.Theme()
	current := settingsOption[T]{label: fallback, icon: "warning"}
	for _, option := range options {
		if option.value == *selected {
			current = option
			break
		}
	}
	sel := ui.SelectBase(c.Key(label), selected)
	trigger := sel.Trigger.Label(label).Value(current.label).Width(144).Height(30).
		Padding(0, 9).Gap(7).Radius(6).Border(1, t.Border).Shrink(0)
	feedback(c, trigger, t.Background, t.Text).Children(func() {
		settingsOptionLabel(c, current.icon, current.label)
		uiIcon(c, "chevron-down", 12).TextColor(t.TextMuted)
	})
	sel.Popup(func(panel ui.Element) {
		toolSurface(c, panel, 6).MinWidth(164).Margin(3, 0, 0).Padding(4).Gap(1)
		paneMotion(c, panel, 0, 0)
		for _, option := range options {
			chosen := option.value == *selected
			item := sel.Item(option.value).Role(ui.RoleMenuItemRadio).Label(option.label).Selected(chosen).
				FillWidth().Height(28).Padding(0, 8).Gap(7).Radius(5).AlignItems(ui.Center).Cursor(ui.CursorPointer)
			bg := ui.Transparent
			switch {
			case item.Highlighted():
				bg = t.SurfaceHover
			case chosen:
				bg = t.Surface.Mix(t.SurfacePressed, .52)
			}
			item.Background(bg).TextColor(menuText(c, chosen)).Children(func() {
				settingsOptionLabel(c, option.icon, option.label)
				if chosen {
					uiIcon(c, "check", 13)
				}
			})
		}
	})
	return trigger
}

func settingsOptionLabel(c *ui.Context, icon, label string) {
	uiIcon(c, icon, 14)
	ui.Text(c, label).Grow(1).MinWidth(0).SingleLine().FontSize(12)
}

type settingsTone uint8

const (
	settingsNeutral settingsTone = iota
	settingsReady
	settingsWarning
	settingsError
)

func settingsToneColor(c *ui.Context, tone settingsTone) ui.Color {
	t := c.Theme()
	if c.Preferences().HighContrast {
		return t.Text
	}
	switch tone {
	case settingsReady:
		if t.Dark {
			return ui.Hex("#8bd5ab")
		}
		return ui.Hex("#246445")
	case settingsWarning:
		if t.Dark {
			return ui.Hex("#ebc688")
		}
		return ui.Hex("#7e5212")
	case settingsError:
		if t.Dark {
			return ui.Hex("#f1a1a8")
		}
		return ui.Hex("#a22d39")
	}
	return t.TextMuted
}

func settingsStatus(c *ui.Context, label string, tone settingsTone) {
	ui.Text(c, label).SingleLine().FontSize(11).FontWeight(500).TextColor(settingsToneColor(c, tone)).Shrink(0)
}

func settingsMessage(c *ui.Context, text string, tone settingsTone) {
	if text == "" {
		return
	}
	ui.Text(c, text).FillWidth().Padding(6, 0, 8).Background(c.Theme().Background).
		FontSize(11.5).TextColor(settingsToneColor(c, tone))
}

func flatSwitch(c *ui.Context, enabled *bool, label string) ui.Element {
	t := c.Theme()
	sw := ui.SwitchBase(c, enabled).Label(label).Size(34, 30).AlignItems(ui.Center)
	track, knob, position := t.SurfacePressed, t.TextMuted, float32(0)
	if *enabled {
		track, knob, position = t.Accent, t.AccentText, 1
	}
	track = selectionColor(c, sw, track)
	offset := sw.AnimateWith("switch-position", position, selectionDuration, ui.EaseOut)
	if sw.Hovered() {
		if *enabled {
			track = t.AccentHover
		} else {
			track = track.Mix(t.Text, .17)
		}
	}
	sw.Children(func() {
		ui.Box(c).Size(32, 18).Radius(9).Background(track).Children(func() {
			ui.Box(c).Absolute().Left(3+14*offset).Top(3).Size(12, 12).Radius(6).Background(knob)
		})
	})
	return sw.Cursor(ui.CursorPointer)
}

func (a *desktop) setAppearance(mode string) {
	if a.store != nil {
		if _, err := a.store.DB.Exec("INSERT OR REPLACE INTO native_meta VALUES('appearance',?)", mode); err != nil {
			a.message(err.Error())
			return
		}
	}
	a.appearance = mode
	if a.win != nil {
		mygo.Theme.SetSource(mygo.ThemeSource(mode))
	}
	if a.assistant != nil && a.assistant.win != nil {
		a.assistant.win.Invalidate()
	}
}

func (a *desktop) setFrosted(enabled bool) {
	if a.store != nil {
		if _, err := a.store.DB.Exec("INSERT OR REPLACE INTO native_meta VALUES('frosted_appearance',?)", enabled); err != nil {
			a.message(err.Error())
			return
		}
	}
	a.frosted = enabled
	a.applyWindowMaterial()
}

func (a *desktop) windowMaterial() mygo.Vibrancy {
	if a.frosted {
		return mygo.VibrancyPopover
	}
	return mygo.VibrancyNone
}

func (a *desktop) applyWindowMaterial() {
	material := a.windowMaterial()
	if a.win != nil {
		a.win.SetVibrancy(material)
	}
	if a.settingsWin != nil {
		a.settingsWin.SetVibrancy(material)
	}
}

func (a *desktop) workspaceSurface(c *ui.Context, base ui.Color, opacity float32) ui.Color {
	if c.Vibrancy() && a.frostEnabled(c) {
		return base.Alpha(opacity)
	}
	return base
}

func (a *desktop) frostEnabled(c *ui.Context) bool {
	return a.frosted && !c.Preferences().HighContrast &&
		(a.win == nil || !mygo.Theme.Preferences().ReduceTransparency)
}

func (a *desktop) chromeSurface(c *ui.Context, e ui.Element, radius float32) ui.Element {
	e = toolSurface(c, e, radius)
	if a.frostEnabled(c) {
		tint := c.Theme().Surface.Alpha(.75)
		e.Material(glass.Blur{Radius: 8}).Draw(func(p *ui.Painter, r ui.Rect) {
			p.Fill(r, tint, radius)
		})
	}
	return e
}
