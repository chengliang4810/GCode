package desktop

import (
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/workflow"
	"github.com/egoist/mygo/ui"
)

// hubDefinition is one saved definition the hub tests reuse.
func hubDefinition(id, name, scope, projectID string) contracts.WorkflowDefinition {
	return contracts.WorkflowDefinition{
		ID: id, Name: name, Scope: scope, ProjectID: projectID,
		Description: "Reviews a change and reports back",
		Script:      `{"phases":[]}`,
		Program:     contracts.WorkflowProgram{Phases: []contracts.WorkflowPhaseSpec{{ID: "plan", Name: "Plan"}}},
		UpdatedAt:   time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC),
	}
}

// hubApp builds an app with two saved definitions and the projects they live in.
func hubApp(t *testing.T) *App {
	t.Helper()
	app := testApp(t, "default")
	app.palette = design.Palette("rcode-dark", true)
	app.state.Projects = []contracts.Project{
		{ID: "proj-a", Name: "Alpha"},
		{ID: "proj-b", Name: "Beta", DeviceID: "cloud-1"},
	}
	app.projectID = "proj-a"
	app.state.WorkflowDefinitions = []contracts.WorkflowDefinition{
		hubDefinition("def-a", "Review", workflow.SavedScopeProject, "proj-a"),
		hubDefinition("def-b", "Ship", workflow.SavedScopeProject, "proj-b"),
		hubDefinition("def-global", "Nightly", workflow.SavedScopeGlobal, ""),
	}
	return app
}

func hubTester(app *App) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.automationUI.Module = automationModuleWorkflows
		app.automationsView(c)
	}, 1000, 900)
}

func TestWorkflowHubListsEachProjectAndThisMachine(t *testing.T) {
	app := hubApp(t)
	tt := hubTester(app)
	tt.Frame()
	for _, text := range []string{
		"Workflows", "Alpha", "Beta", "This machine", "Current project", "Remote host",
		"Review", "Ship", "Nightly", "Never run",
	} {
		if !tt.HasText(text) {
			t.Fatalf("hub missing %q: %v", text, tt.Texts())
		}
	}
}

func TestWorkflowHubEmptiesItselfHonestly(t *testing.T) {
	app := testApp(t, "default")
	app.palette = design.Palette("rcode-dark", true)
	app.state.WorkflowDefinitions = nil
	tt := hubTester(app)
	tt.Frame()
	if !tt.HasText("No saved workflows yet") {
		t.Fatalf("an empty hub says so: %v", tt.Texts())
	}
	app.state.WorkflowDefinitions = []contracts.WorkflowDefinition{
		hubDefinition("def-a", "Review", workflow.SavedScopeGlobal, ""),
	}
	app.workflowUI.hub.query = "nothing matches"
	tt = hubTester(app)
	tt.Frame()
	if !tt.HasText("No workflows match this search") {
		t.Fatalf("an empty search says it is the search: %v", tt.Texts())
	}
}

func TestWorkflowHubDetailShowsTheDefinitionApartFromItsScript(t *testing.T) {
	app := hubApp(t)
	definition := app.state.WorkflowDefinitions[0]
	definition.Arguments = []contracts.WorkflowArgument{
		{Name: "target", Type: "string", Required: true, Description: "What to review"},
		{Name: "depth", Type: "number", Default: "2", HasDefault: true},
	}
	app.state.WorkflowDefinitions[0] = definition
	app.workflowUI.hub.selected = "def-a"
	tt := hubTester(app)
	tt.Frame()
	for _, text := range []string{
		"Review", "Definition", "History", "target", "What to review",
		"Required", "Default: 2", "Script", `{"phases":[]}`, "Alpha",
	} {
		if !tt.HasText(text) {
			t.Fatalf("definition page missing %q: %v", text, tt.Texts())
		}
	}
	if err := tt.Click("Back to workflows"); err != nil {
		t.Fatal(err)
	}
	if app.workflowUI.hub.selected != "" {
		t.Fatalf("back leaves the definition page: %q", app.workflowUI.hub.selected)
	}
}

func TestWorkflowHubHistoryListsTheDefinitionsOwnRuns(t *testing.T) {
	app := hubApp(t)
	base := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	app.state.WorkflowRuns = []contracts.WorkflowRun{
		{ID: "run-1", DefinitionID: "def-a", Name: "Review", Status: contracts.WorkflowStatusCompleted, StartedAt: base},
		{ID: "run-2", DefinitionID: "def-other", Name: "Other", Status: contracts.WorkflowStatusRunning, StartedAt: base},
	}
	app.workflowUI.hub.selected = "def-a"
	app.workflowUI.hub.tab = "history"
	tt := hubTester(app)
	tt.Frame()
	if !tt.HasText("Completed") {
		t.Fatalf("history shows the run state: %v", tt.Texts())
	}
	for _, text := range tt.Texts() {
		if text == "Running" {
			t.Fatalf("another definition's run is not in this history: %v", tt.Texts())
		}
	}
}

