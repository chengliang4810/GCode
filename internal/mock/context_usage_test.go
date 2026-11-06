package mock

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"gcode/internal/contracts"
)

func contextSnapshot(t *testing.T, s *Server) contracts.Snapshot {
	t.Helper()
	snapshot, err := s.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func heldContextRun(t *testing.T, s *Server, sessionID, modelID string) contracts.Run {
	t.Helper()
	run, err := s.Send(context.Background(), contracts.SendRequest{SessionID: sessionID, Text: "Inspect the context snapshot", ModelID: modelID})
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func contextGeneration(s *Server, sessionID string) uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.contextSessions[sessionID].Generation
}

func reportContext(s *Server, sessionID, runID string, generation uint64, report contracts.ContextUsage) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.mergeHostContextLocked(sessionID, runID, generation, report) {
		return false
	}
	s.publishLocked("context.updated", sessionID, "", "", "")
	return true
}

func TestContextUsageSeedAndSideSessionIsolation(t *testing.T) {
	s := newTestServer(t, Options{Scenario: "transcript", Instant: true})
	before := contextSnapshot(t, s)
	beforeDetail := snapshotForSessions(t, s, "chat-0")
	for _, tc := range []struct {
		name, id string
		known    bool
		fraction float64
	}{
		{name: "unreported", id: "chat-1"},
		{name: "capacity only", id: "chat-2"},
		{name: "real zero", id: "chat-3", known: true},
		{name: "over capacity", id: "chat-4", known: true, fraction: 1.2},
		{name: "warning boundary", id: "chat-5", known: true, fraction: .75},
		{name: "danger boundary", id: "chat-6", known: true, fraction: .9},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fraction, known := before.ContextUsage[tc.id].Fraction()
			if known != tc.known || fraction != tc.fraction {
				t.Fatalf("usage=%+v fraction=%v known=%v", before.ContextUsage[tc.id], fraction, known)
			}
		})
	}
	if before.ContextUsage["chat-1"].HasWindow() || !before.ContextUsage["chat-2"].HasWindow() {
		t.Fatal("unknown and capacity-only contexts were conflated")
	}
	if *before.ContextUsage["chat-0"].Tokens == *before.ContextUsage["showcase-agent"].Tokens {
		t.Fatal("main and side fixtures must have independent host occupancy")
	}
	events, err := s.Subscribe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	run, err := s.Send(context.Background(), contracts.SendRequest{SessionID: "showcase-agent", Text: "Check the schema fixture", ModelID: "claude-sonnet-4-6"})
	if err != nil {
		t.Fatal(err)
	}
	after := await(t, events, func(snapshot contracts.Snapshot) bool {
		for _, candidate := range snapshot.Runs {
			if candidate.ID == run.ID {
				return candidate.Status == "completed" && snapshot.ContextUsage["showcase-agent"].Tokens != nil && *snapshot.ContextUsage["showcase-agent"].Tokens > 8400
			}
		}
		return false
	})
	afterDetail := snapshotForSessions(t, s, "chat-0")
	if !reflect.DeepEqual(before.ContextUsage["chat-0"], after.ContextUsage["chat-0"]) || !reflect.DeepEqual(beforeDetail.Messages["chat-0"], afterDetail.Messages["chat-0"]) {
		t.Fatal("side host activity changed the main context or transcript")
	}
}

