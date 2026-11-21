package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestModelSelectionsPersistAndAreSnapshottedForGeneration(t *testing.T) {
	s := testStore(t)
	a, err := newDesktop(s)
	if err != nil {
		t.Fatal(err)
	}
	if a.models.Image != (modelChoice{"gpt-6.2-sol", "medium"}) || a.models.Text != (modelChoice{"gpt-6.1-terra", "medium"}) {
		t.Fatal("unexpected defaults", a.models)
	}
	tt := ui.NewTester(a.settingsView, 1000, 1100)
	choose := func(label, option string) {
		t.Helper()
		for _, name := range []string{label, option} {
			if err := tt.Click(name); err != nil {
				t.Fatal(err)
			}
		}
		tt.Advance(paneDuration + 20*time.Millisecond)
	}
	choose("选择生图模型", "GPT-6.2 Astra")
	choose("选择生图推理强度", "高")
	choose("选择文本模型", "GPT-6 Luna")
	choose("选择文本推理强度", "低")

	reopened, err := newDesktop(s)
	if err != nil {
		t.Fatal(err)
	}
	image, text := modelChoice{"gpt-6.2-astra", "high"}, modelChoice{"gpt-6-luna", "low"}
	if reopened.models.Image != image || reopened.models.Text != text {
		t.Fatal("reopening lost a choice from the settings", reopened.models)
	}
	reopened.createCanvas()
	reopened.addNode(Point{40, 40})
	reopened.prompt = "记录当前选择"
	runs := reopened.prepareCanvasGeneration()
	reopened.saveModelChoice(true, modelChoice{"gpt-6.2-sol", "medium"})
	if len(runs) != 1 || runs[0].Model != image.Model || runs[0].Effort != image.Effort || reopened.models.Text != text {
		t.Fatal("changing the image model affected another saved choice or a queued run", runs, reopened.models)
	}
}
