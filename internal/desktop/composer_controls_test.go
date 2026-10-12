package desktop

import (
	"context"
	"testing"
	"time"

	"gcode/internal/appmodel"
	"github.com/egoist/mygo/ui"
)

type composerControlsProbe struct {
	desktopServiceStub
	providers chan [2]string
	stops     chan string
}

func (p *composerControlsProbe) SetProvider(_ context.Context, sessionID, providerID string) error {
	p.providers <- [2]string{sessionID, providerID}
	return nil
}

func (p *composerControlsProbe) Stop(_ context.Context, sessionID string) error {
	p.stops <- sessionID
	return nil
}

func TestComposerKeyboardModelChoiceKeepsRunningConfigAndSeparateActions(t *testing.T) {
	initial := SampleSnapshot()
	initial.Sessions[0].ActiveRun = &appmodel.RunConfig{Model: "captured-running-model", ProviderID: "provider-openai"}
	providerCalls := make(chan [2]string, 1)
	stopCalls := make(chan string, 1)
	type submission struct {
		sessionID, text string
		mode            appmodel.SendMode
	}
	sendCalls := make(chan submission, 1)
	service := &composerControlsProbe{providers: providerCalls, stops: stopCalls}
	service.sendFn = func(_ context.Context, sessionID, text string, mode appmodel.SendMode) error {
		sendCalls <- submission{sessionID: sessionID, text: text, mode: mode}
		return nil
	}
	app := Open(service, OpenOptions{Initial: initial})
	t.Cleanup(func() { _ = app.Close() })
	tester := ui.NewTester(app.view, 1024, 700)
	if err := tester.Click("下一任务模型"); err != nil {
		t.Fatalf("open model popup: %v", err)
	}
	if !tester.HasText("本地评审") {
		t.Fatalf("model popup did not expose choices: %v", tester.Texts())
	}
	tester.Key(0, ui.KeyEnd)
	tester.Key(0, ui.KeyEnter)
	select {
	case got := <-providerCalls:
		if want := [2]string{initial.Sessions[0].ID, "provider-local"}; got != want {
			t.Fatalf("model change = %v, want %v", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("keyboard selection did not reach the service")
	}
	updated := cloneSnapshot(initial)
	updated.Sessions[0].ProviderID = "provider-local"
	app.Update(updated)
	tester.Frame()
	if !tester.HasText("本地评审") {
		t.Fatalf("selected next model missing: %v", tester.Texts())
	}
	app.mu.RLock()
	captured := app.data.Sessions[0].ActiveRun
	app.mu.RUnlock()
	if captured.Model != "captured-running-model" || captured.ProviderID != "provider-openai" {
		t.Fatalf("next model choice changed current run: %#v", captured)
	}
	if err := tester.Click("队列"); err != nil {
		t.Fatalf("choose queue mode: %v", err)
	}
	if err := tester.Click("任务输入"); err != nil {
		t.Fatalf("focus task input: %v", err)
	}
	tester.Type("保留当前任务，排队验证中文输入 🙂")
	if err := tester.Click("发送"); err != nil {
		t.Fatalf("submit queued task: %v", err)
	}
	select {
	case got := <-sendCalls:
		if got.sessionID != initial.Sessions[0].ID || got.text != "保留当前任务，排队验证中文输入 🙂" || got.mode != appmodel.Queue {
			t.Fatalf("submission = %#v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("queue submission did not reach the service")
	}
	select {
	case sessionID := <-stopCalls:
		t.Fatalf("queue submission also stopped session %q", sessionID)
	default:
	}
	if err := tester.Click("停止"); err != nil {
		t.Fatalf("explicit stop action missing: %v", err)
	}
	select {
	case sessionID := <-stopCalls:
		if sessionID != initial.Sessions[0].ID {
			t.Fatalf("stopped session = %q", sessionID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("explicit stop did not reach the service")
	}
}

func TestComposerSelectsProviderByIDWithDuplicateDisplayLabels(t *testing.T) {
	initial := SampleSnapshot()
	for i := range initial.Settings.Providers {
		initial.Settings.Providers[i].Name = "同名服务"
		initial.Settings.Providers[i].Model = "same-model"
	}
	providerCalls := make(chan [2]string, 1)
	service := &composerControlsProbe{providers: providerCalls}
	app := Open(service, OpenOptions{Initial: initial})
	t.Cleanup(func() { _ = app.Close() })
	tester := ui.NewTester(app.view, 1024, 700)
	if err := tester.Click("下一任务模型"); err != nil {
		t.Fatalf("open duplicate-name model popup: %v", err)
	}
	tester.Key(0, ui.KeyEnd)
	tester.Key(0, ui.KeyEnter)
	select {
	case got := <-providerCalls:
		if want := [2]string{initial.Sessions[0].ID, "provider-local"}; got != want {
			t.Fatalf("duplicate-name model change = %v, want %v", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second provider with duplicate label was not selected")
	}
}