func TestContextUsageReportsAndSnapshotOwnership(t *testing.T) {
	s := newTestServer(t, Options{TokenInterval: time.Hour})
	run := heldContextRun(t, s, "chat-0", "claude-opus-4-6")
	generation := contextGeneration(s, "chat-0")
	beforeDetail := snapshotForSessions(t, s, "chat-0")
	tokens, window := uint64(23100), uint64(180000)
	if !reportContext(s, "chat-0", run.ID, generation, contracts.ContextUsage{Tokens: &tokens, Window: &window}) {
		t.Fatal("current host report rejected")
	}
	tokens, window = 99, 1
	a := contextSnapshot(t, s)
	aDetail := snapshotForSessions(t, s, "chat-0")
	if *a.ContextUsage["chat-0"].Tokens != 23100 || *a.ContextUsage["chat-0"].Window != 180000 {
		t.Fatal("backend retained caller-owned report pointers")
	}
	if !reflect.DeepEqual(beforeDetail.Messages, aDetail.Messages) {
		t.Fatal("host occupancy changed legacy message billing usage")
	}
	first, err := s.Subscribe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Subscribe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	one, two := <-first, <-second
	*one.Snapshot.ContextUsage["chat-0"].Tokens = 9
	*one.Snapshot.ContextUsage["chat-0"].Window = 10
	delete(one.Snapshot.ContextUsage, "chat-1")
	*a.ContextUsage["chat-0"].Tokens = 11
	if *two.Snapshot.ContextUsage["chat-0"].Tokens != 23100 || !reflect.DeepEqual(two.Snapshot.ContextUsage, contextSnapshot(t, s).ContextUsage) {
		t.Fatal("snapshot or subscribers share mutable context counters")
	}
	zero := uint64(0)
	if !reportContext(s, "chat-0", run.ID, generation, contracts.ContextUsage{Tokens: &zero, Window: &zero}) {
		t.Fatal("zero occupancy rejected")
	}
	a = contextSnapshot(t, s)
	if *a.ContextUsage["chat-0"].Tokens != 0 || *a.ContextUsage["chat-0"].Window != 180000 {
		t.Fatal("zero tokens or ignored zero window merged incorrectly")
	}
	if !reportContext(s, "chat-0", run.ID, generation, contracts.ContextUsage{Window: contextCount(240000)}) {
		t.Fatal("capacity-only report rejected")
	}
	if *contextSnapshot(t, s).ContextUsage["chat-0"].Tokens != 0 {
		t.Fatal("nil tokens erased the reported zero")
	}
}

