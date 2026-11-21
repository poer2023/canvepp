package main

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"sync"

	_ "golang.org/x/image/webp"
	_ "modernc.org/sqlite"
)

const maxImageBytes = 20 << 20

type Store struct {
	openAI *openAIClient
	DB     *sql.DB
	Dir    string
	writes sync.Mutex
	reveal sync.Mutex
}

func openStore(dir string) (*Store, error) {
	var err error
	dir, err = filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(dir, "thumbnails"), 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "canvepp.sqlite"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL; PRAGMA busy_timeout=5000;
	CREATE TABLE IF NOT EXISTS projects(id TEXT PRIMARY KEY,body TEXT NOT NULL);
	CREATE TABLE IF NOT EXISTS assets(id TEXT PRIMARY KEY,project_id TEXT NOT NULL,body TEXT NOT NULL,file TEXT NOT NULL);
	CREATE TABLE IF NOT EXISTS runs(id TEXT PRIMARY KEY,project_id TEXT NOT NULL,body TEXT NOT NULL);
	CREATE TABLE IF NOT EXISTS canvas_trash(id TEXT PRIMARY KEY,project_id TEXT NOT NULL,node_id TEXT NOT NULL,body TEXT NOT NULL,restored_at TEXT);
	CREATE TABLE IF NOT EXISTS native_meta(key TEXT PRIMARY KEY,value TEXT NOT NULL);
	CREATE TABLE IF NOT EXISTS native_runs(id TEXT PRIMARY KEY,project_id TEXT NOT NULL,body TEXT NOT NULL);
	CREATE TABLE IF NOT EXISTS operation_events(id TEXT PRIMARY KEY, project_id TEXT NOT NULL, body TEXT NOT NULL);
	CREATE INDEX IF NOT EXISTS operation_events_project ON operation_events(project_id);
	CREATE TABLE IF NOT EXISTS operation_summaries(id TEXT PRIMARY KEY, project_id TEXT NOT NULL, body TEXT NOT NULL);
	CREATE TABLE IF NOT EXISTS purged_nodes(id TEXT PRIMARY KEY, project_id TEXT NOT NULL, body TEXT NOT NULL);
	CREATE TABLE IF NOT EXISTS purged_assets(id TEXT PRIMARY KEY, project_id TEXT NOT NULL, body TEXT NOT NULL);
	CREATE TABLE IF NOT EXISTS assistant_chats(id TEXT PRIMARY KEY, project_id TEXT NOT NULL, body TEXT NOT NULL);
	CREATE INDEX IF NOT EXISTS native_runs_project ON native_runs(project_id);`)
	if err != nil {
		db.Close()
		return nil, err
	}
	_ = os.Chmod(filepath.Join(dir, "canvepp.sqlite"), 0600)
	return &Store{DB: db, Dir: dir, openAI: newOpenAIClient(dir)}, nil
}
func (s *Store) Close() error { return s.DB.Close() }
func (s *Store) Projects() ([]Project, error) {
	rows, err := s.DB.Query("SELECT body FROM projects ORDER BY rowid")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Project{}
	for rows.Next() {
		var raw string
		var p Project
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func (s *Store) Assets(project string) ([]Asset, error) {
	rows, err := s.DB.Query("SELECT body,file FROM assets WHERE project_id=? ORDER BY rowid", project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Asset{}
	for rows.Next() {
		var raw, file string
		var a Asset
		if err = rows.Scan(&raw, &file); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &a); err != nil {
			return nil, err
		}
		a.File = file
		out = append(out, a)
	}
	return out, rows.Err()
}
func (s *Store) Trashed(project string) ([]Trash, error) {
	rows, err := s.DB.Query("SELECT body FROM canvas_trash WHERE project_id=? ORDER BY rowid", project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Trash{}
	for rows.Next() {
		var raw string
		var t Trash
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
func (s *Store) Save(p Project, trash []Trash) error { return s.SaveWithOperations(p, trash, nil) }
func (s *Store) SaveWithOperations(p Project, trash []Trash, events []Operation) error {
	s.writes.Lock()
	defer s.writes.Unlock()
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var prior string
	err = tx.QueryRow("SELECT body FROM projects WHERE id=?", p.ID).Scan(&prior)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if prior != "" {
		var old Project
		if err = json.Unmarshal([]byte(prior), &old); err != nil {
			return err
		}
		if old.Revision > p.Revision {
			return errors.New("较新的画布内容已保存，拒绝覆盖")
		}
	}
	for _, n := range p.Graph.Nodes {
		var count int
		if err = tx.QueryRow("SELECT COUNT(*) FROM purged_nodes WHERE id=?", n.ID).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return errors.New("永久删除的图片不能恢复")
		}
	}
	if _, err = tx.Exec("INSERT INTO projects VALUES(?,?) ON CONFLICT(id) DO UPDATE SET body=excluded.body", p.ID, string(encode(p))); err != nil {
		return err
	}
	for _, t := range trash {
		var purged int
		if err = tx.QueryRow("SELECT COUNT(*) FROM purged_nodes WHERE id=?", t.Node.ID).Scan(&purged); err != nil {
			return err
		}
		if purged > 0 {
			continue
		}
		if _, err = tx.Exec("INSERT INTO canvas_trash VALUES(?,?,?,?,NULL) ON CONFLICT(id) DO UPDATE SET body=excluded.body", t.ID, p.ID, t.Node.ID, string(encode(t))); err != nil {
			return err
		}
	}
	if err = saveOperations(tx, events); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) AssetPath(a Asset) (string, error) {
	// Queued requests retain an immutable asset identity and hash. The local
	// filename lives in its own column and is excluded from serialized assets.
	if a.File == "" && a.ID != "" {
		// After a transfer, an existing task can still resolve the same immutable
		// version by its id and hash, even though its project has changed.
		if err := s.DB.QueryRow("SELECT file FROM assets WHERE id=? AND (project_id=? OR (?<>'' AND json_extract(body,'$.sha256')=?))",
			a.ID, a.ProjectID, a.SHA, a.SHA).Scan(&a.File); err != nil {
			return "", errors.New("找不到参考图片文件")
		}
	}
	if a.File == "" || a.File == "." || a.File == ".." || filepath.Base(a.File) != a.File {
		return "", errors.New("无效图片文件名")
	}
	return filepath.Join(s.Dir, "assets", a.File), nil
}
func digest(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }
func readBounded(path string, max int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readBoundedFile(f, max)
}
func readBoundedFile(f *os.File, max int64) ([]byte, error) {
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > max {
		return nil, errors.New("文件为空或超过大小限制")
	}
	b, err := io.ReadAll(io.LimitReader(f, max+1))
	if int64(len(b)) > max {
		return nil, errors.New("文件超过大小限制")
	}
	return b, err
}
func atomicFile(path string, b []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".pending-")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err == nil {
		err = ce
	}
	if err != nil {
		return err
	}
	return os.Rename(name, path)
}

type assetOrigin struct{ URL, ID string }

func (s *Store) ImportImage(b []byte, project, node, name, source, run string, parents []string, origin ...assetOrigin) (Asset, error) {
	return s.importImage(b, project, node, name, source, run, parents, nil, origin...)
}

// The canvas can use the same decoded pixels for its first preview.
func (s *Store) importImage(b []byte, project, node, name, source, run string, parents []string, decoded *image.Image, origin ...assetOrigin) (Asset, error) {
	limit := maxImageBytes
	if source == "photoshop" || source == "illustrator" {
		limit = maxStoredImageBytes
	}
	if len(b) == 0 || len(b) > limit {
		return Asset{}, fmt.Errorf("图片须小于 %d MB", limit>>20)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return Asset{}, err
	}
	if (format != "png" && format != "jpeg" && format != "webp") || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 40_000_000 {
		return Asset{}, errors.New("仅支持 4000 万像素以内的 PNG、JPEG、WebP")
	}
	im, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return Asset{}, err
	}
	if decoded != nil {
		*decoded = im
	}
	sha := digest(b)
	ext := format
	if ext == "jpeg" {
		ext = "jpg"
	}
	a := Asset{ID: uuid(), ProjectID: project, AssetID: uuid(), SHA: sha, Mime: "image/" + format, Width: cfg.Width, Height: cfg.Height, Bytes: int64(len(b)), CreatedAt: timestamp(), Name: name, Parents: parents, NodeID: node, RunID: run, Source: source, File: sha + "." + ext}
	if len(origin) > 0 {
		a.SourceURL, a.ExternalID = origin[0].URL, origin[0].ID
	}
	path, _ := s.AssetPath(a)
	s.writes.Lock()
	defer s.writes.Unlock()
	if old, err := os.ReadFile(path); err == nil {
		if digest(old) != sha {
			return Asset{}, errors.New("图片内容校验失败")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Asset{}, err
	} else if err = atomicFile(path, b); err != nil {
		return Asset{}, err
	}
	if run != "" {
		// Recovering a receipt after a crash must reuse the imported result.
		var raw, file string
		err := s.DB.QueryRow("SELECT body,file FROM assets WHERE project_id=? AND json_extract(body,'$.runId')=? AND json_extract(body,'$.sha256')=?", project, run, sha).Scan(&raw, &file)
		if err == nil {
			var prior Asset
			if err = json.Unmarshal([]byte(raw), &prior); err != nil {
				return Asset{}, err
			}
			prior.File = file
			return prior, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return Asset{}, err
		}
	}
	_, err = s.DB.Exec("INSERT INTO assets VALUES(?,?,?,?)", a.ID, a.ProjectID, string(encode(a)), a.File)
	return a, err
}
