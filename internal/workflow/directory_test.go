package workflow

import (
	"testing"
	"time"

	"gcode/internal/contracts"
)

func directoryRun(id, sessionID, status string, updated time.Time) contracts.WorkflowRun {
	return contracts.WorkflowRun{
		ID: id, SessionID: sessionID, Name: id, Status: status,
		StartedAt: updated.Add(-time.Minute), UpdatedAt: updated, Concurrency: 1,
	}
}

func TestBuildRunDirectorySplitsRunningFromEnded(t *testing.T) {
	base := time.Date(2026, 11, 23, 14, 0, 0, 0, time.UTC)
	runs := []contracts.WorkflowRun{
		directoryRun("run-done", "chat-1", contracts.WorkflowStatusCompleted, base),
		directoryRun("run-live", "chat-1", contracts.WorkflowStatusRunning, base.Add(time.Minute)),
		directoryRun("run-pending", "chat-1", contracts.WorkflowStatusPending, base.Add(2*time.Minute)),
		directoryRun("run-failed", "chat-1", contracts.WorkflowStatusErrored, base.Add(3*time.Minute)),
		directoryRun("run-other", "chat-2", contracts.WorkflowStatusRunning, base.Add(4*time.Minute)),
	}
	directory := BuildRunDirectory(runs, "chat-1", 0)
	if directory.Count() != 4 {
		t.Fatalf("only this conversation's runs: %+v", directory)
	}
	if len(directory.Running) != 2 {
		t.Fatalf("pending and running are still moving: %+v", directory.Running)
	}
	if len(directory.Ended) != 2 {
		t.Fatalf("the rest ended, with their own status words: %+v", directory.Ended)
	}
	// 最新在前。
	if directory.Running[0].RunID != "run-pending" {
		t.Fatalf("newest first: %+v", directory.Running)
	}
	if directory.Ended[0].RunID != "run-failed" {
		t.Fatalf("newest first: %+v", directory.Ended)
	}
	if directory.Truncated {
		t.Fatal("a short page is not truncated")
	}
}

func TestBuildRunDirectoryBoundsThePage(t *testing.T) {
	base := time.Date(2026, 11, 23, 14, 0, 0, 0, time.UTC)
	runs := make([]contracts.WorkflowRun, 0, 5)
	for i := range 5 {
		runs = append(runs, directoryRun("run-"+string(rune('a'+i)), "chat-1", contracts.WorkflowStatusCompleted, base.Add(time.Duration(i)*time.Minute)))
	}
	directory := BuildRunDirectory(runs, "chat-1", 3)
	if directory.Count() != 3 {
		t.Fatalf("the page keeps its limit: %+v", directory)
	}
	if !directory.Truncated {
		t.Fatal("a full page says more may follow")
	}
	if directory.Ended[0].RunID != "run-e" {
		t.Fatalf("the page keeps the newest: %+v", directory.Ended)
	}
}

func TestBuildRunDirectoryNeedsAConversation(t *testing.T) {
	base := time.Date(2026, 11, 23, 14, 0, 0, 0, time.UTC)
	runs := []contracts.WorkflowRun{directoryRun("run-1", "", contracts.WorkflowStatusRunning, base)}
	if directory := BuildRunDirectory(runs, "", 0); !directory.Empty() {
		t.Fatalf("no conversation, no directory: %+v", directory)
	}
}

func TestBuildRunDirectoryCountsWhatTheRowSays(t *testing.T) {
	base := time.Date(2026, 11, 23, 14, 0, 0, 0, time.UTC)
	run := directoryRun("run-1", "chat-1", contracts.WorkflowStatusStopped, base)
	run.StopReason = contracts.WorkflowStopUser
	run.Artifacts = []contracts.WorkflowArtifact{{ID: "a-1", Name: "report", Version: 1}}
	run.Phases = []contracts.WorkflowPhase{{ID: "plan"}, {ID: "build"}}
	run.Concurrency = 4
	directory := BuildRunDirectory([]contracts.WorkflowRun{run}, "chat-1", 0)
	row := directory.Ended[0]
	if row.StopReason != contracts.WorkflowStopUser || row.Artifacts != 1 || row.Phases != 2 || row.Concurrency != 4 {
		t.Fatalf("the row keeps the run's own facts: %+v", row)
	}
}