func TestContextUsageProviderResumeAndRunOwnership(t *testing.T) {
	s := newTestServer(t, Options{TokenInterval: time.Hour})
	runA := heldContextRun(t, s, "chat-0", "claude-opus-4-6")
	genA := contextGeneration(s, "chat-0")
	beforeSelection := contextSnapshot(t, s).ContextUsage["chat-0"]
	modelB := "gpt-5.4"
	if err := s.UpdateSession(context.Background(), contracts.SessionUpdate{ID: "chat-0", ModelID: &modelB}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(beforeSelection, contextSnapshot(t, s).ContextUsage["chat-0"]) {
		t.Fatal("selecting the next turn's provider cleared current context")
	}
	// The running provider remains authoritative after the composer selection changes.
	if !reportContext(s, "chat-0", runA.ID, genA, contracts.ContextUsage{Tokens: contextCount(12345)}) {
		t.Fatal("mutable composer selection invalidated the running host")
	}
	s.advanceRun("chat-0", runA.ID, "Done", true)
	if !reportContext(s, "chat-0", runA.ID, genA, contracts.ContextUsage{Tokens: contextCount(16000)}) {
		t.Fatal("late context from completed provider A rejected")
	}
	runB := heldContextRun(t, s, "chat-0", modelB)
	genB := contextGeneration(s, "chat-0")
	reset := contextSnapshot(t, s).ContextUsage["chat-0"]
	if reset.Tokens == nil || *reset.Tokens != 4416 || !reset.HasWindow() || *reset.Window != 1000000 {
		t.Fatalf("new provider did not reset to its host capacity: %+v", reset)
	}
	if reportContext(s, "chat-0", runA.ID, genA, contracts.ContextUsage{Tokens: contextCount(90000)}) || reportContext(s, "chat-0", runB.ID, genA, contracts.ContextUsage{Tokens: contextCount(90000)}) {
		t.Fatal("an obsolete run or source generation overwrote the new provider")
	}
	s.advanceRun("chat-0", runB.ID, "Done", true)
	if !reportContext(s, "chat-0", runB.ID, genB, contracts.ContextUsage{Tokens: contextCount(77000)}) {
		t.Fatal("late provider B report rejected")
	}
	runA2 := heldContextRun(t, s, "chat-0", "claude-sonnet-4-6")
	// Starting the resumed execution emits its first reasoning report, using
	// provider A's saved occupancy rather than provider B's last report.
	if *contextSnapshot(t, s).ContextUsage["chat-0"].Tokens != 16320 {
		t.Fatal("resuming provider A used the wrong baseline for its reasoning report")
	}
	s.mu.Lock()
	s.publishLocked("message.delta", "chat-0", runA2.ID, "", "")
	s.mu.Unlock()
	if tokens := *contextSnapshot(t, s).ContextUsage["chat-0"].Tokens; tokens != 16960 {
		t.Fatalf("provider A resumed the wrong host occupancy: %d", tokens)
	}
}

func TestContextUsageLateDoneDoesNotReopenTurn(t *testing.T) {
	s := newTestServer(t, Options{TokenInterval: time.Hour})
	run := heldContextRun(t, s, "chat-0", "claude-opus-4-6")
	generation := contextGeneration(s, "chat-0")
	s.advanceRun("chat-0", run.ID, "Finished", true)
	before := contextSnapshot(t, s)
	beforeDetail := snapshotForSessions(t, s, "chat-0")
	events, err := s.Subscribe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	<-events
	if !reportContext(s, "chat-0", run.ID, generation, contracts.ContextUsage{Tokens: contextCount(7000)}) {
		t.Fatal("completed run's late host report rejected")
	}
	event := <-events
	if event.Kind != "context.updated" || event.RunID != "" || event.Snapshot == nil {
		t.Fatalf("late usage event %+v", event)
	}
	after := *event.Snapshot
	afterDetail := snapshotForSessions(t, s, "chat-0")
	if !reflect.DeepEqual(before.Runs, after.Runs) || !reflect.DeepEqual(before.Sessions, after.Sessions) || !reflect.DeepEqual(beforeDetail.Messages, afterDetail.Messages) {
		t.Fatal("late context report changed a completed turn or its transcript")
	}
	if *after.ContextUsage["chat-0"].Tokens != 7000 || after.Revision <= before.Revision {
		t.Fatal("late context was not published in the full snapshot")
	}
}

func TestContextUsageQueuedRunTakesOwnershipAfterFinalReport(t *testing.T) {
	s := newTestServer(t, Options{TokenInterval: time.Hour})
	first := heldContextRun(t, s, "chat-0", "claude-opus-4-6")
	oldGeneration := contextGeneration(s, "chat-0")
	if _, err := s.Send(context.Background(), contracts.SendRequest{SessionID: "chat-0", Text: "Queued inspection", ModelID: "gpt-5.4"}); err != nil {
		t.Fatal(err)
	}
	s.advanceRun("chat-0", first.ID, "First done", true)
	after := contextSnapshot(t, s)
	if len(after.Queue) != 0 || after.ContextUsage["chat-0"].Tokens == nil || *after.ContextUsage["chat-0"].Tokens != 4416 || *after.ContextUsage["chat-0"].Window != 1000000 {
		t.Fatal("queued new provider did not reset after the prior final context report")
	}
	if reportContext(s, "chat-0", first.ID, oldGeneration, contracts.ContextUsage{Tokens: contextCount(99999)}) {
		t.Fatal("prior run overwrote the queued run's new provider session")
	}
	if after.Sessions[0].Status != contracts.Working {
		t.Fatal("late context update interrupted the queued turn")
	}
}

func TestContextUsagePermissionContinuationKeepsProviderSession(t *testing.T) {
	s := newTestServer(t, Options{TokenInterval: time.Hour})
	run, err := s.Send(context.Background(), contracts.SendRequest{SessionID: "chat-0", Text: "Request permission to inspect checks", ModelID: "claude-opus-4-6"})
	if err != nil {
		t.Fatal(err)
	}
	generation := contextGeneration(s, "chat-0")
	if !reportContext(s, "chat-0", run.ID, generation, contracts.ContextUsage{Tokens: contextCount(17000)}) {
		t.Fatal("waiting provider report rejected")
	}
	snapshot := snapshotForSessions(t, s, "chat-0")
	permissionID := ""
	for _, message := range snapshot.Messages["chat-0"] {
		for _, block := range message.Blocks {
			if block.Permission != nil && message.TurnID == run.ID {
				permissionID = block.Permission.ID
			}
		}
	}
	if permissionID == "" {
		t.Fatal("permission fixture was not produced")
	}
	if err := s.Answer(context.Background(), permissionID, "Allow once"); err != nil {
		t.Fatal(err)
	}
	if contextGeneration(s, "chat-0") != generation {
		t.Fatal("same-turn permission continuation created a new provider source")
	}
	if !reportContext(s, "chat-0", run.ID, generation, contracts.ContextUsage{Tokens: contextCount(16000)}) {
		t.Fatal("continuation stopped accepting the original provider's reports")
	}
}

func TestContextUsageProviderScopeIncludesTargetAndCWD(t *testing.T) {
	s := newTestServer(t, Options{})
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.state.Sessions[0]
	base := s.contextScopeLocked(session)
	other := session
	other.ModelID = "claude-sonnet-4-6"
	if s.contextScopeLocked(other) != base {
		t.Fatal("same provider model selection created a new provider-session scope")
	}
	other.DeviceID = "macbook"
	if s.contextScopeLocked(other) == base {
		t.Fatal("provider sessions on different target devices share a context source")
	}
	other = session
	other.CWD += "/another-checkout"
	if s.contextScopeLocked(other) == base {
		t.Fatal("provider sessions in different working directories share a context source")
	}
}

func TestContextUsageHostCompactionKeepsMessageAccounting(t *testing.T) {
	s := newTestServer(t, Options{Instant: true})
	events, err := s.Subscribe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	before := snapshotForSessions(t, s, "chat-4")
	run, err := s.Send(context.Background(), contracts.SendRequest{SessionID: "chat-4", Text: "Inspect keyboard navigation", ModelID: "claude-opus-4-6"})
	if err != nil {
		t.Fatal(err)
	}
	after := await(t, events, func(snapshot contracts.Snapshot) bool {
		for _, candidate := range snapshot.Runs {
			if candidate.ID == run.ID {
				return candidate.Status == "completed" && snapshot.ContextUsage["chat-4"].Tokens != nil && *snapshot.ContextUsage["chat-4"].Tokens == 60000
			}
		}
		return false
	})
	afterDetail := snapshotForSessions(t, s, "chat-4")
	if *after.ContextUsage["chat-4"].Tokens >= *before.ContextUsage["chat-4"].Tokens {
		t.Fatal("host compaction did not publish a decreased absolute occupancy")
	}
	for i, message := range before.Messages["chat-4"] {
		if !reflect.DeepEqual(message.Usage, afterDetail.Messages["chat-4"][i].Usage) {
			t.Fatal("compaction rewrote historical token accounting")
		}
	}
	last := afterDetail.Messages["chat-4"][len(afterDetail.Messages["chat-4"])-1]
	if last.Usage.InputTokens != max(1, len([]rune("Inspect keyboard navigation"))/4)+128 || last.Usage.ContextLimit != 200000 || uint64(last.Usage.ContextTokens) == *after.ContextUsage["chat-4"].Tokens {
		t.Fatalf("legacy message accounting was replaced by host context: %+v", last.Usage)
	}
}

func TestContextUsageDeleteAndStoppedRunRejectReports(t *testing.T) {
	s := newTestServer(t, Options{TokenInterval: time.Hour})
	run := heldContextRun(t, s, "chat-0", "claude-opus-4-6")
	generation := contextGeneration(s, "chat-0")
	if err := s.Stop(context.Background(), "chat-0"); err != nil {
		t.Fatal(err)
	}
	if reportContext(s, "chat-0", run.ID, generation, contracts.ContextUsage{Tokens: contextCount(4)}) {
		t.Fatal("cancelled run's host report accepted")
	}
	if err := s.Execute(context.Background(), contracts.Command{Kind: "session.delete", ID: "chat-0"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := contextSnapshot(t, s).ContextUsage["chat-0"]; ok {
		t.Fatal("deleted session retained its public context snapshot")
	}
	s.mu.Lock()
	_, remains := s.contextSessions["chat-0"]
	s.mu.Unlock()
	if remains || reportContext(s, "chat-0", run.ID, generation, contracts.ContextUsage{Tokens: contextCount(4)}) {
		t.Fatal("deleted session retained its host or was resurrected by a late report")
	}
}

func TestContextUsagePersistenceAndOldState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "desktop.json")
	s := newTestServer(t, Options{StatePath: path, TokenInterval: time.Hour})
	run := heldContextRun(t, s, "chat-0", "claude-opus-4-6")
	s.advanceRun("chat-0", run.ID, "Done", true)
	if !reportContext(s, "chat-0", run.ID, contextGeneration(s, "chat-0"), contracts.ContextUsage{Tokens: contextCount(17700)}) {
		t.Fatal("host report rejected")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	restored := newTestServer(t, Options{StatePath: path, TokenInterval: time.Hour})
	snapshot := contextSnapshot(t, restored)
	if *snapshot.ContextUsage["chat-0"].Tokens != 17700 || snapshot.ContextUsage["chat-2"].Tokens != nil || *snapshot.ContextUsage["chat-3"].Tokens != 0 {
		t.Fatal("optional context counts did not survive persistence")
	}
	resumed := heldContextRun(t, restored, "chat-0", "claude-sonnet-4-6")
	restored.mu.Lock()
	restored.publishLocked("message.delta", "chat-0", resumed.ID, "", "")
	restored.mu.Unlock()
	if *contextSnapshot(t, restored).ContextUsage["chat-0"].Tokens != 18660 {
		t.Fatal("restart lost the provider session's context baseline")
	}
	// Existing saves without this additive field stay unknown. Never reconstruct
	// host occupancy from old Message.Usage counters or today's fixture seed.
	snapshot.ContextUsage = nil
	legacyPath := filepath.Join(t.TempDir(), "legacy.json")
	data, err := json.Marshal(persisted{Snapshot: snapshot, Next: 3000})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	legacy := newTestServer(t, Options{StatePath: legacyPath})
	for _, usage := range contextSnapshot(t, legacy).ContextUsage {
		if usage.Tokens != nil || usage.Window != nil {
			t.Fatal("legacy accounting was promoted to host context")
		}
	}
}

func TestContextUsageTransactionalSavesPreserveProviderResume(t *testing.T) {
	for _, transaction := range []string{"attachment", "background"} {
		t.Run(transaction, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "desktop.json")
			s := newTestServer(t, Options{StatePath: path, TokenInterval: time.Hour})
			runA := heldContextRun(t, s, "chat-0", "claude-opus-4-6")
			s.advanceRun("chat-0", runA.ID, "Done", true)
			if !reportContext(s, "chat-0", runA.ID, contextGeneration(s, "chat-0"), contracts.ContextUsage{Tokens: contextCount(15000)}) {
				t.Fatal("provider A report rejected")
			}
			runB := heldContextRun(t, s, "chat-0", "gpt-5.4")
			s.advanceRun("chat-0", runB.ID, "Done", true)
			imagePath, _ := writeTestBackground(t, t.TempDir(), "image.png")
			if transaction == "attachment" {
				if _, err := s.ImportAttachments(context.Background(), contracts.AttachmentImportRequest{Paths: []string{imagePath}}); err != nil {
					t.Fatal(err)
				}
			} else if err := s.Execute(context.Background(), contracts.Command{Kind: "background.install", Path: imagePath}); err != nil {
				t.Fatal(err)
			}
			// Open the saved transaction before Close can overwrite it again.
			restored := newTestServer(t, Options{StatePath: path, TokenInterval: time.Hour})
			resumed := heldContextRun(t, restored, "chat-0", "claude-sonnet-4-6")
			if *contextSnapshot(t, restored).ContextUsage["chat-0"].Tokens != 15320 {
				t.Fatal("transaction dropped the earlier provider's resume scope")
			}
			restored.mu.Lock()
			restored.publishLocked("message.delta", "chat-0", resumed.ID, "", "")
			restored.mu.Unlock()
			if *contextSnapshot(t, restored).ContextUsage["chat-0"].Tokens != 15960 {
				t.Fatal("transaction lost provider A's independent host baseline")
			}
		})
	}
}

func TestContextUsageLiveFixtureReceivesHostReports(t *testing.T) {
	s := newTestServer(t, Options{Scenario: "live", Instant: true})
	events, err := s.Subscribe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	after := await(t, events, func(snapshot contracts.Snapshot) bool {
		for _, run := range snapshot.Runs {
			if run.ID == "live-turn" {
				return run.Status == "completed" && snapshot.ContextUsage["chat-0"].Tokens != nil && *snapshot.ContextUsage["chat-0"].Tokens > 42000
			}
		}
		return false
	})
	if usage := after.ContextUsage["chat-0"]; !usage.HasWindow() || *usage.Tokens <= 42000 {
		t.Fatal("live playback retained seed context instead of host reports")
	}
}
