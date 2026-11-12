package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gcode/internal/accounting"
	"gcode/internal/contracts"
	"gcode/internal/model"
)

func TestExecutionMetricsPersistWithoutReopeningHistory(t *testing.T) {
	home := t.TempDir()
	core := openStorageTestCore(t, home)
	if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create"}); err != nil {
		t.Fatal(err)
	}
	session := core.state.Sessions[0]
	start := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	finish := start.Add(2500 * time.Millisecond)
	core.mu.Lock()
	core.loadedSessions[session.ID] = true
	core.state.Messages[session.ID] = []contracts.Message{{ID: "reply", SessionID: session.ID, TurnID: "run", Role: "assistant", ModelID: "p/m", Blocks: []contracts.Block{
		{ID: "tool", Kind: "tool", Tool: &contracts.ToolCall{ID: "tool", Kind: contracts.ToolExec, Status: "completed", StartedAt: start, FinishedAt: &finish}},
	}}}
	core.state.Runs = append(core.state.Runs, contracts.Run{ID: "run", SessionID: session.ID, Status: "completed", StartedAt: start, FinishedAt: &finish, DurationMS: 2500})
	core.state.ModelRequests = append(core.state.ModelRequests, contracts.ModelRequest{ID: "request", SessionID: session.ID, TurnID: "run", ProviderID: "p", ProviderName: "Provider", ModelID: "m", Usage: model.Usage{Known: true, OutputKnown: true, Input: 100, Output: 20}})
	err := core.persistLocked(session.ID)
	stats := core.state.Metrics[session.ID]
	summary := core.state.Runs[len(core.state.Runs)-1].Summary
	milliseconds := core.state.Messages[session.ID][0].DurationMS
	indexPath := filepath.Join(core.storage.ConfigDirectory(), "engine.json")
	recordPath := filepath.Join(core.storage.Directory(), "sessions", session.ID, core.sessionRecords[session.ID]+".json")
	core.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if stats.Total.Totals.Input != 100 || stats.Total.ToolDuration != 2500*time.Millisecond || stats.Total.WallDuration != 2500*time.Millisecond {
		t.Fatalf("wrong saved metrics: %+v", stats.Total)
	}
	if summary.Commands != 1 || milliseconds != 2500 {
		t.Fatal("run summary or message duration was not finalized")
	}
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	var index diskState
	if err = json.Unmarshal(data, &index); err != nil {
		t.Fatal(err)
	}
	if index.Snapshot.Metrics[session.ID].Turns["run"].Totals.Output != 20 {
		t.Fatal("reply aggregate was not persisted")
	}
	if err = core.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openStorageTestCore(t, home)
	defer reopened.Close()
	if reopened.loadedSessions[session.ID] {
		t.Fatal("restart eagerly loaded historical messages")
	}
	if err = os.Rename(recordPath, recordPath+".hidden"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := reopened.Snapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Metrics[session.ID].Total.Totals.Input != 100 {
		t.Fatal("restart needed message bodies to rebuild summary")
	}
	if len(snapshot.Messages) != 0 {
		t.Fatal("global snapshot loaded history")
	}
	report, err := reopened.ReadApplicationUsage(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if report.Report.Summary.LongestSessionMS != 2500 || report.GeneratedAt.IsZero() {
		t.Fatalf("restarted application usage required historical bodies: generated=%v longest=%d", report.GeneratedAt, report.Report.Summary.LongestSessionMS)
	}

}

func TestDelegatedMetricsUpdateUnloadedParent(t *testing.T) {
	home := t.TempDir()
	core := openStorageTestCore(t, home)
	defer core.Close()
	if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create"}); err != nil {
		t.Fatal(err)
	}
	parent := core.state.Sessions[0]
	core.mu.Lock()
	child, err := core.newSessionLocked("", "Child", "")
	if err != nil {
		core.mu.Unlock()
		t.Fatal(err)
	}
	childIndex := core.sessionIndex(child.ID)
	core.state.Sessions[childIndex].ParentSessionID = parent.ID
	core.state.Sessions[childIndex].ParentRunID = "parent-turn"
	core.state.Sessions[childIndex].SubagentType = "Explore"
	core.loadedSessions[parent.ID] = true
	core.state.Messages[parent.ID] = nil
	core.state.ModelRequests = append(core.state.ModelRequests, contracts.ModelRequest{ID: "parent-req", SessionID: parent.ID, TurnID: "parent-turn", Usage: model.Usage{Known: true, Input: 100}})
	err = core.persistLocked(parent.ID, child.ID)
	if err == nil {
		delete(core.state.Messages, parent.ID)
		core.loadedSessions[parent.ID] = false
		core.state.Messages[child.ID] = nil
		core.state.ModelRequests = append(core.state.ModelRequests, contracts.ModelRequest{ID: "child-req", SessionID: child.ID, TurnID: "child-turn", Usage: model.Usage{Known: true, Input: 20}})
		err = core.persistLocked(child.ID)
	}
	stats := core.state.Metrics[parent.ID]
	parentLoaded := core.loadedSessions[parent.ID]
	core.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if parentLoaded {
		t.Fatal("updating child reloaded parent history")
	}
	if stats.Total.Totals.Input != 120 || stats.Turns["parent-turn"].Totals.Input != 120 || stats.Own.Totals.Input != 100 {
		t.Fatalf("delegation totals lost or duplicated: %+v", stats)
	}
	view, err := core.Snapshot(t.Context(), parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Metrics[parent.ID].Total.Totals.Input != 120 {
		t.Fatal("opening parent replaced current summary with stale record data")
	}
}

func TestStatisticsCommitOnlyWithTheirSessionRecords(t *testing.T) {
	home := t.TempDir()
	core := openStorageTestCore(t, home)
	defer core.Close()
	for range 2 {
		if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create"}); err != nil {
			t.Fatal(err)
		}
	}
	bad, good := core.state.Sessions[0], core.state.Sessions[1]
	start := time.Now().Add(-time.Minute)
	core.mu.Lock()
	core.loadedSessions[bad.ID] = true
	core.loadedSessions[good.ID] = true
	core.state.Messages[bad.ID] = []contracts.Message{{ID: "bad-initial", SessionID: bad.ID, Role: "assistant", CreatedAt: start, Usage: contracts.Usage{InputTokens: 10}}}
	core.state.Messages[good.ID] = []contracts.Message{{ID: "good-initial", SessionID: good.ID, Role: "assistant", CreatedAt: start, Usage: contracts.Usage{InputTokens: 20}}}
	core.state.ModelRequests = append(core.state.ModelRequests,
		contracts.ModelRequest{ID: "bad-initial", SessionID: bad.ID, Usage: model.Usage{Known: true, Input: 10}},
		contracts.ModelRequest{ID: "good-initial", SessionID: good.ID, Usage: model.Usage{Known: true, Input: 20}},
	)
	err := core.persistLocked(bad.ID, good.ID)
	path := filepath.Join(core.storage.Directory(), "sessions", bad.ID)
	indexPath := filepath.Join(core.storage.ConfigDirectory(), "engine.json")
	core.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(path, path+".saved"); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("blocked session directory"), 0600); err != nil {
		t.Fatal(err)
	}
	core.mu.Lock()
	core.state.ModelRequests = append(core.state.ModelRequests,
		contracts.ModelRequest{ID: "bad-new", SessionID: bad.ID, Usage: model.Usage{Known: true, Input: 30}},
		contracts.ModelRequest{ID: "good-new", SessionID: good.ID, Usage: model.Usage{Known: true, Input: 40}},
	)
	core.state.Messages[bad.ID] = append(core.state.Messages[bad.ID], contracts.Message{ID: "bad-new", SessionID: bad.ID, CreatedAt: start, Usage: contracts.Usage{InputTokens: 30}})
	core.state.Messages[good.ID] = append(core.state.Messages[good.ID], contracts.Message{ID: "good-new", SessionID: good.ID, CreatedAt: start, Usage: contracts.Usage{InputTokens: 40}})
	core.markSessionDirtyLocked(bad.ID)
	err = core.persistLocked(good.ID)
	core.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	var index diskState
	if err = json.Unmarshal(data, &index); err != nil {
		t.Fatal(err)
	}
	if index.Snapshot.Metrics[bad.ID].Total.Totals.Input != 10 || index.Snapshot.Metrics[good.ID].Total.Totals.Input != 60 {
		t.Fatalf("statistics committed data without its failed session record: bad=%d good=%d", index.Snapshot.Metrics[bad.ID].Total.Totals.Input, index.Snapshot.Metrics[good.ID].Total.Totals.Input)
	}
	if index.Snapshot.ApplicationUsage == nil || index.Snapshot.ApplicationUsage.Report.Summary.Total != 70 {
		t.Fatalf("application usage disagrees with committed session records: %+v", index.Snapshot.ApplicationUsage)
	}
}

