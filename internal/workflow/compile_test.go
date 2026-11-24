package workflow

import (
	"strings"
	"testing"

	"gcode/internal/contracts"
)

const compileFixture = `{
  "name": "Review flow",
  "description": "Read, then check.",
  "arguments": [
    {"name": "target", "type": "string", "required": true, "description": "What to review"},
    {"name": "depth", "type": "number", "default": "2"},
    {"name": "strict", "type": "boolean"}
  ],
  "phases": [
    {
      "id": "collect",
      "name": "Collect",
      "instances": [
        {"id": "reader", "name": "Reader", "prompt": "Read {{target}}", "modelId": "fast"},
        {"id": "scout"}
      ],
      "edges": [
        {"from": "collect", "to": "review", "kind": "next", "when": "phase.collect.status == \"done\""}
      ]
    },
    {
      "id": "review",
      "instances": [{"id": "critic"}],
      "edges": [{"to": "collect", "kind": "loop", "label": "again", "when": "phase.review.status != \"done\""}]
    }
  ]
}`

func TestCompileScriptBuildsTheDeclaredProgram(t *testing.T) {
	compiled := CompileScript(compileFixture)
	if compiled.Failed() {
		t.Fatalf("unexpected diagnostics: %s", compiled.Report())
	}
	if compiled.Name != "Review flow" || compiled.Description != "Read, then check." {
		t.Fatalf("name and description were not read: %#v", compiled)
	}
	if len(compiled.Arguments) != 3 {
		t.Fatalf("arguments = %d, want 3", len(compiled.Arguments))
	}
	if compiled.Arguments[0].Name != "target" || !compiled.Arguments[0].Required || compiled.Arguments[0].Type != "string" {
		t.Fatalf("first argument = %#v", compiled.Arguments[0])
	}
	if compiled.Arguments[1].Type != "number" || !compiled.Arguments[1].HasDefault || compiled.Arguments[1].Default != "2" {
		t.Fatalf("second argument = %#v", compiled.Arguments[1])
	}
	if compiled.PhaseCount() != 2 || compiled.ActorCount() != 3 {
		t.Fatalf("phases = %d, actors = %d", compiled.PhaseCount(), compiled.ActorCount())
	}
	collect := compiled.Program.Phases[0]
	if collect.ID != "collect" || collect.Name != "Collect" || len(collect.Instances) != 2 {
		t.Fatalf("first phase = %#v", collect)
	}
	// 没写名字的实例保持空名：界面用它在阶段里的序号称呼它，不拿 id 顶替。
	if collect.Instances[0].ModelID != "fast" || collect.Instances[1].Name != "" {
		t.Fatalf("instances = %#v", collect.Instances)
	}
	if review := compiled.Program.Phases[1]; review.Name != "" {
		t.Fatalf("an unnamed phase must keep its empty name: %#v", review)
	}
	if len(collect.Edges) != 1 || collect.Edges[0].To != "review" || collect.Edges[0].When == "" {
		t.Fatalf("edges = %#v", collect.Edges)
	}
	// 边写在第二阶段时，From 省略就等于声明它的阶段。
	review := compiled.Program.Phases[1]
	if review.Edges[0].From != "review" || review.Edges[0].Kind != "loop" || review.Edges[0].Label != "again" {
		t.Fatalf("loop edge = %#v", review.Edges[0])
	}
}