// 项目定义、没有参数：直接起，不弹空表单。
func TestWorkflowLaunchSkipsTheFormWhenNothingIsAsked(t *testing.T) {
	app := hubApp(t)
	definition := app.state.WorkflowDefinitions[0]
	app.startWorkflowLaunch(definition, nil)
	if app.workflowUI.hub.launch == nil {
		t.Fatal("a project definition launches without a form")
	}
	if app.workflowUI.hubLaunch == nil {
		t.Fatal("the launch is carried through the core")
	}
	if app.workflowUI.hubLaunch.request.DefinitionID != "def-a" {
		t.Fatalf("the launch names its definition: %+v", app.workflowUI.hubLaunch.request)
	}
	if app.workflowUI.hubLaunch.request.ProjectID != "proj-a" {
		t.Fatalf("a project definition keeps its project: %+v", app.workflowUI.hubLaunch.request)
	}
	if app.workflowUI.hubLaunch.request.Origin != contracts.WorkflowOriginHub {
		t.Fatalf("a hub launch says where it came from: %+v", app.workflowUI.hubLaunch.request)
	}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.workflowLaunchDialog(c)
	}, 900, 800)
	tt.Frame()
	if tt.HasText("Cancel") {
		t.Fatal("a direct launch does not flash a form")
	}
}

// 产物链接打开的是交付它的那一次运行，不是最近的那一次。
func TestWorkflowHubArtifactOpensTheRunThatDeliveredIt(t *testing.T) {
	app := hubApp(t)
	base := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	app.state.WorkflowRuns = []contracts.WorkflowRun{
		{
			ID: "run-delivered", DefinitionID: "def-a", Status: contracts.WorkflowStatusCompleted, StartedAt: base,
			Artifacts: []contracts.WorkflowArtifact{{ID: "art-1", Name: "report", Version: 1}},
		},
		{ID: "run-newer", DefinitionID: "def-a", Status: contracts.WorkflowStatusErrored, StartedAt: base.Add(time.Hour)},
	}
	runID, ok := app.workflowArtifactRun("def-a", "art-1")
	if !ok || runID != "run-delivered" {
		t.Fatalf("the artifact belongs to the run that published it, got %q", runID)
	}
}

// 全局定义：始终先选本机项目；远端项目不是候选。
func TestWorkflowLaunchAsksForALocalProjectOnAGlobalDefinition(t *testing.T) {
	app := hubApp(t)
	definition := app.state.WorkflowDefinitions[2]
	app.startWorkflowLaunch(definition, nil)
	if app.workflowUI.hub.launch == nil || app.workflowUI.hubLaunch != nil {
		t.Fatal("a global definition waits in the form")
	}
	choices := app.localProjectChoices()
	if len(choices) != 1 || choices[0].value != "proj-a" {
		t.Fatalf("only local projects are candidates: %+v", choices)
	}
}

// 有参数就弹表单，表单带上声明与默认值。
func TestWorkflowLaunchFormShowsDeclaredArguments(t *testing.T) {
	app := hubApp(t)
	definition := app.state.WorkflowDefinitions[0]
	definition.Arguments = []contracts.WorkflowArgument{
		{Name: "target", Type: "string", Required: true},
		{Name: "deep", Type: "boolean", Default: "true", HasDefault: true},
		{Name: "meta", Type: "json"},
	}
	app.state.WorkflowDefinitions[0] = definition
	app.startWorkflowLaunch(definition, nil)
	if app.workflowUI.hubLaunch != nil {
		t.Fatal("a definition with arguments waits for the form")
	}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.workflowLaunchDialog(c)
	}, 900, 800)
	tt.Frame()
	for _, text := range []string{"Run workflow", "target *", "deep", "meta"} {
		if !tt.HasText(text) {
			t.Fatalf("launch form missing %q: %v", text, tt.Texts())
		}
	}
}

