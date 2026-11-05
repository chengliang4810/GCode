package engine

import (
	"testing"
	"time"

	"gcode/internal/model"
)

func TestToolPresentationDecodesReadableFieldsAndPreservesArguments(t *testing.T) {
	for _, test := range []struct {
		name, tool, args, command, summary string
	}{
		{"command", "Bash", `{"command":"go test ./...","timeout":60}`, "go test ./...", "Bash"},
		{"search", "Grep", `{"pattern":"func main","path":"src"}`, "", "func main · src"},
		{"Glob", "Glob", `{"pattern":"**/*.go"}`, "", "**/*.go"},
		{"directory", "ListDirectory", `{"path":"src"}`, "", "src"},
		{"invalid", "Bash", `{"command":`, "", "Bash"},
		{"todo", "TodoWrite", `{"todos":[{"text":"one","status":"completed"},{"text":"two","status":"in_progress"}]}`, "", "1/2"},
	} {
		t.Run(test.name, func(t *testing.T) {
			started := time.Now()
			call := model.Call{ID: "call", Name: test.tool, Arguments: test.args}
			got := toolPresentation(call, "/workspace/src", started)
			if got.Command != test.command || got.Summary != test.summary || got.Args != test.args {
				t.Fatalf("incorrect readable projection: %+v", got)
			}
			if got.ID != call.ID || got.Path != "/workspace/src" || got.StartedAt != started {
				t.Fatalf("lost call identity: %+v", got)
			}
		})
	}
}

// A read reports the window it actually returned, so a row can state it
// without the file's text reaching the transcript.
func TestReadLineCountFollowsTheTypedResult(t *testing.T) {
	text := map[string]any{"type": "text", "file": map[string]any{"numLines": 80, "startLine": 1, "totalLines": 120}}
	rewritten := map[string]any{"type": "text", "file": map[string]any{"numLines": float64(3)}}
	for _, test := range []struct {
		name string
		data any
		want int
	}{
		{"typed text read", text, 80},
		{"hook rewrite", rewritten, 3},
		{"image read", map[string]any{"type": "image", "file": map[string]any{"numLines": 9}}, 0},
		{"replaced result", "plain text", 0},
		{"no result", nil, 0},
		{"negative window", map[string]any{"type": "text", "file": map[string]any{"numLines": -1}}, 0},
	} {
		if got := readLineCount(test.data); got != test.want {
			t.Fatalf("%s: readLineCount = %d, want %d", test.name, got, test.want)
		}
	}
}
