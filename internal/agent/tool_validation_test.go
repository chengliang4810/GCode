package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"gcode/internal/model"
)

type schemaProbe struct {
	def model.Tool
	run func(json.RawMessage)
}

func (t schemaProbe) Definition() model.Tool { return t.def }
func (t schemaProbe) Concurrent() bool       { return false }
func (t schemaProbe) Execute(_ context.Context, _ string, raw json.RawMessage) (model.ToolOutput, error) {
	t.run(raw)
	return model.ToolOutput{Text: "done"}, nil
}

func TestToolSchemaValidationAndAgentStringBooleans(t *testing.T) {
	def := model.Tool{Name: "Agent", Parameters: map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"prompt"},
		"properties": map[string]any{
			"prompt":            map[string]any{"type": "string"},
			"run_in_background": map[string]any{"type": "boolean"},
		},
	}}
	for _, test := range []struct {
		name, args, wantError string
		background            bool
	}{
		{"boolean", `{"prompt":"task","run_in_background":true}`, "", true},
		{"string true", `{"prompt":"task","run_in_background":"true"}`, "", true},
		{"string false", `{"prompt":"task","run_in_background":"false"}`, "", false},
		{"omitted", `{"prompt":"task"}`, "", false},
		{"bad string", `{"prompt":"task","run_in_background":"yes"}`, "must be a boolean", false},
		{"null", `{"prompt":"task","run_in_background":null}`, "must be a boolean", false},
		{"number", `{"prompt":"task","run_in_background":1}`, "must be a boolean", false},
		{"wrong prompt", `{"prompt":5}`, "prompt argument type is wrong", false},
		{"missing", `{}`, "required", false},
		{"unknown", `{"prompt":"task","extra":true}`, "additionalProperties", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			called := false
			tool := schemaProbe{def: def, run: func(raw json.RawMessage) {
				called = true
				var args struct {
					Background bool `json:"run_in_background"`
				}
				if err := json.Unmarshal(raw, &args); err != nil || args.Background != test.background {
					t.Fatalf("not normalized: %s (%v)", raw, err)
				}
			}}
			var result model.Message
			err := executeTools(t.Context(), Options{Emit: func(Event) error { return nil }},
				map[string]Tool{"Agent": tool}, []model.Call{{ID: "call", Name: "Agent", Arguments: test.args}},
				func(m model.Message) error { result = m; return nil })
			if err != nil {
				t.Fatal(err)
			}
			if test.wantError != "" {
				if called || !result.IsError || !strings.Contains(result.Text, test.wantError) {
					t.Fatalf("invalid input executed or feedback missing: called=%v result=%+v", called, result)
				}
			} else if !called || result.IsError {
				t.Fatalf("valid input rejected: %+v", result)
			}
		})
	}
}

func TestToolSchemaRejectsHookRewrittenType(t *testing.T) {
	called := false
	tool := schemaProbe{def: model.Tool{Name: "Write", Parameters: map[string]any{
		"type": "object", "properties": map[string]any{"content": map[string]any{"type": "string"}},
	}}, run: func(json.RawMessage) { called = true }}
	_, err := ExecuteHookedTool(t.Context(), tool, "", json.RawMessage(`{"content":"ok"}`),
		func(context.Context, HookInput) (HookResult, error) {
			return HookResult{UpdatedInput: json.RawMessage(`{"content":false}`)}, nil
		}, "call")
	if called || err == nil || !strings.Contains(err.Error(), "content argument type is wrong") {
		t.Fatalf("hook bypassed validation: called=%v err=%v", called, err)
	}
}

func TestToolSchemaNestedConstraintsAndNoGeneralCoercion(t *testing.T) {
	def := model.Tool{Name: "Other", Parameters: map[string]any{
		"type": "object", "properties": map[string]any{
			"items": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
			"flag":  map[string]any{"type": "boolean"},
		},
	}}
	for _, raw := range []string{`{"items":[1,"secret"]}`, `{"flag":"true"}`} {
		_, err := PrepareToolArguments(def, json.RawMessage(raw))
		if err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("invalid input accepted or value leaked: %v", err)
		}
	}
}

func TestToolSchemaCacheTracksDefinitionChanges(t *testing.T) {
	parameters := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"value": map[string]any{"type": "string"},
		},
	}
	def := model.Tool{Name: "Mutable", Parameters: parameters}
	if _, err := PrepareToolArguments(def, json.RawMessage(`{"value":"ok"}`)); err != nil {
		t.Fatal(err)
	}
	parameters["properties"].(map[string]any)["value"] = map[string]any{"type": "integer"}
	if _, err := PrepareToolArguments(def, json.RawMessage(`{"value":"stale"}`)); err == nil || !strings.Contains(err.Error(), "value argument type is wrong") {
		t.Fatalf("changed definition reused stale schema: %v", err)
	}
}

