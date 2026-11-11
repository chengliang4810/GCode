package desktop

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestSessionDeleteDialogDimsSessionInfo(t *testing.T) {
	for _, mode := range []string{"light", "dark"} {
		for _, width := range []int{900, 1700} {
			t.Run(fmt.Sprintf("%s-%d", mode, width), func(t *testing.T) {
				a := testApp(t, "default")
				a.selectSession("chat-0")
				a.state.Settings.Appearance.Mode = mode
				a.state.Settings.Appearance.ThemeID = "rcode-" + mode
				a.state.Settings.General.Language = "zh-CN"
				a.right, a.files = false, false
				a.expanded["session-info-user"], a.expanded["session-info"] = true, true
				tt := ui.NewTester(a.View, width, 880)
				tt.Advance(300 * time.Millisecond)
				info, ok := tt.Find(a.t("Branch changes"))
				if !ok {
					t.Fatal("session information card missing")
				}
				x, y := int(info.X+3), int(info.Y+3)
				before := tt.Image().RGBAAt(x, y)
				captureWorkbenchAudit(t, tt, fmt.Sprintf("session-info-%s-%d", mode, width))
				a.sidebarUI.deleteID, a.sidebarUI.deleteOpen = a.sessionID, true
				tt.Frame()
				tt.Advance(300 * time.Millisecond)
				after := tt.Image().RGBAAt(x, y)
				captureWorkbenchAudit(t, tt, fmt.Sprintf("delete-session-%s-%d", mode, width))
				if float32(after.R) > float32(before.R)*.75 || float32(after.G) > float32(before.G)*.75 || float32(after.B) > float32(before.B)*.75 {
					t.Fatalf("session info was not dimmed by modal: before=%v after=%v", before, after)
				}
				if !tt.HasText("删除后，该对话将无法找回。如果只是想从列表中隐藏，可以使用归档操作。") {
					t.Fatal("irreversible deletion and archive guidance missing")
				}
				// Intentionally send a pointer press to the covered
				// background. A named click rejects this target before
				// dispatch, so it would not exercise modal blocking.
				tt.ClickAt(info.X+info.W/2, info.Y+info.H/2)
				if a.right {
					t.Fatal("modal allowed the background session-info action")
				}
				tt.Frame()
				a.sidebarUI.deleteOpen = true
				tt.Frame()
				if err := tt.Click(a.t("Cancel")); err != nil {
					t.Fatal(err)
				}
				if a.sidebarUI.deleteOpen || sidebarSession(t, a, "chat-0").ID != "chat-0" {
					t.Fatal("cancel did not preserve the conversation")
				}
				a.sidebarUI.deleteOpen = true
				tt.Frame()
				tt.Key(0, ui.KeyEscape)
				if a.sidebarUI.deleteOpen {
					t.Fatal("Escape did not dismiss deletion")
				}
				tt.Frame()
				if err := tt.Click(a.t("Branch changes")); err != nil {
					t.Fatal(err)
				}
				if !a.right {
					t.Fatal("session-info action did not recover after dismissal")
				}
			})
		}
	}
}
