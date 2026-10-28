package desktop

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

func activityFixture() contracts.Message {
	bad, good := 1, 0
	return contracts.Message{ID: "audit-turn", SessionID: "chat-0", TurnID: "audit-run", Role: "assistant", DurationMS: 147000, CreatedAt: time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC), Blocks: []contracts.Block{
		{ID: "thought-1", Kind: "reasoning", Text: "I’ll read the layout contract, then run the regression check.\n\n**Keep the existing geometry.** The sidebar width and editor focus should survive the change."},
		{ID: "read", Kind: "tool", Tool: &contracts.ToolCall{ID: "read", Kind: contracts.ToolReadFile, Path: "README.md", Status: "completed", Output: "# Fieldnotes\nA native desktop workspace."}},
		{ID: "run-failed", Kind: "tool", Tool: &contracts.ToolCall{ID: "run-failed", Kind: contracts.ToolExec, Command: "go test ./internal/layout -run TestPanelGeometry -count=1", Status: "failed", ExitCode: &bad, IsError: true, Output: "--- FAIL: TestPanelGeometry\n    layout_test.go:42: sidebar width changed\nFAIL"}},
		{ID: "thought-2", Kind: "reasoning", Text: "The saved sidebar width is being replaced during resize. I’ll preserve the preferred width and clamp only the visible allocation."},
		{ID: "edit", Kind: "tool", Tool: &contracts.ToolCall{ID: "edit", Kind: contracts.ToolEditFile, Path: "src/layout.go", Status: "completed", Output: "Updated src/layout.go"}},
		{ID: "write", Kind: "tool", Tool: &contracts.ToolCall{ID: "write", Kind: contracts.ToolWriteFile, Path: "src/layout_test.go", Status: "completed", Output: "Created src/layout_test.go"}},
		{ID: "run-success", Kind: "tool", Tool: &contracts.ToolCall{ID: "run-success", Kind: contracts.ToolExec, Command: "go test ./internal/layout -count=1", Status: "completed", ExitCode: &good, Output: "ok  fieldnotes/internal/layout  0.012s"}},
		{ID: "reply", Kind: "text", Text: "The sidebar keeps its preferred width after a resize. The panel regression check now passes."},
	}}
}

func transcriptTester(t *testing.T, a *App, message *contracts.Message, width, height int, dark bool) *ui.Tester {
	t.Helper()
	a.palette = design.Palette("zeron", dark)
	return ui.NewTester(func(c *ui.Context) {
		c.DisableAnimations()
		c.SetTheme(a.palette.UI())
		c.Root().Background(a.palette.Background).Font("Geist").FontSize(13).TextColor(a.palette.Text)
		a.md.Theme = &markdown.Theme{Font: "Geist", FontSize: 14, FixedLineHeight: 22}
		ui.Scroll(c).Fill().Padding(24).Children(func() { a.assistantTranscript(c, *message) })
	}, width, height)
}

