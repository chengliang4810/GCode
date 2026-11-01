package desktop

import (
	"fmt"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func TestActivityDetailTenThousandLinesShapesOnlyVisibleRows(t *testing.T) {
	a := testApp(t, "default")
	a.palette = design.Palette("rcode", false)
	var text strings.Builder
	for i := range 10000 {
		fmt.Fprintf(&text, "%05d\n", i)
	}
	block := contracts.Block{ID: "ten-thousand", Kind: "tool", Tool: &contracts.ToolCall{Output: text.String()}}
	visible := true
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(a.palette.UI())
		ui.Column(c).Fill().Children(func() {
			if visible {
				a.activityTextDetail(c, block, false)
			}
		})
	}, 480, 700)
	key := activityDetailKey{block: block.ID, session: firstNonempty(a.linkSource, a.sessionID)}
	entry := a.activityDetails.entries[key]
	if entry == nil || len(entry.lines) != 10000 {
		t.Fatal("complete detail was not retained")
	}
	if len(entry.measured) > 2*activityDetailLines {
		t.Fatalf("first viewport measured %d of 10000 rows", len(entry.measured))
	}
	t.Logf("first viewport measured %d of %d retained rows", len(entry.measured), len(entry.lines))
	firstLine := &entry.lines[0]
	visible = false
	tt.Frame()
	visible = true
	tt.Frame()
	if reopened := a.activityDetails.entries[key]; reopened != entry || &reopened.lines[0] != firstLine {
		t.Fatal("closing and reopening reparsed the detail")
	}
	rect, ok := tt.Find("Activity detail")
	if !ok {
		t.Fatal("detail viewport missing")
	}
	tt.Scroll(rect.X+20, rect.Y+20, 0, 1e6)
	if !tt.HasText("09999") || len(entry.measured) > 4*activityDetailLines {
		t.Fatalf("end navigation shaped too much or lost content: measured=%d", len(entry.measured))
	}
	t.Logf("after end navigation measured %d rows total", len(entry.measured))
}

func TestActivityDetailsCacheInvalidatesSourceAndFontWithoutCrossingSessions(t *testing.T) {
	var cache activityDetailsCache
	key := activityDetailKey{session: "root", block: "reasoning"}
	source := activityDetailSource{kind: "reasoning", text: "**Wide WWW** 中文 e\u0301 `code`\n\nsecond", live: true}
	font := activityDetailFont{body: "Geist", code: "Geist Mono", size: 13}
	first := cache.get(key, source, font)
	first.measured[0] = 100
	if same := cache.get(key, source, font); same != first {
		t.Fatal("unchanged content did not reuse parsed lines")
	}
	font.body = "Arial"
	changedFont := cache.get(key, source, font)
	if changedFont == first || &changedFont.lines[0] != &first.lines[0] || len(changedFont.measured) != 0 {
		t.Fatal("font change did not retain parsing and invalidate geometry")
	}
	font.code, font.size = "Consolas", 20
	changedCode := cache.get(key, source, font)
	if changedCode == changedFont || changedCode.width <= changedFont.width || len(changedCode.measured) != 0 {
		t.Fatal("code font or size change reused obsolete geometry")
	}
	child := cache.get(activityDetailKey{session: "child", block: key.block}, source, font)
	if child == changedCode {
		t.Fatal("child session shared detail cache identity")
	}
	source.text += "\n\nstreamed 中文 **tail**"
	streamed := cache.get(key, source, font)
	if streamed == changedCode || !strings.Contains(thoughtPlain(streamed.lines[len(streamed.lines)-1]), "tail") {
		t.Fatal("streaming update kept obsolete parsed lines")
	}
	source.live = false
	if settled := cache.get(key, source, font); settled == streamed {
		t.Fatal("stream completion did not invalidate live Markdown parsing")
	}
}

func TestActivityDetailWidthsPreserveUnicodeMixedStylesAndHorizontalTail(t *testing.T) {
	a := testApp(t, "default")
	a.palette = design.Palette("rcode", false)
	source := "**WWW mixed bold** 普通 中文 e\u0301 👩‍💻\n\n" + strings.Repeat("宽", 96)
	block := contracts.Block{ID: "mixed-width", Kind: "reasoning", Text: source}
	var exact float32
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(a.palette.UI())
		ui.Column(c).Fill().Children(func() {
			a.activityTextDetail(c, block, false)
			for _, entry := range a.activityDetails.entries {
				for _, line := range entry.lines {
					width, _ := c.MeasureText(0, detailSpans(line, entry.font.code, entry.font.size, a.palette.Faint)...)
					exact = max(exact, width)
				}
				if entry.width < exact+16 {
					t.Errorf("horizontal width=%v cannot retain full line=%v", entry.width, exact)
				}
			}
		})
	}, 250, 500)
	if !tt.HasText("宽") {
		t.Fatal("Unicode detail disappeared")
	}
	rect, _ := tt.Find("Activity detail")
	if exact <= rect.W {
		t.Fatal("test did not exercise horizontal overflow")
	}
	if len(a.activityDetails.entries) != 1 {
		t.Fatal("detail cache did not keep a stable identity")
	}
}

func TestActivityDetailsCacheEvictsByCountAndMemory(t *testing.T) {
	var cache activityDetailsCache
	font := activityDetailFont{size: 13}
	for i := range activityDetailsMaxEntries + 2 {
		cache.get(activityDetailKey{block: fmt.Sprint(i)}, activityDetailSource{output: "small"}, font)
	}
	if len(cache.entries) != activityDetailsMaxEntries || cache.entries[activityDetailKey{block: "0"}] != nil {
		t.Fatal("cache did not evict oldest entries by count")
	}
	for i := range 16 {
		cache.get(activityDetailKey{block: fmt.Sprintf("large-%d", i)}, activityDetailSource{output: strings.Repeat("record\n", 10000)}, font)
		if cache.bytes > activityDetailsMaxBytes || len(cache.entries) > activityDetailsMaxEntries {
			t.Fatal("cache exceeded its memory bound")
		}
	}
	if cache.entries[activityDetailKey{block: "large-0"}] != nil {
		t.Fatal("large entries did not trigger memory eviction")
	}
}
