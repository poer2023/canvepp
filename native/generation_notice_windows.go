package main

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const generationNoticeAppID = "app.canvepp.native"

func generationNoticeProfile(dir string) string {
	base, err := os.UserConfigDir()
	if err == nil && strings.EqualFold(filepath.Clean(dir), filepath.Join(base, "CanveppNative")) {
		return ""
	}
	return "-" + digest([]byte(strings.ToLower(filepath.Clean(dir))))[:16]
}

func generationNoticeScheme(dir string) string {
	return "canvepp-result" + generationNoticeProfile(dir)
}

func registerWindowsGenerationNotice(dir, appID, scheme string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	keys := []struct {
		path   string
		values map[string]string
	}{
		{`AppUserModelId\` + appID, map[string]string{"DisplayName": "Canvepp"}},
		{scheme + `\shell\open\command`, map[string]string{"": syscall.EscapeArg(exe) + " --data-dir " + syscall.EscapeArg(dir) + ` "%1"`}},
		{scheme, map[string]string{"": "Canvepp 生成结果", "URL Protocol": ""}},
	}
	for _, item := range keys {
		key, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\`+item.path, registry.SET_VALUE)
		if err != nil {
			return err
		}
		for name, value := range item.values {
			if err = key.SetStringValue(name, value); err != nil {
				break
			}
		}
		key.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func windowsNoticeXML(n generationNotice, launch, preview string) string {
	escape := func(text string) string {
		var b strings.Builder
		_ = xml.EscapeText(&b, []byte(text))
		return b.String()
	}
	image := ""
	if preview != "" {
		path := filepath.ToSlash(preview)
		u := url.URL{Scheme: "file", Path: "/" + path}
		if rest, unc := strings.CutPrefix(path, "//"); unc {
			host, path, _ := strings.Cut(rest, "/")
			u.Host, u.Path = host, "/"+path
		}
		image = `<image src="` + escape(u.String()) + `"/>`
	}
	return `<toast activationType="protocol" launch="` + escape(launch) + `"><visual><binding template="ToastGeneric"><text>` +
		escape(n.title) + `</text><text>` + escape(n.body) + `</text>` + image +
		`</binding></visual><audio silent="true"/></toast>`
}

// The built-in Windows PowerShell runtime bridges to WinRT. The script is
// fixed and the notification data is read on stdin as UTF-8 JSON.
const windowsNoticeScript = `
$ErrorActionPreference = 'Stop'
$data = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String([Console]::In.ReadToEnd())) | ConvertFrom-Json
$manager = [Windows.UI.Notifications.ToastNotificationManager,Windows.UI.Notifications,ContentType=WindowsRuntime]
$null = [Windows.Data.Xml.Dom.XmlDocument,Windows.Data.Xml.Dom.XmlDocument,ContentType=WindowsRuntime]
$xml = New-Object Windows.Data.Xml.Dom.XmlDocument
$xml.LoadXml($data.content)
$toastType = [Windows.UI.Notifications.ToastNotification,Windows.UI.Notifications,ContentType=WindowsRuntime]
$toast = $toastType::new($xml)
$toast.ExpirationTime = [DateTimeOffset]::Now.AddDays(3)
$manager::CreateToastNotifier($data.appID).Show($toast)
`

func deliverGenerationNotice(s *Store, n generationNotice) error {
	appID := generationNoticeAppID + generationNoticeProfile(s.Dir)
	scheme := generationNoticeScheme(s.Dir)
	if err := registerWindowsGenerationNotice(s.Dir, appID, scheme); err != nil {
		return err
	}
	dir := filepath.Join(s.Dir, "notification-previews")
	if entries, err := os.ReadDir(dir); err == nil {
		for _, entry := range entries {
			if !entry.Type().IsRegular() || !strings.HasPrefix(entry.Name(), "generation-") || !strings.HasSuffix(entry.Name(), ".jpg") {
				continue
			}
			if info, err := entry.Info(); err == nil && time.Since(info.ModTime()) > 7*24*time.Hour {
				_ = os.Remove(filepath.Join(dir, entry.Name()))
			}
		}
	}
	path, err := s.generationNoticePreview(n.images, dir, true)
	if err != nil {
		log.Printf("生成通知缩略图：%v", err)
		path = ""
	}
	system, err := windows.GetSystemDirectory()
	if err != nil {
		return err
	}
	cmdPath := filepath.Join(system, "WindowsPowerShell", "v1.0", "powershell.exe")
	var script []byte
	for _, code := range utf16.Encode([]rune(windowsNoticeScript)) {
		script = binary.LittleEndian.AppendUint16(script, code)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, cmdPath, "-NoLogo", "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(script))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	xml := windowsNoticeXML(n, scheme+"://generation/"+n.id, path)
	cmd.Stdin = strings.NewReader(base64.StdEncoding.EncodeToString(encode(map[string]string{"appID": appID, "content": xml})))
	if output, err := cmd.CombinedOutput(); err != nil {
		if path != "" {
			_ = os.Remove(path)
		}
		return fmt.Errorf("Windows 通知失败：%w %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