func TestTranscriptNativeDisclosureAndFixedTreeGeometry(t *testing.T) {
	a := testApp(t, "default")
	m := activityFixture()
	tt := transcriptTester(t, a, &m, 760, 900, false)
	summary := contracts.SummarizeActivity(m.Blocks[:len(m.Blocks)-1]).Label()
	if tt.HasText("Run") || tt.HasText("Thought process") {
		t.Fatal("settled work starts expanded")
	}
	header, ok := tt.Find("Activity: " + summary)
	if !ok || header.H != 26 {
		t.Fatalf("header geometry %+v", header)
	}
	if err := tt.Click("Activity: " + summary); err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"Thought process", "Read", "Run", "Edit", "Write", "README.md", "layout.go", "layout_test.go"} {
		if !tt.HasText(label) {
			t.Fatalf("missing activity %q: %v", label, tt.Texts())
		}
	}
	read, _ := tt.Find("Read: README.md")
	edit, _ := tt.Find("Edit: src/layout.go")
	write, _ := tt.Find("Write: src/layout_test.go")
	if read.H != 32 || edit.H != 32 || write.H != 32 || write.Y-edit.Y != 32 {
		t.Fatalf("tree row drift: read=%+v edit=%+v write=%+v", read, edit, write)
	}
	label, _ := tt.Find("Read")
	if label.X-header.X != 56 {
		t.Fatalf("activity text starts at %v, want source 48+8 gutter", label.X-header.X)
	}
	if err := tt.Click("Thought process: thought-1"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("I’ll read the layout contract") || !tt.HasText("Keep the existing geometry.") {
		t.Fatal("full reasoning did not expand")
	}
	if err := tt.Click("Run: go test ./internal/layout -run TestPanelGeometry -count=1"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("sidebar width changed") || !tt.HasText("--- FAIL: TestPanelGeometry") {
		t.Fatal("failed command output missing")
	}
	// The focused disclosure uses the official keyboard interaction.
	tt.Key(0, ui.KeySpace)
	if tt.HasText("sidebar width changed") {
		t.Fatal("Space did not fold focused native disclosure")
	}
	tt.Key(0, ui.KeyEnter)
	if !tt.HasText("sidebar width changed") {
		t.Fatal("Enter did not reopen focused disclosure")
	}
}

func TestTranscriptExplicitFoldSurvivesStreamingAndVirtualRecreation(t *testing.T) {
	a := testApp(t, "default")
	m := activityFixture()
	m.Blocks = m.Blocks[:1]
	m.Streaming = true
	tt := transcriptTester(t, a, &m, 760, 700, true)
	if !tt.HasText("I’ll read the layout contract") {
		t.Fatal("live tail thought did not automatically expand")
	}
	groupID := m.ID + ":activity:" + m.Blocks[0].ID
	summary := contracts.SummarizeActivity(m.Blocks).Label()
	if err := tt.Click("Activity: " + summary); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("I’ll read the layout contract") {
		t.Fatal("explicit close ignored")
	}
	m.Blocks[0].Text += " The next delta must not reopen a user-closed group."
	tt.Frame()
	if tt.HasText("next delta") {
		t.Fatal("stream delta overrode explicit fold")
	}
	if !a.expanded["activity-explicit:"+groupID] {
		t.Fatal("fold identity was not retained")
	}
	// A fresh virtualized row's local widget state must restore that same choice.
	tt = transcriptTester(t, a, &m, 760, 700, true)
	if tt.HasText("next delta") {
		t.Fatal("virtual row recreation lost explicit collapse")
	}
	if err := tt.Click("Activity: " + summary); err != nil {
		t.Fatal(err)
	}
	m.Streaming = false
	m.Blocks = append(m.Blocks, contracts.Block{ID: "reply", Kind: "text", Text: "The check is complete."})
	tt.Frame()
	if !tt.HasText("The check is complete.") || !tt.HasText("Thought process") {
		t.Fatal("completion hid reply or discarded explicit open group")
	}
}

func TestTranscriptCompactNarrationAndWorkedDuration(t *testing.T) {
	a := testApp(t, "default")
	a.state.Settings.General.CompactMode = true
	m := activityFixture()
	m.Blocks = append(m.Blocks[:2], append([]contracts.Block{{ID: "note", Kind: "text", Text: "I found the layout contract."}}, m.Blocks[2:]...)...)
	tt := transcriptTester(t, a, &m, 760, 900, false)
	if !tt.HasText("Worked for 2m 27s") || tt.HasText("I found the layout contract.") || !tt.HasText("The sidebar keeps") {
		t.Fatal("compact final reply boundary or duration mismatch")
	}
	summary := contracts.SummarizeActivity(m.Blocks[:len(m.Blocks)-1]).Label()
	if err := tt.Click("Activity: " + summary); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Wrote") || !tt.HasText("I found the layout contract.") {
		t.Fatal("compact narration is not a Wrote activity")
	}
	m.Streaming = true
	tt.Frame()
	if tt.HasText("Worked for 2m 27s") {
		t.Fatal("streaming turn retained settled duration")
	}
}

func TestTranscriptCompactLiveNeverAutoOpensThought(t *testing.T) {
	a := testApp(t, "default")
	a.state.Settings.General.CompactMode = true
	m := activityFixture()
	m.Blocks = m.Blocks[:1]
	m.Streaming = true
	tt := transcriptTester(t, a, &m, 760, 700, false)
	summary := contracts.SummarizeActivity(m.Blocks).Label()
	if _, exists := tt.Find("Thought process: thought-1"); exists {
		t.Fatal("compact group automatically opened")
	}
	if err := tt.Click("Activity: " + summary); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Thought process") || tt.HasText("I’ll read the layout contract") {
		t.Fatal("compact live thought automatically opened its detail")
	}
	if err := tt.Click("Thought process: thought-1"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("I’ll read the layout contract") {
		t.Fatal("manual compact thought expansion failed")
	}
}

