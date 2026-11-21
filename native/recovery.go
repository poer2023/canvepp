package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
)

func (s *Store) NativeRun(id string) (NativeRun, error) {
	var raw string
	var r NativeRun
	err := s.DB.QueryRow("SELECT body FROM native_runs WHERE id=?", id).Scan(&raw)
	if err == nil {
		err = json.Unmarshal([]byte(raw), &r)
	}
	return r, err
}

// A durable claim is the only path into a new turn. It cannot be repeated on
// reconnection, double click, or restart, even if the submission reply is lost.
func (s *Store) claimRun(id string) (NativeRun, bool, error) {
	return s.advanceRun(id, "queued", "preparing")
}

func (s *Store) advanceRun(id, from, to string) (NativeRun, bool, error) {
	s.writes.Lock()
	defer s.writes.Unlock()
	r, err := s.NativeRun(id)
	if err != nil {
		return r, false, err
	}
	if r.Status != from || r.Cancelled {
		return r, false, nil
	}
	r.Status = to
	r.UpdatedAt = timestamp()
	_, err = s.DB.Exec("UPDATE native_runs SET body=? WHERE id=?", string(encode(r)), id)
	return r, err == nil, err
}
func (s *Store) CancelRun(id string) error {
	s.writes.Lock()
	defer s.writes.Unlock()
	r, err := s.NativeRun(id)
	if err != nil {
		return err
	}
	r.Cancelled = true
	if r.Status == "queued" {
		r.Status = "cancelled"
	}
	r.UpdatedAt = timestamp()
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("UPDATE native_runs SET body=? WHERE id=?", string(encode(r)), id); err != nil {
		return err
	}
	if err = saveOperations(tx, []Operation{cancelOperation(r)}); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) QueueRuns(runs []NativeRun) error {
	s.writes.Lock()
	defer s.writes.Unlock()
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, r := range runs {
		if _, err = tx.Exec("INSERT INTO native_runs VALUES(?,?,?)", r.ID, r.ProjectID, string(encode(r))); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Reconcile immutable outputs before showing a reopened canvas. A completed
// receipt and image are durable even if the UI closed before updating its node.
func reconcileResults(e *Editor, runs []NativeRun) bool {
	changed := false
	// Load order is newest first. Match live adoption by trying earlier
	// results first when several versions arrived for the same empty card.
	for _, r := range slices.Backward(runs) {
		if r.ProjectID != e.Project.ID || r.Result == "" {
			continue
		}
		if _, exists := e.Assets[r.Result]; !exists {
			continue
		}
		changed = adoptGeneratedResult(e, r.NodeID, r.Result) || changed
	}
	return changed
}

func adoptGeneratedResult(e *Editor, nodeID, assetID string) bool {
	if n := e.Node(nodeID); n != nil {
		if n.Asset != "" {
			return false
		}
		n.Asset, n.Stale = assetID, false
		return true
	}
	changed := false
	for i := range e.Trash {
		tr := &e.Trash[i]
		if tr.Node.ID != nodeID {
			continue
		}
		if !slices.Contains(tr.Assets, assetID) {
			tr.Assets = append(tr.Assets, assetID)
			changed = true
		}
		if tr.Node.Asset == "" {
			tr.Node.Asset, tr.Display, tr.Node.Stale = assetID, assetID, false
			changed = true
		}
	}
	return changed
}
func (s *Store) LegacyPrompts() (map[string]string, error) {
	rows, err := s.DB.Query("SELECT id,body FROM runs")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	prompts := map[string]string{}
	for rows.Next() {
		var id, raw string
		if err = rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		var r struct {
			Snapshot struct {
				Prompt string `json:"prompt"`
			} `json:"inputSnapshot"`
		}
		if err = json.Unmarshal([]byte(raw), &r); err != nil {
			return nil, err
		}
		prompts[id] = r.Snapshot.Prompt
	}
	return prompts, rows.Err()
}

func runMissing(err error) bool { return errors.Is(err, sql.ErrNoRows) }
