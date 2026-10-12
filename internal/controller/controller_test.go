package controller

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"gcode/internal/appmodel"
	"gcode/internal/runtime"
	"gcode/internal/worktrees"
)

type fakeRunPlan func(context.Context, runtime.Input, runtime.Callbacks) (runtime.Result, error)

type fakeRunner struct {
	mu        sync.Mutex
	plans     []fakeRunPlan
	calls     []runtime.Input
	started   chan runtime.Input
	active    int
	maxActive int
}

func (runner *fakeRunner) Run(ctx context.Context, input runtime.Input, callbacks runtime.Callbacks) (runtime.Result, error) {
	runner.mu.Lock()
	index := len(runner.calls)
	runner.calls = append(runner.calls, input)
	runner.active++
	if runner.active > runner.maxActive {
		runner.maxActive = runner.active
	}
	var plan fakeRunPlan
	if index < len(runner.plans) {
		plan = runner.plans[index]
	}
	runner.mu.Unlock()
	if runner.started != nil {
		runner.started <- input
	}
	defer func() {
		runner.mu.Lock()
		runner.active--
		runner.mu.Unlock()
	}()
	if plan != nil {
		return plan(ctx, input, callbacks)
	}
	return runtime.Result{FinalAnswer: input.Prompt}, nil
}

func (runner *fakeRunner) callCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return len(runner.calls)
}

func (runner *fakeRunner) maxConcurrency() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.maxActive
}

func newControllerTest(t *testing.T, runner *fakeRunner, dataDir string) (*Controller, string) {
	t.Helper()
	projectPath := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(projectPath, 0o700); err != nil {
		t.Fatal(err)
	}
	project := appmodel.Project{ID: "project", Name: "project", Path: projectPath}
	settings := appmodel.Settings{
		Providers:         []appmodel.Provider{{ID: "provider", Name: "test-provider", Model: "test-model"}},
		DefaultProviderID: "provider",
	}
	instance, err := New(Options{
		DataDir:  dataDir,
		Runner:   runner,
		Projects: []appmodel.Project{project},
		Settings: settings,
	})
	if err != nil {
		t.Fatal(err)
	}
	sessionID, err := instance.NewSession(appmodel.NewSessionOptions{ProjectID: project.ID})
	if err != nil {
		_ = instance.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = instance.Close() })
	return instance, sessionID
}