// 表单里的每个字段都是同一个指针对面：打字不会被下一帧擦掉。
func TestWorkflowLaunchFormKeepsWhatIsTyped(t *testing.T) {
	app := hubApp(t)
	definition := app.state.WorkflowDefinitions[0]
	definition.Arguments = []contracts.WorkflowArgument{{Name: "target", Type: "string"}}
	app.state.WorkflowDefinitions[0] = definition
	app.startWorkflowLaunch(definition, nil)
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.workflowLaunchDialog(c)
	}, 900, 800)
	tt.Frame()
	label, ok := tt.Find("target")
	if !ok {
		t.Fatal("the field has a label")
	}
	// 标签下面就是输入框本身。
	tt.ClickAt(label.X+40, label.Y+label.H+18)
	tt.Frame()
	tt.Type("src/app.go")
	tt.Frame()
	state := app.workflowUI.hub.launch
	if got := *state.values["target"]; got != "src/app.go" {
		t.Fatalf("the field keeps what was typed, got %q", got)
	}
	// 打字之后，启动读到的是同一个值。
	app.launchWorkflow(state, definition)
	if app.workflowUI.hubLaunch == nil {
		t.Fatal("a filled form launches")
	}
	sent := app.workflowUI.hubLaunch.request.Args
	if len(sent) != 1 || sent[0].Text != "src/app.go" {
		t.Fatalf("the launch sends what the user typed: %+v", sent)
	}
}

// 每个字段在帧之间是同一个指针：框架靠它把打字留在原地。
func TestWorkflowLaunchFieldsAreStableBetweenFrames(t *testing.T) {
	app := hubApp(t)
	definition := app.state.WorkflowDefinitions[0]
	definition.Arguments = []contracts.WorkflowArgument{{Name: "target", Type: "string"}}
	app.state.WorkflowDefinitions[0] = definition
	app.startWorkflowLaunch(definition, nil)
	first := app.workflowUI.hub.launch.values["target"]
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.workflowLaunchDialog(c)
	}, 900, 800)
	tt.Frame()
	tt.Frame()
	if got := app.workflowUI.hub.launch.values["target"]; got != first {
		t.Fatal("the field pointer changed between frames")
	}
}

// 校验失败留在表单里，说清是哪一项。
func TestWorkflowLaunchStaysInTheFormWhenAValueIsRefused(t *testing.T) {
	app := hubApp(t)
	definition := app.state.WorkflowDefinitions[0]
	definition.Arguments = []contracts.WorkflowArgument{{Name: "target", Type: "string", Required: true}}
	app.state.WorkflowDefinitions[0] = definition
	app.startWorkflowLaunch(definition, nil)
	state := app.workflowUI.hub.launch
	if state == nil {
		t.Fatal("the form is open")
	}
	app.launchWorkflow(state, definition)
	if state.issue == nil || state.issue.Name != "target" {
		t.Fatalf("the first bad field is named: %+v", state.issue)
	}
	if app.workflowUI.hubLaunch != nil {
		t.Fatal("nothing was dispatched")
	}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.workflowLaunchDialog(c)
	}, 900, 800)
	tt.Frame()
	if !tt.HasText("target is required.") {
		t.Fatalf("the refusal names the field: %v", tt.Texts())
	}
}

// 再次运行：从那次运行用过的实参预填，定义已删掉的键不跟过来。
func TestWorkflowLaunchPrefillsFromTheRunThatUsedIt(t *testing.T) {
	app := hubApp(t)
	definition := app.state.WorkflowDefinitions[0]
	definition.Arguments = []contracts.WorkflowArgument{
		{Name: "target", Type: "string"},
		{Name: "depth", Type: "number", Default: "2", HasDefault: true},
	}
	app.state.WorkflowDefinitions[0] = definition
	run := contracts.WorkflowRun{ID: "run-1", DefinitionID: "def-a", Args: []contracts.WorkflowArgValue{
		{Name: "target", Kind: "string", Text: "src"},
		{Name: "gone", Kind: "string", Text: "dropped"},
	}}
	app.startWorkflowLaunch(definition, &run)
	values := app.workflowUI.hub.launch.values
	if *values["target"] != "src" || *values["depth"] != "2" {
		t.Fatalf("the run's own values come back: %+v", values)
	}
	if _, ok := values["gone"]; ok {
		t.Fatalf("a key the definition dropped is not carried over: %+v", values)
	}
}

