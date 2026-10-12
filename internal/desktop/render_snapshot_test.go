package desktop

import (
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	"gcode/internal/appmodel"
	"github.com/egoist/mygo/ui"
)

func renderSnapshotOwnershipFixture() appmodel.Snapshot {
	snapshot := performanceSnapshot("ownership", 24)
	snapshot.Projects = []appmodel.Project{{ID: "project", Name: "项目", Path: "/tmp/ownership"}}
	snapshot.Settings = appmodel.Settings{
		DefaultProviderID: "provider",
		DefaultSendMode:   appmodel.Correction,
		Theme:             "light",
		Providers: []appmodel.Provider{{
			ID:        "provider",
			Name:      "本地服务",
			Format:    appmodel.ChatCompletions,
			BaseURL:   "https://example.invalid",
			Model:     "fixture",
			HasAPIKey: true,
		}},
	}
	session := &snapshot.Sessions[0]
	session.ProjectID = "project"
	session.FullAccess = false
	session.Queue = []appmodel.QueuedMessage{{
		ID:    "queued",
		Text:  "排队消息",
		Mode:  appmodel.Queue,
		State: "queued",
	}}
	session.Permissions = []appmodel.Permission{{
		ID:        "permission",
		RunID:     "run",
		Tool:      "exec_command",
		Arguments: "{\"cmd\":\"pwd\"}",
		Options:   []appmodel.PermissionOption{{ID: "allow", Label: "允许"}},
	}}
	session.Questions = []appmodel.AskRequest{{
		ID:    "question",
		RunID: "run",
		Questions: []appmodel.Question{{
			ID:   "question-body",
			Text: "继续吗？",
			Options: []appmodel.QuestionOption{{
				Label:       "继续",
				Description: "继续执行 fixture",
			}},
		}},
	}}
	return snapshot
}

func TestRenderSnapshotCallerOwnershipSurvivesOpenAndUpdate(t *testing.T) {
	input := renderSnapshotOwnershipFixture()
	expectedOpen := cloneSnapshot(input)
	app := Open(nil, OpenOptions{Initial: input})
	tester := ui.NewTester(app.view, 900, 640)
	t.Cleanup(func() { _ = app.Close() })

	// Mutating the caller's initial snapshot after Open must not mutate App.
	input.Projects[0].Name = "caller project"
	input.Settings.Theme = "dark"
	input.Settings.Providers[0].Model = "caller model"
	input.Sessions[0].Messages[0].Text = "caller message"
	input.Sessions[0].Messages[0].Tools[0].Output = "caller output"
	input.Sessions[0].Queue[0].Text = "caller queue"
	input.Sessions[0].Permissions[0].Options[0].Label = "caller permission"
	input.Sessions[0].Questions[0].Questions[0].Options[0].Description = "caller question"
	tester.Frame()
	assertAppDataEqual(t, app, expectedOpen, "initial caller mutation")

	updated := renderSnapshotOwnershipFixture()
	expectedUpdate := cloneSnapshot(updated)
	app.Update(updated)
	updated.Projects[0].Path = "/tmp/caller-project"
	updated.Settings.Providers[0].Name = "caller provider"
	updated.Sessions[0].Messages[1].Text = "caller update"
	updated.Sessions[0].Messages[5].Tools[0].Arguments = "caller arguments"
	updated.Sessions[0].Queue[0].State = "caller state"
	updated.Sessions[0].Permissions[0].Options[0].ID = "caller option"
	updated.Sessions[0].Questions[0].Questions[0].Text = "caller question text"
	tester.Frame()
	assertAppDataEqual(t, app, expectedUpdate, "updated caller mutation")
}