func waitForSnapshot(t *testing.T, instance *Controller, predicate func(appmodel.Snapshot) bool) appmodel.Snapshot {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		snapshot := instance.Snapshot()
		if predicate(snapshot) {
			return snapshot
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("condition was not reached; last snapshot = %#v", instance.Snapshot())
	return appmodel.Snapshot{}
}

func sessionFromSnapshot(t *testing.T, snapshot appmodel.Snapshot, sessionID string) appmodel.Session {
	t.Helper()
	for _, session := range snapshot.Sessions {
		if session.ID == sessionID {
			return session
		}
	}
	t.Fatalf("session %q not found in snapshot %#v", sessionID, snapshot)
	return appmodel.Session{}
}

func waitForCalls(t *testing.T, runner *fakeRunner, count int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if runner.callCount() >= count {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("wanted at least %d calls, got %d", count, runner.callCount())
}

func TestCorrectionPreservesOrderAndNeverRunsInParallel(t *testing.T) {
	firstReleased := make(chan struct{})
	firstCanceled := make(chan struct{})
	runner := &fakeRunner{
		started: make(chan runtime.Input, 8),
		plans: []fakeRunPlan{
			func(ctx context.Context, _ runtime.Input, _ runtime.Callbacks) (runtime.Result, error) {
				select {
				case <-firstReleased:
					return runtime.Result{FinalAnswer: "first"}, nil
				case <-ctx.Done():
					close(firstCanceled)
					<-firstReleased
					return runtime.Result{}, ctx.Err()
				}
			},
			func(_ context.Context, input runtime.Input, _ runtime.Callbacks) (runtime.Result, error) {
				return runtime.Result{FinalAnswer: input.Prompt}, nil
			},
			func(_ context.Context, input runtime.Input, _ runtime.Callbacks) (runtime.Result, error) {
				return runtime.Result{FinalAnswer: input.Prompt}, nil
			},
			func(_ context.Context, input runtime.Input, _ runtime.Callbacks) (runtime.Result, error) {
				return runtime.Result{FinalAnswer: input.Prompt}, nil
			},
		},
	}
	instance, sessionID := newControllerTest(t, runner, t.TempDir())
	if err := instance.Send(sessionID, "first", appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	select {
	case <-runner.started:
	case <-time.After(3 * time.Second):
		t.Fatal("first run did not start")
	}
	if err := instance.Send(sessionID, "queued", appmodel.Queue); err != nil {
		t.Fatal(err)
	}
	if err := instance.Send(sessionID, "fix-one", appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	if err := instance.Send(sessionID, "fix-two", appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	select {
	case <-firstCanceled:
	case <-time.After(3 * time.Second):
		t.Fatal("first run did not observe cancellation after all corrections were queued")
	}
	close(firstReleased)
	waitForCalls(t, runner, 4)
	snapshot := waitForSnapshot(t, instance, func(snapshot appmodel.Snapshot) bool {
		session := sessionFromSnapshot(t, snapshot, sessionID)
		return session.State == appmodel.Idle && len(session.Queue) == 0
	})
	if runner.maxConcurrency() != 1 {
		t.Fatalf("max concurrent runs = %d, want 1", runner.maxConcurrency())
	}
	runner.mu.Lock()
	gotPrompts := make([]string, 0, len(runner.calls))
	for _, call := range runner.calls {
		gotPrompts = append(gotPrompts, call.Prompt)
	}
	runner.mu.Unlock()
	wantPrompts := []string{"first", "fix-one", "fix-two", "queued"}
	if len(gotPrompts) != len(wantPrompts) {
		t.Fatalf("prompts = %#v, want %#v", gotPrompts, wantPrompts)
	}
	for index := range wantPrompts {
		if gotPrompts[index] != wantPrompts[index] {
			t.Fatalf("prompt[%d] = %q, want %q; all = %#v", index, gotPrompts[index], wantPrompts[index], gotPrompts)
		}
	}
	model := sessionFromSnapshot(t, snapshot, sessionID)
	var states []string
	for _, message := range model.Messages {
		if message.Role == "user" {
			states = append(states, message.State)
		}
	}
	if len(states) != 4 || states[0] != messageStateInterrupted || states[1] != messageStateCompleted || states[2] != messageStateCompleted || states[3] != messageStateCompleted {
		t.Fatalf("user message states = %#v", states)
	}
}

func TestCorrectionCancelsActiveRunBeforeEmission(t *testing.T) {
	runRelease := make(chan struct{})
	runReturned := make(chan struct{})
	var runReleaseOnce sync.Once
	releaseRun := func() { runReleaseOnce.Do(func() { close(runRelease) }) }
	t.Cleanup(releaseRun)
	runner := &fakeRunner{
		started: make(chan runtime.Input, 4),
		plans: []fakeRunPlan{
			func(ctx context.Context, input runtime.Input, _ runtime.Callbacks) (runtime.Result, error) {
				<-runRelease
				close(runReturned)
				if ctx.Err() == nil {
					return runtime.Result{}, errors.New("run returned before cancellation")
				}
				return runtime.Result{FinalAnswer: input.Prompt}, nil
			},
			func(_ context.Context, input runtime.Input, _ runtime.Callbacks) (runtime.Result, error) {
				return runtime.Result{FinalAnswer: input.Prompt}, nil
			},
		},
	}
	instance, sessionID := newControllerTest(t, runner, t.TempDir())
	if err := instance.Send(sessionID, "first", appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	select {
	case <-runner.started:
	case <-time.After(3 * time.Second):
		t.Fatal("first run did not start")
	}

	emitEntered := make(chan struct{})
	releaseEmit := make(chan struct{})
	var emitReleaseOnce sync.Once
	releaseFirstEmit := func() { emitReleaseOnce.Do(func() { close(releaseEmit) }) }
	t.Cleanup(releaseFirstEmit)
	var once sync.Once
	instance.Subscribe(func(snapshot appmodel.Snapshot) {
		model := sessionFromSnapshot(t, snapshot, sessionID)
		if model.State != appmodel.Cancelling {
			return
		}
		once.Do(func() {
			close(emitEntered)
			<-releaseEmit
		})
	})

	sendDone := make(chan error, 1)
	go func() {
		sendDone <- instance.Send(sessionID, "replacement", appmodel.Correction)
	}()
	select {
	case <-emitEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("correction send did not enter subscriber")
	}
	releaseRun()
	select {
	case <-runReturned:
	case <-time.After(3 * time.Second):
		t.Fatal("active run did not return while correction send was blocked")
	}

	waitForCalls(t, runner, 2)
	releaseFirstEmit()
	select {
	case err := <-sendDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("correction send did not finish")
	}
	snapshot := waitForSnapshot(t, instance, func(snapshot appmodel.Snapshot) bool {
		model := sessionFromSnapshot(t, snapshot, sessionID)
		return model.State == appmodel.Idle && len(model.Queue) == 0
	})
	model := sessionFromSnapshot(t, snapshot, sessionID)
	if model.PauseReason != "" || runner.callCount() != 2 {
		t.Fatalf("correction result = %#v, calls = %d", model, runner.callCount())
	}
}

func TestStopCancelsActiveRunBeforeEmission(t *testing.T) {
	runRelease := make(chan struct{})
	runReturned := make(chan struct{})
	runSawCancellation := make(chan struct{})
	var runReleaseOnce sync.Once
	var runCancellationOnce sync.Once
	releaseRun := func() { runReleaseOnce.Do(func() { close(runRelease) }) }
	t.Cleanup(releaseRun)
	runner := &fakeRunner{
		started: make(chan runtime.Input, 4),
		plans: []fakeRunPlan{
			func(ctx context.Context, input runtime.Input, _ runtime.Callbacks) (runtime.Result, error) {
				<-runRelease
				close(runReturned)
				if ctx.Err() == nil {
					return runtime.Result{}, errors.New("run returned before cancellation")
				}
				runCancellationOnce.Do(func() { close(runSawCancellation) })
				return runtime.Result{FinalAnswer: input.Prompt}, nil
			},
		},
	}
	instance, sessionID := newControllerTest(t, runner, t.TempDir())
	if err := instance.Send(sessionID, "running", appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	select {
	case <-runner.started:
	case <-time.After(3 * time.Second):
		t.Fatal("run did not start")
	}

	emitEntered := make(chan struct{})
	releaseEmit := make(chan struct{})
	var emitReleaseOnce sync.Once
	releaseFirstEmit := func() { emitReleaseOnce.Do(func() { close(releaseEmit) }) }
	t.Cleanup(releaseFirstEmit)
	var once sync.Once
	instance.Subscribe(func(snapshot appmodel.Snapshot) {
		model := sessionFromSnapshot(t, snapshot, sessionID)
		if model.State != appmodel.Cancelling {
			return
		}
		once.Do(func() {
			close(emitEntered)
			<-releaseEmit
		})
	})

	stopDone := make(chan error, 1)
	go func() {
		stopDone <- instance.Stop(sessionID)
	}()
	select {
	case <-emitEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("stop did not enter subscriber")
	}
	releaseRun()
	select {
	case <-runReturned:
	case <-time.After(3 * time.Second):
		t.Fatal("active run did not return while stop was blocked")
	}
	select {
	case <-runSawCancellation:
	case <-time.After(3 * time.Second):
		t.Fatal("active run returned before observing cancellation")
	}
	waitForSnapshot(t, instance, func(snapshot appmodel.Snapshot) bool {
		return sessionFromSnapshot(t, snapshot, sessionID).State == appmodel.Interrupted
	})
	releaseFirstEmit()
	select {
	case err := <-stopDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("stop did not finish")
	}
}

func TestStopPersistenceFailureDoesNotCancelActiveRun(t *testing.T) {
	cancelObserved := make(chan struct{})
	var cancelOnce sync.Once
	runner := &fakeRunner{
		started: make(chan runtime.Input, 2),
		plans: []fakeRunPlan{
			func(ctx context.Context, _ runtime.Input, _ runtime.Callbacks) (runtime.Result, error) {
				<-ctx.Done()
				cancelOnce.Do(func() { close(cancelObserved) })
				return runtime.Result{}, ctx.Err()
			},
		},
	}
	instance, sessionID := newControllerTest(t, runner, t.TempDir())
	if err := instance.Send(sessionID, "running", appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	select {
	case <-runner.started:
	case <-time.After(3 * time.Second):
		t.Fatal("run did not start")
	}
	originalStatePath := instance.statePath
	instance.statePath = t.TempDir()
	if err := instance.Stop(sessionID); err == nil {
		t.Fatal("Stop succeeded while controller state path was a directory")
	}
	select {
	case <-cancelObserved:
		t.Fatal("active run was cancelled after stop persistence failed")
	default:
	}
	model := sessionFromSnapshot(t, instance.Snapshot(), sessionID)
	if model.State != appmodel.Running || model.QueuePaused || model.ActiveRun == nil {
		t.Fatalf("failed stop changed active session: %#v", model)
	}

	instance.statePath = originalStatePath
	if err := instance.Stop(sessionID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-cancelObserved:
	case <-time.After(3 * time.Second):
		t.Fatal("active run was not cancelled after stop persistence succeeded")
	}
	waitForSnapshot(t, instance, func(snapshot appmodel.Snapshot) bool {
		return sessionFromSnapshot(t, snapshot, sessionID).State == appmodel.Interrupted
	})
}

func TestCloseCancelsActiveRunAfterPersistenceFailure(t *testing.T) {
	cancelObserved := make(chan struct{})
	var cancelOnce sync.Once
	runner := &fakeRunner{
		started: make(chan runtime.Input, 2),
		plans: []fakeRunPlan{
			func(ctx context.Context, _ runtime.Input, _ runtime.Callbacks) (runtime.Result, error) {
				<-ctx.Done()
				cancelOnce.Do(func() { close(cancelObserved) })
				return runtime.Result{}, ctx.Err()
			},
		},
	}
	instance, sessionID := newControllerTest(t, runner, t.TempDir())
	if err := instance.Send(sessionID, "running", appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	select {
	case <-runner.started:
	case <-time.After(3 * time.Second):
		t.Fatal("run did not start")
	}
	instance.statePath = t.TempDir()
	if err := instance.Close(); err == nil {
		t.Fatal("Close succeeded while controller state path was a directory")
	}
	select {
	case <-cancelObserved:
	case <-time.After(3 * time.Second):
		t.Fatal("Close left the active run running after persistence failure")
	}
}

func TestQueueRunsFIFOAfterNormalCompletion(t *testing.T) {
	release := make(chan struct{})
	runner := &fakeRunner{
		started: make(chan runtime.Input, 8),
		plans: []fakeRunPlan{
			func(_ context.Context, _ runtime.Input, _ runtime.Callbacks) (runtime.Result, error) {
				<-release
				return runtime.Result{}, nil
			},
		},
	}
	instance, sessionID := newControllerTest(t, runner, t.TempDir())
	if err := instance.Send(sessionID, "first", appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	select {
	case <-runner.started:
	case <-time.After(3 * time.Second):
		t.Fatal("first run did not start")
	}
	for _, prompt := range []string{"one", "two", "three"} {
		if err := instance.Send(sessionID, prompt, appmodel.Queue); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := instance.Snapshot()
	model := sessionFromSnapshot(t, snapshot, sessionID)
	if len(model.Queue) != 3 || model.Queue[0].State != messageStateQueued || model.Queue[1].State != messageStateQueued || model.Queue[2].State != messageStateQueued {
		t.Fatalf("queued messages = %#v", model.Queue)
	}
	close(release)
	waitForCalls(t, runner, 4)
	waitForSnapshot(t, instance, func(snapshot appmodel.Snapshot) bool {
		model := sessionFromSnapshot(t, snapshot, sessionID)
		return model.State == appmodel.Idle && len(model.Queue) == 0
	})
	runner.mu.Lock()
	defer runner.mu.Unlock()
	for index, want := range []string{"first", "one", "two", "three"} {
		if runner.calls[index].Prompt != want {
			t.Fatalf("call[%d] prompt = %q, want %q", index, runner.calls[index].Prompt, want)
		}
	}
}

func TestStopAndFailurePauseQueue(t *testing.T) {
	runner := &fakeRunner{
		started: make(chan runtime.Input, 4),
		plans: []fakeRunPlan{
			func(ctx context.Context, _ runtime.Input, _ runtime.Callbacks) (runtime.Result, error) {
				<-ctx.Done()
				return runtime.Result{}, ctx.Err()
			},
		},
	}
	instance, sessionID := newControllerTest(t, runner, t.TempDir())
	if err := instance.Send(sessionID, "long", appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	select {
	case <-runner.started:
	case <-time.After(3 * time.Second):
		t.Fatal("run did not start")
	}
	if err := instance.Send(sessionID, "later", appmodel.Queue); err != nil {
		t.Fatal(err)
	}
	if err := instance.Stop(sessionID); err != nil {
		t.Fatal(err)
	}
	snapshot := waitForSnapshot(t, instance, func(snapshot appmodel.Snapshot) bool {
		model := sessionFromSnapshot(t, snapshot, sessionID)
		return model.State == appmodel.Interrupted && model.QueuePaused
	})
	model := sessionFromSnapshot(t, snapshot, sessionID)
	if len(model.Queue) != 1 || model.Queue[0].Text != "later" || model.PauseReason != pauseReasonStop {
		t.Fatalf("stopped model = %#v", model)
	}

	failing := &fakeRunner{
		started: make(chan runtime.Input, 2),
		plans: []fakeRunPlan{
			func(context.Context, runtime.Input, runtime.Callbacks) (runtime.Result, error) {
				return runtime.Result{}, errors.New("provider failed")
			},
		},
	}
	failedController, failedSessionID := newControllerTest(t, failing, t.TempDir())
	if err := failedController.Send(failedSessionID, "fail", appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	failedSnapshot := waitForSnapshot(t, failedController, func(snapshot appmodel.Snapshot) bool {
		return sessionFromSnapshot(t, snapshot, failedSessionID).State == appmodel.Failed
	})
	failedModel := sessionFromSnapshot(t, failedSnapshot, failedSessionID)
	if !failedModel.QueuePaused || failedModel.PauseReason != pauseReasonFailure || failedModel.Error == "" {
		t.Fatalf("failed model = %#v", failedModel)
	}
}

func TestCancellationFailureDoesNotStartReplacement(t *testing.T) {
	runner := &fakeRunner{
		started: make(chan runtime.Input, 4),
		plans: []fakeRunPlan{
			func(ctx context.Context, _ runtime.Input, _ runtime.Callbacks) (runtime.Result, error) {
				<-ctx.Done()
				return runtime.Result{}, errors.New("cannot cancel provider request")
			},
		},
	}
	instance, sessionID := newControllerTest(t, runner, t.TempDir())
	if err := instance.Send(sessionID, "running", appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	select {
	case <-runner.started:
	case <-time.After(3 * time.Second):
		t.Fatal("run did not start")
	}
	if err := instance.Send(sessionID, "queued", appmodel.Queue); err != nil {
		t.Fatal(err)
	}
	if err := instance.Send(sessionID, "replacement", appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	snapshot := waitForSnapshot(t, instance, func(snapshot appmodel.Snapshot) bool {
		return sessionFromSnapshot(t, snapshot, sessionID).State == appmodel.Failed
	})
	model := sessionFromSnapshot(t, snapshot, sessionID)
	if !model.QueuePaused || model.PauseReason != pauseReasonCancelFailure || len(model.Queue) != 2 {
		t.Fatalf("cancellation failure model = %#v", model)
	}
	if runner.callCount() != 1 {
		t.Fatalf("replacement started after cancellation failure; calls = %d", runner.callCount())
	}
}

func TestOldPermissionAndAskRepliesAreRejectedAfterCorrection(t *testing.T) {
	tests := []struct {
		name  string
		plan  fakeRunPlan
		reply func(*Controller, string) error
	}{
		{
			name: "permission",
			plan: func(ctx context.Context, _ runtime.Input, callbacks runtime.Callbacks) (runtime.Result, error) {
				_, err := callbacks.OnPermissionRequest(ctx, runtime.PermissionRequest{
					ID:      "permission-old",
					Tool:    "exec",
					Options: []runtime.PermissionOption{{ID: "allow", Label: "允许"}},
				})
				return runtime.Result{}, err
			},
			reply: func(instance *Controller, sessionID string) error {
				return instance.ReplyPermission(sessionID, "permission-old", "allow")
			},
		},
		{
			name: "ask",
			plan: func(ctx context.Context, _ runtime.Input, callbacks runtime.Callbacks) (runtime.Result, error) {
				_, err := callbacks.OnAskUser(ctx, runtime.AskRequest{
					ID:        "ask-old",
					Questions: []runtime.Question{{Header: "确认", Text: "继续吗？"}},
				})
				return runtime.Result{}, err
			},
			reply: func(instance *Controller, sessionID string) error {
				return instance.ReplyAsk(sessionID, "ask-old", []string{"是"})
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &fakeRunner{
				started: make(chan runtime.Input, 4),
				plans: []fakeRunPlan{
					test.plan,
					func(_ context.Context, input runtime.Input, _ runtime.Callbacks) (runtime.Result, error) {
						return runtime.Result{FinalAnswer: input.Prompt}, nil
					},
				},
			}
			instance, sessionID := newControllerTest(t, runner, t.TempDir())
			if err := instance.Send(sessionID, "需要交互", appmodel.Correction); err != nil {
				t.Fatal(err)
			}
			select {
			case <-runner.started:
			case <-time.After(3 * time.Second):
				t.Fatal("interactive run did not start")
			}
			waitForSnapshot(t, instance, func(snapshot appmodel.Snapshot) bool {
				model := sessionFromSnapshot(t, snapshot, sessionID)
				return model.State == appmodel.WaitingPermission || model.State == appmodel.WaitingQuestion
			})
			if err := instance.Send(sessionID, "纠正", appmodel.Correction); err != nil {
				t.Fatal(err)
			}
			waitForCalls(t, runner, 2)
			if err := test.reply(instance, sessionID); err == nil {
				t.Fatalf("old %s reply was accepted", test.name)
			}
		})
	}
}

func TestRestartMarksActiveRunInterruptedAndDoesNotRerun(t *testing.T) {
	runner := &fakeRunner{
		started: make(chan runtime.Input, 4),
		plans: []fakeRunPlan{
			func(ctx context.Context, _ runtime.Input, _ runtime.Callbacks) (runtime.Result, error) {
				<-ctx.Done()
				return runtime.Result{}, ctx.Err()
			},
		},
	}
	dataDir := t.TempDir()
	first, sessionID := newControllerTest(t, runner, dataDir)
	if err := first.Send(sessionID, "正在执行", appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	select {
	case <-runner.started:
	case <-time.After(3 * time.Second):
		t.Fatal("run did not start")
	}
	if err := first.Send(sessionID, "稍后执行", appmodel.Queue); err != nil {
		t.Fatal(err)
	}

	restartedRunner := &fakeRunner{started: make(chan runtime.Input, 2)}
	restarted, err := New(Options{DataDir: dataDir, Runner: restartedRunner})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restarted.Close() })
	snapshot := restarted.Snapshot()
	model := sessionFromSnapshot(t, snapshot, sessionID)
	if model.State != appmodel.Interrupted || !model.QueuePaused || model.PauseReason != pauseReasonRestart {
		t.Fatalf("restarted model = %#v", model)
	}
	if len(model.Queue) != 1 || model.Queue[0].State != messageStateQueued {
		t.Fatalf("restarted queue = %#v", model.Queue)
	}
	if restartedRunner.callCount() != 0 {
		t.Fatalf("restart reran session, calls = %d", restartedRunner.callCount())
	}
	var interrupted bool
	for _, message := range model.Messages {
		if message.Role == "user" && message.Text == "正在执行" {
			interrupted = message.State == messageStateInterrupted && message.Interrupted
		}
	}
	if !interrupted {
		t.Fatalf("active message was not marked interrupted: %#v", model.Messages)
	}
}

func TestStoppingOneSessionDoesNotCancelAnother(t *testing.T) {
	block := func(ctx context.Context, _ runtime.Input, _ runtime.Callbacks) (runtime.Result, error) {
		<-ctx.Done()
		return runtime.Result{}, ctx.Err()
	}
	runner := &fakeRunner{started: make(chan runtime.Input, 4), plans: []fakeRunPlan{block, block}}
	instance, firstID := newControllerTest(t, runner, t.TempDir())
	secondID, err := instance.NewSession(appmodel.NewSessionOptions{ProjectID: "project"})
	if err != nil {
		t.Fatal(err)
	}
	if err := instance.Send(firstID, "one", appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	if err := instance.Send(secondID, "two", appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	waitForCalls(t, runner, 2)
	if err := instance.Stop(firstID); err != nil {
		t.Fatal(err)
	}
	waitForSnapshot(t, instance, func(snapshot appmodel.Snapshot) bool {
		first := sessionFromSnapshot(t, snapshot, firstID)
		second := sessionFromSnapshot(t, snapshot, secondID)
		return first.State == appmodel.Interrupted && second.State == appmodel.Running
	})
	if err := instance.Stop(secondID); err != nil {
		t.Fatal(err)
	}
	waitForSnapshot(t, instance, func(snapshot appmodel.Snapshot) bool {
		return sessionFromSnapshot(t, snapshot, secondID).State == appmodel.Interrupted
	})
}

func TestPersistenceFailureRollsBackSendAndRetryDoesNotDuplicate(t *testing.T) {
	runner := &fakeRunner{started: make(chan runtime.Input, 2)}
	instance, sessionID := newControllerTest(t, runner, t.TempDir())
	originalStatePath := instance.statePath
	instance.statePath = t.TempDir()
	if err := instance.Send(sessionID, "一次发送", appmodel.Correction); err == nil {
		t.Fatal("Send succeeded while controller state path was a directory")
	}
	if runner.callCount() != 0 {
		t.Fatalf("run started after persistence failure; calls = %d", runner.callCount())
	}
	model := sessionFromSnapshot(t, instance.Snapshot(), sessionID)
	if model.State != appmodel.Idle || len(model.Messages) != 0 || len(model.Queue) != 0 {
		t.Fatalf("failed send changed in-memory session: %#v", model)
	}

	instance.statePath = originalStatePath
	if err := instance.Send(sessionID, "一次发送", appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	waitForCalls(t, runner, 1)
	waitForSnapshot(t, instance, func(snapshot appmodel.Snapshot) bool {
		model := sessionFromSnapshot(t, snapshot, sessionID)
		return model.State == appmodel.Idle && len(model.Messages) == 2
	})
	if runner.callCount() != 1 {
		t.Fatalf("retry ran more than once; calls = %d", runner.callCount())
	}
}

func TestPersistenceFailureRollsBackProjectAndSettings(t *testing.T) {
	instance, sessionID := newControllerTest(t, &fakeRunner{}, t.TempDir())
	_ = sessionID
	originalStatePath := instance.statePath
	instance.statePath = t.TempDir()

	projectPath := filepath.Join(t.TempDir(), "extra-project")
	if err := os.MkdirAll(projectPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := instance.AddProject(projectPath); err == nil {
		t.Fatal("AddProject succeeded while controller state path was a directory")
	}
	if got := len(instance.Snapshot().Projects); got != 1 {
		t.Fatalf("project rollback left %d projects, want 1", got)
	}

	before := instance.Snapshot().Settings
	changed := before
	changed.Theme = "dark"
	if err := instance.SaveSettings(changed); err == nil {
		t.Fatal("SaveSettings succeeded while controller state path was a directory")
	}
	after := instance.Snapshot().Settings
	if after.Theme != before.Theme || len(after.Providers) != len(before.Providers) {
		t.Fatalf("settings rollback failed: before=%#v after=%#v", before, after)
	}
	instance.statePath = originalStatePath
}

func TestPermissionReplyPersistsBeforeWakingRun(t *testing.T) {
	replied := make(chan struct{})
	runner := &fakeRunner{
		started: make(chan runtime.Input, 2),
		plans: []fakeRunPlan{
			func(ctx context.Context, _ runtime.Input, callbacks runtime.Callbacks) (runtime.Result, error) {
				option, err := callbacks.OnPermissionRequest(ctx, runtime.PermissionRequest{
					ID:      "permission-persist",
					Tool:    "exec",
					Options: []runtime.PermissionOption{{ID: "allow", Label: "允许"}},
				})
				if err != nil {
					return runtime.Result{}, err
				}
				close(replied)
				return runtime.Result{FinalAnswer: option}, nil
			},
		},
	}
	instance, sessionID := newControllerTest(t, runner, t.TempDir())
	originalStatePath := instance.statePath
	if err := instance.Send(sessionID, "需要授权", appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	waitForSnapshot(t, instance, func(snapshot appmodel.Snapshot) bool {
		model := sessionFromSnapshot(t, snapshot, sessionID)
		return model.State == appmodel.WaitingPermission && len(model.Permissions) == 1
	})

	instance.statePath = t.TempDir()
	if err := instance.ReplyPermission(sessionID, "permission-persist", "allow"); err == nil {
		t.Fatal("ReplyPermission succeeded while controller state path was a directory")
	}
	select {
	case <-replied:
		t.Fatal("permission response reached the runner before persistence succeeded")
	default:
	}
	model := sessionFromSnapshot(t, instance.Snapshot(), sessionID)
	if model.State != appmodel.WaitingPermission || len(model.Permissions) != 1 {
		t.Fatalf("failed permission reply was not rolled back: %#v", model)
	}

	instance.statePath = originalStatePath
	if err := instance.ReplyPermission(sessionID, "permission-persist", "allow"); err != nil {
		t.Fatal(err)
	}
	waitForSnapshot(t, instance, func(snapshot appmodel.Snapshot) bool {
		return sessionFromSnapshot(t, snapshot, sessionID).State == appmodel.Idle
	})
	select {
	case <-replied:
	case <-time.After(3 * time.Second):
		t.Fatal("permission response did not reach the runner after persistence succeeded")
	}
}

func TestWorktreeSessionBindsAndRestoresAcrossControllerRestart(t *testing.T) {
	repo := controllerTestGitRepository(t)
	dataDir := t.TempDir()
	baseDir := filepath.Join(t.TempDir(), "worktrees")
	settings := appmodel.Settings{Providers: []appmodel.Provider{{ID: "provider", Name: "test-provider", Model: "test-model"}}, DefaultProviderID: "provider"}
	project := appmodel.Project{ID: "project", Name: "project", Path: repo}
	manager := worktrees.NewManager(worktrees.ManagerOptions{BaseDir: baseDir})
	first, err := New(Options{
		DataDir: dataDir, Runner: &fakeRunner{}, Projects: []appmodel.Project{project}, Settings: settings,
		WorktreeManager: manager,
	})
	if err != nil {
		t.Fatal(err)
	}
	sessionID, err := first.NewSession(appmodel.NewSessionOptions{ProjectID: project.ID, Worktree: true})
	if err != nil {
		_ = first.Close()
		t.Fatal(err)
	}
	session := sessionFromSnapshot(t, first.Snapshot(), sessionID)
	if !session.Worktree || session.Cwd == repo || session.Branch == "" {
		t.Fatalf("worktree session = %#v", session)
	}
	if users := manager.Users(session.Cwd); len(users) != 1 || users[0] != sessionID {
		t.Fatalf("initial worktree users = %#v", users)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	restartedManager := worktrees.NewManager(worktrees.ManagerOptions{BaseDir: baseDir})
	restarted, err := New(Options{DataDir: dataDir, Runner: &fakeRunner{}, WorktreeManager: restartedManager})
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	restartedSession := sessionFromSnapshot(t, restarted.Snapshot(), sessionID)
	if users := restartedManager.Users(restartedSession.Cwd); len(users) != 1 || users[0] != sessionID {
		t.Fatalf("restored worktree users = %#v", users)
	}
	if err := restartedManager.Remove(context.Background(), restartedSession.Cwd); !errors.Is(err, worktrees.ErrSessionWorktreeInUse) {
		t.Fatalf("remove restored worktree = %v, want in-use error", err)
	}
}

func controllerTestGitRepository(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	controllerGit(t, repo, "init", "-q")
	controllerGit(t, repo, "config", "user.email", "gcode@example.invalid")
	controllerGit(t, repo, "config", "user.name", "GCode Test")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("initial\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	controllerGit(t, repo, "add", "README.md")
	controllerGit(t, repo, "commit", "-qm", "initial")
	return repo
}

func controllerGit(t *testing.T, directory string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}
