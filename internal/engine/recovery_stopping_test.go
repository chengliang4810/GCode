package engine

import (
	"testing"
	"time"

	"gcode/internal/contracts"
)

// A stopping run is transient: it only exists between an accepted stop request
// and the run settling. When the host stops inside that window the status is
// written to disk, so recovery has to settle it exactly like a running run.
// Otherwise the conversation keeps showing "stopping" forever and the run
// never reaches a terminal state.

func TestRecoverSettlesStoppingRunOnSessionLoad(t *testing.T) {
	core := openStorageTestCore(t, t.TempDir())
	defer core.Close()
	target := newTestSession(t, core, "", "stopping")
	other := newTestSession(t, core, "", "still-owned")
	core.state.Runs = []contracts.Run{
		{ID: "stopping", SessionID: target.ID, Status: contracts.RunStatusStopping, FinishReason: contracts.FinishReasonUserStop},
		{ID: "running", SessionID: target.ID, Status: contracts.RunStatusRunning},
		{ID: "completed", SessionID: target.ID, Status: contracts.RunStatusCompleted, FinishReason: contracts.FinishReasonStop},
		{ID: "other-stopping", SessionID: other.ID, Status: contracts.RunStatusStopping, FinishReason: contracts.FinishReasonUserStop},
	}
	core.reconcileSessionLocked(target.ID)
	for _, index := range []int{0, 1} {
		run := core.state.Runs[index]
		if run.Status != contracts.RunStatusInterrupted || run.FinishReason != "engineRestarted" {
			t.Fatalf("run %d was not settled by recovery: %+v", index, run)
		}
	}
	completed := core.state.Runs[2]
	if completed.Status != contracts.RunStatusCompleted || completed.FinishReason != contracts.FinishReasonStop {
		t.Fatalf("recovery changed a settled run: %+v", completed)
	}
	if other := core.state.Runs[3]; other.Status != contracts.RunStatusStopping || other.FinishReason != contracts.FinishReasonUserStop {
		t.Fatalf("recovery changed another session's run: %+v", other)
	}
}

func TestRestartSettlesPersistedStoppingRun(t *testing.T) {
	home := t.TempDir()
	core := openStorageTestCore(t, home)
	session := newTestSession(t, core, "", "stopping")
	storeTestMessage(t, core, session.ID, "hello")
	core.mu.Lock()
	core.state.Runs = append(core.state.Runs, contracts.Run{
		ID: "run", SessionID: session.ID, Status: contracts.RunStatusStopping,
		FinishReason: contracts.FinishReasonUserStop, StartedAt: time.Now(),
	})
	err := core.persistLocked(session.ID)
	core.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if err = core.Close(); err != nil {
		t.Fatal(err)
	}

	reopened := openStorageTestCore(t, home)
	defer reopened.Close()
	// The index restores without loading any conversation body.
	restored := runByID(reopened.state.Runs, "run")
	if restored == nil || restored.Status != contracts.RunStatusInterrupted || restored.FinishReason != "engineRestarted" {
		t.Fatalf("persisted stopping run was not settled on restart: %+v", reopened.state.Runs)
	}
	// Loading the conversation replays the session record, which still holds
	// the transient status and must be settled again.
	if _, err = reopened.Snapshot(t.Context(), session.ID); err != nil {
		t.Fatal(err)
	}
	loaded := runByID(reopened.state.Runs, "run")
	if loaded == nil || loaded.Status != contracts.RunStatusInterrupted || loaded.FinishReason != "engineRestarted" {
		t.Fatalf("loaded stopping run was not settled by session recovery: %+v", reopened.state.Runs)
	}
}

func runByID(runs []contracts.Run, id string) *contracts.Run {
	for i := range runs {
		if runs[i].ID == id {
			return &runs[i]
		}
	}
	return nil
}
