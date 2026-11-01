package desktop

import (
	"encoding/csv"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/markdown/parser"
)

const transcriptTableFixture = `| 支柱 | 当前状态 |
| --- | --- |
| 修订感知 | ` + "`internal/storage/store.go`" + ` 已有 **revision + CAS** |
| 事件日志 | [说明][docs] 与 ` + "`Runtime.Events()`" + ` 可读回 |
| 中文与转义 | A\|B、逗号,以及"引号" |
| Supervisor 交接 | 长内容会在本列中自动换行，保持行内代码、链接、强调和完整文本。 |

[docs]: https://example.com/docs
`

func parsedTranscriptTable(t *testing.T, source string) transcriptTableData {
	t.Helper()
	document := parser.New(parser.Default | parser.ExtDollarMath)
	document.Set(source)
	for index := range document.Children(0) {
		if document.Block(index).Kind == parser.Table {
			return readTranscriptTable(document, index)
		}
	}
	t.Fatal("fixture has no parsed Markdown table")
	return transcriptTableData{}
}

func TestTranscriptTableCopyAndCSVKeepCellContents(t *testing.T) {
	table := parsedTranscriptTable(t, transcriptTableFixture)
	if len(table.rows) != 5 || len(table.align) != 2 || table.rows[3][1].text != `A|B、逗号,以及"引号"` {
		t.Fatalf("parsed cells changed: %+v", table.rows)
	}
	if !strings.Contains(table.rows[2][1].markdown, "https://example.com/docs") ||
		!strings.Contains(table.rows[1][1].markdown, "**revision") {
		t.Fatal("cell normalization lost resolved links or inline styling")
	}
	copied := table.markdownText()
	if !strings.Contains(copied, `A\|B`) || strings.Contains(copied, "**") || strings.Contains(copied, "https://") {
		t.Fatalf("copy must use displayed text and escape table separators: %q", copied)
	}
	roundtrip := parsedTranscriptTable(t, copied)
	for i := range table.rows {
		for j := range table.rows[i] {
			if roundtrip.rows[i][j].text != table.rows[i][j].text {
				t.Fatalf("Markdown copy changed cell %d/%d", i, j)
			}
		}
	}
	csvText := table.csvText()
	if !strings.HasPrefix(csvText, "\ufeff") || !strings.Contains(csvText, "\r\n") {
		t.Fatal("CSV must use a UTF-8 BOM and CRLF rows")
	}
	records, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(csvText, "\ufeff"))).ReadAll()
	if err != nil || records[3][1] != table.rows[3][1].text {
		t.Fatalf("CSV quoting changed cell content: records=%v error=%v", records, err)
	}
	for _, value := range []string{"=HYPERLINK(\"x\")", "+cmd", "-1+1", "@SUM(A1)", "  =1", "\tunsafe"} {
		data := transcriptTableData{rows: [][]transcriptTableCell{{{text: value}}}}
		if !strings.HasPrefix(data.csvText(), "\ufeff'") && !strings.HasPrefix(data.csvText(), "\ufeff\"'") {
			t.Fatalf("CSV retained a formula prefix: %q", data.csvText())
		}
	}
}

func TestTranscriptTableNativeActionsAndPreview(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(map[bool]string{false: "light", true: "dark"}[dark], func(t *testing.T) {
			app := testApp(t, "default")
			app.palette = design.Palette("rcode", dark)
			export := filepath.Join(t.TempDir(), "table.csv")
			app.chooseTableExportPath = func() (string, error) { return export, nil }
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(app.palette.UI())
				c.Root().Background(app.palette.Background).TextColor(app.palette.Text)
				app.prepareTranscriptMarkdown(c)
				ui.Column(c).Fill().Padding(24).Children(func() { markdown.View(c, transcriptTableFixture, &app.md) })
			}, 900, 700)
			tt.Advance(time.Second)
			for _, text := range []string{"修订感知", "当前状态", "internal/storage/store.go", "说明"} {
				if !tt.HasText(text) {
					t.Fatalf("native table lost %q: %v", text, tt.Texts())
				}
			}
			frame, ok := tt.Find("Markdown table")
			if !ok || frame.X < 0 || frame.X+frame.W > 901 {
				t.Fatalf("table escaped viewport: %+v", frame)
			}
			if err := tt.Click("Copy table as Markdown"); err != nil {
				t.Fatal(err)
			}
			if tt.Clipboard() != parsedTranscriptTable(t, transcriptTableFixture).markdownText() {
				t.Fatal("copy action did not publish displayed table Markdown")
			}
			if err := tt.Click("Download table as CSV"); err != nil {
				t.Fatal(err)
			}
			payload, err := os.ReadFile(export)
			if err != nil || string(payload) != parsedTranscriptTable(t, transcriptTableFixture).csvText() {
				t.Fatalf("CSV action did not write its selected path: %v", err)
			}
			if err := tt.Click("Expand table"); err != nil {
				t.Fatal(err)
			}
			if _, ok := tt.Find("Close table preview"); !ok {
				t.Fatal("preview did not open")
			}
			if err := tt.Click("Close table preview"); err != nil {
				t.Fatal(err)
			}
			if _, ok := tt.Find("Close table preview"); ok {
				t.Fatal("preview did not close")
			}
			writeTranscriptTableEvidence(t, tt, "table-"+map[bool]string{false: "light", true: "dark"}[dark])
		})
	}
}

