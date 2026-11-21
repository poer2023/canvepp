package main

import (
	"encoding/json"
	"slices"
	"strings"
)

// imageRecord is a read-only view of a submitted request. Canvas node config
// and visual edges are deliberately not used to reconstruct its history.
type imageRecord struct {
	ID, ProjectID, NodeID, Prompt, Status, CreatedAt, Mode string
	Params                                                 Params
	Sources, Results                                       []string
	Native                                                 bool
}

func (s *Store) ImageRecords() ([]imageRecord, error) {
	rows, err := s.DB.Query("SELECT body FROM runs ORDER BY rowid")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []imageRecord
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var r struct {
			ID, ProjectID, NodeID, Status, CreatedAt, Mode string
			Results                                        []string `json:"assetVersionIds"`
			Snapshot                                       struct {
				Prompt  string
				Params  Params
				Source  string   `json:"sourceAssetVersionId"`
				Sources []string `json:"sourceAssetVersionIds"`
			} `json:"inputSnapshot"`
		}
		if err := json.Unmarshal([]byte(raw), &r); err != nil {
			return nil, err
		}
		sources := r.Snapshot.Sources
		if len(sources) == 0 && r.Snapshot.Source != "" {
			sources = []string{r.Snapshot.Source}
		}
		out = append(out, imageRecord{ID: r.ID, ProjectID: r.ProjectID, NodeID: r.NodeID, Prompt: r.Snapshot.Prompt, Params: r.Snapshot.Params, Sources: sources, Results: r.Results, Status: r.Status, CreatedAt: r.CreatedAt, Mode: r.Mode})
	}
	return out, rows.Err()
}

type imageLineage struct {
	revision, assetCount int
	assets               map[string]Asset
	records              map[string]imageRecord
	byResult             map[string]string
	bySource             map[string][]string
}

func (a *desktop) imageHistory() *imageLineage {
	count := 0
	for _, e := range a.editors {
		count += len(e.Assets)
	}
	if a.lineage != nil && a.lineage.revision == a.runRevision && a.lineage.assetCount == count {
		return a.lineage
	}
	h := &imageLineage{revision: a.runRevision, assetCount: count, assets: map[string]Asset{}, records: map[string]imageRecord{}, byResult: map[string]string{}, bySource: map[string][]string{}}
	for _, e := range a.editors {
		for id, asset := range e.Assets {
			h.assets[id] = asset
		}
	}
	for _, r := range a.legacyRecords {
		h.records[r.ID] = r
	}
	for _, r := range a.runs {
		record := imageRecord{ID: r.ID, ProjectID: r.ProjectID, NodeID: r.NodeID, Prompt: r.Prompt, Params: r.Params, Status: r.Status, CreatedAt: r.CreatedAt, Mode: "codex", Native: true}
		if r.Funding == apiFunding {
			record.Mode = "api"
		}
		for _, source := range r.Sources {
			record.Sources = append(record.Sources, source.ID)
			if _, ok := h.assets[source.ID]; !ok {
				h.assets[source.ID] = source
			}
		}
		if r.Result != "" {
			record.Results = []string{r.Result}
		}
		h.records[r.ID] = record
	}
	for _, asset := range h.assets {
		key := asset.RunID
		if key == "" {
			if len(asset.Parents) == 0 {
				continue
			}
			key = "asset:" + asset.ID
		}
		r, ok := h.records[key]
		if !ok {
			r = imageRecord{ID: key, ProjectID: asset.ProjectID, NodeID: asset.NodeID, CreatedAt: asset.CreatedAt, Status: "succeeded", Mode: asset.Source}
		}
		if !slices.Contains(r.Results, asset.ID) {
			r.Results = append(slices.Clone(r.Results), asset.ID)
		}
		// Preserve ordered task references; append only metadata-only parents.
		for _, parent := range asset.Parents {
			if !slices.Contains(r.Sources, parent) {
				r.Sources = append(slices.Clone(r.Sources), parent)
			}
		}
		h.records[key] = r
	}
	for id, r := range h.records {
		for _, result := range r.Results {
			h.byResult[result] = id
		}
		for _, source := range r.Sources {
			if !slices.Contains(h.bySource[source], id) {
				h.bySource[source] = append(h.bySource[source], id)
			}
		}
	}
	for _, ids := range h.bySource {
		slices.SortFunc(ids, func(x, y string) int {
			xd, _ := galleryDate(h.records[x].CreatedAt)
			yd, _ := galleryDate(h.records[y].CreatedAt)
			if cmp := strings.Compare(yd, xd); cmp != 0 {
				return cmp
			}
			return strings.Compare(x, y)
		})
	}
	a.lineage = h
	return h
}

func (a *desktop) selectedImage() (Node, Asset, bool) {
	if a.editor == nil || len(a.draftSelection) != 1 {
		return Node{}, Asset{}, false
	}
	n := a.editor.Node(a.draftSelection[0])
	if n == nil {
		return Node{}, Asset{}, false
	}
	asset, ok := a.editor.Assets[n.Asset]
	return *n, asset, ok
}

func (a *desktop) sourceRecord(asset Asset) (imageRecord, bool) {
	h := a.imageHistory()
	id := h.byResult[asset.ID]
	if id == "" {
		id = asset.RunID
	}
	r, ok := h.records[id]
	return r, ok
}

func (a *desktop) subsequentRecords(asset Asset) []string {
	return a.imageHistory().bySource[asset.ID]
}

func (a *desktop) hasCreationDraft(n Node, asset Asset) bool {
	if n.Config.CreationStarted || n.Asset == "" {
		return true
	}
	// An older node can contain a saved next-step draft. Keep it, but do not
	// present the original generation prompt as an automatically new creation.
	if strings.TrimSpace(n.Config.Text) == "" {
		return false
	}
	if r, ok := a.sourceRecord(asset); ok && r.Prompt == n.Config.Text {
		// Native receipts are per version, even when the original request had a higher count.
		return (r.Params.Size != "" && n.Config.Params.Size != "" && r.Params.Size != n.Config.Params.Size) ||
			(!r.Native && r.Params.N > 0 && n.Config.Params.N > 0 && r.Params.N != n.Config.Params.N)
	}
	return true
}
