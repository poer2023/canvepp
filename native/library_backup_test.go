package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLibraryBackupRestoreAndDuplicateImport(t *testing.T) {
	s := testStore(t)
	e := fixture()
	e.Project.Name = "备份版本"
	img, err := s.ImportImage(tinyPNG(), e.Project.ID, "0", "test.png", "import", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	e.Node("0").Asset = img.ID
	if err = s.Save(e.Project, nil); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(t.TempDir(), "说明.txt")
	if err = os.WriteFile(input, []byte("本地附件检查"), 0600); err != nil {
		t.Fatal(err)
	}
	attachment, err := stageAssistantAttachment(s.Dir, input)
	if err != nil {
		t.Fatal(err)
	}
	parent := t.TempDir()
	backup, restored := filepath.Join(parent, "备份 #1"), filepath.Join(parent, "恢复的资料库")
	if err = s.Backup(backup); err != nil {
		t.Fatal(err)
	}
	// Add a different version and trash state after the backup.
	e.Project.Name = "当前版本"
	e.Project.Revision++
	e.Selection["0"] = true
	e.Delete()
	if err = s.Save(e.Project, e.Trash); err != nil {
		t.Fatal(err)
	}
	report, err := s.mergeLibrary(backup, true)
	if err != nil || report != (libraryImport{Skipped: 1}) {
		t.Fatalf("duplicate import: %+v, %v", report, err)
	}
	if err = restoreLibrary(backup, restored); err != nil {
		t.Fatal(err)
	}
	copy, err := openStore(restored)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = copy.Close() })
	current, err := s.Projects()
	if err != nil || len(current) != 1 || current[0].Name != "当前版本" || len(current[0].Graph.Nodes) != len(e.Project.Graph.Nodes) {
		t.Fatalf("current library changed: %+v, %v", current, err)
	}
	projects, err := copy.Projects()
	if err != nil || len(projects) != 1 || projects[0].Name != "备份版本" || projects[0].Graph.Nodes[0].Asset != img.ID {
		t.Fatalf("restored project: %+v, %v", projects, err)
	}
	for _, dir := range []string{backup, restored} {
		b, err := os.ReadFile(filepath.Join(dir, "assistant-work", "attachments", attachment.SHA, attachment.Name))
		if err != nil || digest(b) != attachment.SHA {
			t.Fatalf("attachment in %s: %v", dir, err)
		}
	}
	var marker string
	if err = copy.DB.QueryRow("SELECT value FROM native_meta WHERE key='restored_library'").Scan(&marker); err != nil || marker != "true" {
		t.Fatalf("restored library marker %q: %v", marker, err)
	}
	if err = restoreLibrary(backup, restored); err == nil {
		t.Fatal("restoration accepted an existing destination")
	}
}

func TestCorruptBackupLeavesNoDestination(t *testing.T) {
	s := testStore(t)
	e := fixture()
	img, err := s.ImportImage(tinyPNG(), e.Project.ID, "0", "test.png", "import", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	e.Node("0").Asset = img.ID
	if err = s.Save(e.Project, nil); err != nil {
		t.Fatal(err)
	}
	parent := t.TempDir()
	backup := filepath.Join(parent, "snapshot")
	if err = s.Backup(backup); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(backup, "assets", img.File), []byte("bad"), 0600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(parent, "restored")
	if err = restoreLibrary(backup, dest); err == nil {
		t.Fatal("restored corrupted content")
	}
	if _, err = os.Stat(dest); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed restore left a visible library", err)
	}
	if err = os.Remove(filepath.Join(s.Dir, "assets", img.File)); err != nil {
		t.Fatal(err)
	}
	if err = s.Backup(dest); err == nil {
		t.Fatal("backed up missing original")
	}
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 1 || entries[0].Name() != "snapshot" {
		t.Fatal("partial library left on disk", entries, err)
	}
}
