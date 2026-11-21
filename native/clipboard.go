package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"image"
	"image/png"
	"strings"
)

const cardFormat transfer.Format = "application/vnd.canvepp.cards+json"

func (a *desktop) copySelection() {
	if a.editor == nil {
		return
	}
	clip := a.editor.Copy()
	if len(clip.Nodes) == 0 {
		return
	}
	raw := encode(clip)
	text := []byte("CANVEPP_NATIVE_V1\n" + string(raw))
	singleText := len(clip.Nodes) == 1 && clip.Nodes[0].Kind == textNodeKind
	if singleText {
		text = []byte(clip.Nodes[0].Config.Text)
	}
	reps := []transfer.Representation{transfer.Bytes(cardFormat, raw), transfer.Bytes(transfer.Text, text)}
	if err := mygo.Clipboard.Write(transfer.New(transfer.NewItem(reps...))); err != nil {
		a.message(err.Error())
		return
	}
	if singleText {
		a.message("已复制文字")
		return
	}
	a.message("已复制选中内容")
	// Native image alternatives let the same copy paste into other Mac apps.
	// Encoding and file reads never run inside pointer/paint callbacks.
	if len(clip.Nodes) == 1 {
		asset, ok := a.editor.Assets[clip.Nodes[0].Asset]
		if !ok {
			return
		}
		go func() {
			path, err := a.store.AssetPath(asset)
			if err != nil {
				return
			}
			data, err := readBounded(path, maxStoredImageBytes)
			if err != nil || digest(data) != asset.SHA {
				return
			}
			if asset.Mime != "image/png" {
				im, _, err := image.Decode(bytes.NewReader(data))
				if err != nil {
					return
				}
				var buf bytes.Buffer
				if png.Encode(&buf, im) != nil {
					return
				}
				data = buf.Bytes()
			}
			a.publish(func() {
				// Do not replace a newer clipboard owned by the user or another app.
				if mygo.Clipboard.ReadText() != string(text) {
					return
				}
				withImage := append(reps, transfer.Bytes(transfer.PNG, data))
				_ = mygo.Clipboard.Write(transfer.New(transfer.NewItem(withImage...)))
			})
		}()
	}
}
func (a *desktop) paste() {
	if a.editor == nil {
		return
	}
	raw, err := mygo.Clipboard.ReadFormat(cardFormat)
	if err != nil {
		text := mygo.Clipboard.ReadText()
		if strings.HasPrefix(text, "CANVEPP_NATIVE_V1\n") {
			raw = []byte(strings.TrimPrefix(text, "CANVEPP_NATIVE_V1\n"))
		}
	}
	if len(raw) > 0 && len(raw) < 4<<20 {
		var clip Clipboard
		if json.Unmarshal(raw, &clip) == nil {
			if clip.ProjectID != a.editor.Project.ID {
				a.pasteAcrossCanvases(clip)
				return
			}
			if err := a.editor.Paste(clip, a.center()); err != nil {
				a.message(err.Error())
			}
			a.draftKey = ""
			return
		}
	}
	if paths, err := mygo.Clipboard.ReadFiles(); err == nil && len(paths) > 0 {
		a.importFiles(paths, a.center())
		return
	}
	if b := mygo.Clipboard.ReadImage(); len(b) > 0 {
		e, at := a.editor, a.center()
		go func() {
			n := newNode(e.Project.ID, at)
			asset, err := a.store.ImportImage(b, e.Project.ID, n.ID, "剪贴板图片.png", "import", "", []string{})
			a.publish(func() {
				if err != nil {
					a.message(err.Error())
					return
				}
				e.Assets[asset.ID] = asset
				n.Asset = asset.ID
				n.Name = asset.Name
				e.AddNamed([]Node{n}, nil, "导入")
				a.draftKey = ""
			})
		}()
		return
	}
	a.message("剪贴板中没有可粘贴的图片")
}
func (a *desktop) pasteAcrossCanvases(clip Clipboard) {
	source := a.editors[clip.ProjectID]
	if source == nil {
		a.message("原画布不可用，请改为复制原图")
		return
	}
	// Validate before writing anything. Use a throwaway model so failed input
	// cannot alter the current canvas or its undo history.
	check := newEditor(Project{ID: clip.ProjectID}, nil, nil)
	check.Assets = source.Assets
	if err := check.Paste(clip, Point{}); err != nil {
		a.message(err.Error())
		return
	}
	target, at := a.editor, a.center()
	if len(target.Project.Graph.Nodes)+len(clip.Nodes) > target.Project.nodeLimit() {
		a.message("画布图片数量超过限制")
		return
	}
	assets := []Asset{}
	for _, n := range clip.Nodes {
		if asset, ok := source.Assets[n.Asset]; ok {
			assets = append(assets, asset)
		}
	}
	go func() {
		mapped, err := a.store.CloneAssets(assets, target.Project.ID)
		a.publish(func() {
			if err != nil {
				a.message(err.Error())
				return
			}
			for i := range clip.Nodes {
				if asset, ok := mapped[clip.Nodes[i].Asset]; ok {
					target.Assets[asset.ID] = asset
					clip.Nodes[i].Asset = asset.ID
				}
			}
			clip.ProjectID = target.Project.ID
			if err = target.Paste(clip, at); err != nil {
				a.message(err.Error())
			}
			a.draftKey = ""
		})
	}()
}
func (s *Store) CloneAssets(assets []Asset, project string) (map[string]Asset, error) {
	s.writes.Lock()
	defer s.writes.Unlock()
	tx, err := s.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	mapped := map[string]Asset{}
	for _, src := range assets {
		if _, ok := mapped[src.ID]; ok {
			continue
		}
		path, err := s.AssetPath(src)
		if err != nil {
			return nil, err
		}
		b, err := readBounded(path, maxStoredImageBytes)
		if err != nil {
			return nil, err
		}
		if digest(b) != src.SHA {
			return nil, errors.New("原图校验失败")
		}
		asset := src
		asset.ID = uuid()
		asset.AssetID = uuid()
		asset.ProjectID = project
		asset.NodeID = ""
		asset.Parents = []string{src.ID}
		if _, err = tx.Exec("INSERT INTO assets VALUES(?,?,?,?)", asset.ID, project, string(encode(asset)), asset.File); err != nil {
			return nil, err
		}
		mapped[src.ID] = asset
	}
	return mapped, tx.Commit()
}
