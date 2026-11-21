package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"slices"
	"strings"

	"github.com/egoist/mygo"
)

const generationNoticePrefix = "generation-result:"
const generatedPictures = "生成结果"

type generationNoticeGroup struct {
	id      string
	runs    []string
	results []Asset
}

type generationNoticeRun struct {
	group *generationNoticeGroup
	index int
}

// Groups share a notice while any of their runs overlap. The gesture that
// queued a run determines its group, even when jobs finish out of order.
type generationNoticeTracker struct {
	groups []*generationNoticeGroup
	active map[string]generationNoticeRun
}

func (t *generationNoticeTracker) start(runs []NativeRun) {
	if len(runs) == 0 {
		return
	}
	id := runs[0].GenerationGroup
	if id == "" {
		id = runs[0].ID
	}
	var group *generationNoticeGroup
	for _, g := range t.groups {
		if g.id == id {
			group = g
			break
		}
	}
	if group == nil {
		group = &generationNoticeGroup{id: id}
		t.groups = append(t.groups, group)
	}
	if t.active == nil {
		t.active = make(map[string]generationNoticeRun)
	}
	for _, run := range runs {
		if _, exists := t.active[run.ID]; exists {
			continue
		}
		index := slices.Index(group.runs, run.ID)
		if index < 0 {
			index = len(group.runs)
			group.runs = append(group.runs, run.ID)
			group.results = append(group.results, Asset{})
		}
		t.active[run.ID] = generationNoticeRun{group, index}
	}
}

func (t *generationNoticeTracker) finish(run NativeRun, asset *Asset) []*generationNoticeGroup {
	tracked, ok := t.active[run.ID]
	if !ok {
		return nil
	}
	switch run.Status {
	case "succeeded", "failed", "cancelled", "unknown":
	default:
		return nil
	}
	if run.Status == "succeeded" && asset != nil && asset.ID != "" && asset.ID == run.Result && asset.RunID == run.ID {
		tracked.group.results[tracked.index] = *asset
	}
	delete(t.active, run.ID)
	if len(t.active) != 0 {
		return nil
	}
	groups := t.groups
	t.groups = nil
	return groups
}

type generationNoticeReceipt struct {
	Assets []string `json:"assets"`
	Groups int      `json:"groups"`
}

type generationNotice struct {
	id, title, body string
	images          []Asset
}

func (a *desktop) finishGenerationNotice(run NativeRun, asset *Asset) {
	groups := a.generationNotices.finish(run, asset)
	if len(groups) == 0 {
		return
	}
	store := a.store
	go func() {
		notice, err := store.reserveGenerationNotice(groups)
		if err == nil && notice.id != "" {
			err = deliverGenerationNotice(store, notice)
		}
		if err != nil {
			log.Printf("图片生成通知：%v", err)
		}
	}()
}