func TestRenderSnapshotOldViewStaysStableAcrossGatedUpdate(t *testing.T) {
	old := renderSnapshotOwnershipFixture()
	old.Sessions[0].Messages = old.Sessions[0].Messages[:3]
	old.Sessions[0].Messages[0].Text = "旧历史正文"
	old.Sessions[0].Messages[0].Tools[0].Output = "旧工具输出"
	old.Sessions[0].Messages[2].Text = "旧最新正文"
	old.Sessions[0].ActiveRun = &appmodel.RunConfig{
		ProviderID: "old-provider",
		Model:      "old-model",
		Cwd:        "/tmp/old",
	}
	old.Sessions[0].Permissions[0].Arguments = "旧授权参数"
	old.Sessions[0].Questions[0].Questions[0].Text = "旧问题文本"
	oldViewWant := cloneSnapshot(old)

	app := Open(nil, OpenOptions{Initial: old})
	t.Cleanup(func() { _ = app.Close() })
	app.mu.RLock()
	oldView := snapshotForView(app.data)
	activeID := app.activeSessionID
	app.mu.RUnlock()

	next := cloneSnapshot(old)
	next.Sessions[0].Messages[0].Text = "新历史正文"
	next.Sessions[0].Messages[0].Tools[0].Output = "新工具输出"
	next.Sessions[0].Messages[2].Text = "新最新正文"
	next.Sessions[0].Messages = append(next.Sessions[0].Messages, appmodel.Message{
		ID:   "new-history",
		Role: "assistant",
		Text: "新增历史正文",
	})
	next.Sessions[0].ActiveRun = &appmodel.RunConfig{
		ProviderID: "new-provider",
		Model:      "new-model",
		Cwd:        "/tmp/new",
	}
	next.Sessions[0].Permissions[0].Arguments = "新授权参数"
	next.Sessions[0].Questions[0].Questions[0].Text = "新问题文本"

	updateGate := make(chan struct{})
	updateDone := make(chan struct{})
	go func() {
		<-updateGate
		app.Update(next)
		close(updateDone)
	}()
	close(updateGate)
	<-updateDone

	if !reflect.DeepEqual(oldView, oldViewWant) {
		t.Fatal("captured old view snapshot changed after the gated Update")
	}

	// Feed the captured snapshot through the actual conversation renderer. This
	// catches a stale or partially replaced view model beyond structural checks.
	tester := ui.NewTester(func(c *ui.Context) {
		app.conversation(c, newPalette(false), oldView, activeID, "", appmodel.Correction, "", false, true)
	}, 900, 640)
	if !tester.HasText("旧最新正文") {
		t.Fatalf("captured old view did not render its old body: %v", tester.Texts())
	}
	if tester.HasText("新最新正文") || tester.HasText("新增历史正文") {
		t.Fatalf("captured old view rendered content from the new Update: %v", tester.Texts())
	}
}

func TestRenderSnapshotStreamingUpdatesPreserveHistory(t *testing.T) {
	base := performanceSnapshot("stream-history", 220)
	app := Open(nil, OpenOptions{Initial: base})
	tester := ui.NewTester(app.view, 900, 640)
	t.Cleanup(func() { _ = app.Close() })

	history := cloneSnapshot(base).Sessions[0].Messages
	last := len(base.Sessions[0].Messages) - 1
	for i := 0; i < 32; i++ {
		next := cloneSnapshot(base)
		next.Sessions[0].Messages[last].Text = "流式片段 " + strconv.Itoa(i) + "\n\n```go\nreturn chunk\n```"
		app.Update(next)
		tester.Frame()

		app.mu.RLock()
		current := app.data.Sessions[0].Messages
		app.mu.RUnlock()
		if len(current) != len(history) {
			t.Fatalf("stream update changed history length: got=%d want=%d", len(current), len(history))
		}
		for index := 0; index < last; index++ {
			if !reflect.DeepEqual(current[index], history[index]) {
				t.Fatalf("stream update/frame changed historical message %d", index)
			}
		}
		if !tester.HasText("流式片段 " + strconv.Itoa(i)) {
			t.Fatalf("latest stream text is not visible at iteration %d", i)
		}
	}
}

