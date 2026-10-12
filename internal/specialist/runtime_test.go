package specialist

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"gcode/internal/background"
	"gcode/internal/streamjson"
)

func TestRuntimeCloseDoesNotCreateUnusedManager(t *testing.T) {
	created := false
	runtime := NewRuntime(RuntimeOptions{
		ManagerFunc: func() (*background.Manager, error) {
			created = true
			return background.NewManager(t.TempDir())
		},
	})

	if err := runtime.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if created {
		t.Fatal("Close created an unused background manager")
	}
}

func TestRuntimeManagerRequiresConfiguredManager(t *testing.T) {
	runtime := NewRuntime(RuntimeOptions{})
	if _, err := runtime.Manager(); err == nil || err.Error() != "background manager is not configured" {
		t.Fatalf("Manager error = %v, want unconfigured manager error", err)
	}
	if _, err := (*Runtime)(nil).Manager(); err == nil || err.Error() != "specialist runtime is not configured" {
		t.Fatalf("nil Runtime Manager error = %v, want unconfigured runtime error", err)
	}
}

func TestRuntimeStartTaskPublishesOwnerBeforeCloseCanCancel(t *testing.T) {
	manager, err := background.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	if _, err := manager.Register(background.RegisterInput{TaskID: "child_task", Type: "specialist"}); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	runtime := NewRuntime(RuntimeOptions{Manager: manager})
	childContext, cancel := context.WithCancel(context.Background())
	if err := runtime.StartTask("child_task", manager, cancel); err != nil {
		t.Fatalf("StartTask returned error: %v", err)
	}
	childDone := make(chan struct{})
	go func() {
		defer close(childDone)
		defer runtime.FinishTask("child_task")
		<-childContext.Done()
	}()

	closeDone := make(chan error, 1)
	go func() { closeDone <- runtime.Close() }()
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not cancel and wait for the child")
	}
	select {
	case <-childDone:
	case <-time.After(time.Second):
		t.Fatal("child did not finish after Close")
	}
	task, ok := manager.Get("child_task")
	if !ok || task.Status != background.StatusKilled {
		t.Fatalf("task after Close = %#v", task)
	}
}

func TestRuntimeCloseIsIdempotentAndConcurrentCallersShareCompletion(t *testing.T) {
	manager, err := background.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	if _, err := manager.Register(background.RegisterInput{TaskID: "child_task", Type: "specialist"}); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	runtime := NewRuntime(RuntimeOptions{Manager: manager})
	childContext, cancel := context.WithCancel(context.Background())
	if err := runtime.StartTask("child_task", manager, cancel); err != nil {
		t.Fatalf("StartTask returned error: %v", err)
	}
	childDone := make(chan struct{})
	go func() {
		defer close(childDone)
		defer runtime.FinishTask("child_task")
		<-childContext.Done()
	}()

	const callers = 4
	results := make(chan error, callers)
	var callersWait sync.WaitGroup
	callersWait.Add(callers)
	for range callers {
		go func() {
			defer callersWait.Done()
			results <- runtime.Close()
		}()
	}
	callersWait.Wait()
	for index := 0; index < callers; index++ {
		if err := <-results; err != nil {
			t.Fatalf("concurrent Close returned error: %v", err)
		}
	}
	select {
	case <-childDone:
	case <-time.After(time.Second):
		t.Fatal("child did not finish after concurrent Close")
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("repeated Close returned error: %v", err)
	}
}

func TestRuntimeCloseReportsUnresponsiveTaskAndReleasesFixture(t *testing.T) {
	manager, err := background.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	if _, err := manager.Register(background.RegisterInput{TaskID: "ignored_task", Type: "specialist"}); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	runtime := NewRuntime(RuntimeOptions{Manager: manager})
	_, cancel := context.WithCancel(context.Background())
	if err := runtime.StartTask("ignored_task", manager, cancel); err != nil {
		t.Fatalf("StartTask returned error: %v", err)
	}
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseFixture := func() { releaseOnce.Do(func() { close(release) }) }
	childDone := make(chan struct{})
	go func() {
		defer close(childDone)
		defer runtime.FinishTask("ignored_task")
		<-release
	}()
	t.Cleanup(func() {
		releaseFixture()
		select {
		case <-childDone:
		case <-time.After(time.Second):
			t.Error("unresponsive child fixture did not release")
		}
	})

	results := make(chan error, 2)
	go func() { results <- runtime.Close() }()
	go func() { results <- runtime.Close() }()
	firstErr := <-results
	secondErr := <-results
	if firstErr == nil || !strings.Contains(firstErr.Error(), "timed out") || !strings.Contains(firstErr.Error(), "ignored_task") {
		t.Fatalf("first Close error = %v, want timeout naming ignored_task", firstErr)
	}
	if secondErr == nil || secondErr.Error() != firstErr.Error() {
		t.Fatalf("concurrent Close error = %v, want same timeout result as %v", secondErr, firstErr)
	}

	releaseFixture()
	select {
	case <-childDone:
	case <-time.After(time.Second):
		t.Fatal("unresponsive child fixture did not finish after release")
	}
	repeatedErr := runtime.Close()
	if repeatedErr == nil || repeatedErr.Error() != firstErr.Error() {
		t.Fatalf("repeated Close error = %v, want same timeout result as %v", repeatedErr, firstErr)
	}
}

