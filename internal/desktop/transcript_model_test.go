package desktop

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"gcode/internal/contracts"
)

func TestTranscriptContinuationJoinsOnlyAdjacentTurn(t *testing.T) {
	first := contracts.Message{ID: "first", TurnID: "turn", Role: "assistant", Blocks: []contracts.Block{{ID: "thought", Kind: "reasoning", Text: "Check geometry"}}, Streaming: true}
	second := contracts.Message{ID: "second", TurnID: "turn", ContinuationOf: "first", Role: "assistant", Blocks: []contracts.Block{{ID: "reply", Kind: "text", Text: "Done"}}, DurationMS: 9200}
	user := contracts.Message{ID: "user", Role: "user"}
	joined := transcriptEntries([]contracts.Message{first, second, user, second})
	if len(joined) != 3 || len(joined[0].Blocks) != 2 || joined[0].ID != first.ID || joined[0].Streaming || joined[0].DurationMS != 9200 {
		t.Fatalf("joined entry lost timeline identity or completion: %+v", joined)
	}
	joined[0].Blocks[0].Text = "changed view"
	if first.Blocks[0].Text != "Check geometry" {
		t.Fatal("projection mutated backend blocks")
	}
	chain := transcriptEntries([]contracts.Message{
		{ID: "one", Role: "assistant"},
		{ID: "two", Role: "assistant", ContinuationOf: "one"},
		{ID: "three", Role: "assistant", ContinuationOf: "two"},
	})
	if len(chain) != 1 || chain[0].ID != "one" {
		t.Fatalf("continuation chain split after first join: %+v", chain)
	}
}

func TestTranscriptRowsPreserveActivityAndNarrationOrder(t *testing.T) {
	blocks := []contracts.Block{
		{ID: "thought1", Kind: "reasoning", Text: "Inspect"},
		{ID: "read", Kind: "tool", Tool: &contracts.ToolCall{ID: "read", Kind: contracts.ToolReadFile, Path: "main.go"}},
		{ID: "narration", Kind: "text", Text: "The editor uses this width."},
		{ID: "thought2", Kind: "reasoning", Text: "Check resize"},
		{ID: "run", Kind: "tool", Tool: &contracts.ToolCall{ID: "run", Kind: contracts.ToolExec, Command: "go test ./..."}},
		{ID: "answer", Kind: "text", Text: "Verified the change."},
	}
	for _, test := range []struct {
		name               string
		compact, streaming bool
		want               []string
		auto               bool
	}{
		{"normal complete", false, false, []string{"thought1/read", "narration", "thought2/run", "answer"}, false},
		{"normal streaming text", false, true, []string{"thought1/read", "narration", "thought2/run", "answer"}, false},
		{"compact complete", true, false, []string{"thought1/read/narration/thought2/run", "answer"}, false},
		{"compact streaming", true, true, []string{"thought1/read/narration/thought2/run/answer"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			rows := transcriptRows(contracts.Message{ID: "entry", Blocks: blocks, Streaming: test.streaming}, test.compact)
			var got []string
			for _, row := range rows {
				if len(row.Activity) > 0 {
					var ids []string
					for _, block := range row.Activity {
						ids = append(ids, block.ID)
					}
					got = append(got, strings.Join(ids, "/"))
					if row.AutoOpen != test.auto {
						t.Fatalf("unexpected automatic fold state %+v", row)
					}
				} else {
					got = append(got, row.Block.ID)
				}
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("order %v, want %v", got, test.want)
			}
		})
	}
}

func TestTranscriptLiveTailAndCallResultIdentity(t *testing.T) {
	m := contracts.Message{ID: "entry", Streaming: true, Blocks: []contracts.Block{
		{ID: "first", Kind: "toolCall", Tool: &contracts.ToolCall{ID: "cmd", Kind: contracts.ToolExec, Status: "working"}},
		{ID: "result", Kind: "toolResult", Tool: &contracts.ToolCall{ID: "cmd", Kind: contracts.ToolExec, Status: "failed", Output: "exit status 1"}},
		{ID: "tail", Kind: "reasoning", Text: "Inspect failure"},
	}}
	rows := transcriptRows(m, false)
	if len(rows) != 1 || !rows[0].AutoOpen || len(rows[0].Activity) != 2 || rows[0].Activity[0].ID != "first" || !rows[0].Activity[0].Tool.Failed() {
		t.Fatalf("call/result did not update in place: %+v", rows)
	}
	if contracts.SummarizeActivity(rows[0].Activity).Failed != 1 {
		t.Fatal("failure summary did not use resolved state")
	}
}

func TestTranscriptCompactKeepsRequestsAndFinalImagesInDocumentOrder(t *testing.T) {
	m := contracts.Message{ID: "entry", Blocks: []contracts.Block{
		{ID: "question", Kind: "question", Question: &contracts.Question{ID: "question"}},
		{ID: "thought", Kind: "reasoning", Text: "Check"},
		{ID: "permission", Kind: "permission", Permission: &contracts.Permission{ID: "permission"}},
		{ID: "run", Kind: "tool", Tool: &contracts.ToolCall{ID: "run", Kind: contracts.ToolExec}},
		{ID: "reply", Kind: "text", Text: "Here is the preview."},
		{ID: "image", Kind: "image", Attachment: &contracts.Attachment{ID: "image"}},
	}}
	rows := transcriptRows(m, true)
	if len(rows) != 5 || rows[0].Block.ID != "question" || len(rows[1].Activity) != 2 || rows[2].Block.ID != "permission" || rows[3].Block.ID != "reply" || rows[4].Block.ID != "image" {
		t.Fatalf("compact order: %+v", rows)
	}
}