func TestTranscriptFileBadgeOpensNativeEditorWithoutTogglingRow(t *testing.T) {
	a := testApp(t, "default")
	m := activityFixture()
	groupID := m.ID + ":activity:" + m.Blocks[0].ID
	a.rememberActivityFold(groupID, true)
	tt := transcriptTester(t, a, &m, 760, 900, false)
	if err := tt.Click("Open file: README.md"); err != nil {
		t.Fatal(err)
	}
	if a.workspace.ActiveKind() != "file" || a.workspace.TabCount() != 1 {
		t.Fatal("file badge did not open native editor")
	}
	if tt.HasText("# Fieldnotes") {
		t.Fatal("file badge click also toggled its containing tool disclosure")
	}
}

func TestTranscriptCopyContainsReplyOnlyAndNoDefaultRetry(t *testing.T) {
	a := testApp(t, "default")
	m := activityFixture()
	tt := transcriptTester(t, a, &m, 760, 900, false)
	if tt.HasText("Retry response") {
		t.Fatal("completed source footer has no retry action")
	}
	if err := tt.Click("Copy message"); err != nil {
		t.Fatal(err)
	}
	if tt.Clipboard() != m.Blocks[len(m.Blocks)-1].Text {
		t.Fatalf("copy included work record: %q", tt.Clipboard())
	}
}

func TestTranscriptSubagentOpensSideTabAndPreservesCentralDraft(t *testing.T) {
	a := testApp(t, "transcript")
	a.selectSession("chat-0")
	a.draft = "Keep this central draft"
	a.state.Messages["chat-0"] = []contracts.Message{{ID: "parent", SessionID: "chat-0", Role: "assistant", Blocks: []contracts.Block{
		{ID: "child", Kind: "subagent", Subagent: &contracts.Subagent{ID: "child", SessionID: "showcase-agent", Name: "Schema reviewer", Task: "Check the schema fields", Status: "completed"}},
		{ID: "final", Kind: "text", Text: "The independent review is complete."},
	}}}
	a.workspace.Update(a.state)
	tt := ui.NewTester(a.View, 1320, 880)
	if err := tt.Click("Open agent: Schema reviewer"); err != nil {
		t.Fatal(err)
	}
	if a.sessionID != "chat-0" || a.draft != "Keep this central draft" || a.workspace.ActiveKind() != "session" {
		t.Fatal("agent link replaced the central conversation or draft")
	}
	if !tt.HasText("The fixture uses schemaVersion 1") {
		t.Fatal("side tab did not render the actual child transcript")
	}
}

func TestTranscriptSideFoldDoesNotChangeCentralFold(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	m := activityFixture()
	a.state.Messages["chat-0"] = []contracts.Message{m}
	a.workspace.Update(a.state)
	mainID := m.ID + ":activity:" + m.Blocks[0].ID
	sideID := "side:chat-0:" + mainID
	a.rememberActivityFold(mainID, true)
	a.workspace.OpenSession("chat-0")
	tt := ui.NewTester(a.View, 1600, 1000)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	count := func() int {
		n := 0
		for _, text := range tt.Texts() {
			if text == "Read" {
				n++
			}
		}
		return n
	}
	if count() != 1 {
		t.Fatalf("side inherited main fold: %d Read rows", count())
	}
	a.rememberActivityFold(sideID, true)
	tt.Frame()
	if count() != 2 {
		t.Fatalf("side did not use independent fold: %d Read rows", count())
	}
	a.rememberActivityFold(mainID, false)
	tt.Frame()
	if count() != 1 {
		t.Fatalf("main close also closed side: %d Read rows", count())
	}
}

func TestTranscriptAllThemesKeepGeometryAndFailureTint(t *testing.T) {
	for _, variant := range design.Themes() {
		t.Run(variant.ID, func(t *testing.T) {
			a := testApp(t, "default")
			m := activityFixture()
			a.palette = design.Palette(variant.ID, variant.Appearance == "dark")
			a.rememberActivityFold(m.ID+":activity:"+m.Blocks[0].ID, true)
			tt := transcriptTester(t, a, &m, 760, 900, variant.Appearance == "dark")
			a.palette = design.Palette(variant.ID, variant.Appearance == "dark")
			tt.Frame()
			row, ok := tt.Find("Run: go test ./internal/layout -run TestPanelGeometry -count=1")
			if !ok || row.H != 32 || row.W < 600 {
				t.Fatalf("row bounds %+v", row)
			}
			if strings.Contains(strings.Join(tt.Texts(), "\n"), "Thinking") {
				t.Fatal("old generic thought card remains")
			}
			image := tt.Image()
			colored := 0
			for y := int(row.Y); y < int(row.Y+row.H); y++ {
				for x := int(row.X + 56); x < int(row.X+row.W); x++ {
					pixel := image.RGBAAt(x, y)
					if absByte(pixel.R, a.palette.Danger.R) < 6 && absByte(pixel.G, a.palette.Danger.G) < 6 && absByte(pixel.B, a.palette.Danger.B) < 6 {
						colored++
					}
				}
			}
			if colored < 8 {
				t.Fatalf("failed row did not paint its theme danger color; %d pixels", colored)
			}
		})
	}
}