func TestRuntimeCloseKillsRunningTasksAndCleansPromptFiles(t *testing.T) {
	killed := []int{}
	manager, err := background.NewManagerWithOptions(background.ManagerOptions{
		RootDir: t.TempDir(),
		KillProcess: func(pid int) error {
			killed = append(killed, pid)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("NewManagerWithOptions returned error: %v", err)
	}
	if _, err := manager.Register(background.RegisterInput{TaskID: "child_task", Type: "specialist", PID: 42}); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	promptDir := filepath.Join(t.TempDir(), "gcode-specialist-test")
	if err := os.MkdirAll(promptDir, 0o700); err != nil {
		t.Fatal(err)
	}
	promptFile := filepath.Join(promptDir, "prompt.md")
	if err := os.WriteFile(promptFile, []byte("prompt"), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime := NewRuntime(RuntimeOptions{Manager: manager})
	runtime.TrackPromptFile("child_task", promptFile)

	if err := runtime.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	if !reflect.DeepEqual(killed, []int{42}) {
		t.Fatalf("killed pids = %#v", killed)
	}
	task, ok := manager.Get("child_task")
	if !ok || task.Status != background.StatusKilled {
		t.Fatalf("task after close = %#v", task)
	}
	if _, err := os.Stat(promptFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("prompt file cleanup error = %v", err)
	}
}

func TestRuntimeClosePreservesKilledTaskAfterChildExit(t *testing.T) {
	manager, err := background.NewManagerWithOptions(background.ManagerOptions{
		RootDir: t.TempDir(),
		KillProcess: func(pid int) error {
			return nil
		},
	})
	if err != nil {
		t.Fatalf("NewManagerWithOptions returned error: %v", err)
	}
	runtime := NewRuntime(RuntimeOptions{Manager: manager})
	childStarted := make(chan struct{})
	executor := Executor{
		BinaryPath:        "/usr/local/bin/zero",
		BackgroundRuntime: runtime,
		NewSessionID:      func() (string, error) { return "child_task", nil },
		Load: func(LoadOptions) (LoadResult, error) {
			return LoadResult{Specialists: []Manifest{{
				Metadata:      Metadata{Name: "worker", Description: "Does focused work"},
				SystemPrompt:  "Work carefully.",
				ResolvedTools: []string{"read_file"},
			}}}, nil
		},
		RunChild: func(ctx context.Context, binaryPath string, args []string, progress func(streamjson.Event)) (ChildRunResult, error) {
			if binaryPath != "" {
				t.Fatalf("background binaryPath = %q", binaryPath)
			}
			close(childStarted)
			<-ctx.Done()
			return ChildRunResult{ExitCode: -1, Started: true}, ctx.Err()
		},
	}

	result, err := executor.Run(context.Background(), TaskParameters{
		Name:            "worker",
		Prompt:          "inspect auth",
		RunInBackground: true,
	}, TaskRunOptions{})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.SessionID != "child_task" {
		t.Fatalf("session id = %q", result.SessionID)
	}
	select {
	case <-childStarted:
	case <-time.After(time.Second):
		t.Fatal("background child did not start")
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	if err := manager.MarkExited("child_task", background.StatusError, 1); err != nil {
		t.Fatalf("late child exit returned error: %v", err)
	}

	task, ok := manager.Get("child_task")
	if !ok || task.Status != background.StatusKilled || task.ExitCode != -1 {
		t.Fatalf("child exit clobbered killed task: %#v", task)
	}
}