func TestActivityTypedLabelsAndPaths(t *testing.T) {
	for _, test := range []struct {
		name         string
		kind         contracts.ToolKind
		label, glyph string
		file         bool
	}{
		{"run", contracts.ToolExec, "Run", "terminal", false},
		{"read", contracts.ToolReadFile, "Read", "document", true},
		{"write", contracts.ToolWriteFile, "Write", "document-add", true},
		{"edit", contracts.ToolEditFile, "Edit", "pen", true},
		{"patch", contracts.ToolApplyPatch, "Patch", "document", true},
		{"search", contracts.ToolSearch, "Search", "magnifer", false},
		{"glob", contracts.ToolGlob, "Glob", "folder-with-files", false},
		{"fetch", contracts.ToolWebFetch, "Fetch", "global", false},
		{"web", contracts.ToolWebSearch, "Web", "global", false},
		{"todo", contracts.ToolTodo, "Todo", "checklist", false},
		{"mcp", contracts.ToolMCP, "Tool", "widget", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			b := contracts.Block{Kind: "tool", Tool: &contracts.ToolCall{Kind: test.kind, Path: "src/main.go", Command: "go test ./...", Name: "server.query"}}
			label, _, glyph := activityTitle(b)
			if label != test.label || glyph != test.glyph || activityFile(b) != test.file {
				t.Fatalf("label=%s icon=%s file=%v", label, glyph, activityFile(b))
			}
		})
	}
	_, detail, _ := activityTitle(contracts.Block{Kind: "reasoning", Text: "Full reasoning stays inside the disclosure"})
	if detail != "" {
		t.Fatal("pinned .128 Thought header has no preview")
	}
	if activityBasename(`C:\work\src\main.go`) != "main.go" || activityBasename("src/main.go") != "main.go" {
		t.Fatal("cross-platform basename mismatch")
	}
}

func TestActivityLineCapsAndElapsedFormatting(t *testing.T) {
	text := strings.Repeat("line\n", 30) + "last"
	head, n := activityLines(text, false, 24)
	tail, m := activityLines(text, true, 24)
	if n != 7 || m != 7 || strings.Contains(head, "last") || !strings.HasSuffix(tail, "last") {
		t.Fatal("head/tail cap mismatch")
	}
	for _, test := range []struct {
		name string
		ms   int64
		want string
	}{
		{"seconds", 59000, "Worked for 59s"}, {"minutes", 61000, "Worked for 1m 1s"}, {"hours", 3661000, "Worked for 1h 1m"}, {"days", 90000000, "Worked for 1d 1h"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := workedFor(test.ms); got != test.want {
				t.Fatalf("%s", got)
			}
		})
	}
}

func TestThoughtFlattenUsesSourceRecordLayout(t *testing.T) {
	lines, _ := thoughtLines("# Check **the panel**\n\n- Read `main.go`\n- Keep *focus*\n\n> Avoid a [new control](https://example.com).\n\n| File | Result |\n|---|---|\n| main.go | pass |", false)
	var text []string
	bold, code, italic, link := false, false, false, false
	for _, line := range lines {
		text = append(text, thoughtPlain(line))
		for _, run := range line {
			bold = bold || run.style.bold
			code = code || run.style.code
			italic = italic || run.style.italic
			link = link || run.style.link
		}
	}
	got := strings.Join(text, "\n")
	for _, want := range []string{"Check the panel", "• Read main.go", "• Keep focus", "│ Avoid a new control.", "File · Result", "main.go · pass"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
	if !bold || !code || !italic || !link {
		t.Fatal("flatten discarded inline styles")
	}
	if strings.Contains(got, "**") || strings.Contains(got, "https://") {
		t.Fatal("markdown syntax leaked into record")
	}
	long := strings.Repeat("word ", 1000)
	wrapped, _ := thoughtLines(long, false)
	for _, line := range wrapped {
		if len([]rune(thoughtPlain(line))) > 96 {
			t.Fatal("thought exceeds 96-column source budget")
		}
	}
	var log strings.Builder
	for i := 0; i < 31; i++ {
		fmt.Fprintf(&log, "line %d  \n", i)
	}
	head, headCut := thoughtLines(log.String(), false)
	tail, tailCut := thoughtLines(log.String(), true)
	if len(head) != 24 || len(tail) != 24 || headCut != 7 || tailCut != 7 || thoughtPlain(head[0]) != "line 0" || thoughtPlain(tail[0]) != "line 7" || thoughtPlain(tail[len(tail)-1]) != "line 30" {
		t.Fatalf("thought head/tail truncation: head=%v/%d tail=%v/%d", head, headCut, tail, tailCut)
	}
}
