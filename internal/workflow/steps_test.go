package workflow

import (
	"encoding/json"
	"testing"
	"time"

	"gcode/internal/contracts"
)

func stepRecord(sequence uint64, kind string, payload any) JournalRecord {
	record := JournalRecord{Sequence: sequence, Kind: kind, At: time.Date(2026, 11, 23, 13, 0, int(sequence), 0, time.UTC)}
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			panic(err)
		}
		record.Data = data
	}
	return record
}

func TestBuildStepsGroupsByPhaseInSequenceOrder(t *testing.T) {
	records := []JournalRecord{
		stepRecord(1, JournalRunStarted, RunStartedPayload{Name: "Review"}),
		stepRecord(2, JournalPhaseEntered, PhasePayload{PhaseID: "plan", Name: "Plan"}),
		stepRecord(3, JournalActorSettled, ActorPayload{ActorID: "a-1", PhaseID: "plan", Name: "Reviewer", Status: "done"}),
		stepRecord(4, JournalPhaseEntered, PhasePayload{PhaseID: "build", Name: "Build"}),
		stepRecord(5, JournalArtifact, ArtifactPayload{Artifact: mustArtifact("report")}),
		stepRecord(6, JournalRunSettled, SettlePayload{Status: "completed"}),
	}
	groups := BuildSteps(records)
	if len(groups) != 3 {
		t.Fatalf("run facts and two phases: %+v", groups)
	}
	if groups[0].PhaseID != "" {
		t.Fatalf("the run's own facts come first: %+v", groups[0])
	}
	// 产物载荷里没有阶段：它归运行本身，不替它猜一个阶段。
	if len(groups[0].Steps) != 3 || groups[0].Steps[0].Kind != JournalRunStarted {
		t.Fatalf("the run group keeps its steps in order: %+v", groups[0])
	}
	if groups[1].PhaseID != "plan" || len(groups[1].Steps) != 2 {
		t.Fatalf("the plan phase holds its own steps: %+v", groups[1])
	}
	if groups[2].PhaseID != "build" {
		t.Fatalf("groups keep the order their phase appears: %+v", groups)
	}
}

func TestBuildStepsNamesWhatHappened(t *testing.T) {
	records := []JournalRecord{
		stepRecord(1, JournalPhaseEntered, PhasePayload{PhaseID: "plan", Name: "Plan"}),
		stepRecord(2, JournalActorStarted, ActorPayload{ActorID: "a-1", PhaseID: "plan", Name: "Reviewer"}),
		stepRecord(3, JournalQuestionAsked, QuestionPayload{QuestionID: "q-1", ActorID: "a-1", Text: "Which directory?"}),
		stepRecord(4, JournalHoleWaiting, HolePayload{HoleID: "h-1", PhaseID: "plan", Title: "Missing target"}),
		stepRecord(5, JournalArtifact, ArtifactPayload{Artifact: mustArtifact("report")}),
		stepRecord(6, JournalRunRetuned, RetunePayload{Concurrency: 3, HasConc: true}),
		stepRecord(7, JournalRunStopped, SettlePayload{Status: "stopped", StopReason: "user"}),
	}
	details := map[string]string{}
	for _, group := range BuildSteps(records) {
		for _, step := range group.Steps {
			details[step.Kind] = step.Detail
		}
	}
	for kind, want := range map[string]string{
		JournalPhaseEntered:  "Plan",
		JournalActorStarted:  "Reviewer",
		JournalQuestionAsked: "Which directory?",
		JournalHoleWaiting:   "Missing target",
		JournalArtifact:      "report",
		JournalRunRetuned:    "3",
		JournalRunStopped:    "user",
	} {
		if details[kind] != want {
			t.Fatalf("%s says %q, want %q", kind, details[kind], want)
		}
	}
}

func TestBuildStepsToleratesADamagedPayload(t *testing.T) {
	records := []JournalRecord{
		{Sequence: 1, Kind: JournalPhaseEntered, Data: json.RawMessage("{not json")},
		{Sequence: 2, Kind: JournalRunSettled},
	}
	groups := BuildSteps(records)
	if len(groups) != 1 || len(groups[0].Steps) != 2 {
		t.Fatalf("a broken payload still becomes a step: %+v", groups)
	}
	if groups[0].Steps[0].Detail != "" {
		t.Fatalf("nothing is invented for a broken payload: %+v", groups[0].Steps[0])
	}
}

func TestBuildStepsHandlesAnEmptyJournal(t *testing.T) {
	if groups := BuildSteps(nil); len(groups) != 0 {
		t.Fatalf("no records means no steps: %+v", groups)
	}
}

func mustArtifact(name string) contracts.WorkflowArtifact {
	return contracts.WorkflowArtifact{ID: "artifact-1", Name: name, Version: 1}
}
