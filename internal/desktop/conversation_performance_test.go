package desktop

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"gcode/internal/appmodel"
	"github.com/egoist/mygo/ui"
)

// performanceSnapshot is deliberately deterministic. Every message has
// Chinese prose, Markdown structure and a Go code block; every fifth message
// also carries a completed tool call. The fixture exercises the same list,
// Markdown and tool rendering paths as a normal coding conversation without
// depending on a service, network, or user data.
func performanceSnapshot(sessionID string, count int) appmodel.Snapshot {
	messages := make([]appmodel.Message, count)
	for i := range messages {
		role := "assistant"
		if i%2 == 0 {
			role = "user"
		}
		text := strings.Join([]string{
			fmt.Sprintf("第 %04d 条消息：检查中文输出、Markdown 排版和可视范围。", i),
			"",
			fmt.Sprintf("- 结果项 %d 已完成，保留稳定的阅读锚点。", i),
			fmt.Sprintf("```go\nfunc step%04d() string { return \"消息 %04d\" }\n```", i, i),
			"",
			"| 字段 | 值 |",
			"| --- | --- |",
			fmt.Sprintf("| index | %d |", i),
		}, "\n")
		message := appmodel.Message{
			ID:        fmt.Sprintf("%s-%05d", sessionID, i),
			Role:      role,
			Text:      text,
			CreatedAt: time.Unix(int64(i), 0),
		}
		if i%5 == 0 {
			message.Tools = []appmodel.ToolCall{{
				ID:        fmt.Sprintf("tool-%s-%05d", sessionID, i),
				Name:      "exec_command",
				Arguments: fmt.Sprintf(`{"cmd":"go test ./internal/desktop","cwd":"/tmp/%s"}`, sessionID),
				Output:    fmt.Sprintf("测试消息 %04d 已完成", i),
				State:     "done",
			}}
		}
		messages[i] = message
	}
	return appmodel.Snapshot{Sessions: []appmodel.Session{{
		ID:       sessionID,
		Title:    "长会话性能 fixture",
		Cwd:      "/tmp/" + sessionID,
		State:    appmodel.Idle,
		Messages: messages,
	}}}
}

func newPerformanceTester(count int) (*App, *ui.Tester) {
	app := Open(nil, OpenOptions{Initial: performanceSnapshot("performance", count)})
	return app, ui.NewTester(app.view, 900, 640)
}

func BenchmarkConversationInitialFrame(b *testing.B) {
	for _, count := range []int{100, 1000, 5000} {
		b.Run(fmt.Sprintf("messages=%d", count), func(b *testing.B) {
			snapshot := performanceSnapshot("initial", count)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				app := Open(nil, OpenOptions{Initial: snapshot})
				_ = ui.NewTester(app.view, 900, 640)
				_ = app.Close()
			}
		})
	}
}

func BenchmarkConversationWarmFrameAtEnd(b *testing.B) {
	for _, count := range []int{100, 1000, 5000} {
		b.Run(fmt.Sprintf("messages=%d", count), func(b *testing.B) {
			app, tester := newPerformanceTester(count)
			defer app.Close()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tester.Frame()
			}
		})
	}
}

func BenchmarkConversationWarmFrameInMiddle(b *testing.B) {
	for _, count := range []int{100, 1000, 5000} {
		b.Run(fmt.Sprintf("messages=%d", count), func(b *testing.B) {
			app, tester := newPerformanceTester(count)
			defer app.Close()
			list := app.conversationListState("performance")
			list.ScrollTo(count/2, ui.Start)
			tester.Frame()
			first, last := list.Visible()
			if first > count/2 || count/2 >= last {
				b.Fatalf("middle row is not visible: %d..%d for %d", first, last, count/2)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tester.Frame()
			}
		})
	}
}

