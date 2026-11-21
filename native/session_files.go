package main

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type sessionImage struct {
	Asset
	Path string `json:"path"`
}

type sessionSnapshot struct {
	Project
	Images []sessionImage `json:"assets"`
}

// Capture the editor on the UI thread, then materialize originals off-thread.
// Include versions of live nodes and the source images used to generate them.
func snapshotSession(e *Editor) (sessionSnapshot, error) {
	s := sessionSnapshot{Project: e.Project, Images: []sessionImage{}}
	s.Graph = cloneGraph(s.Graph)
	s.Graph.View = e.View
	live := make(map[string]bool)
	var pending []string
	for _, node := range s.Graph.Nodes {
		live[node.ID] = true
		if node.Asset != "" {
			pending = append(pending, node.Asset)
		}
	}
	for id, a := range e.Assets {
		if live[a.NodeID] {
			pending = append(pending, id)
		}
	}
	seen := make(map[string]bool)
	for len(pending) > 0 {
		id := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if seen[id] {
			continue
		}
		seen[id] = true
		a, ok := e.Assets[id]
		if !ok {
			return s, fmt.Errorf("找不到画布引用的图片版本 %s", id)
		}
		s.Images = append(s.Images, sessionImage{Asset: a, Path: "assets/" + a.File})
		pending = append(pending, a.Parents...)
	}
	slices.SortFunc(s.Images, func(a, b sessionImage) int { return cmp.Compare(a.ID, b.ID) })
	return s, nil
}

func (s *Store) sessionFile(snapshot sessionSnapshot) (string, error) {
	s.reveal.Lock()
	defer s.reveal.Unlock()
	dir := filepath.Join(s.Dir, "Canvases", digest([]byte(snapshot.ID)))
	folder := filepath.Join(dir, "assets")
	if err := os.MkdirAll(folder, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "canvas.json")
	var old sessionSnapshot
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &old)
	}
	current := make(map[string]bool)
	for _, img := range snapshot.Images {
		src, err := s.AssetPath(img.Asset)
		if err != nil {
			return "", err
		}
		if current[img.Path] {
			continue
		}
		current[img.Path] = true
		dst := filepath.Join(folder, img.File)
		if data, err := readBounded(dst, maxStoredImageBytes); err == nil && digest(data) == img.SHA {
			continue
		}
		data, err := readBounded(src, maxStoredImageBytes)
		if err != nil {
			return "", fmt.Errorf("无法读取原图 %s：%w", img.Name, err)
		}
		if digest(data) != img.SHA {
			return "", fmt.Errorf("原图校验失败：%s", img.Name)
		}
		if err := atomicFile(dst, data); err != nil {
			return "", err
		}
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return "", err
	}
	if err := atomicFile(path, data); err != nil {
		return "", err
	}
	// Only retire our own unchanged, content-addressed exports after the new
	// manifest has committed. Other files in the revealed folder belong to the user.
	for _, img := range old.Images {
		name := strings.TrimPrefix(img.Path, "assets/")
		if current[img.Path] || img.Path != "assets/"+name || filepath.Base(name) != name || strings.TrimSuffix(name, filepath.Ext(name)) != img.SHA {
			continue
		}
		dst := filepath.Join(folder, name)
		if b, err := readBounded(dst, maxStoredImageBytes); err == nil && digest(b) == img.SHA {
			if err := os.Remove(dst); err != nil && !errors.Is(err, os.ErrNotExist) {
				return "", err
			}
		}
	}
	return path, nil
}
