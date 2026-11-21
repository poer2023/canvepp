package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type pinterestLoginRequest uint8

const (
	noPinterestLogin     pinterestLoginRequest = iota
	pinterestLoginPrompt                       // Navigation without a user gesture.
	pinterestLoginToolbar
	pinterestGoogleClick
)

type pinterestState struct {
	vault                                *pinterestVault
	client                               *pinterestClient
	config, draft                        pinterestConfig
	ready, working, connected, configure bool
	status, configError, query           string
	auth                                 *pinterestAuthorization
	authCancel                           context.CancelFunc
	authEpoch                            uint64
	pendingLogin                         pinterestLoginRequest
	connectAfterConfig                   bool
	boards                               []pinterestBoard
	boardBookmark                        string
	boardsLoading                        bool
	boardCancel                          context.CancelFunc
	boardEpoch                           uint64
	board                                pinterestBoard
	batch                                []pinterestPicture
	part                                 int
	pictures                             []pinterestPicture
	bookmark                             string
	pages                                []string
	loading                              bool
	loadCancel                           context.CancelFunc
	loadEpoch                            uint64
	thumbs                               map[string]*ui.Bitmap
	thumbErrors                          map[string]bool
	grid                                 ui.GridState
	previewOpen                          bool
	preview                              pinterestPicture
	previewBitmap                        *ui.Bitmap
	previewError                         string
	previewCancel                        context.CancelFunc
	previewEpoch                         uint64
	search                               searchControl
}

func (p *pinterestState) stop() {
	p.boardsLoading = false
	p.loading = false
	p.authEpoch++
	p.boardEpoch++
	p.loadEpoch++
	p.previewEpoch++
	if p.authCancel != nil {
		p.authCancel()
	}
	if p.boardCancel != nil {
		p.boardCancel()
	}
	if p.loadCancel != nil {
		p.loadCancel()
	}
	if p.previewCancel != nil {
		p.previewCancel()
	}
}
func (a *desktop) initPinterest() {
	m := a.materials
	if m.pin != nil {
		return
	}
	p := &pinterestState{vault: newPinterestVault(a.store.Dir), thumbs: map[string]*ui.Bitmap{}, thumbErrors: map[string]bool{}}
	m.pin = p
	go func() {
		session, err := p.vault.load()
		a.publish(func() {
			p.ready = true
			if err != nil {
				p.status = err.Error()
			} else {
				p.config = session.Config
				p.client = newPinterestClient(session, p.vault)
				pinterestAcceptanceSetup(p)
				p.connected = p.client.token.Access != "" || p.client.token.Refresh != ""
				if a.dockVisible() && !p.connected && p.config.validate() != nil && m.showNativePins && pinterestBrowserAvailable() {
					a.setPinterestView(false)
					a.initPinterestWeb()
				}
				if p.connected {
					if !m.pinViewChosen {
						m.showNativePins = true
					}
					a.loadPinterestBoards(false)
				}
			}
			if request := p.pendingLogin; request != noPinterestLogin {
				p.pendingLogin = noPinterestLogin
				a.requestPinterestLogin(request)
			}
			a.invalidateMaterials()
		})
	}()
}

// A website login and the optional boards API have different sessions.
// Only a configured API client can return from a desktop browser into boards.
func (a *desktop) requestPinterestLogin(request pinterestLoginRequest) {
	m := a.materials
	if m == nil || a.dock.active != dockPinterest || m.showNativePins || a.closing || !a.dockVisible() {
		return
	}
	a.initPinterest()
	p := m.pin
	if !p.ready {
		// A background redirect must not overwrite a pending user click.
		p.pendingLogin = max(p.pendingLogin, request)
		return
	}
	if request == pinterestGoogleClick && a.win != nil && pinterestBrowserAvailable() {
		a.setPinterestView(false)
		a.initPinterestWeb()
		a.openWorkspace("画布")
		a.openDock(dockPinterest)
		a.invalidateMaterials()
		return
	}
	if !p.connected && p.config.validate() != nil {
		if request != pinterestGoogleClick {
			a.pinterestSiteLogin()
			return
		}
		a.returnToPinterestCollections()
		a.openPinterestConfig()
		p.connectAfterConfig = true
		a.invalidateMaterials()
		return
	}
	if p.connected {
		a.returnToPinterestCollections()
	} else {
		a.setPinterestView(true)
		if request != pinterestLoginPrompt && p.client != nil {
			a.connectPinterest()
		}
	}
	a.invalidateMaterials()
}

