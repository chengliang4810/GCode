package desktop

import (
	"fmt"
	"strings"
	"testing"

	"gcode/internal/appmodel"
	"github.com/egoist/mygo/ui"
)

// Unlike the history benchmarks, this models one cumulative SSE message
// while its background Markdown result is still waiting on the UI queue.
// Tester uses the platform text stack (CoreText on Darwin) and the same
// list/leafWidths layout path as the native window; it does not exercise AX.
func BenchmarkConversationGrowingMessagePending(b *testing.B) {
	for _, chunks := range []int{30, 100, 300} {
		b.Run(fmt.Sprintf("chunks=%d", chunks), func(b *testing.B) {
			var text strings.Builder
			for i := 0; i < chunks; i++ {
				fmt.Fprintf(&text, "第 %d 个增量：中文流式内容，emoji 🧪。\n\n持续输出尾部令牌：STREAM_TAIL\n\n", i)
				if i%5 == 0 {
					text.WriteString("```go\nfunc streamChunk() string { return \"chunk\" }\n```\n\n| 序号 | 内容 |\n| ---: | --- |\n| 5 | 中文与 emoji 🧪 |\n\n")
				}
			}
			source := text.String()
			snapshot := appmodel.Snapshot{Sessions: []appmodel.Session{{
				ID: "growing", Messages: []appmodel.Message{{ID: "assistant", Role: "assistant", Text: source}},
			}}}
			app := Open(nil, OpenOptions{Initial: snapshot, Dispatch: func(func()) {}})
			defer app.Close()
			tester := ui.NewTester(app.view, 1440, 900)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				snapshot.Sessions[0].Messages[0].Text = source + fmt.Sprintf("第 %d 个新增量：新内容 🧪。\n\nSTREAM_TAIL", i)
				app.Update(snapshot)
				tester.Frame()
			}
		})
	}
}
