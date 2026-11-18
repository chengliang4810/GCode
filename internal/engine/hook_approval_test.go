package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gcode/internal/agent"
	"gcode/internal/contracts"
	"gcode/internal/model"
)

// hookApprovalAsker answers every permission question with the same scripted
// text and records the pages the host would actually show.
type hookApprovalAsker struct {
	answer string
	asked  []contracts.QuestionPage
}

func (a *hookApprovalAsker) ask(_ context.Context, pages []contracts.QuestionPage) ([]contracts.QuestionAnswer, error) {
	a.asked = append(a.asked, pages...)
	return []contracts.QuestionAnswer{{Text: a.answer}}, nil
}

func (a *hookApprovalAsker) permission(approvals *hookPermissionApprovals, sessionID string) func(context.Context, agent.HookInput, string) (bool, error) {
	return hookRuntimePermission(hookRuntimeOptions{Tools: []agent.Tool{agent.QuestionTool{Ask: a.ask}},
		Approvals: approvals, ApprovalScope: "test-host", SessionID: sessionID})
}

func (a *hookApprovalAsker) pages() []contracts.QuestionPage { return a.asked }

func hookApprovalInput(t *testing.T, tool string, raw string, cwd string) agent.HookInput {
	t.Helper()
	input := agent.HookInput{ToolName: tool, ToolInput: json.RawMessage(raw), CWD: cwd}
	if !json.Valid(input.ToolInput) {
		t.Fatalf("invalid fixture tool input: %s", raw)
	}
	return input
}

func hookApprovalOption(t *testing.T, page contracts.QuestionPage, label string) contracts.QuestionOption {
	t.Helper()
	for _, option := range page.Options {
		if option.Label == label {
			return option
		}
	}
	t.Fatalf("option %q missing from %+v", label, page.Options)
	return contracts.QuestionOption{}
}

