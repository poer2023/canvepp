package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("CANVEPP_RPC_FIXTURE") == "1" && len(os.Args) == 3 && os.Args[1] == "app-server" && os.Args[2] == "--stdio" {
		runRPCFixture()
		return
	}
	os.Exit(m.Run())
}

func runRPCFixture() {
	scan := bufio.NewScanner(os.Stdin)
	const threadID = "fixture-thread"
	for scan.Scan() {
		var req struct {
			ID     *int            `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(scan.Bytes(), &req) != nil {
			os.Exit(2)
		}
		if req.ID == nil {
			continue
		}
		f, _ := os.OpenFile(os.Getenv("CANVEPP_RPC_LOG"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		f.WriteString(req.Method + "\n")
		f.Close()
		if path := os.Getenv("CANVEPP_RPC_MODEL_LOG"); path != "" &&
			(req.Method == "thread/start" || req.Method == "thread/resume" || req.Method == "turn/start") {
			var model modelChoice
			_ = json.Unmarshal(req.Params, &model)
			f, _ := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
			f.WriteString(req.Method + " " + model.Model + " " + model.Effort + "\n")
			f.Close()
		}
		var out any = map[string]any{}
		switch req.Method {
		case "account/login/start":
			var args struct{ Type string }
			_ = json.Unmarshal(req.Params, &args)
			if os.Getenv("CANVEPP_LOGIN_PORT_BUSY") == "1" && args.Type == "chatgpt" {
				os.Stdout.Write(append(encode(map[string]any{"id": *req.ID, "error": map[string]any{"message": "failed to start login server: port reserved"}}), '\n'))
				continue
			}
			if args.Type == "chatgptDeviceCode" {
				out = map[string]string{"type": args.Type, "verificationUrl": "https://auth.openai.com/codex/device", "userCode": "EXAMPLE", "loginId": "current"}
			} else {
				out = map[string]string{"type": "chatgpt", "authUrl": "https://auth.openai.com/authorize", "loginId": "current"}
			}
		case "account/read":
			out = map[string]any{"account": map[string]any{"type": "chatgpt", "planType": "pro", "email": "studio@example.com"}}
		case "modelProvider/capabilities/read":
			out = map[string]any{"imageGeneration": true}
		case "model/list":
			if os.Getenv("CANVEPP_RPC_NO_MODELS") != "1" {
				models := []any{}
				for _, option := range gptModelOptions {
					var efforts []map[string]string
					for _, effort := range effortsForModel(option.value) {
						efforts = append(efforts, map[string]string{"reasoningEffort": effort.value})
					}
					models = append(models, map[string]any{"model": option.value, "supportedReasoningEfforts": efforts})
				}
				out = map[string]any{"data": models}
			}
		case "thread/start", "thread/resume":
			var p struct{ Ephemeral bool }
			_ = json.Unmarshal(req.Params, &p)
			out = map[string]any{"thread": map[string]any{"id": threadID, "ephemeral": p.Ephemeral}, "modelProvider": "openai"}
		case "turn/start":
			if os.Getenv("CANVEPP_RPC_DISCONNECT") == "1" {
				os.Exit(0)
			}
			if gate := os.Getenv("CANVEPP_RPC_RELEASE"); gate != "" {
				deadline := time.Now().Add(6 * time.Second)
				for {
					if _, err := os.Stat(gate); err == nil {
						break
					}
					if time.Now().After(deadline) {
						os.Exit(3)
					}
					time.Sleep(10 * time.Millisecond)
				}
			}
			out = map[string]any{"turn": map[string]any{"id": "fixture-turn"}}
		case "thread/turns/list":
			out = map[string]any{"data": []any{map[string]any{"id": "fixture-turn", "status": "completed", "items": []any{map[string]any{"type": "imageGeneration", "result": base64.StdEncoding.EncodeToString(tinyPNG())}}}}, "nextCursor": nil}
		}
		os.Stdout.Write(append(encode(map[string]any{"id": *req.ID, "result": out}), '\n'))
		if req.Method == "account/login/start" {
			for _, id := range []string{"previous", "current"} {
				success := id == "previous" || os.Getenv("CANVEPP_LOGIN_DENIED") == ""
				os.Stdout.Write(append(encode(map[string]any{"method": "account/login/completed", "params": map[string]any{"loginId": id, "success": success}}), '\n'))
			}
		}
		if req.Method == "turn/start" && os.Getenv("CANVEPP_TEXT_FIXTURE") == "1" {
			os.Stdout.Write(append(encode(map[string]any{"method": "item/completed", "params": map[string]any{"threadId": "fixture-thread", "turnId": "fixture-turn", "item": map[string]any{"id": "answer", "type": "agentMessage", "text": "已导入两张图片，随后撤销了删除。", "phase": "final_answer"}}}), '\n'))
			os.Stdout.Write(append(encode(map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": "fixture-thread", "turn": map[string]any{"id": "fixture-turn", "status": "completed"}}}), '\n'))
		} else if req.Method == "turn/start" {
			image := NativeItem{Type: "imageGeneration", Result: base64.StdEncoding.EncodeToString(tinyPNG())}
			os.Stdout.Write(append(encode(map[string]any{"method": "item/completed", "params": map[string]any{"threadId": threadID, "turnId": "fixture-turn", "item": image}}), '\n'))
			os.Stdout.Write(append(encode(map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": threadID, "turn": NativeTurn{ID: "fixture-turn", Status: "completed"}}}), '\n'))
		}
	}
}
func fixtureCodex(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	logfile := filepath.Join(t.TempDir(), "calls")
	t.Setenv("CANVEPP_CODEX_BIN", exe)
	t.Setenv("CANVEPP_RPC_FIXTURE", "1")
	t.Setenv("CANVEPP_RPC_LOG", logfile)
	return logfile
}
func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func TestNativeAtMostOnceAndRecovery(t *testing.T) {
	logfile := fixtureCodex(t)
	modelLog := logfile + ".model"
	t.Setenv("CANVEPP_RPC_MODEL_LOG", modelLog)
	s := testStore(t)
	r := NativeRun{ID: uuid(), ProjectID: "p", NodeID: "n", Prompt: "不可变提示词", Params: defaults(), Status: "queued"}
	if err := s.QueueRuns([]NativeRun{r}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var result NativeRun
	executeNative(ctx, s, r, false, func(out NativeRun, _ *Asset) { result = out })
	if result.Status != "succeeded" || result.Result == "" || result.Model != "gpt-6.2-sol" || result.Effort != "medium" || !result.TemporaryThread {
		t.Fatal("generation receipt", result)
	}
	selected, err := os.ReadFile(modelLog)
	if err != nil || !strings.Contains(string(selected), "turn/start gpt-6.2-sol medium\n") {
		t.Fatal("native generation did not submit the requested model", string(selected), err)
	}
	executeNative(ctx, s, r, false, func(out NativeRun, _ *Asset) { result = out })
	calls, _ := os.ReadFile(logfile)
	if countLines(string(calls), "turn/start") != 1 {
		t.Fatal("duplicate generation", string(calls))
	}
	// Recovery queries can be repeated; the immutable result is imported once.
	executeNative(ctx, s, result, true, func(out NativeRun, _ *Asset) { result = out })
	assets, _ := s.Assets("p")
	if len(assets) != 1 {
		t.Fatal("duplicate recovery asset", len(assets))
	}
	calls, _ = os.ReadFile(logfile)
	if countLines(string(calls), "turn/start") != 1 {
		t.Fatal("recovery resubmitted")
	}
	if strings.Contains(string(calls), "thread/name/set") || strings.Contains(string(calls), "thread/turns/list") {
		t.Fatal("temporary generation must not name or poll a chat", string(calls))
	}
}

func TestTemporaryImageRecoveryAfterDisconnectedTurn(t *testing.T) {
	logfile := fixtureCodex(t)
	t.Setenv("CANVEPP_RPC_DISCONNECT", "1")
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	s := testStore(t)
	r := NativeRun{ID: uuid(), ProjectID: "p", NodeID: "n", Prompt: "一次", Params: defaults(), Status: "queued"}
	if err := s.QueueRuns([]NativeRun{r}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	receive := func(result NativeRun, _ *Asset) { r = result }
	executeNative(ctx, s, r, false, receive)
	if !r.TemporaryThread || r.Status != "unknown" {
		t.Fatal(r)
	}
	dir := filepath.Join(home, "generated_images", r.ThreadID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ig_result.png"), tinyPNG(), 0600); err != nil {
		t.Fatal(err)
	}
	executeNative(ctx, s, r, true, receive)
	if r.Status != "succeeded" || r.Result == "" {
		t.Fatal(r)
	}
	calls, _ := os.ReadFile(logfile)
	if countLines(string(calls), "thread/start") != 1 || countLines(string(calls), "turn/start") != 1 || strings.Contains(string(calls), "thread/turns/list") {
		t.Fatal("disk recovery made an unexpected Codex request", string(calls))
	}
}

func TestLegacyImageLookupStillRecoversWithoutResubmitting(t *testing.T) {
	logfile := fixtureCodex(t)
	s := testStore(t)
	r := NativeRun{ID: uuid(), ProjectID: "p", NodeID: "n", ThreadID: "fixture-thread", TurnID: "fixture-turn", Status: "unknown"}
	if err := s.SaveRun(r); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	executeNative(ctx, s, r, true, func(result NativeRun, _ *Asset) { r = result })
	if r.Status != "succeeded" || r.Result == "" {
		t.Fatal(r)
	}
	calls, _ := os.ReadFile(logfile)
	if countLines(string(calls), "thread/turns/list") != 1 || strings.Contains(string(calls), "turn/start") {
		t.Fatal("legacy recovery resubmitted", string(calls))
	}
}

func TestParallelImageBatchSubmitsWhileOtherTurnsArePending(t *testing.T) {
	logfile := fixtureCodex(t)
	gate := filepath.Join(t.TempDir(), "release")
	t.Setenv("CANVEPP_RPC_RELEASE", gate)
	s := testStore(t)
	runs := make([]NativeRun, 3)
	for i := range runs {
		runs[i] = NativeRun{ID: uuid(), ProjectID: "p", NodeID: uuid(), Prompt: "批次", Params: defaults(), Status: "queued"}
	}
	if err := s.QueueRuns(runs); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		dispatchGenerationBatch(runs, true, make(chan struct{}, 4), func(r NativeRun) {
			executeNative(ctx, s, r, false, func(NativeRun, *Asset) {})
		})
	}()
	defer func() { cancel(); <-done }()
	deadline := time.Now().Add(4 * time.Second)
	for {
		calls, _ := os.ReadFile(logfile)
		if countLines(string(calls), "turn/start") == 3 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("three turns were not submitted before the fixture released any results", string(calls))
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := os.WriteFile(gate, nil, 0600); err != nil {
		t.Fatal(err)
	}
	<-done
	recorded, err := s.NativeRuns()
	if err != nil || len(recorded) != 3 {
		t.Fatal("generation receipts", recorded, err)
	}
	for _, r := range recorded {
		if r.Status != "succeeded" || r.Result == "" || !r.TemporaryThread {
			t.Fatal("generation result", r)
		}
	}
}
func TestAmbiguousNativeReplyCannotResubmit(t *testing.T) {
	logfile := fixtureCodex(t)
	t.Setenv("CANVEPP_RPC_DISCONNECT", "1")
	s := testStore(t)
	r := NativeRun{ID: uuid(), ProjectID: "p", NodeID: "n", Prompt: "once", Params: defaults(), Status: "queued"}
	s.QueueRuns([]NativeRun{r})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var got NativeRun
	executeNative(ctx, s, r, false, func(out NativeRun, _ *Asset) { got = out })
	if got.Status != "unknown" {
		t.Fatal(got.Status, got.Error)
	}
	executeNative(ctx, s, r, false, func(out NativeRun, _ *Asset) { got = out })
	b, _ := os.ReadFile(logfile)
	if countLines(string(b), "turn/start") != 1 {
		t.Fatal("unknown result generated twice")
	}
}
func TestQueuedCancellationAndConcurrentClaim(t *testing.T) {
	s := testStore(t)
	r := NativeRun{ID: uuid(), Status: "queued"}
	s.QueueRuns([]NativeRun{r})
	s.CancelRun(r.ID)
	if _, ok, err := s.claimRun(r.ID); ok || err != nil {
		t.Fatal("cancelled task claimed")
	}
	r.ID = uuid()
	s.QueueRuns([]NativeRun{r})
	var wg sync.WaitGroup
	wins := make(chan bool, 12)
	for range 12 {
		wg.Add(1)
		go func() { defer wg.Done(); _, ok, _ := s.claimRun(r.ID); wins <- ok }()
	}
	wg.Wait()
	close(wins)
	count := 0
	for ok := range wins {
		if ok {
			count++
		}
	}
	if count != 1 {
		t.Fatal("claim count", count)
	}
}
func TestLateResultsSurviveDeleteUndoAndRestart(t *testing.T) {
	e := fixture()
	e.Selection["0"] = true
	e.Delete()
	asset := Asset{ID: "late", NodeID: "0", ProjectID: e.Project.ID}
	e.Assets[asset.ID] = asset
	r := NativeRun{ProjectID: e.Project.ID, NodeID: "0", Result: asset.ID, Status: "succeeded"}
	if !reconcileResults(e, []NativeRun{r}) {
		t.Fatal("result not reconciled to trash")
	}
	e.Undo()
	if e.Node("0").Asset != "late" {
		t.Fatal("undo dropped late image")
	}
	e.Node("1").Asset = "already-adopted"
	r.NodeID = "1"
	reconcileResults(e, []NativeRun{r})
	if e.Node("1").Asset != "already-adopted" {
		t.Fatal("replaced selected version")
	}
	e.Node("2").Asset = ""
	r.NodeID = "2"
	if !reconcileResults(e, []NativeRun{r}) || e.Node("2").Asset != "late" {
		t.Fatal("restart missing result")
	}
}
func TestBackupRetainsNativeReceipts(t *testing.T) {
	s := testStore(t)
	e := fixture()
	s.Save(e.Project, nil)
	r := NativeRun{ID: uuid(), ProjectID: e.Project.ID, ThreadID: "original", TurnID: "round", Status: "unknown"}
	s.QueueRuns([]NativeRun{r})
	dir := filepath.Join(t.TempDir(), "backup")
	if err := s.Backup(dir); err != nil {
		t.Fatal(err)
	}
	dst := testStore(t)
	if _, err := dst.ImportLegacy(dir); err != nil {
		t.Fatal(err)
	}
	restored, err := dst.NativeRun(r.ID)
	if err != nil || restored.ThreadID != "original" || restored.Status != "unknown" {
		t.Fatal("backup lost receipt", err, restored)
	}
}
func TestSaveQueueDoesNotHideEarlierFailure(t *testing.T) {
	s := testStore(t)
	a := fixture()
	a.Project.Revision = 9
	s.Save(a.Project, nil)
	a.Project.Revision = 1
	q := newSaveQueue(s, func(string, int, error) {})
	q.Put(a)
	if q.Flush() == nil {
		t.Fatal("stale save passed")
	}
	b := fixture()
	b.Project.ID = "other"
	q.Put(b)
	if q.Flush() == nil {
		t.Fatal("later success hid earlier failure")
	}
	a.Project.Revision = 10
	q.Put(a)
	if err := q.Flush(); err != nil {
		t.Fatal(err)
	}
}
func countLines(s, word string) int {
	count := 0
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		if sc.Text() == word {
			count++
		}
	}
	return count
}

func TestCrossCanvasCopyPersistsReferences(t *testing.T) {
	s := testStore(t)
	src := fixture()
	src.Selection["0"] = true
	asset, err := s.ImportImage(tinyPNG(), src.Project.ID, "0", "alpha.png", "import", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	src.Assets[asset.ID] = asset
	src.Node("0").Asset = asset.ID
	dest := newEditor(Project{ID: "other"}, nil, nil)
	mapped, err := s.CloneAssets([]Asset{asset}, dest.Project.ID)
	if err != nil {
		t.Fatal(err)
	}
	clone := mapped[asset.ID]
	dest.Assets[clone.ID] = clone
	clip := src.Copy()
	clip.ProjectID = dest.Project.ID
	clip.Nodes[0].Asset = clone.ID
	if err = dest.Paste(clip, Point{12, 13}); err != nil {
		t.Fatal(err)
	}
	if err = s.Save(dest.Project, nil); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Assets(dest.Project.ID)
	if len(got) != 1 || got[0].SHA != asset.SHA || dest.Project.Graph.Nodes[0].Config.Note != "保留备注" {
		t.Fatal("cross canvas lost content")
	}
}