func TestCompileScriptReportsBrokenScripts(t *testing.T) {
	cases := []struct {
		name   string
		source string
		code   string
		path   string
	}{
		{name: "empty", source: "   ", code: "empty_script"},
		{name: "not json", source: "{", code: "invalid_json"},
		{name: "array root", source: "[]", code: "invalid_json"},
		{name: "two documents", source: `{"phases":[]}{"phases":[]}`, code: "invalid_json"},
		{name: "no phases", source: `{"name":"x"}`, code: "phases_missing", path: "phases"},
		{name: "empty phases", source: `{"phases":[]}`, code: "phases_missing", path: "phases"},
		{name: "phase without id", source: `{"phases":[{"name":"A"}]}`, code: "phase_id_missing", path: "phases[0]"},
		{name: "bad phase id", source: `{"phases":[{"id":"a b"}]}`, code: "phase_id_invalid", path: "phases[0].id"},
		{name: "duplicate phase", source: `{"phases":[{"id":"a"},{"id":"a"}]}`, code: "phase_id_duplicate", path: "phases[1].id"},
		{name: "unknown target", source: `{"phases":[{"id":"a","edges":[{"to":"nowhere"}]}]}`, code: "edge_target_unknown", path: "phases[0].edges[0].to"},
		{name: "unknown kind", source: `{"phases":[{"id":"a"},{"id":"b","edges":[{"to":"a","kind":"jump"}]}]}`, code: "edge_kind_invalid", path: "phases[1].edges[0].kind"},
		{name: "bad condition", source: `{"phases":[{"id":"a","edges":[{"to":"a","kind":"loop","when":"a &&"}]}]}`, code: "edge_condition_invalid", path: "phases[0].edges[0].when"},
		{name: "bad argument type", source: `{"arguments":[{"name":"x","type":"date"}],"phases":[{"id":"a"}]}`, code: "argument_type_invalid", path: "arguments[0].type"},
		{name: "argument without name", source: `{"arguments":[{"type":"string"}],"phases":[{"id":"a"}]}`, code: "argument_name_missing", path: "arguments[0]"},
		{name: "duplicate instance", source: `{"phases":[{"id":"a","instances":[{"id":"x"},{"id":"x"}]}]}`, code: "instance_id_duplicate", path: "phases[0].instances[1].id"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			compiled := CompileScript(testCase.source)
			if !compiled.Failed() {
				t.Fatalf("script compiled without an error: %#v", compiled)
			}
			for _, diagnostic := range compiled.Diagnostics {
				if diagnostic.Code != testCase.code {
					continue
				}
				if testCase.path != "" && diagnostic.Path != testCase.path {
					t.Fatalf("diagnostic path = %q, want %q", diagnostic.Path, testCase.path)
				}
				return
			}
			t.Fatalf("diagnostic %q not found in %s", testCase.code, compiled.Report())
		})
	}
}

func TestCompileScriptWarnsAboutUnknownFields(t *testing.T) {
	compiled := CompileScript(`{"phases":[{"id":"a","stage":"A"}],"budget":3}`)
	if compiled.Failed() {
		t.Fatalf("unknown fields must not block the run: %s", compiled.Report())
	}
	report := compiled.Report()
	if !strings.Contains(report, "budget") || !strings.Contains(report, "phases[0].stage") {
		t.Fatalf("report = %q", report)
	}
	if !strings.Contains(report, contracts.WorkflowDiagnosticWarning) {
		t.Fatalf("report should name the severity: %q", report)
	}
}

func TestConditionsCompareFacts(t *testing.T) {
	facts := map[string]string{
		"phase.review.status": "failed",
		"hole.input":          "",
		"answer.text":         "ship it",
		"actor.critic.status": "done",
	}
	cases := []struct {
		expression string
		want       bool
	}{
		{expression: `phase.review.status == "failed"`, want: true},
		{expression: `phase.review.status == failed`, want: true},
		{expression: `phase.review.status != "failed"`, want: false},
		{expression: "!hole.input", want: true},
		{expression: "hole.input", want: false},
		{expression: `actor.critic.status == "done" && phase.review.status == "failed"`, want: true},
		{expression: `actor.critic.status == "failed" || phase.review.status == "failed"`, want: true},
		{expression: `phase.review.status == "done" || actor.critic.status == "done" && hole.input`, want: false},
		{expression: `(phase.review.status == "done" || actor.critic.status == "done") && !hole.input`, want: true},
		{expression: `phase.missing.status`, want: false},
		{expression: `phase.missing.status == ""`, want: true},
		{expression: `answer.text == "ship it"`, want: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.expression, func(t *testing.T) {
			condition, err := ParseCondition(testCase.expression)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got := condition.Eval(facts); got != testCase.want {
				t.Fatalf("Eval = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestConditionsRejectMalformedExpressions(t *testing.T) {
	cases := []string{"", "   ", "a &&", "&& a", "(a", "a)", "a & b", "a | b", "a = b", `a == `, "!", "a b"}
	for _, expression := range cases {
		t.Run(expression, func(t *testing.T) {
			if _, err := ParseCondition(expression); err == nil {
				t.Fatalf("%q parsed without an error", expression)
			}
		})
	}
}

func TestNilConditionAlwaysHolds(t *testing.T) {
	var condition *Condition
	if !condition.Eval(nil) {
		t.Fatal("a missing condition means the edge always applies")
	}
	if condition.Text() != "" {
		t.Fatalf("Text = %q", condition.Text())
	}
}
