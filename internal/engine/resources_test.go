package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
)

func TestPromptReferencesResolveHostResources(t *testing.T) {
	core, root := resourceCore(t)
	skill := contracts.UserSkill{ID: "docs", Name: "文档", Content: "PRIVATE SKILL INSTRUCTIONS", Enabled: true}
	command := contracts.UserCommand{ID: "review", Name: "review", Content: "Review $ARGUMENTS", Enabled: true}
	saveResource(t, core, "resource.skill.save", &contracts.ResourceInput{Skill: &skill})
	saveResource(t, core, "resource.command.save", &contracts.ResourceInput{Command: &command})
	saveResource(t, core, "resource.mcp.save", &contracts.ResourceInput{MCP: &contracts.MCPServerInput{ID: "tools", Name: "tools", Transport: "stdio", Command: "unstarted-test-command", Enabled: true}})
	ref := contracts.PromptReference{Kind: "skill", ID: "docs", Label: "文档"}
	mcp := contracts.PromptReference{Kind: "mcp", ID: "tools", Label: "工具"}
	prompt, err := expandPrompt(root, "", "/review task "+ref.Markdown()+" "+ref.Markdown()+" "+mcp.Markdown())
	if err != nil || !strings.HasPrefix(prompt, "Review task ") || strings.Count(prompt, skill.Content) != 1 || !strings.Contains(prompt, "MCP server tools") || strings.Contains(prompt, "gcode://") {
		t.Fatalf("resource expansion: %q %v", prompt, err)
	}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "resource.skill.toggle", ID: "docs", Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if _, err := expandPrompt(root, "", ref.Markdown()); err == nil {
		t.Fatal("disabled skill remained invocable")
	}
	err = core.Execute(context.Background(), contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{Text: "/goal build " + ref.Markdown()}})
	if err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("goal path bypassed resource validation: %v", err)
	}
	if _, err := expandPrompt(root, "", (contracts.PromptReference{Kind: "mcp", ID: "missing", Label: "fake"}).Markdown()); err == nil {
		t.Fatal("client label bypassed host validation")
	}
}

func TestPromptBuiltinAndInlineCommandSemantics(t *testing.T) {
	ref := contracts.PromptReference{Kind: "command", ID: "init", Label: "Init"}
	root := t.TempDir()
	prompt, err := expandPrompt(root, "", ref.Markdown()+" task")
	if err != nil || !strings.Contains(prompt, "AGENTS.md") || !strings.HasSuffix(prompt, "task") {
		t.Fatalf("init did not execute: %q %v", prompt, err)
	}
	inline, err := expandPrompt(root, "", "你好 "+ref.Markdown())
	if err != nil || inline != "你好 /init" {
		t.Fatalf("inline token executed a command: %q %v", inline, err)
	}
	if err := validateUserCommand(contracts.UserCommand{ID: "init", Name: "init", Content: "shadow builtin"}); err == nil {
		t.Fatal("builtin could be shadowed")
	}
}

func TestQueueKeepsReferenceDisplaySeparateFromExecution(t *testing.T) {
	core, _ := resourceCore(t)
	raw := (contracts.PromptReference{Kind: "skill", ID: "docs", Label: "文档"}).Markdown()
	core.mu.Lock()
	defer core.mu.Unlock()
	core.state.Queue = []contracts.QueuedMessage{{ID: "queued", SessionID: "session", Text: raw, Prompt: "RESOLVED SKILL CONTENT", CreatedAt: time.Now()}}
	messages := core.drainPendingLocked("session", true)
	if len(messages) != 1 || messages[0].Text != "RESOLVED SKILL CONTENT" {
		t.Fatalf("execution used canonical display: %+v", messages)
	}
	if got := core.state.Messages["session"][0].Blocks[0].Text; got != raw {
		t.Fatalf("transcript lost original input: %q", got)
	}
}

func TestGoalRejectsInvalidStructuredResourceReference(t *testing.T) {
	core, _ := resourceCore(t)
	ctx := context.Background()
	if err := core.Execute(ctx, contracts.Command{Kind: "session.create"}); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := core.Snapshot(ctx)
	err := core.Execute(ctx, contracts.Command{
		Kind: "goal.set", SessionID: snapshot.Sessions[0].ID,
		Goal: &contracts.GoalInput{Objective: "Build the project", References: []contracts.PromptReference{{Kind: "skill", ID: "../ghost", Label: "invalid"}}},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid goal resource") {
		t.Fatalf("structured reference bypassed validation: %v", err)
	}
}

func TestGoalPreservesInlineReferencesAndExecutionInstructions(t *testing.T) {
	core, _ := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := subagentRequest(t, r)
		if isGoalVerification(body) {
			goalResponse(w, model.Chat, `{"passed":true,"reason":"The requested comparison was completed."}`, nil, true)
			return
		}
		payload, _ := json.Marshal(body)
		if !strings.Contains(string(payload), "DOC_A") || !strings.Contains(string(payload), "DOC_B") {
			t.Error("goal executor lost selected skill instructions")
		}
		goalResponse(w, model.Chat, "Comparison completed.", nil, true)
	}))
	for _, id := range []string{"A", "B"} {
		saveResource(t, core, "resource.skill.save", &contracts.ResourceInput{Skill: &contracts.UserSkill{ID: id, Name: id, Content: "DOC_" + id, Enabled: true}})
	}
	raw := "/goal 比较" + (contracts.PromptReference{Kind: "skill", ID: "A", Label: "A"}).Markdown() +
		"和" + (contracts.PromptReference{Kind: "skill", ID: "B", Label: "B"}).Markdown() + "的结果"
	sendGoal(t, core, raw)
	state := waitSubagentState(t, core, func(s contracts.Snapshot) bool {
		return runsFinished(s) && len(s.Sessions) == 1 && s.Sessions[0].Goal != nil && s.Sessions[0].Goal.Status == contracts.GoalComplete
	})
	session := state.Sessions[0]
	if session.Goal.Objective != "比较A和B的结果" || !strings.Contains(session.Goal.Instructions, "DOC_A") {
		t.Fatalf("goal semantics changed: %+v", session.Goal)
	}
	if got := state.Messages[session.ID][0].Blocks[0].Text; got != raw {
		t.Fatalf("goal moved inline references: %q", got)
	}
}
