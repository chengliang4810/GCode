package engine

import (
	"context"
	"net/http"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/storage"
)

func workflowTestProgram() contracts.WorkflowProgram {
	return contracts.WorkflowProgram{Phases: []contracts.WorkflowPhaseSpec{{
		ID: "phase-1", Name: "Plan",
		Instances: []contracts.WorkflowActorSpec{{ID: "actor-1", Name: "Planner", Prompt: "Plan the work"}},
	}}}
}

// waitWorkflowState 等待工作流快照满足条件，超时即失败。
func waitWorkflowState(t *testing.T, core *Engine, ready func(contracts.Snapshot) bool) contracts.Snapshot {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		state, err := core.Snapshot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if ready(state) {
			return state
		}
		time.Sleep(20 * time.Millisecond)
	}
	state, _ := core.Snapshot(context.Background())
	t.Fatalf("workflow state did not settle: %+v", state.WorkflowRuns)
	return state
}

func workflowRunSettled(state contracts.Snapshot) bool {
	if len(state.WorkflowRuns) == 0 {
		return false
	}
	for _, run := range state.WorkflowRuns {
		if run.Status == contracts.WorkflowStatusPending || run.Status == contracts.WorkflowStatusRunning {
			return false
		}
	}
	return true
}

func runWorkflow(t *testing.T, core *Engine, program contracts.WorkflowProgram) contracts.Snapshot {
	t.Helper()
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "workflow.run",
		Workflow: &contracts.WorkflowCommand{Run: &contracts.WorkflowRunRequest{
			Name: "Fixture workflow", Program: program, ModelID: "goal/model", Concurrency: 2,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	return waitWorkflowState(t, core, workflowRunSettled)
}

func TestWorkflowRunExecutesActorsAndSettles(t *testing.T) {
	core, _ := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = subagentRequest(t, r)
		goalResponse(w, model.Chat, "Plan complete <<workflow:artifact>>plan|the plan<<workflow:artifact>>", nil, true)
	}))
	state := runWorkflow(t, core, workflowTestProgram())
	run := state.WorkflowRuns[0]
	if run.Status != contracts.WorkflowStatusCompleted {
		t.Fatalf("expected a completed run, got %q (%s)", run.Status, run.Error)
	}
	if len(run.Actors) != 1 || run.Actors[0].Status != contracts.WorkflowActorDone {
		t.Fatalf("expected one settled instance, got %+v", run.Actors)
	}
	if run.Actors[0].SessionID == "" {
		t.Fatal("a settled instance keeps its session for the actor view")
	}
	if len(run.Artifacts) != 1 || run.Artifacts[0].Name != "plan" || !run.Artifacts[0].Primary {
		t.Fatalf("expected one primary artifact, got %+v", run.Artifacts)
	}
	if run.Result != "plan" {
		t.Fatalf("expected the artifact to be the run result, got %q", run.Result)
	}
	for _, phase := range run.Phases {
		if phase.Status != contracts.WorkflowPhaseDone {
			t.Fatalf("expected a settled phase, got %+v", phase)
		}
	}
}

func TestWorkflowStopLeavesRunResumable(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	core, _ := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = subagentRequest(t, r)
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		goalResponse(w, model.Chat, "late reply", nil, true)
	}))
	defer close(release)
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "workflow.run",
		Workflow: &contracts.WorkflowCommand{Run: &contracts.WorkflowRunRequest{
			Name: "Stoppable", Program: workflowTestProgram(), ModelID: "goal/model", Concurrency: 1,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("the fixture model never received the instance request")
	}
	state, _ := core.Snapshot(context.Background())
	runID := state.WorkflowRuns[0].ID
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "workflow.stop", Workflow: &contracts.WorkflowCommand{RunID: runID},
	}); err != nil {
		t.Fatal(err)
	}
	state = waitWorkflowState(t, core, workflowRunSettled)
	run := state.WorkflowRuns[0]
	if run.Status != contracts.WorkflowStatusStopped || run.StopReason != contracts.WorkflowStopUser {
		t.Fatalf("expected a user stop, got %+v", run)
	}
	if !run.Resumable {
		t.Fatal("a user stop keeps the resume entry")
	}
}

