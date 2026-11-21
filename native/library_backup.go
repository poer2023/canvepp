package main

import (
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type libraryImport struct {
	Imported, Skipped int
}

func openLibrarySnapshot(dir string) (*sql.DB, error) {
	path, err := filepath.Abs(filepath.Join(dir, "canvepp.sqlite"))
	if err != nil {
		return nil, err
	}
	uri := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	if filepath.VolumeName(path) != "" && !strings.HasPrefix(uri.Path, "/") {
		uri.Path = "/" + uri.Path
	}
	db, err := sql.Open("sqlite", uri.String()+"?mode=ro")
	if err == nil {
		db.SetMaxOpenConns(1)
	}
	return db, err
}

// Legacy startup callers use the number of newly inserted canvases.
func (s *Store) ImportLegacy(dir string) (int, error) {
	report, err := s.mergeLibrary(dir, true)
	return report.Imported, err
}

func (s *Store) mergeLibrary(dir string, legacy bool) (libraryImport, error) {
	var report libraryImport
	source, err := os.Stat(filepath.Join(dir, "canvepp.sqlite"))
	if err != nil {
		return report, err
	}
	own, err := os.Stat(filepath.Join(s.Dir, "canvepp.sqlite"))
	if err != nil {
		return report, err
	}
	if os.SameFile(source, own) {
		return report, errors.New("请选择其他资料库或备份目录")
	}
	db, err := openLibrarySnapshot(dir)
	if err != nil {
		return report, err
	}
	defer db.Close()
	src, err := db.Begin()
	if err != nil {
		return report, err
	}
	defer src.Rollback()

	s.writes.Lock()
	defer s.writes.Unlock()
	dst, err := s.DB.Begin()
	if err != nil {
		return report, err
	}
	defer dst.Rollback()
	created := []string{}
	committed := false
	defer func() {
		if !committed {
			for _, path := range created {
				_ = os.Remove(path)
			}
		}
	}()
	skipped := make(map[string]bool)
	for _, table := range []struct {
		name, columns string
		n             int
	}{
		{"projects", "id,body", 2},
		{"assets", "id,project_id,body,file", 4},
		{"runs", "id,project_id,body", 3},
		{"canvas_trash", "id,project_id,node_id,body,restored_at", 5},
		{"native_runs", "id,project_id,body", 3},
		{"operation_events", "id,project_id,body", 3},
		{"operation_summaries", "id,project_id,body", 3},
		{"purged_nodes", "id,project_id,body", 3},
		{"purged_assets", "id,project_id,body", 3},
		{"assistant_chats", "id,project_id,body", 3},
	} {
		var exists int
		if err = src.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table.name).Scan(&exists); err != nil {
			return libraryImport{}, err
		}
		if exists == 0 {
			continue
		}
		rows, err := src.Query("SELECT " + table.columns + " FROM " + table.name + " ORDER BY rowid")
		if err != nil {
			return libraryImport{}, err
		}
		marks := strings.TrimSuffix(strings.Repeat("?,", table.n), ",")
		err = func() error {
			defer rows.Close()
			for rows.Next() {
				vals, ptrs := make([]any, table.n), make([]any, table.n)
				for i := range vals {
					ptrs[i] = &vals[i]
				}
				if err := rows.Scan(ptrs...); err != nil {
					return err
				}
				if skipped[fmt.Sprint(vals[1])] && table.name != "projects" {
					continue
				}
				var existing int
				err := dst.QueryRow("SELECT 1 FROM "+table.name+" WHERE id=?", vals[0]).Scan(&existing)
				if err == nil {
					if table.name == "projects" {
						skipped[fmt.Sprint(vals[0])] = true
						report.Skipped++
					}
					continue
				}
				if !errors.Is(err, sql.ErrNoRows) {
					return err
				}
				switch table.name {
				case "assets":
					if err = dst.QueryRow("SELECT COUNT(*) FROM purged_assets WHERE id=?", vals[0]).Scan(&existing); err != nil {
						return err
					}
					if existing != 0 {
						continue
					}
					path, err := copyLibraryImage(dir, s.Dir, fmt.Sprint(vals[3]), fmt.Sprint(vals[2]))
					if err != nil {
						return err
					}
					if path != "" {
						created = append(created, path)
					}
				case "canvas_trash":
					if err = dst.QueryRow("SELECT COUNT(*) FROM purged_nodes WHERE id=?", vals[2]).Scan(&existing); err != nil {
						return err
					}
					if existing != 0 {
						continue
					}
				}
				if _, err = dst.Exec("INSERT INTO "+table.name+" VALUES("+marks+")", vals...); err != nil {
					return err
				}
				if table.name == "projects" {
					report.Imported++
				}
			}
			return rows.Err()
		}()
		if err != nil {
			return libraryImport{}, err
		}
	}
	if report.Imported+report.Skipped == 0 {
		return libraryImport{}, errors.New("这个目录里没有画布")
	}
	if legacy {
		if _, err = dst.Exec("INSERT OR REPLACE INTO native_meta VALUES('legacy_import',?)", timestamp()); err != nil {
			return libraryImport{}, err
		}
	}
	if err = dst.Commit(); err != nil {
		return libraryImport{}, err
	}
	committed = true
	return report, nil
}