func TestHookApprovalDisclosesCommandPathAndReversibility(t *testing.T) {
	directory := t.TempDir()
	existing := filepath.Join(directory, "existing.txt")
	if err := os.WriteFile(existing, []byte("current content"), 0600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(directory, "missing.txt")
	for _, test := range []struct {
		name, tool, raw string
		questionHas     []string
		scopeHas        []string
		effectHas       []string
	}{
		{
			name: "shell command", tool: "Bash", raw: `{"command":"go test ./internal/engine -run TestHookApproval"}`,
			questionHas: []string{`"command":"go test ./internal/engine -run TestHookApproval"`},
			scopeHas:    []string{"go test ./internal/engine -run TestHookApproval", directory},
			effectHas:   []string{"shell command", "not automatically reversible"},
		},
		{
			name: "overwrites existing file", tool: "Write", raw: `{"file_path":"existing.txt","content":"replacement"}`,
			scopeHas:  []string{existing, "existing file"},
			effectHas: []string{"existing file", "previous content is lost"},
		},
		{
			name: "creates new file", tool: "Write", raw: `{"file_path":"missing.txt","content":"new"}`,
			scopeHas:  []string{missing},
			effectHas: []string{"Creates a new file", "Deleting the file reverts this change"},
		},
		{
			name: "read only", tool: "Read", raw: `{"file_path":"existing.txt"}`,
			scopeHas:  []string{existing},
			effectHas: []string{"Reads only. No file content changes."},
		},
		{
			name: "unreadable input degrades to tool name", tool: "mcp__plugin_service__check", raw: `{"opaque":"value"}`,
			scopeHas:  []string{"mcp__plugin_service__check"},
			effectHas: []string{"cannot infer the effect", "may not be reversible"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			asker := &hookApprovalAsker{answer: hookPermissionDeny}
			allowed, err := asker.permission(nil, "session-a")(context.Background(), hookApprovalInput(t, test.tool, test.raw, directory), "guard")
			if err != nil || allowed || len(asker.pages()) != 1 {
				t.Fatalf("allowed=%v pages=%d err=%v", allowed, len(asker.pages()), err)
			}
			page := asker.pages()[0]
			if page.Header != hookPermissionHeader || len(page.Options) != 3 || page.MultiSelect {
				t.Fatalf("permission page changed shape: %+v", page)
			}
			// The question stays the answer key, so its previous text survives.
			if !strings.HasPrefix(page.Question, "Allow "+test.tool+"?") {
				t.Fatalf("question no longer starts with the previous text: %q", page.Question)
			}
			for _, want := range test.questionHas {
				if !strings.Contains(page.Question, want) {
					t.Fatalf("question lost %q: %q", want, page.Question)
				}
			}
			for _, label := range []string{hookPermissionAllowOnce, hookPermissionAllowSession, hookPermissionDeny} {
				option := hookApprovalOption(t, page, label)
				disclosed := option.Description + "\n" + option.Preview
				for _, want := range test.scopeHas {
					if !strings.Contains(disclosed, want) {
						t.Fatalf("%s option lost scope %q: %+v", label, want, option)
					}
				}
				for _, want := range test.effectHas {
					if !strings.Contains(disclosed, want) {
						t.Fatalf("%s option lost effect %q: %+v", label, want, option)
					}
				}
			}
			session := hookApprovalOption(t, page, hookPermissionAllowSession)
			if !strings.Contains(session.Description, "restarting the host") && !strings.Contains(session.Preview, "restarting the host") {
				t.Fatalf("session approval hides its lifetime: %+v", session)
			}
		})
	}
}

func TestHookApprovalAllowOnceIsNotRemembered(t *testing.T) {
	approvals := &hookPermissionApprovals{}
	input := hookApprovalInput(t, "Bash", `{"command":"go test ./internal/engine"}`, t.TempDir())
	asker := &hookApprovalAsker{answer: hookPermissionAllowOnce}
	permission := asker.permission(approvals, input.SessionID)
	for attempt := 1; attempt <= 2; attempt++ {
		allowed, err := permission(context.Background(), input, "")
		if err != nil || !allowed {
			t.Fatalf("attempt %d allowed=%v err=%v", attempt, allowed, err)
		}
		if len(asker.pages()) != attempt {
			t.Fatalf("attempt %d asked %d times: single approval was remembered", attempt, len(asker.pages()))
		}
	}
	if approvals.allowed("test-host", "", hookPermissionScopeOf(input).Key) {
		t.Fatal("plain Allow created a session approval")
	}
}

func TestHookApprovalAlwaysAllowSilencesOnlyTheSameScope(t *testing.T) {
	directory := t.TempDir()
	approvals := &hookPermissionApprovals{}
	input := hookApprovalInput(t, "Bash", `{"command":"go test ./internal/engine\ncat proof.txt"}`, directory)
	input.SessionID = "session-a"
	asker := &hookApprovalAsker{answer: hookPermissionAllowSession}
	permission := asker.permission(approvals, input.SessionID)
	if allowed, err := permission(context.Background(), input, ""); err != nil || !allowed {
		t.Fatalf("session approval was denied: %v %v", allowed, err)
	}
	if len(asker.pages()) != 1 {
		t.Fatalf("session approval did not ask once: %d", len(asker.pages()))
	}
	// The same first command line in the same conversation executes silently.
	if allowed, err := permission(context.Background(), input, ""); err != nil || !allowed {
		t.Fatalf("remembered approval was denied: %v %v", allowed, err)
	}
	if len(asker.pages()) != 1 {
		t.Fatalf("remembered approval asked again: %d questions", len(asker.pages()))
	}
	// A different command is a different scope and still asks.
	asker.answer = hookPermissionDeny
	other := hookApprovalInput(t, "Bash", `{"command":"rm -rf ./tmp"}`, directory)
	other.SessionID = input.SessionID
	if allowed, err := permission(context.Background(), other, ""); err != nil || allowed {
		t.Fatalf("different command reused the approval: %v %v", allowed, err)
	}
	if len(asker.pages()) != 2 {
		t.Fatalf("different command did not ask: %d questions", len(asker.pages()))
	}
	// A file target is memory for that path only, not for its directory.
	asker.answer = hookPermissionAllowSession
	first := hookApprovalInput(t, "Write", `{"file_path":"one.txt","content":"a"}`, directory)
	first.SessionID = input.SessionID
	if allowed, err := permission(context.Background(), first, ""); err != nil || !allowed {
		t.Fatalf("file approval was denied: %v %v", allowed, err)
	}
	asker.answer = hookPermissionDeny
	second := hookApprovalInput(t, "Write", `{"file_path":"two.txt","content":"b"}`, directory)
	second.SessionID = input.SessionID
	if allowed, err := permission(context.Background(), second, ""); err != nil || allowed {
		t.Fatalf("another file reused the approval: %v %v", allowed, err)
	}
	if len(asker.pages()) != 4 {
		t.Fatalf("another file did not ask: %d questions", len(asker.pages()))
	}
}

func TestHookApprovalDenyRecordsNothing(t *testing.T) {
	approvals := &hookPermissionApprovals{}
	input := hookApprovalInput(t, "Bash", `{"command":"pwd"}`, t.TempDir())
	input.SessionID = "session-a"
	asker := &hookApprovalAsker{answer: hookPermissionDeny}
	permission := asker.permission(approvals, input.SessionID)
	if allowed, err := permission(context.Background(), input, ""); err != nil || allowed {
		t.Fatalf("denial allowed the call: %v %v", allowed, err)
	}
	if approvals.allowed("test-host", input.SessionID, hookPermissionScopeOf(input).Key) {
		t.Fatal("denial created a session approval")
	}
	asker.answer = hookPermissionAllowOnce
	if allowed, err := permission(context.Background(), input, ""); err != nil || !allowed {
		t.Fatalf("denied call could no longer be approved: %v %v", allowed, err)
	}
	if len(asker.pages()) != 2 {
		t.Fatalf("denial changed the next request: %d questions", len(asker.pages()))
	}
}

func TestHookApprovalIsolatedPerSessionAndForgotten(t *testing.T) {
	approvals := &hookPermissionApprovals{}
	input := hookApprovalInput(t, "Bash", `{"command":"go test ./internal/engine"}`, t.TempDir())
	key := hookPermissionScopeOf(input).Key
	first := input
	first.SessionID = "session-a"
	second := input
	second.SessionID = "session-b"
	granted := &hookApprovalAsker{answer: hookPermissionAllowSession}
	if allowed, err := granted.permission(approvals, first.SessionID)(context.Background(), first, ""); err != nil || !allowed {
		t.Fatalf("first session was denied: %v %v", allowed, err)
	}
	// Another conversation must still see the question.
	other := &hookApprovalAsker{answer: hookPermissionDeny}
	if allowed, err := other.permission(approvals, second.SessionID)(context.Background(), second, ""); err != nil || allowed {
		t.Fatalf("approval crossed sessions: %v %v", allowed, err)
	}
	if len(other.pages()) != 1 || approvals.allowed("test-host", second.SessionID, key) {
		t.Fatalf("second session reused another conversation's approval: pages=%d", len(other.pages()))
	}
	if !approvals.allowed("test-host", first.SessionID, key) || approvals.allowed("other-host", first.SessionID, key) {
		t.Fatal("approval scope is not host specific")
	}
	// Deleted and archived conversations lose their grants.
	approvals.retain("test-host", []string{second.SessionID})
	if approvals.allowed("test-host", first.SessionID, key) {
		t.Fatal("retain kept a vanished session")
	}
	approvals.allow("test-host", second.SessionID, key)
	approvals.allow("test-host", "session-c", key)
	approvals.retain("test-host", []string{second.SessionID})
	if !approvals.allowed("test-host", second.SessionID, key) || approvals.allowed("test-host", "session-c", key) {
		t.Fatal("retain dropped a live session or kept a vanished one")
	}
	approvals.retain("test-host", nil)
	if approvals.allowed("test-host", second.SessionID, key) {
		t.Fatal("retain kept a grant without live sessions")
	}
}

func TestHookApprovalFailsClosedWithoutSessionOrMemory(t *testing.T) {
	// Without approval memory the host keeps the previous behaviour: it asks.
	asker := &hookApprovalAsker{answer: hookPermissionAllowOnce}
	permission := hookRuntimePermission(hookRuntimeOptions{Tools: []agent.Tool{agent.QuestionTool{Ask: asker.ask}}})
	input := hookApprovalInput(t, "Bash", `{"command":"pwd"}`, "")
	for attempt := 1; attempt <= 2; attempt++ {
		if allowed, err := permission(context.Background(), input, ""); err != nil || !allowed || len(asker.pages()) != attempt {
			t.Fatalf("attempt %d allowed=%v pages=%d err=%v", attempt, allowed, len(asker.pages()), err)
		}
	}
	// A custom answer never becomes an approval.
	custom := &hookApprovalAsker{answer: "maybe"}
	if allowed, err := custom.permission(&hookPermissionApprovals{}, "session-a")(context.Background(), input, ""); err != nil || allowed {
		t.Fatalf("custom answer was accepted: %v %v", allowed, err)
	}
}

func TestHookApprovalScopeKeyDegradesToToolName(t *testing.T) {
	approvals := &hookPermissionApprovals{}
	// An unreadable input cannot name a target, so the key is the tool name and
	// one grant covers every call of that tool in the conversation.
	session := hookApprovalInput(t, "CustomTool", `{"unknown":[1,2]}`, t.TempDir())
	session.SessionID = "session-a"
	asker := &hookApprovalAsker{answer: hookPermissionAllowSession}
	if allowed, err := asker.permission(approvals, session.SessionID)(context.Background(), session, ""); err != nil || !allowed {
		t.Fatalf("degenerate approval was denied: %v %v", allowed, err)
	}
	scope := hookPermissionScopeOf(session)
	if scope.Kind != hookPermissionUnknownKind || scope.Key != "CustomTool\x00" {
		t.Fatalf("unexpected degenerate scope: %+v", scope)
	}
	other := hookApprovalInput(t, "CustomTool", `{"unknown":"other"}`, t.TempDir())
	other.SessionID = session.SessionID
	if allowed, err := asker.permission(approvals, session.SessionID)(context.Background(), other, ""); err != nil || !allowed {
		t.Fatalf("tool-name grant was not reused: %v %v", allowed, err)
	}
	if len(asker.pages()) != 1 {
		t.Fatalf("degenerate scope asked again: %d questions", len(asker.pages()))
	}
}

// TestHookApprovalSessionGrantSkipsTheNextQuestion proves the host wiring: a
// real run asks once, the session grant is reused for the same file, and a
// deleted session loses the grant.
func TestHookApprovalSessionGrantSkipsTheNextQuestion(t *testing.T) {
	var requests atomic.Int32
	core, workspace := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = subagentRequest(t, r)
		switch requests.Add(1) {
		case 1:
			goalResponse(w, model.Chat, "", &model.Call{ID: "first", Name: "Write", Arguments: `{"file_path":"proof.txt","content":"first"}`}, true)
		case 2:
			goalResponse(w, model.Chat, "", &model.Call{ID: "second", Name: "Write", Arguments: `{"file_path":"proof.txt","content":"second"}`}, true)
		default:
			goalResponse(w, model.Chat, "Finished.", nil, true)
		}
	}))
	program, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	state, _ := core.Snapshot(context.Background())
	settings := state.Settings
	settings.Hooks = []contracts.HookRule{{
		ID: "ask", Event: contracts.HookPreToolUse, Type: "command", Command: program, Enabled: true,
		Args: []string{"-test.run=^TestEngineHookProcessHelper$", "--", "engine-hook-helper", filepath.Join(workspace, "ask.json"),
			`{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"ask","permissionDecisionReason":"Write needs review"}}`},
	}}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "settings.update", Settings: &settings}); err != nil {
		t.Fatal(err)
	}
	sendGoal(t, core, "Write proof.txt twice")
	summary, err := core.Snapshot(context.Background())
	if err != nil || len(summary.Sessions) != 1 {
		t.Fatalf("goal session was not created: %+v, %v", summary.Sessions, err)
	}
	sessionID := summary.Sessions[0].ID
	releaseView := pinSessionView(t, core, sessionID)
	defer releaseView()
	question, seen := waitHookApprovalQuestion(t, core, sessionID)
	if !seen {
		t.Fatal("no permission question was published")
	}
	if question.Pages[0].Header != hookPermissionHeader {
		t.Fatalf("permission page lost its header: %+v", question.Pages[0])
	}
	disclosed := ""
	for _, option := range question.Pages[0].Options {
		disclosed += option.Description + "\n" + option.Preview + "\n"
	}
	if !strings.Contains(disclosed, filepath.Join(workspace, "proof.txt")) || !strings.Contains(disclosed, "Creates a new file") {
		t.Fatalf("permission page hid the target: %s", disclosed)
	}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "request.answer", ID: question.ID,
		QuestionAnswers: []contracts.QuestionAnswer{{Options: []string{hookPermissionAllowSession}}}}); err != nil {
		t.Fatal(err)
	}
	state = waitIdle(t, core, sessionID)
	if state.Runs[0].Status != "completed" || requests.Load() != 3 {
		t.Fatalf("run did not finish after the session grant: requests=%d runs=%+v", requests.Load(), state.Runs)
	}
	if content, err := os.ReadFile(filepath.Join(workspace, "proof.txt")); err != nil || string(content) != "second" {
		t.Fatalf("second write did not run without asking: %q %v", content, err)
	}
	scope := core.hookPermissionApprovalScope()
	granted := hookPermissionScopeOf(hookApprovalInput(t, "Write", `{"file_path":"proof.txt","content":"second"}`, workspace))
	if !core.hookApprovals.allowed(scope, sessionID, granted.Key) {
		t.Fatal("session grant was not recorded for the real session")
	}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "session.delete", ID: sessionID}); err != nil {
		t.Fatal(err)
	}
	if core.hookPermissionSessionUsable(sessionID) {
		t.Fatal("deleted session still reports a usable approval memory")
	}
	core.pruneHookPermissionApprovals(scope)
	if core.hookApprovals.allowed(scope, sessionID, granted.Key) {
		t.Fatal("deleted session kept its approval")
	}
}

// waitHookApprovalQuestion polls the published transcript for the pending
// permission question, mirroring how the desktop answers it.
func waitHookApprovalQuestion(t *testing.T, core *Engine, sessionID string) (*contracts.Question, bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		state, err := core.Snapshot(context.Background(), sessionID)
		if err == nil {
			for _, messages := range state.Messages {
				for _, message := range messages {
					for _, block := range message.Blocks {
						if block.Question != nil && len(block.Question.Pages) == 1 && block.Question.Pages[0].Header == hookPermissionHeader {
							return block.Question, true
						}
					}
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil, false
}
