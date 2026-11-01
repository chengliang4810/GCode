package desktop

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func sidebarSession(t *testing.T, a *App, id string) contracts.Session {
	t.Helper()
	snapshot, err := a.service.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, session := range snapshot.Sessions {
		if session.ID == id {
			return session
		}
	}
	t.Fatalf("session %s missing", id)
	return contracts.Session{}
}

func TestSidebarRowHeightStaysCompact(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 1320, 880)
	for _, session := range a.visibleSessions() {
		row, ok := tt.Find(session.Title)
		if !ok || row.H != design.Sidebar().RowHeight {
			t.Fatalf("sidebar row left the compact form: %+v want %g", row, design.Sidebar().RowHeight)
		}
	}
}

func TestSidebarSelectionUsesSourceWashWithoutSolidAccent(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	tt := ui.NewTester(a.View, 1320, 880)
	row, ok := tt.Find(a.currentSession().Title)
	if !ok {
		t.Fatal("selected sidebar row missing")
	}
	pixel := tt.Image().RGBAAt(int(row.X+row.W-4), int(row.Y+row.H/2))
	if pixel.R == a.palette.Accent.R && pixel.G == a.palette.Accent.G && pixel.B == a.palette.Accent.B {
		t.Fatal("native list solid accent obscured row wash")
	}
	if pixel.R < 100 && pixel.B > 150 {
		t.Fatalf("solid purple selection remained: %+v", pixel)
	}
}

func TestSidebarNativeViewMenuMutatesBackend(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 1320, 880)
	for _, path := range [][]string{{"By group"}, {"By project"}} {
		if err := tt.Click("Sidebar view options"); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem(path...); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := a.service.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	s := snapshot.Settings
	if s.SidebarGrouping != "projects" {
		t.Fatalf("view options did not persist: %+v", s)
	}
	if _, ok := tt.Find("All projects"); ok {
		t.Fatal("hidden project filter still mounted")
	}
}

func TestSidebarRecencyOrderingAndLiteralSearch(t *testing.T) {
	a := testApp(t, "default")
	base := time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC)
	a.state.Sessions = []contracts.Session{{ID: "a", Title: "Pinned workshop", ProjectID: "rcode", CreatedAt: base, UpdatedAt: base.Add(time.Hour)}, {ID: "b", Title: "Second", ProjectID: "rcode", CreatedAt: base.Add(time.Minute), UpdatedAt: base}, {ID: "child", Title: "Child", ParentSessionID: "a", CreatedAt: base, UpdatedAt: base.Add(2 * time.Hour)}}
	if sessions := a.visibleSessions(); sessions[0].ID != "a" {
		t.Fatalf("project conversations are not activity ordered: %+v", sessions)
	}
	a.search = "Pinned"
	sessions := a.visibleSessions()
	if len(sessions) != 1 || sessions[0].ID != "a" {
		t.Fatal("literal search was interpreted as a pin filter")
	}
	for _, session := range a.visibleSessions() {
		if session.ParentSessionID != "" || session.SideChat {
			t.Fatal("child session leaked into top-level sidebar")
		}
	}
}

func TestSidebarKeyboardSelectionAndInlineRename(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 1320, 880)
	first := a.visibleSessions()[0]
	if err := tt.Click(first.Title); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyDown)
	if a.sessionID == first.ID || a.sessionID == "" {
		t.Fatal("Down did not open the next native list row")
	}
	selected := a.sessionID
	tt.Key(0, ui.KeyF2)
	if a.sidebarUI.renameID != selected {
		t.Fatalf("F2 did not start inline rename: selected=%s cursor=%d rename=%s listFocus=%v rowFocus=%v messageFocus=%v", selected, a.sidebarUI.selection, a.sidebarUI.renameID, tt.Focused("Sessions list"), tt.Focused(a.currentSession().Title), tt.Focused("Message"))
	}
	tt.Type("Renamed from sidebar")
	tt.Key(0, ui.KeyEnter)
	if sidebarSession(t, a, selected).Title != "Renamed from sidebar" {
		t.Fatal("Enter did not commit rename")
	}
	if a.renameOpen || a.sidebarUI.renameID != "" {
		t.Fatal("inline rename retained a modal/editor")
	}
	if err := tt.RightClick("Renamed from sidebar"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Rename"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Discarded title")
	tt.Key(0, ui.KeyEscape)
	if sidebarSession(t, a, selected).Title != "Renamed from sidebar" {
		t.Fatal("Escape committed rename")
	}
}