func absByte(a, b byte) int { return max(int(a)-int(b), int(b)-int(a)) }

func TestTranscriptTreeRailHasNoDoubleAlphaSeams(t *testing.T) {
	a := testApp(t, "default")
	m := activityFixture()
	a.rememberActivityFold(m.ID+":activity:"+m.Blocks[0].ID, true)
	tt := transcriptTester(t, a, &m, 760, 900, false)
	image := tt.Image()
	// Header bottom, top-pad seam, first branch, and next-row seam have the
	// same 12% ink contribution. Overlapping round stroke caps darken seams.
	for _, y := range []int{49, 50, 51, 52, 53, 58, 61, 62, 63, 64, 65, 68, 80, 84, 85} {
		pixel := image.RGBAAt(36, y)
		if pixel.R != 224 || pixel.G != 224 || pixel.B != 224 {
			t.Fatalf("rail alpha seam at y=%d: %+v", y, pixel)
		}
	}
}

func TestTranscriptOutputSelectionSpansNativeLines(t *testing.T) {
	a := testApp(t, "default")
	m := activityFixture()
	groupID := m.ID + ":activity:" + m.Blocks[0].ID
	a.rememberActivityFold(groupID, true)
	a.rememberActivityFold(groupID+":run-failed", true)
	tt := transcriptTester(t, a, &m, 760, 900, false)
	first, ok := tt.Find("--- FAIL: TestPanelGeometry")
	if !ok {
		t.Fatal("output not rendered")
	}
	last, ok := tt.Find("FAIL")
	if !ok {
		t.Fatal("last output not rendered")
	}
	tt.Press(first.X, first.Y+first.H/2)
	tt.Move(last.X+40, last.Y+last.H/2)
	tt.Release(last.X+40, last.Y+last.H/2)
	tt.Key(ui.Ctrl, ui.KeyC)
	if !strings.Contains(tt.Clipboard(), "--- FAIL: TestPanelGeometry") || !strings.Contains(tt.Clipboard(), "sidebar width changed") || !strings.Contains(tt.Clipboard(), "FAIL") {
		t.Fatalf("multi-line output selection copied %q", tt.Clipboard())
	}
}

func TestTranscriptLogicalGeometryAtDesktopScales(t *testing.T) {
	for _, scale := range []float32{1, 1.25, 1.5, 2} {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			a := testApp(t, "default")
			m := activityFixture()
			a.rememberActivityFold(m.ID+":activity:"+m.Blocks[0].ID, true)
			tt := transcriptTester(t, a, &m, 600, 800, false)
			tt.SetScale(scale)
			row, ok := tt.Find("Read: README.md")
			if !ok || row.H != 32 {
				t.Fatalf("scaled row %+v", row)
			}
			label, _ := tt.Find("Read")
			if label.X-row.X != 56 {
				t.Fatalf("scaled native gutter: %v", label.X-row.X)
			}
			if err := tt.Click("Read: README.md"); err != nil {
				t.Fatal(err)
			}
			if !tt.HasText("A native desktop workspace.") {
				t.Fatal("scaled detail did not open")
			}
		})
	}
}

