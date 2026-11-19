package desktop

import (
	"fmt"
	"reflect"
	"slices"
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

func TestTranscriptContinuationSumsRoundUsage(t *testing.T) {
	first := contracts.Message{ID: "one", Role: "assistant", TurnID: "turn",
		Usage: contracts.Usage{InputTokens: 100, OutputTokens: 20, CachedTokens: 40, Cost: .25, ContextTokens: 100, ContextLimit: 1000}}
	second := contracts.Message{ID: "two", Role: "assistant", TurnID: "turn",
		Usage: contracts.Usage{InputTokens: 30, OutputTokens: 10, CachedTokens: 5, Cost: .5, ContextTokens: 30, ContextLimit: 2000}}
	joined := transcriptEntries([]contracts.Message{first, second})
	want := contracts.Usage{InputTokens: 130, OutputTokens: 30, CachedTokens: 45, Cost: .75, ContextTokens: 30, ContextLimit: 2000}
	if len(joined) != 1 || joined[0].Usage != want {
		t.Fatalf("multi-round usage lost earlier requests or summed context occupancy: %+v", joined)
	}
	if first.Usage.InputTokens != 100 || second.Usage.InputTokens != 30 {
		t.Fatal("projection changed source accounting")
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
	}{
		{"normal complete", false, false, []string{"thought1/read", "narration", "thought2/run", "answer"}},
		{"normal streaming text", false, true, []string{"thought1/read", "narration", "thought2/run", "answer"}},
		{"compact complete", true, false, []string{"thought1/read/narration/thought2/run", "answer"}},
		{"compact streaming", true, true, []string{"thought1/read/narration/thought2/run/answer"}},
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

func TestTranscriptHooksJoinToolsAndReasoningInActivityGroups(t *testing.T) {
	start := contracts.Block{ID: "start", Kind: "hook", Hook: &contracts.HookRecord{
		Name: "startup", Event: contracts.HookSessionStart, Status: "completed",
	}}
	stop := contracts.Block{ID: "stop", Kind: "hook", Hook: &contracts.HookRecord{
		Name: "stop", Event: contracts.HookStop, Status: "completed",
	}}
	blocks := []contracts.Block{
		start,
		{ID: "thought", Kind: "reasoning", Text: "Inspect"},
		{ID: "read", Kind: "tool", Tool: &contracts.ToolCall{ID: "read", Kind: contracts.ToolReadFile}},
		{ID: "reply", Kind: "text", Text: "Done"},
		stop,
	}
	for _, test := range []struct {
		name               string
		compact, streaming bool
		want               []string
	}{
		{"normal", false, false, []string{"start/thought/read", "reply", "stop"}},
		{"compact", true, false, []string{"start/thought/read/stop", "reply"}},
		{"normal streaming", false, true, []string{"start/thought/read", "reply", "stop"}},
		{"compact streaming", true, true, []string{"start/thought/read/reply/stop"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			entries := transcriptEntries([]contracts.Message{
				{ID: "startup-message", Role: "assistant", TurnID: "turn", Blocks: blocks[:1]},
				{ID: "reply-message", Role: "assistant", TurnID: "turn", Blocks: blocks[1:], Streaming: test.streaming},
			})
			if len(entries) != 1 {
				t.Fatal("same-turn hook messages did not join the reply")
			}
			rows := transcriptRows(entries[0], test.compact)
			var got []string
			hooks, tools := 0, 0
			for _, row := range rows {
				if len(row.Activity) == 0 {
					if row.Block.Hook != nil {
						t.Fatal("hook rendered outside an activity group")
					}
					got = append(got, row.Block.ID)
					continue
				}
				var ids []string
				for _, block := range row.Activity {
					ids = append(ids, block.ID)
				}
				got = append(got, strings.Join(ids, "/"))
				hooks += row.Summary.Hooks
				tools += row.Summary.ToolCalls
			}
			if !reflect.DeepEqual(got, test.want) || hooks != 2 || tools != 1 {
				t.Fatalf("hook grouping or tool count incorrect: rows=%v hooks=%d tools=%d", got, hooks, tools)
			}
			if rows[0].ID != "startup-message:activity:start" {
				t.Fatal("hook group lost its identity across continuation messages")
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
	if len(rows) != 1 || len(rows[0].Activity) != 2 || rows[0].Activity[0].ID != "first" || !rows[0].Activity[0].Tool.Failed() {
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
		{ID: "run", Kind: "tool", Tool: &contracts.ToolCall{ID: "run", Kind: contracts.ToolExec}},
		{ID: "reply", Kind: "text", Text: "Here is the preview."},
		{ID: "image", Kind: "image", Attachment: &contracts.Attachment{ID: "image"}},
	}}
	rows := transcriptRows(m, true)
	if len(rows) != 4 || rows[0].Block.ID != "question" || len(rows[1].Activity) != 2 || rows[2].Block.ID != "reply" || rows[3].Block.ID != "image" {
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
		{"directory", contracts.ToolDirectory, "List directory", "folder-with-files", false},
		{"fetch", contracts.ToolWebFetch, "Read web page", "global", false},
		{"web", contracts.ToolWebSearch, "Search web", "global", false},
		{"todo", contracts.ToolTodo, "Todo", "checklist", false},
		{"task output", contracts.ToolTaskOutput, "Read task output", "terminal", false},
		{"stop", contracts.ToolTaskStop, "Stop task", "terminal", false},
		{"skill", contracts.ToolSkill, "Use skill", "widget", false},
		{"goal", contracts.ToolGoalRead, "Read goal", "checklist", false},
		{"agent", contracts.ToolAgent, "Start agent", "bot", false},
		{"mcp", contracts.ToolMCP, "Call tool", "widget", false},
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

func TestTranscriptToolDetailsRefreshForEqualLengthInputChanges(t *testing.T) {
	for _, field := range []string{"summary", "command", "path"} {
		t.Run(field, func(t *testing.T) {
			first := contracts.ToolCall{ID: "call", Kind: contracts.ToolSkill, Status: "running",
				Summary: "first", Command: "src/a", Path: "src/a", Args: `{"args":"src/a"}`}
			message := contracts.Message{ID: "message", Role: "assistant",
				Blocks: []contracts.Block{{ID: "block", Kind: "tool", Tool: &first}}}
			var cache transcriptCache
			cache.rowsFor(message, false)
			second := first
			switch field {
			case "summary":
				second.Summary = "other"
			case "command":
				second.Command = "src/b"
			case "path":
				second.Path = "src/b"
			}
			message.Blocks = []contracts.Block{{ID: "block", Kind: "tool", Tool: &second}}
			rows := cache.rowsFor(message, false)
			if got := rows[0].Activity[0].Tool; got.Summary != second.Summary ||
				got.Command != second.Command || got.Path != second.Path {
				t.Fatalf("same-length tool update retained stale presentation: %+v", got)
			}
		})
	}
}

func TestWorkedDurationUsesSharedFormat(t *testing.T) {
	a := testApp(t, "default")
	a.state.Settings.General.Language = "en"
	for _, test := range []struct {
		name string
		ms   int64
		want string
	}{
		{"seconds", 59000, "Worked for 59s"}, {"minutes", 61000, "Worked for 1m 01s"}, {"hours", 3661000, "Worked for 1h 01m 01s"}, {"days", 90000000, "Worked for 1d 01h 00m 00s"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := a.workedFor(test.ms); got != test.want {
				t.Fatalf("%s", got)
			}
		})
	}
}

func TestThoughtFlattenUsesSourceRecordLayout(t *testing.T) {
	lines := thoughtLines("# Check **the panel**\n\n- Read `main.go`\n- Keep *focus*\n\n> Avoid a [new control](https://example.com).\n\n| File | Result |\n|---|---|\n| main.go | pass |", false)
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
	for _, want := range []string{"Check the panel", "• Read main.go", "• Keep focus", "│ Avoid a new control.", "File │ Result", "main.go │ pass"} {
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
	wrapped := thoughtLines(long, false)
	for _, line := range wrapped {
		if len([]rune(thoughtPlain(line))) > 96 {
			t.Fatal("thought exceeds 96-column source budget")
		}
	}
	var log strings.Builder
	for i := 0; i < 31; i++ {
		fmt.Fprintf(&log, "line %d  \n", i)
	}
	for _, live := range []bool{false, true} {
		lines := thoughtLines(log.String(), live)
		if len(lines) != 31 || thoughtPlain(lines[0]) != "line 0" || thoughtPlain(lines[len(lines)-1]) != "line 30" {
			t.Fatalf("thought lost content: live=%t lines=%v", live, lines)
		}
	}
}

func TestTranscriptEntriesJoinsLongTurnInOnePass(t *testing.T) {
	messages := []contracts.Message{{ID: "user", Role: "user"}}
	for i := 1; i < 190; i++ {
		messages = append(messages, contracts.Message{
			ID: fmt.Sprintf("m%d", i), TurnID: "turn", Role: "assistant",
			Blocks: []contracts.Block{{ID: fmt.Sprintf("b%d", i), Kind: "tool"}, {ID: fmt.Sprintf("c%d", i), Kind: "reasoning"}},
		})
	}
	joined := transcriptEntries(messages)
	if len(joined) != 2 || len(joined[1].Blocks) != 2*(len(messages)-1) {
		t.Fatalf("long turn joined into %d entries with %d blocks", len(joined), len(joined[1].Blocks))
	}
	// The turn owns one growing block slice: a join per message must not copy
	// every block accumulated so far, or a long turn costs O(n²) each frame.
	if allocs := testing.AllocsPerRun(5, func() { transcriptEntries(messages) }); allocs > 16 {
		t.Fatalf("joining a long turn allocates %v times per call", allocs)
	}
}

func TestTranscriptCacheReusesRowsUntilContentChanges(t *testing.T) {
	cache := transcriptCache{}
	messages := []contracts.Message{
		{ID: "user", Role: "user", Blocks: []contracts.Block{{ID: "u", Kind: "text", Text: "hello"}}},
		{ID: "reply", Role: "assistant", Blocks: []contracts.Block{{ID: "t", Kind: "tool"}, {ID: "answer", Kind: "text", Text: "done"}}},
	}
	entries := cache.entriesFor(7, "chat", messages, false)
	if len(entries) != 2 {
		t.Fatalf("entries = %d", len(entries))
	}
	rows := cache.rowsFor(entries[1], false)
	if len(rows) != 2 {
		t.Fatalf("rows = %d", len(rows))
	}
	// The same snapshot reuses both the entries and their rows.
	if again := cache.entriesFor(7, "chat", messages, false); len(again) != 2 || !sameRows(cache.rowsFor(again[1], false), rows) {
		t.Fatal("unchanged snapshot re-projected the conversation")
	}
	// A streamed delta clones the message it changes: that revision must show
	// the new text, and must not reuse the old rows.
	streamed := []contracts.Message{messages[0], {ID: "reply", Role: "assistant", Streaming: true, Blocks: []contracts.Block{{ID: "t", Kind: "tool"}, {ID: "answer", Kind: "text", Text: "done, and more"}}}}
	entries = cache.entriesFor(8, "chat", streamed, false)
	if changed := cache.rowsFor(entries[1], false); len(changed) != 2 || changed[1].Block.Text != "done, and more" {
		t.Fatalf("changed message kept its old text: %+v", changed)
	} else if changed[1].Block.Text == rows[1].Block.Text {
		t.Fatal("changed message served the cached rows")
	}
	// Compact mode reshapes the rows.
	entries = cache.entriesFor(8, "chat", streamed, true)
	if rows := cache.rowsFor(entries[1], true); len(rows) != 2 || !rows[1].Compact {
		t.Fatalf("compact projection did not reshape: %+v", rows)
	}
}

func TestTranscriptTailUpdateMatchesFullProjection(t *testing.T) {
	for _, prefix := range []string{"", "side:chat:"} {
		for _, compact := range []bool{false, true} {
			t.Run(fmt.Sprintf("prefix=%q/compact=%t", prefix, compact), func(t *testing.T) {
				messages := railFixtureMessages(600)
				last := &messages[len(messages)-1]
				last.TurnID = "active"
				messages = append(messages, contracts.Message{
					ID: "continued", Role: "assistant", TurnID: "active", Streaming: true,
					Blocks: []contracts.Block{{ID: "partial", Kind: "text", Text: "Partial"}},
				})
				cache := transcriptCache{prefix: prefix}
				entries := cache.entriesFor(1, "chat", messages, compact)
				rows := cache.conversationFor(entries, compact)
				firstTick := &cache.rail[0]

				after := slices.Clone(messages)
				after[len(after)-1].Blocks = []contracts.Block{{ID: "partial", Kind: "text", Text: "Partial extended"}}
				cache.updateTail(2, messages, after)
				entries = cache.entriesFor(2, "chat", after, compact)
				gotRows := cache.conversationFor(entries, compact)
				if &gotRows[0] != &rows[0] || &cache.rail[0] != firstTick {
					t.Fatal("text append rebuilt history or outline")
				}
				full := transcriptCache{prefix: prefix}
				want := full.entriesFor(2, "chat", after, compact)
				wantRows := full.conversationFor(want, compact)
				if !reflect.DeepEqual(entries, want) || !reflect.DeepEqual(gotRows, wantRows) ||
					!reflect.DeepEqual(cache.tail.rows, full.tail.rows) || !reflect.DeepEqual(cache.rail, full.rail) ||
					cache.streamingText != full.streamingText {
					t.Fatal("incremental projection differed from a clean build")
				}
				if messages[len(messages)-1].Blocks[0].Text != "Partial" {
					t.Fatal("append mutated the previous snapshot")
				}
			})
		}
	}
}

func TestTranscriptTailUpdateInvalidatesWhenRowsChange(t *testing.T) {
	messages := []contracts.Message{
		{ID: "prompt", Role: "user"},
		{ID: "reply", Role: "assistant", Streaming: true, Blocks: []contracts.Block{{ID: "first", Kind: "text", Text: " "}}},
	}
	cache := transcriptCache{}
	old := cache.conversationFor(cache.entriesFor(1, "chat", messages, false), false)
	after := slices.Clone(messages)
	after[1].Blocks = []contracts.Block{{ID: "first", Kind: "text", Text: " First visible text"}}
	cache.updateTail(2, messages, after)
	if cache.revision != 1 {
		t.Fatal("a change from bare to content reused old row geometry")
	}
	rows := cache.conversationFor(cache.entriesFor(2, "chat", after, false), false)
	if !old[1].Bare || rows[1].Bare || cache.rail[0].Reply != "First visible text" {
		t.Fatalf("fallback did not refresh the row and preview: %+v %+v", rows, cache.rail)
	}
	afterAgain := slices.Clone(after)
	afterAgain[1].Blocks = []contracts.Block{{ID: "first", Kind: "text", Text: " First visible text continued"}}
	cache.updateTail(3, after, afterAgain)
	if cache.revision != 3 || cache.rail[0].Reply != "First visible text continued" {
		t.Fatal("optimized append did not refresh the preview")
	}
}

func TestResolveActivityCopiesOnlyWhenResultsReplaceCalls(t *testing.T) {
	call := contracts.Block{ID: "first", Tool: &contracts.ToolCall{ID: "tool"}}
	text := contracts.Block{ID: "text", Kind: "text", Text: "Next"}
	second := contracts.Block{ID: "second", Tool: &contracts.ToolCall{ID: "second"}}
	unique := []contracts.Block{call, text, second}
	if got := resolvedActivity(unique); &got[0] != &unique[0] {
		t.Fatal("a unique block list was copied")
	}
	result := contracts.Block{ID: "result", Tool: &contracts.ToolCall{ID: "tool", Output: "Done"}}
	secondResult := contracts.Block{ID: "second-result", Tool: &contracts.ToolCall{ID: "second", Output: "Second done"}}
	input := []contracts.Block{call, result, text, second, secondResult}
	got := resolvedActivity(input)
	if len(got) != 3 || got[0].ID != "first" || got[0].Tool.Output != "Done" || got[1].ID != "text" ||
		got[2].ID != "second" || got[2].Tool.Output != "Second done" || input[0].Tool.Output != "" {
		t.Fatalf("tool replacement changed order, identity or source: %+v", got)
	}
}

func sameRows(a, b []transcriptRow) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].Compact != b[i].Compact {
			return false
		}
	}
	return true
}

func TestActivitySummaryProjectionRefreshesOnlyWithItsContent(t *testing.T) {
	cache := transcriptCache{}
	message := contracts.Message{ID: "summary-message", Role: "assistant", Blocks: []contracts.Block{{ID: "tool", Kind: "tool", Tool: &contracts.ToolCall{ID: "tool", Kind: contracts.ToolExec, Status: "completed"}}}}
	rows := cache.rowsFor(message, false)
	if len(rows) != 1 || rows[0].Summary.Commands != 1 {
		t.Fatal("activity summary not projected")
	}
	if allocs := testing.AllocsPerRun(20, func() { cache.rowsFor(message, false) }); allocs != 0 {
		t.Fatalf("unchanged projection rebuilt summary: %v", allocs)
	}
	updated := message
	updated.Blocks = contracts.CloneBlocks(message.Blocks)
	updated.Blocks[0].Tool.Kind = contracts.ToolReadFile
	updated.Blocks[0].Tool.Status = "failed"
	rows = cache.rowsFor(updated, false)
	if rows[0].Summary.Reads != 1 || rows[0].Summary.Commands != 0 || rows[0].Summary.Failed != 1 {
		t.Fatal("changed tool kind/status retained old summary")
	}
}
