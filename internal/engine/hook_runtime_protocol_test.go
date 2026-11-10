package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"gcode/internal/agent"
	"gcode/internal/contracts"
	"gcode/internal/model"
)

func TestHookRuntimeAgentFiltersMCPNamesForBoundProtocol(t *testing.T) {
	for _, protocol := range []model.Protocol{model.Chat, model.Responses, model.Messages} {
		t.Run(string(protocol), func(t *testing.T) {
			longName := "mcp__fixture__" + strings.Repeat("x", 70)
			selected := longName
			if protocol == model.Chat {
				selected = "mcp__fixture__short"
			}
			var modelCalls, toolCalls atomic.Int32
			core, cwd := goalTestCore(t, protocol, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := subagentRequest(t, r)
				raw, _ := json.Marshal(body)
				if !strings.Contains(string(raw), "discovery diagnostic fixture") {
					t.Error("hook lost discovery diagnostics")
				}
				if modelCalls.Add(1) == 1 {
					var names []string
					for _, rawTool := range body["tools"].([]any) {
						tool := rawTool.(map[string]any)
						if protocol == model.Chat {
							tool = tool["function"].(map[string]any)
						}
						names = append(names, tool["name"].(string))
					}
					if !slices.Contains(names, "mcp__fixture__short") || slices.Contains(names, longName) != (protocol != model.Chat) {
						t.Errorf("incorrect hook tool catalog: %v", names)
					}
					if protocol == model.Chat && !strings.Contains(string(raw), "limit 64") {
						t.Error("hook verifier did not receive protocol skip reason")
					}
					subagentWireCall(w, protocol, selected, `{}`)
					return
				}
				if !strings.Contains(string(raw), "HOOK MCP EXECUTED") {
					t.Error("tool execution evidence did not return to hook verifier")
				}
				subagentText(w, protocol, `{"ok":true}`)
			}))
			provider, configured, err := core.resolveModel("goal/model")
			if err != nil {
				t.Fatal(err)
			}
			gateway, err := core.client.Bind(provider, configured)
			if err != nil {
				t.Fatal(err)
			}
			var tools []agent.Tool
			for _, name := range []string{longName, "mcp__fixture__short"} {
				name := name
				tools = append(tools, hookFixtureTool{name: name, execute: func(context.Context, string, json.RawMessage) (model.ToolOutput, error) {
					toolCalls.Add(1)
					if name != selected {
						t.Errorf("hook tool was renamed or wrongly selected: %s", name)
					}
					return model.ToolOutput{Text: "HOOK MCP EXECUTED"}, nil
				}})
			}
			evaluate := hookRuntimeEvaluator(hookRuntimeOptions{Model: gateway, Protocol: provider.Protocol, Tools: tools, Warnings: []string{"discovery diagnostic fixture"}}, contracts.Session{})
			result, err := evaluate(context.Background(), contracts.HookRule{Type: "agent", Model: "haiku", Prompt: "Verify using MCP."}, agent.HookInput{CWD: cwd})
			if err != nil || result.Output != `{"ok":true}` || modelCalls.Load() != 2 || toolCalls.Load() != 1 {
				t.Fatalf("hook verifier result=%+v error=%v models=%d tools=%d", result, err, modelCalls.Load(), toolCalls.Load())
			}
		})
	}
}

func TestHookRuntimeCompactionRetainsMCPDiagnosticsAtModelBoundary(t *testing.T) {
	backend := hookFixtureModel(func(_ context.Context, request model.Request, _ func(model.Delta) error) (model.Result, error) {
		if request.ContextChange != "summary" || !strings.Contains(request.System, "MCP skipped fixture") {
			t.Fatalf("compaction dropped diagnostics while preparing its summary prompt: %s", request.System)
		}
		return model.Result{Message: model.Message{Role: "assistant", Text: "Summary."}}, nil
	})
	var history []model.Message
	for range 6 {
		history = append(history, model.Message{Role: "user", Text: "Earlier work"})
	}
	_, err := agent.Compact(context.Background(), hookRuntimeDiagnosticModel{inner: backend, warnings: []string{"MCP skipped fixture"}}, model.Request{History: history})
	if err != nil {
		t.Fatal(err)
	}
}