func (a *desktop) returnToPinterestCollections() {
	a.openDock(dockPinterest)
	a.setPinterestView(true)
}
func (a *desktop) openPinterestConfig() {
	p := a.materials.pin
	p.connectAfterConfig = false
	p.draft = p.config
	p.draft.ClientSecret = ""
	if p.draft.RedirectURI == "" {
		p.draft.RedirectURI = pinterestRedirect
		p.draft.IncludeSecret = true
	}
	p.configure = true
	p.configError = ""
}
func (a *desktop) savePinterestConfig() {
	p := a.materials.pin
	draft := p.draft
	draft.ClientID = strings.TrimSpace(draft.ClientID)
	draft.ClientSecret = strings.TrimSpace(draft.ClientSecret)
	draft.RedirectURI = strings.TrimSpace(draft.RedirectURI)
	if draft.ClientID != p.config.ClientID && draft.ClientSecret == "" {
		p.configError = "更换 App ID 时请填写对应的 App Secret"
		return
	}
	if draft.ClientSecret == "" {
		draft.ClientSecret = p.config.ClientSecret
	}
	if err := draft.validate(); err != nil {
		p.configError = err.Error()
		return
	}
	if draft == p.config {
		p.draft.ClientSecret = ""
		p.configure = false
		return
	}
	p.working = true
	p.configError = ""
	p.stop()
	client := p.client
	go func() {
		var err error
		if client != nil {
			err = client.replaceConfig(draft)
		} else {
			err = p.vault.save(pinterestSession{Config: draft})
		}
		a.publish(func() {
			p.working = false
			if err != nil {
				p.configError = err.Error()
				a.invalidateMaterials()
				return
			}
			p.config = draft
			p.draft.ClientSecret = ""
			p.configure = false
			p.connected = false
			p.auth = nil
			p.client = newPinterestClient(pinterestSession{Config: draft}, p.vault)
			pinterestAcceptanceSetup(p)
			p.boards = nil
			p.pictures = nil
			p.thumbs = map[string]*ui.Bitmap{}
			p.status = "连接设置已保存"
			if p.connectAfterConfig {
				p.connectAfterConfig = false
				a.connectPinterest()
			}
			a.invalidateMaterials()
		})
	}()
}
func (a *desktop) connectPinterest() {
	p := a.materials.pin
	if p.working || p.auth != nil {
		return
	}
	if err := p.config.validate(); err != nil {
		a.openPinterestConfig()
		return
	}
	p.stop()
	if !p.connected || p.client == nil {
		p.client = newPinterestClient(pinterestSession{Config: p.config}, p.vault)
		pinterestAcceptanceSetup(p)
	}
	a.beginPinterestAuthorization()
	a.invalidateMaterials()
}
func (a *desktop) beginPinterestAuthorization() {
	p := a.materials.pin
	epoch := p.authEpoch
	ctx, cancel := context.WithCancel(context.Background())
	p.authCancel = cancel
	client := p.client
	auth, err := startPinterestAuthorization(ctx, p.config, client.authorize)
	if err != nil {
		cancel()
		p.status = err.Error()
		return
	}
	p.auth = auth
	p.status = ""
	if err := mygo.Shell.OpenExternal(auth.URL); err != nil {
		auth.Cancel()
		cancel()
		p.auth = nil
		p.status = "浏览器未能打开，请检查默认浏览器后重试"
		return
	}
	go func() {
		result := <-auth.Done
		err := result.Err
		if err == nil {
			exchange, stop := context.WithTimeout(ctx, 40*time.Second)
			err = client.Exchange(exchange, result.Code)
			stop()
		}
		cancel()
		a.publish(func() {
			if p.authEpoch != epoch {
				return
			}
			p.auth = nil
			p.working = false
			if err != nil {
				p.status = err.Error()
			} else {
				p.status = ""
				p.connected = true
				p.boards = nil
				p.board = pinterestBoard{}
				p.pages = nil
				a.loadPinterestBoards(false)
			}
			a.returnToPinterestCollections()
			mygo.App.Focus()
			a.win.Show()
			a.win.Focus()
			if result.Err == nil {
				a.focusPinterestCallback(auth.callbackTitle)
			}
			a.invalidateMaterials()
		})
	}()
}
func (a *desktop) cancelPinterestAuth() {
	p := a.materials.pin
	if p.auth != nil && p.authCancel != nil {
		p.working = true
		p.authCancel()
	}
}
func (a *desktop) disconnectPinterest() {
	p := a.materials.pin
	if p.working || p.client == nil {
		return
	}
	p.stop()
	p.working = true
	client := p.client
	go func() {
		err := client.Disconnect()
		a.publish(func() {
			p.working = false
			if err != nil {
				p.status = err.Error()
			} else {
				p.connected = false
				p.boards = nil
				p.pictures = nil
				p.thumbs = map[string]*ui.Bitmap{}
				p.previewBitmap = nil
				p.previewOpen = false
				p.status = "已断开连接，已导入的图片仍保留在画布中"
			}
			a.invalidateMaterials()
		})
	}()
}
func (a *desktop) loadPinterestBoards(more bool) {
	p := a.materials.pin
	if p.boardsLoading {
		return
	}
	if p.boardCancel != nil {
		p.boardCancel()
	}
	p.boardEpoch++
	epoch := p.boardEpoch
	ctx, cancel := context.WithCancel(context.Background())
	p.boardCancel = cancel
	p.boardsLoading = true
	p.status = ""
	client := p.client
	bookmark := ""
	if more {
		bookmark = p.boardBookmark
	}
	go func() {
		page, err := client.Boards(ctx, bookmark)
		a.publish(func() {
			if p.boardEpoch != epoch {
				return
			}
			p.boardsLoading = false
			if err != nil {
				p.status = err.Error()
			} else {
				if !more {
					p.boards = nil
				}
				seen := map[string]bool{}
				for _, b := range p.boards {
					seen[b.ID] = true
				}
				for _, b := range page.Items {
					if pinterestID(b.ID) && !seen[b.ID] {
						p.boards = append(p.boards, b)
						seen[b.ID] = true
					}
				}
				p.boardBookmark = page.Bookmark
				if page.Bookmark == bookmark {
					p.boardBookmark = ""
				}
				if p.board.ID == "" && len(p.boards) > 0 {
					a.selectPinterestBoard(p.boards[0])
				}
			}
			a.invalidateMaterials()
		})
	}()
}
func (a *desktop) selectPinterestBoard(board pinterestBoard) {
	p := a.materials.pin
	p.board = board
	p.pages = []string{""}
	a.loadPinterestPins("")
}
func (a *desktop) loadPinterestPins(bookmark string, desiredPart ...int) {
	p := a.materials.pin
	if p.loadCancel != nil {
		p.loadCancel()
	}
	p.loadEpoch++
	epoch := p.loadEpoch
	ctx, cancel := context.WithCancel(context.Background())
	p.loadCancel = cancel
	p.loading = true
	p.status = ""
	p.pictures = nil
	p.batch = nil
	p.bookmark = ""
	p.thumbs = map[string]*ui.Bitmap{}
	p.thumbErrors = map[string]bool{}
	p.grid.ScrollTo(0, ui.Start)
	client, board := p.client, p.board.ID
	go func() {
		page, err := client.Pins(ctx, board, bookmark)
		pictures := pinterestPictures(page.Items)
		a.publish(func() {
			if p.loadEpoch != epoch {
				return
			}
			p.loading = false
			if err != nil {
				p.status = err.Error()
			} else {
				p.batch = pictures
				p.part = 0
				if len(desiredPart) > 0 && desiredPart[0] < 0 {
					p.part = max(0, (len(pictures)-1)/48)
				}
				p.bookmark = page.Bookmark
				if page.Bookmark == bookmark {
					p.bookmark = ""
				}
				a.showPinterestPart(p.part)
			}
			a.invalidateMaterials()
		})
	}()
}