func (s *Store) Backup(dir string) error {
	s.writes.Lock()
	defer s.writes.Unlock()
	return copyLibrarySnapshot(s.DB, s.Dir, dir, false)
}

func restoreLibrary(source, dest string) error {
	db, err := openLibrarySnapshot(source)
	if err != nil {
		return err
	}
	defer db.Close()
	return copyLibrarySnapshot(db, source, dest, true)
}

func copyLibrarySnapshot(source *sql.DB, sourceDir, dest string, restored bool) error {
	if _, err := os.Lstat(dest); err == nil {
		return errors.New("目标资料库已存在")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	staged, err := os.MkdirTemp(filepath.Dir(dest), ".canvepp-library-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staged)
	path := filepath.Join(staged, "canvepp.sqlite")
	if _, err = source.Exec("VACUUM INTO ?", path); err != nil {
		return err
	}
	if err = os.Chmod(path, 0600); err != nil {
		return err
	}
	if err = copySnapshotFiles(sourceDir, staged); err != nil {
		return err
	}
	if restored {
		store, err := openStore(staged)
		if err != nil {
			return err
		}
		_, err = store.DB.Exec(`INSERT OR REPLACE INTO native_meta VALUES
			('restored_library','true'), ('assistant_hotkey','false'), ('explorer_preview','false')`)
		closeErr := store.Close()
		if err != nil || closeErr != nil {
			return errors.Join(err, closeErr)
		}
	}
	if _, err = os.Lstat(dest); err == nil {
		return errors.New("目标资料库已存在")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(staged, dest)
}

func copySnapshotFiles(source, dest string) error {
	snapshot, err := openLibrarySnapshot(dest)
	if err != nil {
		return err
	}
	defer snapshot.Close()
	var integrity string
	if err = snapshot.QueryRow("PRAGMA quick_check").Scan(&integrity); err != nil {
		return err
	}
	if integrity != "ok" {
		return errors.New("资料库完整性校验失败")
	}
	if err = os.Mkdir(filepath.Join(dest, "assets"), 0700); err != nil {
		return err
	}
	rows, err := snapshot.Query("SELECT DISTINCT file,body FROM assets")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var file, body string
		if err = rows.Scan(&file, &body); err != nil {
			return err
		}
		if _, err = copyLibraryImage(source, dest, file, body); err != nil {
			return err
		}
	}
	if err = rows.Err(); err != nil {
		return err
	}
	return copyLibraryAttachments(source, dest)
}

func copyLibraryImage(source, dest, file, body string) (string, error) {
	if !filepath.IsLocal(file) || filepath.Base(file) != file {
		return "", errors.New("资料库图片路径无效")
	}
	var a Asset
	if err := json.Unmarshal([]byte(body), &a); err != nil {
		return "", err
	}
	path, err := copyLibraryFile(source, dest, filepath.Join("assets", file), a.SHA, maxStoredImageBytes)
	if err != nil {
		return "", fmt.Errorf("图片 %q：%w", a.Name, err)
	}
	return path, nil
}

func copyLibraryAttachments(source, dest string) error {
	rel := filepath.Join("assistant-work", "attachments")
	root, err := os.OpenRoot(source)
	if err != nil {
		return err
	}
	defer root.Close()
	dirs, err := fs.ReadDir(root.FS(), filepath.ToSlash(rel))
	if errors.Is(err, os.ErrNotExist) {
		return nil // Backups predating local assistant attachments.
	}
	if err != nil {
		return err
	}
	for _, dir := range dirs {
		sha := dir.Name()
		if _, err := hex.DecodeString(sha); !dir.IsDir() || len(sha) != 64 || err != nil {
			return errors.New("附件校验目录无效")
		}
		subdir := filepath.Join(rel, sha)
		files, err := fs.ReadDir(root.FS(), filepath.ToSlash(subdir))
		if err != nil {
			return err
		}
		if err = os.MkdirAll(filepath.Join(dest, subdir), 0700); err != nil {
			return err
		}
		for _, file := range files {
			if strings.HasPrefix(file.Name(), ".pending-") {
				continue // An attachment that has not been published yet.
			}
			if !file.Type().IsRegular() {
				return errors.New("附件路径无效")
			}
			if _, err = copyLibraryFile(source, dest, filepath.Join(subdir, file.Name()), sha, maxAttachmentBytes); err != nil {
				return fmt.Errorf("附件 %q：%w", file.Name(), err)
			}
		}
	}
	return nil
}

// Both images and attachments are immutable and content-addressed. Keep an
// existing file only if it has the expected content.
func copyLibraryFile(source, dest, rel, sha string, limit int64) (string, error) {
	root, err := os.OpenRoot(source)
	if err != nil {
		return "", err
	}
	defer root.Close()
	info, err := root.Lstat(rel)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("资源路径无效")
	}
	f, err := root.Open(rel)
	if err != nil {
		return "", err
	}
	b, err := readBoundedFile(f, limit)
	_ = f.Close()
	if err != nil {
		return "", err
	}
	if digest(b) != sha {
		return "", errors.New("文件内容校验失败")
	}
	path := filepath.Join(dest, rel)
	if _, err = os.Lstat(path); err == nil {
		old, err := readBounded(path, limit)
		if err != nil {
			return "", err
		}
		if digest(old) != sha {
			return "", errors.New("目标已存在同名文件，内容不同")
		}
		return "", nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return path, atomicFile(path, b)
}