func TestSidebarArchiveRestorePinAndDelete(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	tt := ui.NewTester(a.View, 1320, 880)
	first := sidebarSession(t, a, "chat-0")
	if err := tt.RightClick(first.Title); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Archive"); err != nil {
		t.Fatal(err)
	}
	if !sidebarSession(t, a, first.ID).Archived || a.sessionID == first.ID {
		t.Fatal("archive did not remove/open another active conversation")
	}
	if tt.HasText(first.Title) {
		t.Fatal("archived conversation stayed in the sidebar")
	}
	// Archived data is managed only in settings: view and restore it there.
	a.navigate("settings/Archived sessions")
	tt.Frame()
	if a.route != "settings" || a.section != "Archived sessions" {
		t.Fatalf("archived settings section missing: route=%s section=%s", a.route, a.section)
	}
	if err := tt.Click("View"); err != nil {
		t.Fatal(err)
	}
	if a.route != "chat" || a.sessionID != first.ID {
		t.Fatalf("view did not open the archived conversation: route=%s session=%s", a.route, a.sessionID)
	}
	a.navigate("settings/Archived sessions")
	tt.Frame()
	if err := tt.Click("Restore"); err != nil {
		t.Fatal(err)
	}
	restored := sidebarSession(t, a, first.ID)
	if restored.Archived || !restored.Pinned || restored.PinOrder != first.PinOrder {
		t.Fatal("restore lost archive or pin identity/order")
	}
	a.navigate("chat/" + first.ID)
	tt.Frame()
	if err := tt.RightClick(first.Title); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Unpin"); err != nil {
		t.Fatal(err)
	}
	if sidebarSession(t, a, first.ID).Pinned {
		t.Fatal("unpin did not persist")
	}
	if err := tt.RightClick(first.Title); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Delete"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	_ = sidebarSession(t, a, first.ID)
	if err := tt.RightClick(first.Title); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Delete"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Delete"); err != nil {
		t.Fatal(err)
	}
	for _, session := range a.state.Sessions {
		if session.ID == first.ID {
			t.Fatal("confirmed deletion did not persist")
		}
	}
}

func TestSidebarSearchResetsToConversations(t *testing.T) {
	a := testApp(t, "default")
	a.commands.tab, a.commands.selected, a.commands.activeKey = 3, 40, "old-file"
	tt := ui.NewTester(a.View, 1320, 880)
	if err := tt.Click("Search conversations"); err != nil {
		t.Fatal(err)
	}
	if !a.paletteOpen || a.commands.tab != 1 || a.commands.selected != 0 || a.commands.activeKey == "old-file" {
		t.Fatalf("conversation search retained stale state: %+v", a.commands)
	}
}

func TestSidebarPinnedNativeReorder(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 1320, 880)
	pins := a.visibleSessions()[:2]
	first, _ := tt.Find(pins[0].Title)
	second, _ := tt.Find(pins[1].Title)
	x, y := second.X+80, second.Y+second.H/2
	tt.Press(x, y)
	tt.Move(x, y-8)
	tt.Move(first.X+80, first.Y+3)
	tt.Release(first.X+80, first.Y+3)
	if got := a.visibleSessions(); got[0].ID != pins[1].ID {
		t.Fatalf("native list reorder did not persist pins: %v", []string{got[0].ID, got[1].ID})
	}
}

func TestSidebarAuditEvidence(t *testing.T) {
	for _, mode := range []string{"light", "dark"} {
		for _, group := range []string{"project-tree"} {
			t.Run(fmt.Sprintf("%s-%s", mode, group), func(t *testing.T) {
				a := testApp(t, "default")
				a.state.Settings.Appearance.Mode = mode
				a.state.Settings.Appearance.ThemeID = "rcode-" + mode
				a.selectSession("chat-0")
				tt := ui.NewTester(a.View, 1320, 880)
				captureWorkbenchAudit(t, tt, fmt.Sprintf("sidebar-%s-%s", mode, group))
			})
		}
	}
}

func TestSidebarCommandAndShortcutPromoteHiddenPanels(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	a.workspace.OpenSurface("changes")
	a.files = true
	a.panelOpened = [3]uint64{3, 2, 1}
	tt := ui.NewTester(a.View, 900, 600)
	if a.fit.Files {
		t.Fatal("fixture files must be hidden by fit")
	}
	a.toggleCommandPalette()
	a.paletteQuery = "internal"
	a.commands.tab = 3
	a.refreshPaletteSearch()
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	if a.paletteOpen || !a.files || !a.fit.Files {
		t.Fatal("command directory result closed a fit-hidden panel")
	}
	a.setPanel(0, true)
	tt.Frame()
	if !a.sidebar {
		t.Fatal("sidebar did not open")
	}
	tt.Key(ui.Cmd, ui.KeyB)
	if a.sidebar {
		t.Fatal("sidebar shortcut did not close a visible sidebar")
	}
}

func TestSidebarCommandSearchKeyboardOpensTopLevelSession(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 1320, 880)
	a.paletteOpen = true
	a.paletteQuery = "authentication redirects"
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	if a.paletteOpen || a.sessionID != "chat-2" {
		t.Fatalf("command search did not open matched session: %s", a.sessionID)
	}
}

func TestSidebarAuditTransientEvidence(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 1320, 880)
	if err := tt.Click("Search conversations"); err != nil {
		t.Fatal(err)
	}
	tt.Type("GCode")
	captureWorkbenchAudit(t, tt, "sidebar-conversation-search")
	tt.Key(0, ui.KeyEscape)
	a.startSidebarRename("chat-0")
	tt.Frame()
	captureWorkbenchAudit(t, tt, "sidebar-inline-rename")
	a.sidebarUI.renameID = ""
	a.paletteOpen = true
	a.paletteQuery = "Files"
	tt.Frame()
	captureWorkbenchAudit(t, tt, "sidebar-command-files")
}
