package desktop

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func TestTranscriptErrorPreservesOriginalProviderText(t *testing.T) {
	body := "Run failed: model request failed (HTTP 503)\n" +
		`{"message":"**upstream_error** <not markup> [request](req-42)","code":50301}` +
		"\n  # provider diagnostics  \n"
	lines := detailLines(activityDetailSource{kind: "error", text: body})
	var plain []string
	for _, line := range lines {
		var text strings.Builder
		for _, run := range line {
			if run.style != (thoughtStyle{}) {
				t.Fatal("provider error interpreted as markup")
			}
			text.WriteString(run.text)
		}
		plain = append(plain, text.String())
	}
	if strings.Join(plain, "\n") != body {
		t.Fatal("provider error text was rewritten")
	}
}

func TestTranscriptReasoningPreviewUpdatesAndManualExpansionPersists(t *testing.T) {
	a := testApp(t, "default")
	m := contracts.Message{ID: "preview", Role: "assistant", Streaming: true, Blocks: []contracts.Block{
		{ID: "thought", Kind: "reasoning", Text: "First line\n\nLatest"},
	}}
	tt := transcriptTester(t, a, &m, 760, 700, false)
	if !tt.HasText("Latest") {
		t.Fatal("latest unterminated line is not previewed")
	}
	m.Blocks[0].Text += " update"
	tt.Frame()
	if !tt.HasText("Latest update") {
		t.Fatal("preview waits for a newline")
	}
	if err := tt.Click("Thought process: thought"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("Activity detail"); !ok || !tt.HasText("First line") {
		t.Fatal("manual expansion did not expose complete text")
	}
	m.Streaming = false
	tt.Frame()
	if _, ok := tt.Find("Activity detail"); !ok {
		t.Fatal("completion discarded manual expansion")
	}
	if err := tt.Click("Thought process: thought"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("First line") || tt.HasText("Latest update") {
		t.Fatal("settled folded preview is not the first source line")
	}
}

func TestTranscriptDetailsScrollCompleteContentWithinTwentyFourLines(t *testing.T) {
	for _, kind := range []string{"reasoning", "text", "invocation", "output"} {
		t.Run(kind, func(t *testing.T) {
			a := testApp(t, "default")
			a.palette = design.Palette("rcode", false)
			var content strings.Builder
			for i := 0; i < 100; i++ {
				fmt.Fprintf(&content, "record %03d  \n", i)
			}
			block := contracts.Block{Kind: kind, Text: content.String()}
			switch kind {
			case "invocation":
				block.Tool = &contracts.ToolCall{Command: content.String()}
			case "output":
				block.Tool = &contracts.ToolCall{Output: content.String()}
			}
			var outer ui.ScrollState
			var size float32
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(a.palette.UI())
				c.DisableAnimations()
				size = design.TextSize(c, design.TextBase)
				if block.Tool != nil {
					size = float32(a.state.Settings.Appearance.CodeFontSize)
				}
				ui.Scroll(c.Key("outer")).TrackScroll(&outer).Fill().Children(func() {
					a.activityDetail(c, block, false)
					ui.Text(c, "After the detail")
					ui.Box(c).Height(1200)
				})
			}, 420, 650)
			viewport, ok := tt.Find("Activity detail")
			if !ok || viewport.H != float32(activityDetailLines)*size*1.5 {
				t.Fatalf("unbounded detail: %+v", viewport)
			}
			below, _ := tt.Find("After the detail")
			if !tt.HasText("record 000") || tt.HasText("record 099") {
				t.Fatal("initial viewport is not virtualized from the start")
			}
			if err := tt.Click("Activity detail"); err != nil {
				t.Fatal(err)
			}
			tt.Key(0, ui.KeyEnd)
			if !tt.HasText("record 099") {
				t.Fatal("focused detail cannot reach its end with the keyboard")
			}
			tt.Key(0, ui.KeyHome)
			tt.Scroll(viewport.X+20, viewport.Y+20, 0, 10000)
			if !tt.HasText("record 099") {
				t.Fatalf("end of detail is inaccessible: %v", tt.Texts())
			}
			after, _ := tt.Find("After the detail")
			if outer.Y != 0 || after.Y != below.Y {
				t.Fatal("scrolling detail moved the surrounding conversation")
			}
			tt.Frame()
			if !tt.HasText("record 099") {
				t.Fatal("detail scroll position was lost on render")
			}
		})
	}
}

func TestTranscriptQuestionAppearsOnlyAsQuestionCard(t *testing.T) {
	for _, compact := range []bool{false, true} {
		for _, streaming := range []bool{false, true} {
			m := contracts.Message{ID: "question", Role: "assistant", Streaming: streaming, Blocks: []contracts.Block{
				{ID: "ask", Kind: "tool", Tool: &contracts.ToolCall{ID: "ask", Name: "AskUserQuestion"}},
				{ID: "input", Kind: "question", Question: &contracts.Question{ID: "input", Pages: []contracts.QuestionPage{{Question: "Choose", Header: "Choose"}}}},
			}}
			rows := transcriptRows(m, compact)
			if len(rows) != 1 || rows[0].Block.Question == nil {
				t.Fatalf("question is duplicated: compact=%t streaming=%t rows=%+v", compact, streaming, rows)
			}
			if summary := contracts.SummarizeActivity(m.Blocks); summary.ToolCalls != 0 {
				t.Fatalf("question counted as a tool: %+v", summary)
			}
		}
	}
}