func TestRenderSnapshotCapturedFrameAndLocalMetadataAreIsolated(t *testing.T) {
	input := renderSnapshotOwnershipFixture()
	expectedInput := cloneSnapshot(input)
	app := Open(nil, OpenOptions{Initial: input})
	tester := ui.NewTester(app.view, 900, 640)
	t.Cleanup(func() { _ = app.Close() })
	capturedTexts := append([]string(nil), tester.Texts()...)

	// These controls exercise the local UI mutations that currently write
	// a.data directly. The service is nil, so the request side effect is
	// rejected while the local checkbox state remains observable.
	if err := tester.Click("完全访问"); err != nil {
		t.Fatalf("toggle FullAccess through Tester: %v", err)
	}
	app.mu.RLock()
	fullAccessAfterClick := app.data.Sessions[0].FullAccess
	app.mu.RUnlock()
	if !fullAccessAfterClick {
		t.Fatal("Tester did not update local FullAccess state")
	}
	if err := tester.Click("设置"); err != nil {
		t.Fatalf("open settings through Tester: %v", err)
	}
	if err := tester.Click("深色"); err != nil {
		t.Fatalf("change theme through Tester: %v", err)
	}
	if err := tester.Click("队列"); err != nil {
		t.Fatalf("change send mode through Tester: %v", err)
	}

	if !reflect.DeepEqual(input, expectedInput) {
		t.Fatal("local FullAccess/settings edits leaked into the caller snapshot")
	}
	if !containsCapturedText(capturedTexts, "第 0023 条消息") {
		t.Fatalf("captured frame lost the old conversation body: %v", capturedTexts)
	}
	tester.Frame()
	app.mu.RLock()
	fullAccess := app.data.Sessions[0].FullAccess
	theme := app.data.Settings.Theme
	sendMode := app.data.Settings.DefaultSendMode
	app.mu.RUnlock()
	if !fullAccess || theme != "dark" || sendMode != appmodel.Queue {
		t.Fatalf("local metadata edits were not applied: full_access=%v theme=%q send_mode=%q", fullAccess, theme, sendMode)
	}
	if !reflect.DeepEqual(input.Sessions[0].Messages, expectedInput.Sessions[0].Messages) {
		t.Fatal("local metadata edits changed historical messages")
	}
}

func TestRenderSnapshotConcurrentUpdatesAndFrames(t *testing.T) {
	base := performanceSnapshot("concurrent", 320)
	app := Open(nil, OpenOptions{Initial: base})
	tester := ui.NewTester(app.view, 900, 640)
	t.Cleanup(func() { _ = app.Close() })

	type updateFrameRound struct {
		begin chan struct{}
		done  chan struct{}
	}
	rounds := make(chan updateFrameRound)
	var updates sync.WaitGroup
	updates.Add(1)
	go func() {
		defer updates.Done()
		next := cloneSnapshot(base)
		last := len(next.Sessions[0].Messages) - 1
		for i := 0; i < 128; i++ {
			next.Sessions[0].Messages[last].Text = "并发流式片段 " + strconv.Itoa(i)
			round := updateFrameRound{begin: make(chan struct{}), done: make(chan struct{})}
			rounds <- round
			<-round.begin
			app.Update(next)
			close(round.done)
		}
		close(rounds)
	}()
	for round := range rounds {
		close(round.begin)
		tester.Frame()
		<-round.done
	}
	updates.Wait()
	tester.Frame()

	app.mu.RLock()
	current := app.data
	app.mu.RUnlock()
	if len(current.Sessions) != 1 || len(current.Sessions[0].Messages) != len(base.Sessions[0].Messages) {
		t.Fatalf("concurrent update/frame corrupted snapshot shape: sessions=%d messages=%d", len(current.Sessions), len(current.Sessions[0].Messages))
	}
	if current.Sessions[0].Messages[0].Text != base.Sessions[0].Messages[0].Text {
		t.Fatal("concurrent update/frame changed historical message")
	}
}

func containsCapturedText(texts []string, want string) bool {
	for _, text := range texts {
		if strings.Contains(text, want) {
			return true
		}
	}
	return false
}

func assertAppDataEqual(t *testing.T, app *App, want appmodel.Snapshot, label string) {
	t.Helper()
	app.mu.RLock()
	got := app.data
	app.mu.RUnlock()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s changed App-owned snapshot", label)
	}
}
