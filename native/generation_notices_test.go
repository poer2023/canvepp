package main

import (
	"image"
	"image/color"
	"reflect"
	"testing"
)

func TestGenerationNoticeSavedResults(t *testing.T) {
	for _, tc := range []struct {
		name            string
		sizes           []int
		total, inNotice int
		title, page     string
	}{
		{"one", []int{1}, 1, 1, "已生成 1 张图片", "画布"},
		{"group", []int{4}, 4, 3, "已生成 4 张图片", generatedPictures},
		{"batch", []int{3, 4, 1}, 8, 3, "生成完成 · 3 组 8 张", generatedPictures},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testStore(t)
			p := Project{ID: uuid(), Name: "海岸色稿"}
			if err := s.Save(p, nil); err != nil {
				t.Fatal(err)
			}
			var tracker generationNoticeTracker
			var runs []NativeRun
			var assets []Asset
			var representatives []string
			for _, n := range tc.sizes {
				group := uuid()
				var pending []NativeRun
				for i := range n {
					node := newNode(p.ID, Point{})
					r := NativeRun{ID: uuid(), ProjectID: p.ID, NodeID: node.ID, GenerationGroup: group}
					asset, err := s.ImportImage(tinyPNG(), p.ID, node.ID, "图片.png", "codex", r.ID, nil)
					if err != nil {
						t.Fatal(err)
					}
					if i == 0 {
						representatives = append(representatives, asset.ID)
					}
					r.Result, r.Status = asset.ID, "succeeded"
					if err := s.SaveRun(r); err != nil {
						t.Fatal(err)
					}
					node.Asset = asset.ID
					p.Graph.Nodes = append(p.Graph.Nodes, node)
					pending = append(pending, r)
					runs, assets = append(runs, r), append(assets, asset)
				}
				tracker.start(pending)
			}
			if err := s.Save(p, nil); err != nil {
				t.Fatal(err)
			}
			unrelated, err := s.ImportImage(tinyPNG(), p.ID, "", "手动导入.png", "upload", "", nil)
			if err != nil {
				t.Fatal(err)
			}
			var groups []*generationNoticeGroup
			for i := len(runs) - 1; i >= 0; i-- {
				groups = tracker.finish(runs[i], &assets[i])
				if i != 0 && len(groups) != 0 {
					t.Fatal("sent before all overlapping groups settled")
				}
			}
			n, err := s.reserveGenerationNotice(groups)
			if err != nil || n.title != tc.title || len(n.images) != tc.inNotice {
				t.Fatalf("notice %+v, error %v", n, err)
			}
			for i, id := range representatives {
				if n.images[i].ID != id {
					t.Fatal("did not sample one result per group in submission order")
				}
			}
			if duplicate, err := s.reserveGenerationNotice(groups); err != nil || duplicate.id != "" {
				t.Fatal("duplicate completion reserved another notice", err)
			}
			app, err := newDesktop(s)
			if err != nil {
				t.Fatal(err)
			}
			if !app.openGenerationNotice(n.id) || app.left != tc.page {
				t.Fatal("stored click destination did not open")
			}
			if tc.total == 1 {
				if app.editor.Project.ID != p.ID || !app.editor.Selection[runs[0].NodeID] {
					t.Fatal("did not locate the generated node")
				}
			} else {
				app.layoutOverview(850)
				if app.noticeOverview.count != tc.total || app.noticeOverview.only[unrelated.ID] || app.overview.count != 0 {
					t.Fatal("result page included unrelated pictures or modified the main gallery")
				}
			}
		})
	}
}

func TestGenerationNoticeRequiresCommittedSuccess(t *testing.T) {
	s := testStore(t)
	var tracker generationNoticeTracker
	a := NativeRun{ID: uuid(), GenerationGroup: uuid(), ProjectID: "test"}
	b := NativeRun{ID: uuid(), GenerationGroup: a.GenerationGroup, ProjectID: a.ProjectID}
	c := NativeRun{ID: uuid(), GenerationGroup: a.GenerationGroup, ProjectID: a.ProjectID}
	runs := []NativeRun{a, b, c}
	tracker.start(runs)
	a.Status = "unknown"
	tracker.finish(a, nil)
	tracker.start([]NativeRun{a}) // A recovery while another run is still pending.
	for _, run := range []*NativeRun{&a, &b, &c} {
		img, err := s.ImportImage(tinyPNG(), run.ProjectID, run.ID, "图片.png", "codex", run.ID, nil)
		if err != nil {
			t.Fatal(err)
		}
		run.Result, run.Status = img.ID, "succeeded"
		if run.ID != c.ID {
			if err = s.SaveRun(*run); err != nil {
				t.Fatal(err)
			}
		}
		groups := tracker.finish(*run, &img)
		if run.ID == c.ID {
			n, err := s.reserveGenerationNotice(groups)
			if err != nil || n.title != "已生成 2 张图片" || n.body != "已保存到 1 张画布 · 1 张未完成" {
				t.Fatalf("notice %+v, error %v", n, err)
			}
		}
	}
	tracker.start([]NativeRun{c})
	c.Status = "failed"
	if n, err := s.reserveGenerationNotice(tracker.finish(c, nil)); err != nil || n.id != "" {
		t.Fatal("all-failed group posted a notice", err)
	}
}

func TestGenerationNoticeMosaic(t *testing.T) {
	colors := []color.RGBA{{R: 240, A: 255}, {G: 240, A: 255}, {B: 240, A: 255}}
	var images []image.Image
	for _, c := range colors {
		im := image.NewRGBA(image.Rect(0, 0, 16, 16))
		for y := range 16 {
			for x := range 16 {
				im.SetRGBA(x, y, c)
			}
		}
		images = append(images, im)
	}
	for _, strip := range []bool{false, true} {
		im := generationNoticeMosaic(images, strip)
		points := []image.Point{{40, 40}, {201, 40}, {201, 140}}
		if strip {
			points = []image.Point{{70, 50}, {270, 50}, {600, 50}}
		}
		for i, p := range points {
			if !reflect.DeepEqual(im.At(p.X, p.Y), colors[i]) {
				t.Errorf("tile %d (strip %t) did not retain its picture", i, strip)
			}
		}
	}
}

func TestGenerationNoticeURL(t *testing.T) {
	id := uuid()
	scheme := "canvepp-result"
	url := scheme + "://generation/" + id
	if got := generationNoticeFromArgs([]string{"--data-dir", "example", url}, scheme); got != id {
		t.Fatalf("valid URL returned %q", got)
	}
	for _, invalid := range []string{url + "/other", url + "?action=delete", url + "#target", "canvepp://generation/" + id, scheme + "://wrong/" + id, scheme + "://name@generation/" + id} {
		if got := generationNoticeFromArgs([]string{invalid}, scheme); got != "" {
			t.Fatalf("invalid URL returned %q", got)
		}
	}
}