func TestRestartFreezesSavedRunningMetricsWithoutHistory(t *testing.T) {
	start := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	snapshot := contracts.Snapshot{
		Sessions: []contracts.Session{{ID: "session", Status: contracts.Working}},
		Messages: map[string][]contracts.Message{"session": nil},
		Runs:     []contracts.Run{{ID: "run", SessionID: "session", Status: "running", StartedAt: start, DurationMS: 2500}},
	}
	accounting.Refresh(&snapshot, []string{"session"})
	index := persistenceIndex(snapshot)
	restored := index.restore(contracts.Snapshot{})
	for _, stats := range []contracts.ExecutionMetrics{restored.Metrics["session"].Own, restored.Metrics["session"].Total, restored.Metrics["session"].Turns["run"]} {
		duration, known := stats.Elapsed(start.Add(24 * time.Hour))
		if !known || duration != 2500*time.Millisecond || !stats.RunningSince.IsZero() {
			t.Fatalf("restart advanced an abandoned running clock: %+v", stats)
		}
	}
	if len(restored.Messages) != 0 {
		t.Fatal("restart required history to freeze the clock")
	}
}

func TestDeletingDelegatedSessionRemovesItsSavedTotals(t *testing.T) {
	core := openStorageTestCore(t, t.TempDir())
	defer core.Close()
	if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create"}); err != nil {
		t.Fatal(err)
	}
	parent := core.state.Sessions[0]
	core.mu.Lock()
	child, err := core.newSessionLocked("", "Child", "")
	if err != nil {
		core.mu.Unlock()
		t.Fatal(err)
	}
	core.state.Sessions[core.sessionIndex(child.ID)].ParentSessionID = parent.ID
	core.state.Sessions[core.sessionIndex(child.ID)].ParentRunID = "turn"
	core.state.Sessions[core.sessionIndex(child.ID)].SubagentType = "Explore"
	core.loadedSessions[parent.ID] = true
	core.state.Messages[parent.ID] = nil
	core.state.Messages[child.ID] = nil
	core.state.ModelRequests = append(core.state.ModelRequests,
		contracts.ModelRequest{ID: "p", SessionID: parent.ID, TurnID: "turn", Usage: model.Usage{Known: true, Input: 100}},
		contracts.ModelRequest{ID: "c", SessionID: child.ID, TurnID: "child-turn", Usage: model.Usage{Known: true, Input: 20}},
	)
	err = core.persistLocked(parent.ID, child.ID)
	core.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if err = core.Execute(t.Context(), contracts.Command{Kind: "session.delete", ID: child.ID}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := core.Snapshot(t.Context(), parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Metrics[parent.ID].Total.Totals.Input != 100 || snapshot.Metrics[parent.ID].Turns["turn"].Totals.Input != 100 {
		t.Fatal("deleted child retained billed work in parent")
	}
	if _, exists := snapshot.Metrics[child.ID]; exists {
		t.Fatal("deleted session retained metrics")
	}
}
