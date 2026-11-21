package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/egoist/mygo"
)

func (a *desktop) restoreBackup() {
	a.withLibraryFolder(&a.restoreSettings, "选择要恢复的备份目录", func(source string, owner *mygo.Window) (string, error) {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: owner, Title: "选择新资料库的保存位置", Directory: true, CreateDirectories: true,
		})
		if err != nil || len(paths) == 0 {
			return "", err
		}
		dest := filepath.Join(paths[0], "Canvepp-Restored-"+time.Now().Format("20060102-150405"))
		if err = restoreLibrary(source, dest); err != nil {
			return "", err
		}
		if err = launchRestoredLibrary(dest); err != nil {
			return "", fmt.Errorf("资料库已恢复到 %s，打开失败：%w", dest, err)
		}
		return "已恢复资料库 " + filepath.Base(dest), nil
	})
}

func (a *desktop) openRestoredLibrary() {
	a.withLibraryFolder(&a.openLibrarySettings, "选择已恢复的资料库目录", func(dir string, _ *mygo.Window) (string, error) {
		return "", launchRestoredLibrary(dir)
	})
}

func (a *desktop) withLibraryFolder(task *settingsTask, title string, action func(string, *mygo.Window) (string, error)) {
	if a.closing || task.busy {
		return
	}
	task.busy = true
	owner := a.settingsWin
	if owner == nil {
		owner = a.win
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: owner, Title: title, Directory: true, ShowHiddenFiles: true,
		})
		message := ""
		if err == nil && len(paths) > 0 {
			message, err = action(paths[0], owner)
		}
		a.publishSettings(func() { a.finishSettingsTask(task, message, err) })
	}()
}

func launchRestoredLibrary(dir string) error {
	db, err := openLibrarySnapshot(dir)
	if err != nil {
		return err
	}
	var restored string
	err = db.QueryRow("SELECT value FROM native_meta WHERE key='restored_library'").Scan(&restored)
	_ = db.Close()
	if err != nil || restored != "true" {
		return fmt.Errorf("请先将备份恢复到新的资料库")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if runtime.GOOS == "darwin" {
		macos := filepath.Dir(exe)
		contents := filepath.Dir(macos)
		bundle := filepath.Dir(contents)
		if filepath.Base(macos) == "MacOS" && filepath.Base(contents) == "Contents" && filepath.Ext(bundle) == ".app" {
			// LaunchServices gives the second instance a native window session.
			return exec.Command("/usr/bin/open", "-n", "-a", bundle, "--args", "--data-dir", dir).Run()
		}
	}
	cmd := exec.Command(exe, "--data-dir", dir)
	if err = cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
