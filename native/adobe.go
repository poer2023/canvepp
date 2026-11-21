package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

// 40 MP of incompressible RGBA plus PNG scanline and deflate overhead.
const maxStoredImageBytes = 164 << 20
const adobeShortcut = "CmdOrCtrl+Alt+Shift+9"

//go:embed assets/adobe-snapshot.jsx
var adobeScript string

type adobeBoard struct {
	LayerCount int     `json:"layerCount"`
	Width      float64 `json:"width"`
	Height     float64 `json:"height"`
}

type adobePlacement struct {
	W     float64 `json:"width"`
	H     float64 `json:"height"`
	Group string  `json:"group,omitempty"`
}
type adobeItem struct {
	File, Name, Group string
	X, Y, W, H        float64
}
type adobeManifest struct {
	Host, Name    string
	Width, Height float64
	Version       int
	Layers        []adobeItem
}

func adobeShortcutLabel() string {
	if runtime.GOOS == "darwin" {
		return "⌘ ⌥ ⇧ 9"
	}
	return "Ctrl + Alt + Shift + 9"
}

func (a *desktop) registerAdobeShortcut() {
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		a.adobeShortcutError = "此平台不支持 Adobe 桌面应用"
		return
	}
	if err := mygo.GlobalShortcut.Register(adobeShortcut, a.captureAdobe); err != nil {
		a.adobeShortcutError = "快捷键注册失败：" + err.Error()
	}
}

func (a *desktop) captureAdobe() {
	if a.closing || a.adobeBusy {
		return
	}
	host := foregroundAdobe()
	if host == "" {
		return
	}
	a.adobeBusy = true
	go func() {
		id, err := a.runAdobeCapture(host)
		a.publish(func() {
			a.adobeBusy = false
			if err == nil {
				err = a.reload()
			}
			if err == nil {
				a.chooseSession(a.editors[id])
				a.fitNext = true
				a.message("已导入「" + a.editor.Project.Name + "」")
			} else {
				a.message("Adobe 快照失败：" + err.Error())
			}
			if a.closeAfterAdobe {
				a.closeAfterAdobe = false
				a.win.Close()
				return
			}
			a.win.Show()
			mygo.App.Focus()
			a.win.Focus()
		})
	}()
}

func (a *desktop) runAdobeCapture(host string) (string, error) {
	dir, err := os.MkdirTemp("", "canvepp-adobe-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	path, _ := json.Marshal(filepath.ToSlash(dir))
	script := strings.Replace(adobeScript, "__OUTPUT_DIRECTORY__", string(path), 1)
	if runtime.GOOS == "windows" {
		script = "\ufeff" + script
	}
	jsx := filepath.Join(dir, "capture.jsx")
	if err = os.WriteFile(jsx, []byte(script), 0600); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	output, err := exportAdobe(ctx, host, jsx)
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		return "", fmt.Errorf("%w · %s", err, strings.TrimSpace(string(output)))
	}
	if data, err := os.ReadFile(filepath.Join(dir, "error.txt")); err == nil {
		return "", errors.New(strings.TrimPrefix(string(data), "\ufeff"))
	}
	return a.store.importAdobeSnapshot(dir)
}

// Build and validate a separate library first. Publish its asset records and
// graph to the user's database in one transaction after every layer succeeded.
func (s *Store) importAdobeSnapshot(dir string) (string, error) {
	raw, err := readBounded(filepath.Join(dir, "manifest.json"), 32<<20)
	if err != nil {
		return "", err
	}
	var m adobeManifest
	if err = json.Unmarshal(bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf}), &m); err != nil {
		return "", err
	}
	if m.Version != 1 || (m.Host != "photoshop" && m.Host != "illustrator") || strings.TrimSpace(m.Name) == "" {
		return "", errors.New("无效的 Adobe 快照清单")
	}
	work, err := os.MkdirTemp(dir, "library-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(work)
	stage, err := openStore(work)
	if err != nil {
		return "", err
	}
	defer stage.Close()
	p := Project{AdobeBoard: &adobeBoard{Width: m.Width, Height: m.Height, LayerCount: len(m.Layers)}, ID: uuid(), Name: m.Name, SchemaVersion: 1, CreatedAt: timestamp(), UpdatedAt: timestamp(), Graph: Graph{Edges: []Edge{}, Nodes: []Node{}}}
	importPNG := func(file, node, name string) (Asset, error) {
		if filepath.Base(file) != file || filepath.Ext(file) != ".png" {
			return Asset{}, errors.New("快照图片路径无效")
		}
		b, err := readBounded(filepath.Join(dir, file), maxStoredImageBytes)
		if err != nil {
			return Asset{}, err
		}
		if _, err = png.DecodeConfig(bytes.NewReader(b)); err != nil {
			return Asset{}, err
		}
		return stage.ImportImage(b, p.ID, node, name+".png", m.Host, "", nil)
	}
	ids := make([]string, 0, len(m.Layers))
	for _, item := range m.Layers {
		if item.W <= 0 || item.H <= 0 || math.IsInf(item.X+item.Y+item.W+item.H, 0) {
			return "", errors.New("快照图层尺寸无效")
		}
		n := newNode(p.ID, Point{item.X, item.Y})
		n.Name = item.Name
		n.Adobe = &adobePlacement{item.W, item.H, item.Group}
		asset, err := importPNG(item.File, n.ID, item.Name)
		if err != nil {
			return "", fmt.Errorf("%s: %w", item.Name, err)
		}
		n.Asset = asset.ID
		p.Graph.Nodes = append(p.Graph.Nodes, n)
		ids = append(ids, n.ID)
	}
	// The reference lives in image history. Keeping it outside the editable
	// stack avoids double-compositing transparent pixels.
	var referenceNode string
	if len(ids) == 0 {
		n := newNode(p.ID, Point{})
		n.Name = m.Name
		n.Adobe = &adobePlacement{W: m.Width, H: m.Height}
		referenceNode = n.ID
		p.Graph.Nodes = append(p.Graph.Nodes, n)
		ids = append(ids, n.ID)
	}
	composite, err := importPNG("composite.png", referenceNode, m.Name+" · 合成参考")
	if err != nil {
		return "", err
	}
	if referenceNode != "" {
		n := &p.Graph.Nodes[0]
		n.Asset = composite.ID
		if n.Adobe.W <= 0 || n.Adobe.H <= 0 {
			n.Adobe.W, n.Adobe.H = float64(composite.Width), float64(composite.Height)
		}
	}
	event := Operation{ID: uuid(), ProjectID: p.ID, At: p.CreatedAt, Action: "import", Text: fmt.Sprintf("从 %s 导入 %d 个图层", sourceLabel(m.Host), len(ids)), Nodes: ids}
	if err = stage.SaveWithOperations(p, nil, []Operation{event}); err != nil {
		return "", err
	}
	if _, err = s.mergeLibrary(work, false); err != nil {
		return "", err
	}
	return p.ID, nil
}