func TestTranscriptOfficialFoldHonorsReducedMotionAndParks(t *testing.T) {
	a := testApp(t, "default")
	a.palette = design.Palette("zeron", false)
	m := activityFixture()
	builds, paints := 0, 0
	tt := ui.NewTester(func(c *ui.Context) {
		builds++
		a.configureMotion(c)
		c.SetTheme(a.palette.UI())
		c.Root().Background(a.palette.Background).Draw(func(_ *ui.Painter, _ ui.Rect) { paints++ })
		a.prepareTranscriptMarkdown(c)
		ui.Scroll(c).Fill().Padding(24).Children(func() { a.assistantTranscript(c, m) })
	}, 760, 900)
	summary := contracts.SummarizeActivity(m.Blocks[:len(m.Blocks)-1]).Label()
	if err := tt.Click("Activity: " + summary); err != nil {
		t.Fatal(err)
	}
	tt.Move(745, 880)
	tt.Advance(100 * time.Millisecond)
	mid, _ := tt.Find(m.Blocks[len(m.Blocks)-1].Text)
	tt.Advance(150 * time.Millisecond)
	end, _ := tt.Find(m.Blocks[len(m.Blocks)-1].Text)
	if end.Y <= mid.Y {
		t.Fatal("native fold did not animate to its final height")
	}
	tt.Advance(2 * time.Second)
	beforeBuild, beforePaint := builds, paints
	tt.Advance(3 * time.Second)
	if builds != beforeBuild || paints != beforePaint || tt.FramePending() {
		t.Fatal("settled transcript kept scheduling native frames")
	}
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	if err := tt.Click("Activity: " + summary); err != nil {
		t.Fatal(err)
	}
	closed, _ := tt.Find(m.Blocks[len(m.Blocks)-1].Text)
	if closed.Y >= end.Y {
		t.Fatal("reduced motion did not collapse immediately")
	}
	tt.Move(745, 880)
	tt.Advance(time.Second)
	beforeBuild, beforePaint = builds, paints
	tt.Advance(3 * time.Second)
	if builds != beforeBuild || paints != beforePaint {
		t.Fatal("reduced-motion transcript did not park")
	}
}

// Opt-in implementation screenshots. These are evidence of this renderer only;
// they never become reference goldens or imply native GPUI pixel parity.
func TestTranscriptRenderEvidence(t *testing.T) {
	directory := os.Getenv("ZERON_TRANSCRIPT_EVIDENCE")
	if directory == "" {
		t.Skip("set ZERON_TRANSCRIPT_EVIDENCE for native-widget implementation PNGs")
	}
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	for _, dark := range []bool{false, true} {
		for _, state := range []string{"collapsed", "expanded", "details", "compact", "live"} {
			a := testApp(t, "default")
			m := activityFixture()
			groupID := m.ID + ":activity:" + m.Blocks[0].ID
			if state == "expanded" || state == "details" {
				a.rememberActivityFold(groupID, true)
			}
			if state == "details" {
				a.rememberActivityFold(groupID+":thought-1", true)
				a.rememberActivityFold(groupID+":run-failed", true)
			}
			if state == "compact" {
				a.state.Settings.General.CompactMode = true
			}
			if state == "live" {
				m.Streaming = true
				m.Blocks = m.Blocks[:1]
			}
			tt := transcriptTester(t, a, &m, 760, 900, dark)
			path := filepath.Join(directory, fmt.Sprintf("activity-%s-%s.png", state, map[bool]string{false: "light", true: "dark"}[dark]))
			f, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(f, tt.Image()); err != nil {
				f.Close()
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			if state == "expanded" || state == "collapsed" {
				a.selectSession("chat-0")
				a.state.Messages["chat-0"] = []contracts.Message{
					{ID: "audit-user", SessionID: "chat-0", Role: "user", Blocks: []contracts.Block{{ID: "audit-prompt", Kind: "text", Text: "Keep the sidebar width after resizing and verify the panel layout."}}}, m,
				}
				a.state.Settings.Appearance.Mode = map[bool]string{false: "light", true: "dark"}[dark]
				a.state.Settings.Appearance.ThemeID = ""
				whole := ui.NewTester(a.View, 1320, 880)
				path = filepath.Join(directory, fmt.Sprintf("desktop-%s-%s.png", state, map[bool]string{false: "light", true: "dark"}[dark]))
				f, err = os.Create(path)
				if err != nil {
					t.Fatal(err)
				}
				if err = png.Encode(f, whole.Image()); err != nil {
					f.Close()
					t.Fatal(err)
				}
				if err = f.Close(); err != nil {
					t.Fatal(err)
				}
			}
		}
		a, _, child := sideViewTester(t)
		a.state.Settings.Appearance.Mode = map[bool]string{false: "light", true: "dark"}[dark]
		a.state.Settings.Appearance.ThemeID = ""
		a.sideComposer(child.ID).Draft = "Check the schema fields again"
		whole := ui.NewTester(a.View, 1320, 880)
		path := filepath.Join(directory, fmt.Sprintf("desktop-side-input-%s.png", map[bool]string{false: "light", true: "dark"}[dark]))
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, whole.Image()); err != nil {
			f.Close()
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
