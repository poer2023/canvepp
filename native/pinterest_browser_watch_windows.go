package main

import (
	"bufio"
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

//go:embed assets/pinterest-window.ps1
var pinWindowTemplate string

//go:embed assets/pinterest-scroll.cs
var pinScrollBridge string

var pinBrowserScript = strings.Replace(pinWindowTemplate, "__CANVEPP_SCROLL_BRIDGE__", pinScrollBridge, 1)

func (b *pinterestBrowser) pageObserver(ctx context.Context, mode string, login bool) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", pinBrowserScript)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	cmd.Env = append(os.Environ(), "CANVEPP_SITE_HWND="+strconv.FormatUint(uint64(b.hwnd), 10), "CANVEPP_SITE_MODE="+mode, "CANVEPP_FROM_SAVED="+strconv.FormatBool(b.fromSaved), "CANVEPP_SITE_LOGIN="+strconv.FormatBool(login))
	return cmd
}

// Probe the live document only on an explicit toolbar action. Keep the
// current page interactive while checking, including during Google sign-in.
func (b *pinterestBrowser) openIfNeeded(a *desktop, login bool, open func(bool)) {
	if !b.owns() {
		open(login)
		return
	}
	if b.stopProbe != nil {
		if b.probeLogin == login {
			return
		}
		b.stopProbe()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	b.stopProbe, b.probeLogin = cancel, login
	b.probeID++
	id := b.probeID
	cmd := b.pageObserver(ctx, "probe", login)
	go func() {
		keep := false
		out, err := cmd.StdoutPipe()
		if err == nil && cmd.Start() == nil {
			scan := bufio.NewScanner(out)
			for scan.Scan() {
				switch scan.Text() {
				case "keep":
					keep = true
				case "back-to-saved":
					a.publish(func() {
						if id == b.probeID && !b.closed && !a.closing && a.wantsPinterestBrowser() {
							b.command(1)
						}
					})
				}
			}
			cmd.Wait()
		}
		a.publish(func() {
			if id != b.probeID || b.closed || a.closing {
				return
			}
			cancel()
			b.stopProbe = nil
			if !keep && a.wantsPinterestBrowser() {
				open(login)
			}
		})
	}()
}

// Observe only the top document of our own newly created window. The helper
// emits viewport insets and a single navigation event; URLs never leave it.
func (b *pinterestBrowser) watchPage(a *desktop, login bool) {
	ctx, cancel := context.WithCancel(context.Background())
	b.stopWatch = cancel
	cmd := b.pageObserver(ctx, "watch", login)
	input, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return
	}
	b.scrollWake = make(chan struct{}, 1)
	var mailbox scrollMailbox
	b.mailbox = &mailbox
	wake := b.scrollWake
	go func() {
		defer cancel()
		out, err := cmd.StdoutPipe()
		if err != nil || cmd.Start() != nil {
			return
		}
		go func() {
			defer input.Close()
			for {
				select {
				case <-ctx.Done():
					return
				case <-wake:
					command := mailbox.take()
					if _, err := fmt.Fprintln(input, command); err != nil {
						return
					}
				}
			}
		}()
		scan := bufio.NewScanner(out)
		for scan.Scan() {
			msg := scan.Text()
			if strings.HasPrefix(msg, "scroll ") {
				var percent, view float64
				if n, _ := fmt.Sscanf(msg, "scroll %f %f", &percent, &view); n == 2 {
					a.publish(func() {
						if !b.closed {
							b.updateScroll(percent, view)
						}
					})
				}
				continue
			}
			var t [4]int
			n, _ := fmt.Sscanf(msg, "insets %d %d %d %d", &t[0], &t[1], &t[2], &t[3])
			if n == 4 && t[0] >= 0 && t[0] <= 24 && t[1] >= 0 && t[1] <= 128 && t[2] >= 0 && t[2] <= 24 && t[3] >= 0 && t[3] <= 32 {
				a.publish(func() {
					if !b.closed && b.owns() && b.trim != t {
						b.trim = t
						b.last = [4]int{}
						if b.visible {
							b.place(b.bounds, b.deviceScale)
						}
					}
				})
			}
			if msg == "saved-page" {
				a.publish(func() { b.fromSaved = true })
			}
			if msg == "saved-after-login" {
				a.publish(func() {
					if b.closed || a.closing || !b.owns() {
						return
					}
					b.close()
					a.materials.web.browser = a.newPinterestBrowser(false)
					a.invalidateMaterials()
				})
			}
		}
		cmd.Wait()
	}()
}