func TestWorkflowRetuneKeepsConcurrencyOnTheSameRun(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	core, _ := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = subagentRequest(t, r)
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		goalResponse(w, model.Chat, "done", nil, true)
	}))
	defer close(release)
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "workflow.run",
		Workflow: &contracts.WorkflowCommand{Run: &contracts.WorkflowRunRequest{
			Name: "Retunable", Program: workflowTestProgram(), ModelID: "goal/model", Concurrency: 1,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("the fixture model never received the instance request")
	}
	state, _ := core.Snapshot(context.Background())
	runID, count := state.WorkflowRuns[0].ID, len(state.WorkflowRuns)
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "workflow.retune",
		Workflow: &contracts.WorkflowCommand{Settings: &contracts.WorkflowSettingsChange{
			RunID: runID, HasConc: true, Concurrency: 4,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	state, _ = core.Snapshot(context.Background())
	if len(state.WorkflowRuns) != count {
		t.Fatal("a concurrency change keeps the same run")
	}
	if state.WorkflowRuns[0].Concurrency != 4 {
		t.Fatalf("expected the live concurrency, got %d", state.WorkflowRuns[0].Concurrency)
	}
}

func TestWorkflowModelChangeCreatesSuccessorRun(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	core, _ := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = subagentRequest(t, r)
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		goalResponse(w, model.Chat, "done", nil, true)
	}))
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "workflow.run",
		Workflow: &contracts.WorkflowCommand{Run: &contracts.WorkflowRunRequest{
			Name: "Model switch", Program: workflowTestProgram(), ModelID: "goal/model", Concurrency: 1,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("the fixture model never received the instance request")
	}
	state, _ := core.Snapshot(context.Background())
	runID := state.WorkflowRuns[0].ID
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "workflow.retune",
		Workflow: &contracts.WorkflowCommand{Settings: &contracts.WorkflowSettingsChange{
			RunID: runID, HasModel: true, ModelID: "goal/model",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	state, _ = core.Snapshot(context.Background())
	if len(state.WorkflowRuns) != 2 {
		t.Fatalf("expected a successor run, got %d runs", len(state.WorkflowRuns))
	}
	previous := state.WorkflowRuns[0]
	if previous.ID != runID || previous.Status != contracts.WorkflowStatusStopped || previous.StopReason != contracts.WorkflowStopModel {
		t.Fatalf("expected the previous run to be superseded, got %+v", previous)
	}
	if previous.Resumable {
		t.Fatal("a superseded run does not offer resume")
	}
	successor := state.WorkflowRuns[1]
	if successor.Status != contracts.WorkflowStatusPending && successor.Status != contracts.WorkflowStatusRunning {
		t.Fatalf("expected a live successor, got %q", successor.Status)
	}
	close(release)
	state = waitWorkflowState(t, core, func(state contracts.Snapshot) bool {
		return state.WorkflowRuns[1].Status == contracts.WorkflowStatusCompleted
	})
	_ = state
}

func TestWorkflowSaveRejectsUnsafeName(t *testing.T) {
	core, _ := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = subagentRequest(t, r)
		goalResponse(w, model.Chat, "done", nil, true)
	}))
	for _, name := range []string{"bad name!", "", "../escape", string(make([]byte, 65))} {
		if err := core.Execute(context.Background(), contracts.Command{
			Kind: "workflow.save",
			Workflow: &contracts.WorkflowCommand{Definition: &contracts.WorkflowDefinitionSave{
				Name: name, Scope: "global", Program: workflowTestProgram(),
			}},
		}); err == nil {
			t.Fatalf("expected %q to be rejected", name)
		}
	}
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "workflow.save",
		Workflow: &contracts.WorkflowCommand{Definition: &contracts.WorkflowDefinitionSave{
			Name: "nightly_audit-2.1", Scope: "global", Program: workflowTestProgram(),
		}},
	}); err != nil {
		t.Fatal(err)
	}
	state, _ := core.Snapshot(context.Background())
	if len(state.WorkflowDefinitions) != 1 || state.WorkflowDefinitions[0].Name != "nightly_audit-2.1" {
		t.Fatalf("expected one saved definition, got %+v", state.WorkflowDefinitions)
	}
}