func TestToolSchemaCacheConcurrentValidation(t *testing.T) {
	def := model.Tool{Name: "Concurrent", Parameters: map[string]any{
		"type": "object", "properties": map[string]any{
			"value": map[string]any{"type": "integer"},
		},
	}}
	const workers = 16
	var group sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			for range 50 {
				if _, err := PrepareToolArguments(def, json.RawMessage(`{"value":1}`)); err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func TestToolSchemaCacheHasBoundedCapacity(t *testing.T) {
	for i := 0; i < compiledToolSchemaCacheLimit+8; i++ {
		property := fmt.Sprintf("field_%d", i)
		def := model.Tool{Name: "Bounded", Parameters: map[string]any{
			"type": "object", "properties": map[string]any{
				property: map[string]any{"type": "string"},
			},
		}}
		if _, err := PrepareToolArguments(def, json.RawMessage(`{"`+property+`":"ok"}`)); err != nil {
			t.Fatal(err)
		}
	}
	compiledToolSchemas.Lock()
	entries, order := len(compiledToolSchemas.entries), len(compiledToolSchemas.order)
	compiledToolSchemas.Unlock()
	if entries > compiledToolSchemaCacheLimit || order > compiledToolSchemaCacheLimit {
		t.Fatalf("schema cache exceeded bound: entries=%d order=%d limit=%d", entries, order, compiledToolSchemaCacheLimit)
	}
}

func TestCoreToolSchemasCompileForExecutionValidation(t *testing.T) {
	for _, tool := range CoreTools() {
		t.Run(tool.Definition().Name, func(t *testing.T) {
			_, err := PrepareToolArguments(tool.Definition(), json.RawMessage(`{}`))
			if err != nil && (strings.Contains(err.Error(), "tool schema is invalid") ||
				strings.Contains(err.Error(), "tool schema cannot")) {
				t.Fatal(err)
			}
		})
	}
}

func TestTodoWriteSemanticValidationRunsBeforeTheSchema(t *testing.T) {
	def := todoWriteTool{}.Definition()
	// The tool keeps the enum constraint it advertises to the model.
	items, _ := def.Parameters["properties"].(map[string]any)["todos"].(map[string]any)["items"].(map[string]any)
	status, _ := items["properties"].(map[string]any)["status"].(map[string]any)
	if enum, _ := status["enum"].([]string); !slices.Contains(enum, string(TodoCompleted)) {
		t.Fatalf("todo status enum lost: %v", status)
	}
	// The tool's own rule explains the mistake instead of the schema
	// reporting a generic enum violation.
	_, err := PrepareToolArguments(def, json.RawMessage(`{"todos":[{"text":"x","status":"done"}]}`))
	if err == nil || !strings.Contains(err.Error(), "item status must be") {
		t.Fatalf("semantic error did not reach the model: %v", err)
	}
	// Rules the tool does not own stay with the schema.
	_, err = PrepareToolArguments(def, json.RawMessage(`{"todos":[{"text":"x","status":"pending"}],"extra":true}`))
	if err == nil || !strings.Contains(err.Error(), "additionalProperties") {
		t.Fatalf("schema constraint lost: %v", err)
	}
}

func TestRunReturnsSchemaErrorToModelForCorrection(t *testing.T) {
	steps, effects := 0, 0
	def := model.Tool{Name: "Agent", Parameters: map[string]any{
		"type": "object", "properties": map[string]any{"run_in_background": map[string]any{"type": "boolean"}},
	}}
	tool := schemaProbe{def: def, run: func(json.RawMessage) { effects++ }}
	backend := testModel{stream: func(request model.Request) (model.Result, error) {
		steps++
		switch steps {
		case 1:
			return model.Result{Message: model.Message{Role: "assistant", Calls: []model.Call{{
				ID: "bad", Name: "Agent", Arguments: `{"run_in_background":"yes"}`,
			}}}, Stop: model.StopTools}, nil
		case 2:
			last := request.History[len(request.History)-1]
			if !last.IsError || !strings.Contains(last.Text, "run_in_background must be a boolean") || effects != 0 {
				t.Fatalf("model did not receive actionable error before effects: %+v, effects=%d", last, effects)
			}
			return model.Result{Message: model.Message{Role: "assistant", Calls: []model.Call{{
				ID: "fixed", Name: "Agent", Arguments: `{"run_in_background":true}`,
			}}}, Stop: model.StopTools}, nil
		default:
			return model.Result{Message: model.Message{Role: "assistant", Text: "finished"}, Stop: model.StopComplete}, nil
		}
	}}
	err := Run(t.Context(), Options{Model: backend, Tools: []Tool{tool},
		Record: func(model.Message) error { return nil }, Emit: func(Event) error { return nil },
		Pending: func(bool) []model.Message { return nil },
	})
	if err != nil || effects != 1 || steps != 3 {
		t.Fatalf("correction failed: err=%v effects=%d steps=%d", err, effects, steps)
	}
}