func BenchmarkConversationStreamingUpdate(b *testing.B) {
	for _, count := range []int{100, 1000, 5000} {
		b.Run(fmt.Sprintf("messages=%d", count), func(b *testing.B) {
			snapshot := performanceSnapshot("streaming", count)
			app := Open(nil, OpenOptions{Initial: snapshot})
			tester := ui.NewTester(app.view, 900, 640)
			defer app.Close()
			last := len(snapshot.Sessions[0].Messages) - 1
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// App.Update models a controller event. The update clones the
				// snapshot, and the following frame renders the growing last row.
				snapshot.Sessions[0].Messages[last].Text = strings.Join([]string{
					fmt.Sprintf("第 %04d 条消息继续流式增长。", last),
					"",
					fmt.Sprintf("增量片段 %d：中文内容仍在底部追加。", i%32),
					"```go\nreturn streamingChunk\n```",
				}, "\n")
				app.Update(snapshot)
				tester.Frame()
			}
		})
	}
}

func BenchmarkConversationSnapshotClone(b *testing.B) {
	for _, count := range []int{100, 1000, 5000} {
		b.Run(fmt.Sprintf("messages=%d", count), func(b *testing.B) {
			snapshot := performanceSnapshot("clone", count)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = cloneSnapshot(snapshot)
			}
		})
	}
}

func BenchmarkConversationMarkdownParse(b *testing.B) {
	source := performanceSnapshot("parse", 1).Sessions[0].Messages[0].Text
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = parseMarkdownDocument(source)
	}
}

func TestConversationPerformanceViewportAndMarkdownReuse(t *testing.T) {
	const count = 5000
	app, tester := newPerformanceTester(count)
	t.Cleanup(func() { _ = app.Close() })

	list := app.conversationListState("performance")
	first, last := list.Visible()
	if last <= first || last-first > 80 {
		t.Fatalf("visible range is not bounded: %d..%d", first, last)
	}
	if first < count-80 {
		t.Fatalf("initial viewport did not start near the end: first=%d count=%d", first, count)
	}
	oldLatestMarker := fmt.Sprintf("第 %04d 条消息", count-1)
	if !tester.HasText(oldLatestMarker) {
		t.Fatalf("initial viewport did not render the latest message body %q: %v", oldLatestMarker, tester.Texts())
	}

	unchangedIndex := last - 1
	if unchangedIndex >= count {
		unchangedIndex = count - 1
	}
	unchangedKey := "performance:" + conversationMessageKey(app.data.Sessions[0].Messages[unchangedIndex], unchangedIndex)
	app.mu.RLock()
	before := app.markdownCache[unchangedKey]
	app.mu.RUnlock()
	if !before.ready || len(before.document.blocks) == 0 {
		t.Fatalf("visible Markdown was not cached for %q", unchangedKey)
	}
	beforeBlock := &before.document.blocks[0]

	snapshot := performanceSnapshot("performance", count)
	lastIndex := len(snapshot.Sessions[0].Messages) - 1
	snapshot.Sessions[0].Messages[lastIndex].Text = "流式增量已到达。\n\n```go\nreturn streamingChunk\n```"
	app.Update(snapshot)
	tester.Frame()
	if !tester.HasText("流式增量已到达") {
		t.Fatalf("updated latest message body is not visible: %v", tester.Texts())
	}
	if tester.HasText(oldLatestMarker) {
		t.Fatalf("old latest message body remained visible after replacement: %v", tester.Texts())
	}

	app.mu.RLock()
	after := app.markdownCache[unchangedKey]
	app.mu.RUnlock()
	if !after.ready || len(after.document.blocks) == 0 || &after.document.blocks[0] != beforeBlock {
		t.Fatalf("unchanged visible Markdown was reparsed")
	}
}

func TestConversationPerformanceMiddleViewport(t *testing.T) {
	const count = 5000
	app, tester := newPerformanceTester(count)
	t.Cleanup(func() { _ = app.Close() })

	list := app.conversationListState("performance")
	middle := count / 2
	list.ScrollTo(middle, ui.Start)
	tester.Frame()
	first, last := list.Visible()
	if first > middle || middle >= last {
		t.Fatalf("middle row is not visible after ScrollTo: %d..%d for %d", first, last, middle)
	}
	if first == 0 || last >= count {
		t.Fatalf("middle viewport unexpectedly covers an edge: %d..%d for %d", first, last, count)
	}
}