func (a *desktop) showPinterestPart(part int) {
	p := a.materials.pin
	if p.loadCancel != nil {
		p.loadCancel()
	}
	p.loadEpoch++
	epoch := p.loadEpoch
	ctx, cancel := context.WithCancel(context.Background())
	p.loadCancel = cancel
	p.part = max(0, min(part, max(0, (len(p.batch)-1)/48)))
	start := min(len(p.batch), p.part*48)
	p.pictures = p.batch[start:min(start+48, len(p.batch))]
	p.thumbs = map[string]*ui.Bitmap{}
	p.thumbErrors = map[string]bool{}
	p.grid.ScrollTo(0, ui.Start)
	pictures := p.pictures
	go func() {
		work := make(chan pinterestPicture)
		for i := 0; i < 2; i++ {
			go func() {
				for pic := range work {
					if ctx.Err() != nil {
						return
					}
					data, err := downloadPinterestPicture(ctx, pic.Thumb.URL)
					var decoded image.Image
					if err == nil {
						decoded, err = materialThumbnailSize(data, 512)
					}
					a.publish(func() {
						if p.loadEpoch != epoch {
							return
						}
						if err == nil {
							p.thumbs[pic.Key] = ui.NewBitmap(decoded)
						} else {
							p.thumbErrors[pic.Key] = true
						}
						a.invalidateMaterials()
					})
				}
			}()
		}
		for _, pic := range pictures {
			select {
			case work <- pic:
			case <-ctx.Done():
				close(work)
				return
			}
		}
		close(work)
	}()
}
func (a *desktop) importPinterestPicture(pic pinterestPicture) {
	a.importPinterestPictureAt(a.editor, a.center(), pic)
}
func (a *desktop) importPinterestPictureAt(e *Editor, at Point, pic pinterestPicture) {
	ext := strings.ToLower(filepath.Ext(strings.Split(pic.Original.URL, "?")[0]))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".webp" {
		ext = ".jpg"
	}
	a.importMaterialInto(e, at, "pinterest", pic.Key, pic.Name+ext, pic.WebURL(), func(ctx context.Context) ([]byte, error) { return downloadPinterestPicture(ctx, pic.Original.URL) })
}
func (a *desktop) previewPinterest(pic pinterestPicture) {
	p := a.materials.pin
	if p.previewCancel != nil {
		p.previewCancel()
	}
	p.previewEpoch++
	epoch := p.previewEpoch
	ctx, cancel := context.WithCancel(context.Background())
	p.previewCancel = cancel
	p.preview = pic
	p.previewOpen = true
	p.previewBitmap = p.thumbs[pic.Key]
	p.previewError = ""
	go func() {
		data, err := downloadPinterestPicture(ctx, pic.Original.URL)
		var img image.Image
		if err == nil {
			var cfg image.Config
			cfg, _, err = image.DecodeConfig(bytes.NewReader(data))
			if err == nil && (cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 40_000_000) {
				err = fmt.Errorf("图片尺寸过大")
			}
			if err == nil {
				img, _, err = image.Decode(bytes.NewReader(data))
			}
		}
		a.publish(func() {
			if p.previewEpoch != epoch {
				return
			}
			if err != nil {
				p.previewError = "预览暂时无法加载，可打开原网页查看"
			} else {
				p.previewBitmap = ui.NewBitmap(img)
			}
			a.invalidateMaterials()
		})
	}()
}