// 启动是两步：先建会话，再在那个会话里起运行；两次快照之间不丢。
func TestWorkflowLaunchCreatesTheConversationFirst(t *testing.T) {
	app := hubApp(t)
	definition := app.state.WorkflowDefinitions[0]
	app.startWorkflowLaunch(definition, nil)
	pending := app.workflowUI.hubLaunch
	if pending == nil || pending.requestID == "" {
		t.Fatalf("the launch asks for a conversation: %+v", pending)
	}
	// 会话出现之前：什么都不发。
	app.syncWorkflowLaunch()
	if pending.sessionID != "" || pending.dispatched {
		t.Fatalf("no conversation yet: %+v", pending)
	}
	app.state.Sessions = append(app.state.Sessions, contracts.Session{ID: "chat-hub", ProjectID: "proj-a", CreationRequestID: pending.requestID})
	app.syncWorkflowLaunch()
	if pending.sessionID != "chat-hub" || !pending.dispatched {
		t.Fatalf("the run is dispatched into the new conversation: %+v", pending)
	}
	// 运行出现：待办结束。
	app.state.WorkflowRuns = append(app.state.WorkflowRuns, contracts.WorkflowRun{
		ID: "run-hub", DefinitionID: "def-a", SessionID: "chat-hub", Name: "Review",
		Status: contracts.WorkflowStatusPending, StartedAt: time.Now(),
	})
	app.syncWorkflowLaunch()
	if app.workflowUI.hubLaunch != nil {
		t.Fatalf("the launch finished: %+v", app.workflowUI.hubLaunch)
	}
	if app.workflowUI.hub.launch != nil {
		t.Fatal("the form closed once the run exists")
	}
}

// 被拒绝的启动：理由回到表单，空的会话尽力收回。
func TestWorkflowLaunchFailureReturnsToTheForm(t *testing.T) {
	app := hubApp(t)
	definition := app.state.WorkflowDefinitions[0]
	definition.Arguments = []contracts.WorkflowArgument{{Name: "target", Type: "string"}}
	app.state.WorkflowDefinitions[0] = definition
	app.startWorkflowLaunch(definition, nil)
	state := app.workflowUI.hub.launch
	*state.values["target"] = "src"
	app.launchWorkflow(state, definition)
	if app.workflowUI.hubLaunch == nil {
		t.Fatal("a valid form dispatches the launch")
	}
	app.workflowUI.hubLaunch.sessionID = "chat-empty"
	app.workflowHubLaunchFailed(errTestStop)
	if app.workflowUI.hubLaunch != nil {
		t.Fatal("a refused launch stops waiting")
	}
	if state.pending {
		t.Fatal("the form is usable again")
	}
	if state.errorText == "" {
		t.Fatal("the form keeps the reason")
	}
}

type testError string

func (e testError) Error() string { return string(e) }

const errTestStop = testError("the core refused this launch")

// 侧栏不摆工作流入口：中枢是自动化页里的一个模块，不是另一条路。
func TestWorkflowHubLivesInTheAutomationsModule(t *testing.T) {
	app := hubApp(t)
	sidebar := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.sidebarNavigation(c)
	}, 320, 600)
	sidebar.Frame()
	if sidebar.HasText("Workflows") {
		t.Fatalf("the sidebar has no workflow entry: %v", sidebar.Texts())
	}
	page := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.automationsView(c)
	}, 1000, 900)
	page.Frame()
	if err := page.Click("Workflows"); err != nil {
		t.Fatal(err)
	}
	if app.automationUI.Module != automationModuleWorkflows {
		t.Fatalf("the module switched: %q", app.automationUI.Module)
	}
	page.Frame()
	for _, text := range []string{"Alpha", "This machine", "Review", "Nightly"} {
		if !page.HasText(text) {
			t.Fatalf("the hub lists the definitions: %v", page.Texts())
		}
	}
}

// 提升全局只是“另存一份到本机”，项目那一份还在原处。
func TestWorkflowHubPromotesBySavingASeparateCopy(t *testing.T) {
	app := hubApp(t)
	definition := app.state.WorkflowDefinitions[0]
	app.promoteWorkflowDefinition(definition)
	if len(app.state.WorkflowDefinitions) != 3 {
		t.Fatalf("promotion does not delete the project's own copy: %d", len(app.state.WorkflowDefinitions))
	}
}

// 删除要先问一句，再说删掉的是哪一份。
func TestWorkflowHubDeleteAsksFirst(t *testing.T) {
	app := hubApp(t)
	app.workflowUI.hub.selected = "def-a"
	tt := hubTester(app)
	tt.Frame()
	if err := tt.Click("Delete workflow"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !app.workflowUI.hub.removeOpen {
		t.Fatal("deleting a definition asks first")
	}
	if !tt.HasText("Remove “Review”?") {
		t.Fatalf("the question names the definition: %v", tt.Texts())
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if app.workflowUI.hub.removeOpen {
		t.Fatal("cancelling keeps the definition")
	}
}
