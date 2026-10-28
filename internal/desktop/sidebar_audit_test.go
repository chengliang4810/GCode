package desktop

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gcode/internal/contracts"
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

func TestSidebarSourceRowHeights(t *testing.T) {
	for _, tc := range []struct {
		name                          string
		compact, location, branch, pr bool
		want                          float32
	}{{"compact", true, true, true, true, 29}, {"plain", false, true, false, false, 45}, {"branch", false, true, true, false, 61}, {"pull request", false, true, true, true, 63}, {"without location", false, false, true, true, 47}} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sidebarRowHeight(tc.compact, tc.location, tc.branch, tc.pr); got != tc.want {
				t.Fatalf("height %g want %g", got, tc.want)
			}
		})
	}
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 1320, 880)
	first := a.visibleSessions(false)[0]
	row, ok := tt.Find(first.Title)
	if !ok || row.H != 29 {
		t.Fatalf("compact rendered row %+v", row)
	}
	a.state.Settings.SidebarCompact = false
	tt.Frame()
	row, _ = tt.Find(first.Title)
	if row.H != 63 {
		t.Fatalf("regular rendered PR row %+v", row)
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
	for _, path := range [][]string{{"Organize", "By project"}, {"Sort", "Created"}, {"Show", "Harness"}, {"Show", "Project filter"}, {"Compact"}} {
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
	if s.SidebarGrouping != "project" || s.SidebarSort != "created" || s.SidebarShowHarness || s.SidebarShowProjectFilter || s.SidebarCompact {
		t.Fatalf("view options did not persist: %+v", s)
	}
	if _, ok := tt.Find("All projects"); ok {
		t.Fatal("hidden project filter still mounted")
	}
}

func TestSidebarSortGroupingAndLiteralSearch(t *testing.T) {
	a := testApp(t, "default")
	base := time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC)
	a.state.Sessions = []contracts.Session{{ID: "a", Title: "Pinned workshop", ProjectID: "zeron", CreatedAt: base, UpdatedAt: base.Add(time.Hour)}, {ID: "b", Title: "Second", ProjectID: "fieldnotes", CreatedAt: base.Add(time.Minute), UpdatedAt: base}, {ID: "child", Title: "Child", ParentSessionID: "a", CreatedAt: base, UpdatedAt: base.Add(2 * time.Hour)}}
	a.state.Settings.SidebarGrouping = "project"
	a.state.Settings.SidebarSort = "updated"
	rows := a.sidebarRows()
	if rows[0].Header != "Zeron" {
		t.Fatalf("groups are not activity ordered: %+v", rows)
	}
	a.state.Settings.SidebarSort = "created"
	rows = a.sidebarRows()
	if rows[1].Session.ID != "b" {
		t.Fatal("Created sort ignored creation time")
	}
	a.search = "Pinned"
	sessions := a.visibleSessions(false)
	if len(sessions) != 1 || sessions[0].ID != "a" {
		t.Fatal("literal search was interpreted as a pin filter")
	}
	for _, session := range a.visibleSessions(false) {
		if session.ParentSessionID != "" || session.SideChat {
			t.Fatal("child session leaked into top-level sidebar")
		}
	}
}

func TestSidebarKeyboardSelectionAndInlineRename(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 1320, 880)
	first := a.visibleSessions(false)[0]
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
	if !a.sidebarUI.archivedOpen {
		if err := tt.Click("Archived"); err != nil {
			t.Fatal(err)
		}
	}
	if err := tt.RightClick(first.Title); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Unarchive"); err != nil {
		t.Fatal(err)
	}
	restored := sidebarSession(t, a, first.ID)
	if restored.Archived || !restored.Pinned || restored.PinOrder != first.PinOrder {
		t.Fatal("unarchive lost pin identity/order")
	}
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

func TestSidebarProjectSearchKeyboardAndComposerIsolation(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 1320, 880)
	composer := a.projectID
	if err := tt.Click("All projects"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Zeron")
	tt.Key(0, ui.KeyEnter)
	if a.sidebarProjectMenu || a.sidebarProjectID != "zeron" || a.projectID != composer {
		t.Fatalf("project search changed wrong scope: filter %s composer %s", a.sidebarProjectID, a.projectID)
	}
}

func TestSidebarPinnedNativeReorder(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 1320, 880)
	pins := a.visibleSessions(false)[:2]
	first, _ := tt.Find(pins[0].Title)
	second, _ := tt.Find(pins[1].Title)
	x, y := second.X+80, second.Y+second.H/2
	tt.Press(x, y)
	tt.Move(x, y-8)
	tt.Move(first.X+80, first.Y+3)
	tt.Release(first.X+80, first.Y+3)
	if got := a.visibleSessions(false); got[0].ID != pins[1].ID {
		t.Fatalf("native list reorder did not persist pins: %v", []string{got[0].ID, got[1].ID})
	}
}

func TestSidebarAuditEvidence(t *testing.T) {
	for _, mode := range []string{"light", "dark"} {
		for _, group := range []string{"list", "project", "device"} {
			for _, compact := range []bool{true, false} {
				t.Run(fmt.Sprintf("%s-%s-%t", mode, group, compact), func(t *testing.T) {
					a := testApp(t, "default")
					a.state.Settings.Appearance.Mode = mode
					a.state.Settings.Appearance.ThemeID = "zeron-" + mode
					a.state.Settings.SidebarGrouping = group
					a.state.Settings.SidebarCompact = compact
					a.selectSession("chat-0")
					tt := ui.NewTester(a.View, 1320, 880)
					captureWorkbenchAudit(t, tt, fmt.Sprintf("sidebar-%s-%s-compact-%t", mode, group, compact))
				})
			}
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
	if err := tt.Click("All projects"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Zeron")
	captureWorkbenchAudit(t, tt, "sidebar-project-search")
	tt.Key(0, ui.KeyEscape)
	a.startSidebarRename("chat-0")
	tt.Frame()
	captureWorkbenchAudit(t, tt, "sidebar-inline-rename")
	a.sidebarUI.renameID = ""
	a.sidebarUI.archivedOpen = true
	tt.Frame()
	captureWorkbenchAudit(t, tt, "sidebar-archived-open")
	a.paletteOpen = true
	a.paletteQuery = "Files"
	tt.Frame()
	captureWorkbenchAudit(t, tt, "sidebar-command-files")
}
