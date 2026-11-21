package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type nativeRunUpdate func(status, message string, asset *Asset)

func receiveNativeImage(s *Store, r NativeRun, item NativeItem, update nativeRunUpdate) bool {
	if item.Type != "imageGeneration" {
		return false
	}
	if item.Failure != nil {
		update("failed", item.Failure.Message, nil)
		return true
	}
	if item.SavedPath == "" && item.Result == "" {
		return false
	}
	data, err := nativeBytes(item, r.ThreadID)
	if err != nil {
		update("unknown", err.Error(), nil)
		return true
	}
	parents := make([]string, 0, len(r.Sources))
	for _, source := range r.Sources {
		parents = append(parents, source.ID)
	}
	asset, err := s.ImportImage(data, r.ProjectID, r.NodeID, "生成结果.png", "codex", r.ID, parents)
	if err != nil {
		update("unknown", err.Error(), nil)
		return true
	}
	update("succeeded", "", &asset)
	return true
}

func waitTemporaryImage(ctx context.Context, c *Codex, s *Store, r *NativeRun, update nativeRunUpdate) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	handle := func(msg rpcNotification) bool {
		if msg.Method != "item/completed" && msg.Method != "turn/completed" {
			return false
		}
		var event struct {
			ThreadID string     `json:"threadId"`
			TurnID   string     `json:"turnId"`
			Item     NativeItem `json:"item"`
			Turn     NativeTurn `json:"turn"`
		}
		if json.Unmarshal(msg.Params, &event) != nil || event.ThreadID != r.ThreadID {
			return false
		}
		if msg.Method == "item/completed" {
			return event.TurnID == r.TurnID && receiveNativeImage(s, *r, event.Item, update)
		}
		if event.Turn.ID != r.TurnID {
			return false
		}
		for _, item := range event.Turn.Items {
			if receiveNativeImage(s, *r, item, update) {
				return true
			}
		}
		if result := localTemporaryImage(s, *r, update); result {
			return true
		}
		if r.Cancelled {
			update("cancelled", "", nil)
		} else if event.Turn.Error != nil {
			update("failed", event.Turn.Error.Message, nil)
		} else {
			update("failed", "本轮未返回图片", nil)
		}
		return true
	}
	for {
		select {
		case msg := <-c.notifications:
			if handle(msg) {
				return
			}
		case <-ticker.C:
			if current, err := s.NativeRun(r.ID); err == nil && current.Cancelled && !r.Cancelled {
				r.Cancelled = true
				_ = c.Call(ctx, "turn/interrupt", map[string]any{"threadId": r.ThreadID, "turnId": r.TurnID}, nil)
			}
		case <-c.done:
			// The exit signal can race with the last complete RPC line.
			for len(c.notifications) > 0 {
				if handle(<-c.notifications) {
					return
				}
			}
			recoverTemporaryImage(s, *r, update)
			return
		case <-ctx.Done():
			recoverTemporaryImage(s, *r, update)
			return
		}
	}
}

func generatedImageDirectory(thread string) (string, error) {
	if thread == "" || thread == "." || thread == ".." || strings.ContainsAny(thread, "/\\:") {
		return "", errors.New("无效的生图会话编号")
	}
	home := os.Getenv("CODEX_HOME")
	if home == "" {
		user, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		home = filepath.Join(user, ".codex")
	}
	parent, err := filepath.EvalSymlinks(filepath.Join(home, "generated_images"))
	if err != nil {
		return "", err
	}
	root := filepath.Join(parent, thread)
	info, err := os.Lstat(root)
	if err == nil && !info.IsDir() {
		return "", errors.New("生图结果目录不可用")
	}
	return root, err
}

func recoverTemporaryImage(s *Store, r NativeRun, update nativeRunUpdate) {
	if !localTemporaryImage(s, r, update) {
		update("unknown", "未找到临时会话的本机结果，未重复生成", nil)
	}
}

func localTemporaryImage(s *Store, r NativeRun, update nativeRunUpdate) bool {
	var raw, file string
	err := s.DB.QueryRow("SELECT body,file FROM assets WHERE project_id=? AND json_extract(body,'$.runId')=? ORDER BY rowid LIMIT 1", r.ProjectID, r.ID).Scan(&raw, &file)
	if err == nil {
		var asset Asset
		if err = json.Unmarshal([]byte(raw), &asset); err == nil {
			asset.File = file
			update("succeeded", "", &asset)
			return true
		}
	}
	if !errors.Is(err, sql.ErrNoRows) {
		update("unknown", "读取本机生成结果失败："+err.Error(), nil)
		return true
	}
	root, err := generatedImageDirectory(r.ThreadID)
	if err != nil {
		return false
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return false
	}
	path := ""
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		switch strings.ToLower(filepath.Ext(entry.Name())) {
		case ".png", ".jpg", ".jpeg", ".webp":
			if path != "" {
				update("unknown", "临时会话有多个图片文件，无法确定原任务的结果", nil)
				return true
			}
			path = filepath.Join(root, entry.Name())
		}
	}
	return path != "" && receiveNativeImage(s, r, NativeItem{Type: "imageGeneration", SavedPath: path}, update)
}