// Save the click destination and de-duplicate completion receipts in the
// same transaction. A notification can open this result after an app restart.
func (s *Store) reserveGenerationNotice(groups []*generationNoticeGroup) (generationNotice, error) {
	var notice generationNotice
	s.writes.Lock()
	defer s.writes.Unlock()
	tx, err := s.DB.Begin()
	if err != nil {
		return notice, err
	}
	defer tx.Rollback()
	var receipt generationNoticeReceipt
	var previews [][]Asset
	projects := make(map[string]string)
	missing := 0
	for _, group := range groups {
		var saved []Asset
		for _, asset := range group.results {
			if asset.ID == "" {
				missing++
				continue
			}
			var projectID, name string
			err = tx.QueryRow(`SELECT a.project_id, COALESCE(json_extract(p.body, '$.name'), '')
				FROM assets a LEFT JOIN projects p ON p.id = a.project_id
				JOIN native_runs r ON r.id = ?
				WHERE a.id = ? AND json_extract(r.body, '$.status') = 'succeeded' AND json_extract(r.body, '$.result') = a.id`,
				asset.RunID, asset.ID).Scan(&projectID, &name)
			if errors.Is(err, sql.ErrNoRows) {
				missing++
				continue
			}
			if err != nil {
				return notice, err
			}
			result, err := tx.Exec("INSERT OR IGNORE INTO native_meta VALUES(?,?)", "generation-notified:"+asset.RunID, asset.ID)
			if err != nil {
				return notice, err
			}
			inserted, err := result.RowsAffected()
			if err != nil {
				return notice, err
			}
			if inserted == 0 {
				continue
			}
			projects[projectID] = name
			saved = append(saved, asset)
			receipt.Assets = append(receipt.Assets, asset.ID)
		}
		if len(saved) > 0 {
			previews = append(previews, saved)
		}
	}
	if len(receipt.Assets) == 0 {
		return notice, nil
	}
	receipt.Groups = len(previews)
	id := uuid()
	if _, err = tx.Exec("INSERT INTO native_meta VALUES(?,?)", generationNoticePrefix+id, string(encode(receipt))); err != nil {
		return notice, err
	}
	if err := tx.Commit(); err != nil {
		return notice, err
	}
	notice.id = id
	notice.title = fmt.Sprintf("已生成 %d 张图片", len(receipt.Assets))
	if receipt.Groups > 1 {
		notice.title = fmt.Sprintf("生成完成 · %d 组 %d 张", receipt.Groups, len(receipt.Assets))
	}
	notice.body = fmt.Sprintf("已保存到 %d 张画布", len(projects))
	if len(projects) == 1 {
		for _, name := range projects {
			if name != "" {
				notice.body = "已保存到「" + name + "」"
			}
		}
	}
	if missing > 0 {
		notice.body += fmt.Sprintf(" · %d 张未完成", missing)
	}
	// Sample the first result of every group before taking a second from any.
	for i := 0; i < 3 && len(notice.images) < min(3, len(receipt.Assets)); i++ {
		for _, group := range previews {
			if i < len(group) {
				notice.images = append(notice.images, group[i])
				if len(notice.images) == 3 {
					break
				}
			}
		}
	}
	return notice, nil
}

func validGenerationNoticeID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, c := range id {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
				return false
			}
		}
	}
	return true
}

func generationNoticeFromArgs(args []string, scheme string) string {
	if scheme == "" || len(args) == 0 {
		return ""
	}
	u, err := url.Parse(args[len(args)-1])
	if err != nil || u.Scheme != scheme || u.User != nil || u.Host != "generation" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return ""
	}
	id := strings.TrimPrefix(u.EscapedPath(), "/")
	if validGenerationNoticeID(id) {
		return id
	}
	return ""
}

func (a *desktop) openGenerationNotice(id string) bool {
	if !validGenerationNoticeID(id) {
		return false
	}
	var raw string
	err := a.store.DB.QueryRow("SELECT value FROM native_meta WHERE key=?", generationNoticePrefix+id).Scan(&raw)
	var receipt generationNoticeReceipt
	if err != nil || json.Unmarshal([]byte(raw), &receipt) != nil || len(receipt.Assets) == 0 {
		return false
	}
	a.previewOpen = false
	a.openWorkspace("画布")
	if len(receipt.Assets) == 1 {
		for _, project := range a.order {
			e := a.editors[project]
			if asset, ok := e.Assets[receipt.Assets[0]]; ok && e.Project.DeletedAt == "" && !archivedPictureVersions(e)[asset.ID] {
				a.returnToAsset(libraryEntry{projectID: project, asset: &asset})
				return true
			}
		}
	}
	title := "本组生成"
	if receipt.Groups > 1 {
		title = "本批生成"
	}
	a.noticeOverview = overviewState{title: title, only: make(map[string]bool, len(receipt.Assets))}
	for _, id := range receipt.Assets {
		a.noticeOverview.only[id] = true
	}
	a.openWorkspace(generatedPictures)
	return true
}

func bindGenerationNoticeActivation(a *desktop, launchArgs []string) func() {
	scheme := generationNoticeScheme(a.store.Dir)
	pending := generationNoticeFromArgs(launchArgs, scheme)
	open := func(id string) {
		if !validGenerationNoticeID(id) {
			return
		}
		if a.win == nil {
			pending = id
			return
		}
		a.win.Update(func() {
			if !a.openGenerationNotice(id) {
				return
			}
			mygo.App.Focus()
			a.win.Show()
			if a.win.IsMinimized() {
				a.win.Restore()
			}
			a.win.Focus()
		})
	}
	mygo.App.OnNotificationClick(func(id string) {
		if receipt, ok := strings.CutPrefix(id, generationNoticePrefix); ok {
			open(receipt)
		}
	})
	mygo.App.OnSecondInstance(func(args []string, _ string) {
		open(generationNoticeFromArgs(args, scheme))
	})
	return func() { open(pending); pending = "" }
}