func TestTranscriptFailedQuestionWithoutCardKeepsItsError(t *testing.T) {
	for _, test := range []struct{ name, args, output string }{
		{"invalid JSON", `{"title":`, "unexpected EOF"},
		{"empty title", `{"title":"","options":[]}`, "invalid question"},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, compact := range []bool{false, true} {
				m := contracts.Message{ID: "failed-question", Role: "assistant", Blocks: []contracts.Block{
					{ID: "failed", Kind: "tool", Tool: &contracts.ToolCall{
						ID: "failed", Name: "AskUserQuestion", Args: test.args, Status: "failed", IsError: true, Output: test.output,
					}},
				}}
				rows := transcriptRows(m, compact)
				if len(rows) != 1 || rows[0].Block.Kind != "error" || rows[0].Block.Text != test.output {
					t.Fatalf("failed question became invisible: %+v", rows)
				}
				if summary := contracts.SummarizeActivity(m.Blocks); summary.ToolCalls != 0 {
					t.Fatalf("question error counted as ordinary work: %+v", summary)
				}
				a := testApp(t, "default")
				a.state.Settings.General.CompactMode = compact
				tt := transcriptTester(t, a, &m, 760, 700, false)
				if !tt.HasText(test.output) {
					t.Fatal("failed question detail did not render")
				}
			}
		})
	}
}

func TestTranscriptDetailViewportAdaptsToThemeWidthAndFont(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, width := range []int{360, 760} {
			for _, size := range []float32{12, 20} {
				t.Run(fmt.Sprintf("dark=%t/width=%d/font=%g", dark, width, size), func(t *testing.T) {
					a := testApp(t, "default")
					a.palette = design.Palette("rcode", dark)
					a.state.Settings.Appearance.CodeFontSize = float64(size)
					for _, count := range []int{1, 25} {
						block := contracts.Block{Tool: &contracts.ToolCall{Output: strings.Repeat("界 long text\n", count)}}
						tt := ui.NewTester(func(c *ui.Context) {
							c.SetTheme(a.palette.UI())
							ui.Column(c).FillWidth().Children(func() { a.activityDetail(c, block, false) })
						}, width, 900)
						rect, ok := tt.Find("Activity detail")
						if !ok || rect.H != float32(min(count, 24))*size*1.5 {
							t.Fatalf("detail height ignores content/font: %+v", rect)
						}
						if rect.X+rect.W > float32(width)+1 {
							t.Fatalf("detail escaped narrow window: %+v", rect)
						}
					}
				})
			}
		}
	}
}

func TestTranscriptDiffUsesOneBoundedScrollingViewport(t *testing.T) {
	a := testApp(t, "default")
	a.palette = design.Palette("rcode", false)
	var old, current strings.Builder
	for i := range 120 {
		fmt.Fprintf(&old, "old %03d\n", i)
		fmt.Fprintf(&current, "new %03d\n", i)
	}
	block := contracts.Block{Tool: &contracts.ToolCall{
		Command: "apply changes", Diff: &contracts.Change{OldContent: old.String(), NewContent: current.String()},
	}}
	var outer ui.ScrollState
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(a.palette.UI())
		ui.Scroll(c).TrackScroll(&outer).Fill().Children(func() {
			a.activityDetail(c, block, false)
			ui.Text(c, "Below diff")
			ui.Box(c).Height(1000)
		})
	}, 420, 650)
	viewport, ok := tt.Find("Activity diff")
	maxHeight := float32(activityDetailLines*a.state.Settings.Appearance.CodeFontSize)*1.5 + 12
	if !ok || viewport.H > maxHeight {
		t.Fatalf("diff exceeds 24 lines including invocation: %+v", viewport)
	}
	before, _ := tt.Find("Below diff")
	pixels := append([]byte(nil), tt.Image().Pix...)
	tt.Scroll(viewport.X+20, viewport.Y+viewport.H/2, 0, 10000)
	after, _ := tt.Find("Below diff")
	if bytes.Equal(pixels, tt.Image().Pix) {
		t.Fatal("diff cannot scroll to hidden content")
	}
	if outer.Y != 0 || after.Y != before.Y {
		t.Fatal("diff scroll moved surrounding conversation")
	}
}

func TestTranscriptDetailPreservesWideOutputWithoutEllipsis(t *testing.T) {
	text := strings.Repeat("界", 9000)
	lines := detailLines(activityDetailSource{output: text})
	var restored strings.Builder
	for _, line := range lines {
		restored.WriteString(thoughtPlain(line))
	}
	if restored.String() != text {
		t.Fatal("long output characters were discarded")
	}
}

func TestTranscriptDetailKeepsAllArgumentsAlongsideReadableCommand(t *testing.T) {
	lines := detailLines(activityDetailSource{
		command: "go test ./...", args: `{"command":"go test ./...","timeout":60}`, output: "finished",
	})
	var text strings.Builder
	for _, line := range lines {
		text.WriteString(thoughtPlain(line))
	}
	for _, want := range []string{`"command": "go test ./..."`, `"timeout": 60`, "finished"} {
		if !strings.Contains(text.String(), want) {
			t.Fatalf("expanded invocation lost %q: %s", want, text.String())
		}
	}
}
