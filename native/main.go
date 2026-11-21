package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func main() {
	debug.SetMemoryLimit(192 << 20)
	base, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(err)
	}
	defaultDir := filepath.Join(base, "CanveppNative")
	dir := flag.String("data-dir", defaultDir, "本地数据目录")
	legacy := flag.String("import-legacy", "", "首次导入的旧版数据目录")
	receipt := flag.String("receipt", "", "关闭时保存原生性能记录")
	shortcutTrace := flag.String("shortcut-trace", "", "限时两分钟的快捷键事件诊断，不含输入内容")
	windowSize := flag.String("window-size", "1440x900", "窗口初始尺寸")
	check := flag.Bool("check-account", false, "只读检查 Codex 订阅连接")
	importOnly := flag.Bool("import-only", false, "只导入旧数据，不打开窗口")
	flag.Parse()
	if err := pinterestAcceptanceDataDir(*dir); err != nil {
		log.Fatal(err)
	}
	if *shortcutTrace != "" {
		if err := startShortcutTrace(*shortcutTrace); err != nil {
			log.Fatal(err)
		}
		defer stopShortcutTrace()
	}
	if *check {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		c, err := startCodex(ctx)
		if err != nil {
			log.Fatal(err)
		}
		defer c.Close()
		plan, err := c.Account(ctx)
		if err != nil {
			log.Fatal(err)
		}
		image, err := c.imageCapability(ctx)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(string(encode(map[string]any{"connected": true, "plan": plan, "imageGeneration": image})))
		return
	}
	// Keep the default instance identity. Additional libraries get their own
	// lock so opening one cannot redirect its data path to another window.
	profileDir := libraryIdentity(*dir)
	name := pinterestAppName
	if profileDir != libraryIdentity(defaultDir) {
		name += " " + digest([]byte(profileDir))[:16]
	}
	mygo.App.SetName(name)
	registerSpatialFonts()
	mygo.App.SetVersion(appVersion)
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	mygo.App.SetName(pinterestAppName)
	store, err := openStore(*dir)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	if *legacy != "" {
		var done string
		err = store.DB.QueryRow("SELECT value FROM native_meta WHERE key='legacy_import'").Scan(&done)
		if err != nil {
			count, err := store.ImportLegacy(*legacy)
			if err != nil {
				log.Fatal("原数据未修改；导入失败：", err)
			}
			log.Printf("导入 %d 张画布", count)
		}
	}
	if *importOnly {
		return
	}
	a, err := newDesktop(store)
	if err != nil {
		log.Fatal(err)
	}
	var restoredLibrary string
	_ = store.DB.QueryRow("SELECT value FROM native_meta WHERE key='restored_library'").Scan(&restoredLibrary)
	title := "Canvepp Native"
	revealWorkspace := func() {
		if a.win != nil && !a.closing && !a.closed {
			a.win.Show()
			a.win.Focus()
		}
	}
	if restoredLibrary == "true" {
		title = "Canvepp Native · " + filepath.Base(store.Dir)
	}
	if runtime.GOOS == "windows" || restoredLibrary == "true" {
		mygo.App.OnSecondInstance(func(_ []string, _ string) { revealWorkspace() })
	}
	activateGenerationNotice := bindGenerationNoticeActivation(a, flag.Args())
	mygo.Theme.SetSource(mygo.ThemeSource(a.appearance))
	if *receipt != "" {
		a.motionTrace = &motionTrace{}
	}
	mygo.App.OnWindowAllClosed(func() { mygo.App.Quit() })
	pendingPinterest := false
	mygo.App.OnOpenURL(func(raw string) {
		u, err := url.Parse(raw)
		if err == nil && u.Scheme == "canvepp" && u.User == nil && u.Host == "materials" && u.Path == "/pinterest" && u.RawQuery == "" && u.Fragment == "" {
			if a.win == nil {
				pendingPinterest = true
				return
			}
			a.returnToPinterestCollections()
			mygo.App.Focus()
			a.win.Show()
			a.win.Focus()
		}
	})
	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{{Role: mygo.RoleAppMenu}, {Label: "文件", Submenu: []*mygo.MenuItem{{Label: "新建画布", Accelerator: "CmdOrCtrl+Shift+N", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.createCanvas() }}, {Label: "导入图片…", Accelerator: "CmdOrCtrl+O", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.chooseImages() }}, {Label: "导出备份…", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { a.backup(a.win) }}, {Role: mygo.RoleClose}}}, {Role: mygo.RoleEditMenu}, {Role: mygo.RoleWindowMenu}}))
	mygo.App.WhenReady(func() {
		width, height := 1440, 900
		if n, _ := fmt.Sscanf(*windowSize, "%dx%d", &width, &height); n != 2 {
			width, height = 1440, 900
		}
		win := mygo.NewWindow(mygo.WindowOptions{Title: title, Width: max(1000, width), Height: max(680, height), MinWidth: 1000, MinHeight: 680, TitleBarStyle: mygo.TitleBarHidden, Vibrancy: mygo.VibrancyPopover, Content: ui.View(a.view)})
		a.attach(win)
		a.applyWindowMaterial()
		tray, trayErr := newWorkspaceTray(name, title, revealWorkspace, func() {
			a.exitFromTray = true
			win.Close()
		})
		if trayErr != nil {
			log.Printf("启动托盘图标：%v", trayErr)
		}
		if tray != nil {
			mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) {
				if !a.closed {
					e.PreventDefault()
				}
			})
		}
		if restoredLibrary != "true" {
			a.registerAdobeShortcut()
		}
		a.startExplorerPreview()
		var restorePinterestDock bool
		_ = store.DB.QueryRow("SELECT value FROM native_meta WHERE key='pinterest_dock_open'").Scan(&restorePinterestDock)
		if restorePinterestDock && !pendingPinterest {
			a.openWorkspace("画布")
			a.openDock(dockPinterest)
		}
		if pendingPinterest {
			a.returnToPinterestCollections()
			mygo.App.Focus()
		}
		activateGenerationNotice()
		win.OnClose(func(ev *mygo.CloseEvent) {
			if a.closed {
				return
			}
			ev.PreventDefault()
			if a.closing {
				return
			}
			if tray != nil && !a.exitFromTray {
				a.commitCanvasText()
				a.saveLayout()
				win.Hide()
				return
			}
			if a.backupSettings.busy || a.importSettings.busy || a.restoreSettings.busy || a.openLibrarySettings.busy {
				a.exitFromTray = false
				revealWorkspace()
				a.message("资料库操作完成后即可关闭")
				win.Invalidate()
				return
			}
			if a.adobeBusy {
				a.closeAfterAdobe = true
				a.message("Adobe 快照完成后关闭窗口")
				win.Invalidate()
				return
			}
			if a.materials != nil && len(a.materials.importing) > 0 {
				a.exitFromTray = false
				revealWorkspace()
				a.message("图片正在导入，完成后即可关闭")
				win.Invalidate()
				return
			}
			a.closing = true
			a.commitCanvasText()
			a.saveLayout()
			reopenPinterestDock := a.dockVisible() && a.dock.active == dockPinterest
			activeID := ""
			if a.editor != nil {
				activeID = a.editor.Project.ID
			}
			for _, e := range a.editors {
				a.saves.Put(e)
			}
			if *receipt != "" {
				if err := os.WriteFile(*receipt, encode(a.performanceReceipt()), 0600); err != nil {
					log.Print(err)
				}
			}
			go func() {
				err := a.saves.Flush()
				if err == nil && activeID != "" {
					_, err = store.DB.Exec("INSERT OR REPLACE INTO native_meta VALUES('active_project',?)", activeID)
				}
				if err == nil {
					_, err = store.DB.Exec("INSERT OR REPLACE INTO native_meta VALUES('pinterest_dock_open',?)", reopenPinterestDock)
				}
				a.publish(func() {
					if err != nil {
						a.closing = false
						a.exitFromTray = false
						revealWorkspace()
						a.message("保存失败，窗口保持打开：" + err.Error())
						return
					}
					a.closed = true
					if tray != nil {
						tray.Destroy()
					}
					a.stopExplorerPreview()
					if a.hotkey != nil {
						a.hotkey.Stop()
					}
					if a.assistant != nil && a.assistant.win != nil {
						a.assistant.win.Close()
					}
					if a.settingsWin != nil {
						a.settingsWin.Close()
					}
					if a.materials != nil {
						a.materials.stop()
					}
					close(a.images.stop)
					win.Close()
				})
			}()
		})
	})
	if err := mygo.App.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func libraryIdentity(dir string) string {
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	if runtime.GOOS == "windows" {
		dir = strings.ToLower(dir)
	}
	return dir
}