func TestTranscriptWideTableScrollAndPreviewHeader(t *testing.T) {
	app := testApp(t, "default")
	app.palette = design.Palette("rcode-light", false)
	source := "| Column 1 | Column 2 | Column 3 | Column 4 | Column 5 |\n|---|---|---|---|---|\n"
	for i := 0; i < 24; i++ {
		source += "| long-cell-abcdefghijklmnopqrstuvwxyz | first | second | third | long-cell-abcdefghijklmnopqrstuvwxyz |\n"
	}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		c.Root().Background(app.palette.Background).TextColor(app.palette.Text)
		app.prepareTranscriptMarkdown(c)
		ui.Column(c).Fill().Padding(20).Children(func() {
			ui.Column(c).FillWidth().Height(400).Bind(&app.conversationViewport).Children(func() {
				ui.Scroll(c).Fill().Children(func() {
					ui.Column(c).Width(320).Children(func() { markdown.View(c, source, &app.md) })
				})
			})
		})
	}, 900, 600)
	tt.Advance(time.Second)
	bar, ok := tt.Find("Scroll table horizontally")
	if !ok || bar.W <= 0 {
		writeTranscriptTableEvidence(t, tt, "wide-missing-scrollbar")
		for _, label := range []string{"Markdown table", "Table contents", "Column 1", "Use conversation width"} {
			r, yes := tt.Find(label)
			t.Logf("%s=%+v found=%t", label, r, yes)
		}
		t.Fatalf("wide table has no operable horizontal scrollbar; texts=%v", tt.Texts())
	}
	if err := tt.Click("Scroll table horizontally"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyEnd)
	tt.Frame()
	if err := tt.Click("Use conversation width"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("Restore table width"); !ok {
		t.Fatal("wide table did not expand into the conversation")
	}
	app.nestedMarkdownTable = true
	tt.Frame()
	if _, ok := tt.Find("Restore table width"); ok {
		t.Fatal("nested table borrowed the main conversation's width")
	}
	app.nestedMarkdownTable = false
	tt.Frame()
	if err := tt.Click("Expand table"); err != nil {
		t.Fatal(err)
	}
	before, ok := tt.Find("Table preview header")
	if !ok {
		t.Fatal("preview header missing")
	}
	content, ok := tt.Find("Table preview contents")
	if !ok {
		t.Fatal("preview scroll container missing")
	}
	tt.Scroll(content.X+content.W/2, content.Y+content.H/2, 0, 200)
	tt.Advance(time.Second)
	after, ok := tt.Find("Table preview header")
	if !ok || math.Abs(float64(after.Y-before.Y)) > 1 {
		t.Fatalf("preview header scrolled away: before=%+v after=%+v", before, after)
	}
	writeTranscriptTableEvidence(t, tt, "wide-table-preview")
}

func TestTranscriptTableStreamingAndWidthAllocation(t *testing.T) {
	app := testApp(t, "default")
	app.palette = design.Palette("rcode-light", false)
	source := "| A | B |\n|---|---|\n| value | data |"
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.prepareTranscriptMarkdown(c)
		app.md.Reveal = &markdown.Reveal{Unit: markdown.AtOnce, Fade: -1}
		markdown.Stream(c, source, &app.md)
	}, 500, 300)
	tt.Advance(2 * time.Second)
	if _, ok := tt.Find("Copy table as Markdown"); !ok {
		t.Fatal("streamed table did not gain native actions")
	}
	source += "\n| longer value | more |"
	tt.Frame()
	tt.Advance(2 * time.Second)
	if !tt.HasText("longer value") {
		t.Fatalf("streamed row not rendered: %v", tt.Texts())
	}
	table := parsedTranscriptTable(t, source)
	fitted := fitTranscriptTableWidths([]float32{64, 128}, 500)
	if sumTableWidths(fitted) < 497 || !slices.Equal(table.align, []parser.Align{parser.AlignNone, parser.AlignNone}) {
		t.Fatal("table widths or alignment changed")
	}
}

func writeTranscriptTableEvidence(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	dir := os.Getenv("RCODE_TABLE_EVIDENCE")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, tt.Image()); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
