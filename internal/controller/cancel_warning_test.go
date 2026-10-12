package controller

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gcode/internal/appmodel"
	"gcode/internal/runtime"
)

func TestExpectedCancellationDoesNotCreateWarning(t *testing.T) {
	for _, test := range []struct {
		name    string
		trigger func(*Controller, string) error
		state   appmodel.RunState
	}{
		{
			name: "correction",
			trigger: func(instance *Controller, sessionID string) error {
				return instance.Send(sessionID, "replacement", appmodel.Correction)
			},
			state: appmodel.Idle,
		},
		{
			name: "stop",
			trigger: func(instance *Controller, sessionID string) error {
				return instance.Stop(sessionID)
			},
			state: appmodel.Interrupted,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := &fakeRunner{
				started: make(chan runtime.Input, 4),
				plans: []fakeRunPlan{
					func(ctx context.Context, _ runtime.Input, callbacks runtime.Callbacks) (runtime.Result, error) {
						<-ctx.Done()
						callbacks.OnError(context.Canceled)
						return runtime.Result{}, context.Canceled
					},
				},
			}
			instance, sessionID := newControllerTest(t, runner, t.TempDir())
			if err := instance.Send(sessionID, "running", appmodel.Correction); err != nil {
				t.Fatal(err)
			}
			select {
			case <-runner.started:
			case <-testControllerTimer(t):
				t.Fatal("run did not start")
			}
			if err := test.trigger(instance, sessionID); err != nil {
				t.Fatal(err)
			}
			if test.name == "correction" {
				waitForCalls(t, runner, 2)
			}
			snapshot := waitForSnapshot(t, instance, func(snapshot appmodel.Snapshot) bool {
				model := sessionFromSnapshot(t, snapshot, sessionID)
				return model.State == test.state && model.ActiveRun == nil
			})
			model := sessionFromSnapshot(t, snapshot, sessionID)
			if model.Error != "" {
				t.Fatalf("expected cancellation error to remain empty, got %q", model.Error)
			}
			for _, message := range model.Messages {
				if message.Role == "warning" || strings.Contains(strings.ToLower(message.Text), "context canceled") {
					t.Fatalf("expected cancellation warning to be suppressed, got %#v", message)
				}
			}
		})
	}
}

func TestRuntimeErrorRemainsVisibleAfterNormalRun(t *testing.T) {
	const message = "provider setup failed"
	runner := &fakeRunner{
		started: make(chan runtime.Input, 2),
		plans: []fakeRunPlan{
			func(_ context.Context, _ runtime.Input, callbacks runtime.Callbacks) (runtime.Result, error) {
				err := errors.New(message)
				callbacks.OnError(err)
				return runtime.Result{}, err
			},
		},
	}
	instance, sessionID := newControllerTest(t, runner, t.TempDir())
	if err := instance.Send(sessionID, "fails", appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	snapshot := waitForSnapshot(t, instance, func(snapshot appmodel.Snapshot) bool {
		return sessionFromSnapshot(t, snapshot, sessionID).State == appmodel.Failed
	})
	model := sessionFromSnapshot(t, snapshot, sessionID)
	if !strings.Contains(model.Error, message) {
		t.Fatalf("normal runtime error = %q, want %q", model.Error, message)
	}
	if !controllerHasWarning(model, message) {
		t.Fatalf("normal runtime error warning is missing: %#v", model.Messages)
	}
}

func TestCancellationFailureRemainsVisible(t *testing.T) {
	const message = "provider ignored cancellation"
	runner := &fakeRunner{
		started: make(chan runtime.Input, 4),
		plans: []fakeRunPlan{
			func(ctx context.Context, _ runtime.Input, callbacks runtime.Callbacks) (runtime.Result, error) {
				<-ctx.Done()
				err := errors.New(message)
				callbacks.OnError(err)
				return runtime.Result{}, err
			},
		},
	}
	instance, sessionID := newControllerTest(t, runner, t.TempDir())
	if err := instance.Send(sessionID, "running", appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	select {
	case <-runner.started:
	case <-testControllerTimer(t):
		t.Fatal("run did not start")
	}
	if err := instance.Send(sessionID, "replacement", appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	snapshot := waitForSnapshot(t, instance, func(snapshot appmodel.Snapshot) bool {
		return sessionFromSnapshot(t, snapshot, sessionID).State == appmodel.Failed
	})
	model := sessionFromSnapshot(t, snapshot, sessionID)
	if model.PauseReason != pauseReasonCancelFailure || !strings.Contains(model.Error, message) {
		t.Fatalf("cancellation failure = %#v, want visible failure %q", model, message)
	}
	if !controllerHasWarning(model, message) {
		t.Fatalf("cancellation failure warning is missing: %#v", model.Messages)
	}
}

func controllerHasWarning(session appmodel.Session, text string) bool {
	for _, message := range session.Messages {
		if message.Role == "warning" && strings.Contains(message.Text, text) {
			return true
		}
	}
	return false
}

func testControllerTimer(t *testing.T) <-chan time.Time {
	t.Helper()
	return time.After(3 * time.Second)
}