func TestWorkflowLaunchRunsSavedDefinition(t *testing.T) {
	core, _ := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = subagentRequest(t, r)
		goalResponse(w, model.Chat, "saved run", nil, true)
	}))
	definition := contracts.WorkflowDefinitionSave{
		Name: "saved-flow", Scope: "global", Program: workflowTestProgram(),
		Arguments: []contracts.WorkflowArgument{{Name: "target", Type: "string", Required: true}},
	}
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "workflow.save", Workflow: &contracts.WorkflowCommand{Definition: &definition},
	}); err != nil {
		t.Fatal(err)
	}
	state, _ := core.Snapshot(context.Background())
	definitionID := state.WorkflowDefinitions[0].ID
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "workflow.launch",
		Workflow: &contracts.WorkflowCommand{Run: &contracts.WorkflowRunRequest{
			DefinitionID: definitionID, ModelID: "goal/model", Concurrency: 1,
		}},
	}); err == nil {
		t.Fatal("a required argument must be supplied")
	}
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "workflow.launch",
		Workflow: &contracts.WorkflowCommand{Run: &contracts.WorkflowRunRequest{
			DefinitionID: definitionID, ModelID: "goal/model", Concurrency: 1,
			Args: []contracts.WorkflowArgValue{{Name: "target", Kind: "string", Text: "src"}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	state = waitWorkflowState(t, core, workflowRunSettled)
	run := state.WorkflowRuns[0]
	if run.DefinitionID != definitionID || run.Name != "saved-flow" {
		t.Fatalf("expected the saved definition to run, got %+v", run)
	}
	if run.Status != contracts.WorkflowStatusCompleted {
		t.Fatalf("expected completion, got %q (%s)", run.Status, run.Error)
	}
}

func TestWorkflowQuestionEscalatesToMainAgent(t *testing.T) {
	var calls int
	released := make(chan struct{})
	core, _ := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = subagentRequest(t, r)
		calls++
		if calls == 1 {
			goalResponse(w, model.Chat, "<<workflow:question>>Which directory?<<workflow:question>>", nil, true)
			return
		}
		goalResponse(w, model.Chat, "Handled /tmp/target", nil, true)
	}))
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "workflow.run",
		Workflow: &contracts.WorkflowCommand{Run: &contracts.WorkflowRunRequest{
			Name: "Asking", Program: workflowTestProgram(), ModelID: "goal/model", Concurrency: 1,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	state := waitWorkflowState(t, core, func(state contracts.Snapshot) bool {
		return len(state.WorkflowRuns) > 0 && len(state.WorkflowRuns[0].Questions) > 0
	})
	run := state.WorkflowRuns[0]
	question := run.Questions[0]
	if question.Text != "Which directory?" || question.Status != "asked" {
		t.Fatalf("expected an escalation, got %+v", question)
	}
	if run.Actors[0].Status != contracts.WorkflowActorAsking {
		t.Fatalf("expected the instance to wait on the main agent, got %q", run.Actors[0].Status)
	}
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "workflow.resolve",
		Workflow: &contracts.WorkflowCommand{Question: &contracts.WorkflowQuestionAnswer{
			RunID: run.ID, ID: question.ID, Text: "/tmp/target",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	close(released)
	state = waitWorkflowState(t, core, workflowRunSettled)
	run = state.WorkflowRuns[0]
	if run.Status != contracts.WorkflowStatusCompleted {
		t.Fatalf("expected completion after the answer, got %q (%s)", run.Status, run.Error)
	}
	if run.Questions[0].Status != "resolved" || run.Questions[0].Answer != "/tmp/target" {
		t.Fatalf("expected the recorded answer, got %+v", run.Questions[0])
	}
}

// 定义与运行要跨重启：中枢里那一份是用户攒下的事实，运行过的历史也要在。
func TestWorkflowDefinitionsAndRunsSurviveRestart(t *testing.T) {
	core, _ := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = subagentRequest(t, r)
		goalResponse(w, model.Chat, "durable workflow", nil, true)
	}))
	definition := contracts.WorkflowDefinitionSave{
		Name: "durable-flow", Scope: "global", Program: workflowTestProgram(),
		Script: `{"phases":[]}`,
	}
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "workflow.save", Workflow: &contracts.WorkflowCommand{Definition: &definition},
	}); err != nil {
		t.Fatal(err)
	}
	state, _ := core.Snapshot(context.Background())
	definitionID := state.WorkflowDefinitions[0].ID
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "workflow.launch",
		Workflow: &contracts.WorkflowCommand{Run: &contracts.WorkflowRunRequest{
			DefinitionID: definitionID, ModelID: "goal/model", Concurrency: 1,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	state = waitWorkflowState(t, core, workflowRunSettled)
	if len(state.WorkflowRuns) != 1 || state.WorkflowRuns[0].Status != contracts.WorkflowStatusCompleted {
		t.Fatalf("the run settled before the restart: %+v", state.WorkflowRuns)
	}
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := storage.Open(storage.Options{Directory: core.storage.ConfigDirectory()})
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restored, err := reopened.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(restored.WorkflowDefinitions) != 1 || restored.WorkflowDefinitions[0].ID != definitionID {
		t.Fatalf("the saved definition survives the restart: %+v", restored.WorkflowDefinitions)
	}
	if len(restored.WorkflowRuns) != 1 || restored.WorkflowRuns[0].DefinitionID != definitionID {
		t.Fatalf("the run history survives the restart: %+v", restored.WorkflowRuns)
	}
	if restored.WorkflowRuns[0].Status != contracts.WorkflowStatusCompleted {
		t.Fatalf("a settled run stays settled: %+v", restored.WorkflowRuns[0])
	}
}

// 日志读取只对这台引擎认识的运行开放：运行身份会拼成文件路径，未知身份不能变成路径。
func TestWorkflowJournalReadsOneRunsOwnLog(t *testing.T) {
	core, _ := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = subagentRequest(t, r)
		goalResponse(w, model.Chat, "journal step", nil, true)
	}))
	state, err := core.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err = core.Execute(context.Background(), contracts.Command{
		Kind: "workflow.run",
		Workflow: &contracts.WorkflowCommand{Run: &contracts.WorkflowRunRequest{
			Name: "Journal run", Program: workflowTestProgram(), ModelID: "goal/model", Concurrency: 1,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	state = waitWorkflowState(t, core, workflowRunSettled)
	runID := state.WorkflowRuns[0].ID
	journal, err := core.WorkflowJournal(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(journal.Records) == 0 {
		t.Fatal("the run wrote its own log")
	}
	kinds := map[string]bool{}
	for _, record := range journal.Records {
		kinds[record.Kind] = true
	}
	for _, want := range []string{"run.started", "phase.entered", "run.settled"} {
		if !kinds[want] {
			t.Fatalf("the log holds %q: %+v", want, journal.Records)
		}
	}
	if journal.Damaged != 0 {
		t.Fatalf("a clean log has no damaged lines: %d", journal.Damaged)
	}
	if _, err = core.WorkflowJournal(context.Background(), "workflow-run-unknown"); err == nil {
		t.Fatal("an unknown run is refused instead of becoming a path")
	}
	if _, err = core.WorkflowJournal(context.Background(), ""); err == nil {
		t.Fatal("an empty run id is refused")
	}
}
