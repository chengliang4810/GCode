package workflow

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gcode/internal/contracts"
)

func actor(id, state string) contracts.WorkflowActor {
	return contracts.WorkflowActor{ID: id, Name: id, Status: state}
}

func TestBuildRosterKeepsSmallCastsWhole(t *testing.T) {
	actors := []contracts.WorkflowActor{
		actor("a1", contracts.WorkflowActorDone),
		actor("a2", contracts.WorkflowActorRunning),
		actor("a3", contracts.WorkflowActorPending),
	}
	roster := BuildRoster(actors)
	if len(roster.Entries) != 3 {
		t.Fatalf("expected three entries, got %d", len(roster.Entries))
	}
	if roster.Overflow != 0 {
		t.Fatalf("expected no overflow, got %d", roster.Overflow)
	}
	if roster.Pending != 1 {
		t.Fatalf("expected one pending, got %d", roster.Pending)
	}
}

func TestBuildRosterBoundsLargeCasts(t *testing.T) {
	actors := []contracts.WorkflowActor{
		actor("a1", contracts.WorkflowActorDone),
		actor("a2", contracts.WorkflowActorDone),
		actor("a3", contracts.WorkflowActorDone),
		actor("a4", contracts.WorkflowActorDone),
		actor("a5", contracts.WorkflowActorDone),
		actor("a6", contracts.WorkflowActorDone),
		actor("a7", contracts.WorkflowActorDone),
	}
	roster := BuildRoster(actors)
	if len(roster.Entries) != RosterFixedSlots {
		t.Fatalf("expected %d fixed slots, got %d", RosterFixedSlots, len(roster.Entries))
	}
	if roster.Overflow != 2 {
		t.Fatalf("expected two out of table instances, got %d", roster.Overflow)
	}
	if roster.Total != 7 {
		t.Fatalf("expected the total to count every instance, got %d", roster.Total)
	}
}

func TestBuildRosterPrefersAskingThenRunningThenFailed(t *testing.T) {
	actors := []contracts.WorkflowActor{
		actor("done-1", contracts.WorkflowActorDone),
		actor("done-2", contracts.WorkflowActorDone),
		actor("done-3", contracts.WorkflowActorDone),
		actor("failed-1", contracts.WorkflowActorFailed),
		actor("running-1", contracts.WorkflowActorRunning),
		actor("asking-1", contracts.WorkflowActorAsking),
		actor("pending-1", contracts.WorkflowActorPending),
	}
	roster := BuildRoster(actors)
	want := []string{"asking-1", "running-1", "failed-1", "done-1", "done-2"}
	if len(roster.Entries) != len(want) {
		t.Fatalf("expected %d entries, got %d", len(want), len(roster.Entries))
	}
	for i, id := range want {
		if roster.Entries[i].ActorID != id {
			t.Fatalf("entry %d: expected %q, got %q", i, id, roster.Entries[i].ActorID)
		}
	}
	if roster.Overflow != 2 {
		t.Fatalf("expected two out of table instances, got %d", roster.Overflow)
	}
	if roster.Pending != 1 {
		t.Fatalf("expected the unsettled out of table instance to count as pending, got %d", roster.Pending)
	}
}

func TestBuildRosterKeepsParticipantOrderInsideAGroup(t *testing.T) {
	actors := []contracts.WorkflowActor{
		actor("r1", contracts.WorkflowActorRunning),
		actor("r2", contracts.WorkflowActorRunning),
		actor("r3", contracts.WorkflowActorRunning),
		actor("r4", contracts.WorkflowActorRunning),
		actor("r5", contracts.WorkflowActorRunning),
		actor("r6", contracts.WorkflowActorRunning),
		actor("r7", contracts.WorkflowActorRunning),
	}
	roster := BuildRoster(actors)
	for i, id := range []string{"r1", "r2", "r3", "r4", "r5"} {
		if roster.Entries[i].ActorID != id {
			t.Fatalf("entry %d: expected %q, got %q", i, id, roster.Entries[i].ActorID)
		}
	}
}

func TestPhaseStatePrefersRunningWork(t *testing.T) {
	actors := []contracts.WorkflowActor{
		actor("a1", contracts.WorkflowActorDone),
		actor("a2", contracts.WorkflowActorRunning),
	}
	if state := PhaseState(actors); state != contracts.WorkflowPhaseRunning {
		t.Fatalf("expected running, got %q", state)
	}
	observed, settled := PhaseCounts(actors)
	if observed != 2 || settled != 1 {
		t.Fatalf("expected 2 observed and 1 settled, got %d and %d", observed, settled)
	}
}

func TestConfigureAndResumeRules(t *testing.T) {
	running := contracts.WorkflowRun{Status: contracts.WorkflowStatusRunning}
	if !CanConfigure(running) {
		t.Fatal("a running run accepts a settings change")
	}
	completed := contracts.WorkflowRun{Status: contracts.WorkflowStatusCompleted}
	if CanConfigure(completed) {
		t.Fatal("a completed run does not accept a settings change")
	}
	stopped := contracts.WorkflowRun{Status: contracts.WorkflowStatusStopped, StopReason: contracts.WorkflowStopUser, Resumable: true}
	if CanResume(stopped) != true {
		t.Fatal("a user stopped run is resumable")
	}
	superseded := contracts.WorkflowRun{Status: contracts.WorkflowStatusStopped, StopReason: contracts.WorkflowStopSuperseded, Resumable: true}
	if CanResume(superseded) {
		t.Fatal("a superseded run is not resumable")
	}
}

