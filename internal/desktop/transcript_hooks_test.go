package desktop

import (
	"fmt"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/design"
	localize "gcode/internal/i18n/view"
	"github.com/egoist/mygo/ui"
)

func TestTranscriptHookDisclosureAcrossLayouts(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, compact := range []bool{false, true} {
			for _, width := range []int{360, 900} {
				t.Run(fmt.Sprintf("dark=%v/compact=%v/width=%d", dark, compact, width), func(t *testing.T) {
					a := testApp(t, "default")
					a.state.Settings.General.CompactMode = compact
					details := "HOOK_DETAIL_SENTINEL\n" + strings.Repeat("raw hook output\n", 100) + "HOOK_DETAIL_TAIL"
					m := contracts.Message{ID: "reply", SessionID: "chat", Role: "assistant", Blocks: []contracts.Block{
						{ID: "start", Kind: "hook", Hook: &contracts.HookRecord{
							Name: "superpowers@superpowers-marketplace", Event: contracts.HookSessionStart,
							Status: "completed", Details: details,
						}},
						{ID: "text", Kind: "text", Text: "Reply only."},
					}}
					tt := transcriptTester(t, a, &m, width, 900, dark)
					group := "Activity: Ran 1 hook"
					if _, ok := tt.Find(group); !ok {
						t.Fatal("hook did not create an activity group")
					}
					if tt.HasText("superpowers@superpowers-marketplace") {
						t.Fatal("hook row escaped its collapsed activity group")
					}
					if err := tt.Click(group); err != nil {
						t.Fatal(err)
					}
					label := "Hook: superpowers@superpowers-marketplace, SessionStart, Completed"
					header, ok := tt.Find(label)
					if !ok || header.W > float32(width) || header.H != activityRowHeight {
						t.Fatalf("missing or overflowing hook summary: %+v", header)
					}
					status, ok := tt.Find("Completed")
					if !ok || status.X+status.W > float32(width) {
						t.Fatalf("hook status hidden in narrow layout: %+v", status)
					}
					if tt.HasText("HOOK_DETAIL_SENTINEL") || tt.HasText("raw hook output") {
						t.Fatal("hook output expanded by default")
					}
					if err := tt.Click("Copy message"); err != nil || tt.Clipboard() != "Reply only." {
						t.Fatalf("hook contaminated reply copy: %q, %v", tt.Clipboard(), err)
					}
					if err := tt.Click(label); err != nil {
						t.Fatal(err)
					}
					if !tt.HasText("HOOK_DETAIL_SENTINEL") {
						t.Fatal("hook details did not expand")
					}
					viewport, ok := tt.Find("Activity detail")
					if !ok || viewport.H > activityDetailLines*14*1.5+1 {
						t.Fatalf("unbounded hook details: %+v", viewport)
					}
					tt.Scroll(viewport.X+20, viewport.Y+20, 0, 1e6)
					if !tt.HasText("HOOK_DETAIL_TAIL") {
						t.Fatal("hook output tail lost")
					}
					if err := tt.Click(label); err != nil {
						t.Fatal(err)
					}
					tt.Key(0, ui.KeyEnter)
					if _, ok := tt.Find("Activity detail"); !ok {
						t.Fatal("keyboard did not reopen hook details")
					}
					tt.Key(0, ui.KeySpace)
					if _, ok := tt.Find("Activity detail"); ok {
						t.Fatal("keyboard did not collapse hook details")
					}
				})
			}
		}
	}
}

// A hook record without a known outcome must not be reported as a success.
func TestTranscriptHookUnknownStatusIsNotCompleted(t *testing.T) {
	for _, status := range []string{"", "mystery"} {
		if label := hookStatusLabel(status); label != "—" {
			t.Fatalf("status %q labelled %q", status, label)
		}
	}
	if label := hookStatusLabel("completed"); label != "Completed" {
		t.Fatalf("success labelled %q", label)
	}
	a := testApp(t, "default")
	a.state.Settings.General.Language = "en"
	m := contracts.Message{ID: "reply", SessionID: "chat", Role: "assistant", Blocks: []contracts.Block{
		{ID: "silent", Kind: "hook", Hook: &contracts.HookRecord{Name: "silent", Event: contracts.HookStop}},
		{ID: "text", Kind: "text", Text: "Reply only."},
	}}
	a.palette = design.Palette("rcode", true)
	tt := ui.NewTester(func(c *ui.Context) {
		c.DisableAnimations()
		c.SetTheme(a.palette.UI())
		localize.Set(c, a.state.Settings.General.Language)
		ui.Column(c).Fill().Children(func() { a.transcriptMessageForTest(c, m) })
	}, 600, 800)
	if err := tt.Click("Activity: Ran 1 hook"); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("Completed") {
		t.Fatal("a hook with no status was reported as completed")
	}
	if !tt.HasText("—") {
		t.Fatalf("neutral placeholder missing: %v", tt.Texts())
	}
}

func TestTranscriptHookStatusAndFoldState(t *testing.T) {
	a := testApp(t, "default")
	a.state.Settings.General.Language = "zh-CN"
	a.state.Settings.General.CompactMode = false
	m := contracts.Message{ID: "reply", SessionID: "chat", Role: "assistant", Streaming: true, Blocks: []contracts.Block{
		{ID: "denied", Kind: "hook", Hook: &contracts.HookRecord{
			Name: "policy", Event: contracts.HookPreToolUse, Status: "blocked", Details: "POLICY_REASON",
		}},
		{ID: "failed", Kind: "hook", Hook: &contracts.HookRecord{
			Name: "check", Event: contracts.HookStop, Status: "failed", Details: "FAILURE_REASON",
		}},
	}}
	a.palette = design.Palette("rcode", true)
	tt := ui.NewTester(func(c *ui.Context) {
		c.DisableAnimations()
		c.SetTheme(a.palette.UI())
		localize.Set(c, a.state.Settings.General.Language)
		ui.Column(c).Fill().Children(func() { a.transcriptMessageForTest(c, m) })
	}, 600, 800)
	for _, label := range []string{"已阻止", "失败"} {
		if !tt.HasText(label) {
			t.Fatalf("missing localized hook status %q", label)
		}
	}
	denied := "钩子: policy, PreToolUse, 已阻止"
	if err := tt.Click(denied); err != nil {
		t.Fatal(err)
	}
	m.Streaming = false
	tt.Frame()
	if !tt.HasText("POLICY_REASON") || tt.HasText("FAILURE_REASON") {
		t.Fatal("hook folds changed on stream completion or share state")
	}
}