func TestConcurrencyHasOnlyALowerBound(t *testing.T) {
	if NormalizeConcurrency(0) != 1 {
		t.Fatal("concurrency clamps up to one")
	}
	if NormalizeConcurrency(-4) != 1 {
		t.Fatal("concurrency clamps up to one")
	}
	if NormalizeConcurrency(64) != 64 {
		t.Fatal("concurrency is not capped")
	}
}

func TestSettingsOutcomeSeparatesRetuneFromSuccessor(t *testing.T) {
	retune := contracts.WorkflowSettingsChange{RunID: "r1", HasConc: true, Concurrency: 4}
	if ChangeOutcome(retune) != OutcomeRetune {
		t.Fatal("a concurrency-only change retunes the same run")
	}
	successor := contracts.WorkflowSettingsChange{RunID: "r1", HasModel: true, ModelID: "m2"}
	if ChangeOutcome(successor) != OutcomeSuccessor {
		t.Fatal("a model change creates a successor run")
	}
	empty := contracts.WorkflowSettingsChange{RunID: "r1"}
	if ChangeOutcome(empty) != OutcomeNone {
		t.Fatal("an empty change changes nothing")
	}
}

func TestNullModelFallsBackToTheSessionModel(t *testing.T) {
	change := contracts.WorkflowSettingsChange{RunID: "r1", HasModel: true}
	model, changed := ResolveModel(change, "session-model")
	if !changed || model != "session-model" {
		t.Fatalf("expected the session model, got %q", model)
	}
	if ModelChanged(change, "session-model", "session-model") {
		t.Fatal("choosing the session model is not a model change")
	}
	if !ModelChanged(change, "other-model", "session-model") {
		t.Fatal("leaving a different model is a model change")
	}
}

func TestArtifactProjectionsStayBounded(t *testing.T) {
	artifacts := make([]contracts.WorkflowArtifact, 0, 8)
	for i := range 8 {
		artifacts = append(artifacts, contracts.WorkflowArtifact{ID: "a", Name: string(rune('a' + i)), Order: i, Version: 1})
	}
	card := CardArtifacts(artifacts)
	if len(card.Shown) != CardArtifactLimit || card.Overflow != 5 {
		t.Fatalf("expected %d links and %d extra, got %d and %d", CardArtifactLimit, 5, len(card.Shown), card.Overflow)
	}
	completion := CompletionArtifacts(artifacts)
	if len(completion.Shown) != CompletionOverflowRows || completion.Overflow != 3 {
		t.Fatalf("expected %d rows and %d extra, got %d and %d", CompletionOverflowRows, 3, len(completion.Shown), completion.Overflow)
	}
	single := CompletionArtifacts(artifacts[:1])
	if len(single.Shown) != 1 || single.Overflow != 0 {
		t.Fatalf("expected one row without overflow, got %d and %d", len(single.Shown), single.Overflow)
	}
}

func TestPrimaryArtifactPrefersExplicitPrimary(t *testing.T) {
	artifacts := []contracts.WorkflowArtifact{
		{Name: "first", Order: 0},
		{Name: "second", Order: 1, Primary: true},
	}
	primary, ok := PrimaryArtifact(artifacts)
	if !ok || primary.Name != "second" {
		t.Fatalf("expected the explicit primary, got %q", primary.Name)
	}
	only, ok := PrimaryArtifact(artifacts[:1])
	if !ok || only.Name != "first" {
		t.Fatal("a single delivery is the primary on its own")
	}
}

func TestJournalRoundTripAndReplay(t *testing.T) {
	dir := t.TempDir()
	path := JournalPath(dir, "run-1")
	started, err := EncodePayload(RunStartedPayload{
		Name:        "Build the thing",
		Concurrency: 2,
		Program: contracts.WorkflowProgram{Phases: []contracts.WorkflowPhaseSpec{
			{ID: "phase-1", Name: "Plan", Instances: []contracts.WorkflowActorSpec{{ID: "actor-1", Name: "Planner"}}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	settled, err := EncodePayload(ActorPayload{ActorID: "actor-1", PhaseID: "phase-1", Status: contracts.WorkflowActorDone, Result: "done"})
	if err != nil {
		t.Fatal(err)
	}
	records := []JournalRecord{
		{Sequence: 1, Kind: JournalRunStarted, At: time.Now(), RunID: "run-1", Data: started},
		{Sequence: 2, Kind: JournalActorSettled, At: time.Now(), RunID: "run-1", Data: settled},
	}
	if err := AppendRecords(path, records); err != nil {
		t.Fatal(err)
	}
	read, damaged, err := ReadJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if damaged != 0 || len(read) != 2 {
		t.Fatalf("expected two healthy records, got %d with %d damaged", len(read), damaged)
	}
	run, err := Replay(read)
	if err != nil {
		t.Fatal(err)
	}
	if run.Name != "Build the thing" || run.Concurrency != 2 {
		t.Fatalf("unexpected run %+v", run)
	}
	if len(run.Actors) != 1 || run.Actors[0].Status != contracts.WorkflowActorDone {
		t.Fatalf("expected one settled actor, got %+v", run.Actors)
	}
}

func TestJournalSkipsDamagedLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run.jsonl")
	line := `{"sequence":1,"kind":"run.started","at":"2026-01-01T00:00:00Z","runId":"run-1","data":{}}` + "\n"
	if err := os.WriteFile(path, []byte(line+"{not json}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	records, damaged, err := ReadJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if damaged != 1 || len(records) != 1 {
		t.Fatalf("expected one record and one damaged line, got %d and %d", len(records), damaged)
	}
}
