package desktop

import (
	"strings"
	"testing"

	"gcode/internal/assets"
	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func TestCommandPaletteFourTabsAndSourceSections(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	a.workspace.OpenFile("file-layout")
	a.toggleCommandPalette()
	rows := a.paletteRows()
	if len(rows) == 0 || rows[0].section != "Recent Files" || rows[0].file.ID != "file-layout" {
		t.Fatalf("recent opened file order: %+v", rows)
	}
	counts := map[string]int{}
	for _, row := range rows {
		counts[row.section]++
		if row.session != nil && (row.session.ID == a.sessionID || row.session.Archived || row.session.ParentSessionID != "") {
			t.Fatal("invalid recent thread")
		}
		if row.glyph != "" && assets.Icon(row.glyph) == nil {
			t.Fatalf("missing original icon %s", row.glyph)
		}
	}
	if counts["Quick Actions"] != 5 || counts["Recent Threads"] != 5 {
		t.Fatalf("source empty All grouping: %+v", counts)
	}
	tt := ui.NewTester(a.View, 1320, 880)
	panel, ok := tt.Find("Command palette")
	if !ok || panel.W != 560 || panel.Y != 200 {
		t.Fatalf("source palette geometry: %+v", panel)
	}
	for _, row := range a.paletteRows() {
		if bounds, visible := tt.Find("Command: " + row.title); visible && (bounds.X < panel.X+8 || bounds.X+bounds.W > panel.X+panel.W-8) {
			t.Fatalf("row escapes 8px gutters: %+v panel %+v", bounds, panel)
		}
	}
	for i, tab := range paletteTabs {
		tt.Key(ui.Ctrl, ui.Key1+ui.Key(i))
		if a.commands.tab != i {
			t.Fatalf("digit tab %s did not switch", tab)
		}
	}
	tt.Key(0, ui.KeyTab)
	if a.commands.tab != 0 {
		t.Fatal("Tab did not wrap")
	}
	tt.Key(ui.Shift, ui.KeyTab)
	if a.commands.tab != 3 {
		t.Fatal("Shift Tab did not wrap")
	}
	if err := tt.Click("Command tab: Commands"); err != nil {
		t.Fatal(err)
	}
	tt.Type("settings")
	if a.paletteQuery != "settings" {
		t.Fatal("tab click lost search focus")
	}
	tt.Key(0, ui.KeyEnter)
	if a.paletteOpen || a.route != "settings" {
		t.Fatalf("command activation: %s", a.route)
	}
}
func TestCommandPaletteContentSearchAndThemeAction(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	a.toggleCommandPalette()
	tt := ui.NewTester(a.View, 1320, 880)
	tt.Key(ui.Ctrl, ui.Key4)
	tt.Type("SidebarWidth")
	rows := a.paletteRows()
	if len(rows) == 0 || rows[0].section != "In Files" || rows[0].line == 0 {
		t.Fatalf("content preview missing: %+v", rows)
	}
	tt.Key(0, ui.KeyEnter)
	if a.paletteOpen || a.workspace.ActiveKind() != "file" {
		t.Fatal("content result did not open file")
	}
	a.toggleCommandPalette()
	tt.Frame()
	tt.Type("Switch to")
	tt.Key(0, ui.KeyEnter)
	if !a.paletteOpen {
		t.Fatal("theme action dismissed palette")
	}
	mode := a.state.Settings.Appearance.Mode
	if mode != "light" && mode != "dark" {
		t.Fatal("theme action did not change preference")
	}
	tt.Key(0, ui.KeyEscape)
	if a.paletteOpen {
		t.Fatal("Escape did not close palette")
	}
	tt.Frame()
	if a.commands.cancel != nil {
		t.Fatal("dismissed palette retained search subscription")
	}
}
func TestCommandPaletteSearchRanksMetadataAndIncludesArchived(t *testing.T) {
	a := testApp(t, "default")
	a.paletteQuery = "redirects fieldnotes"
	a.commands.tab = 1
	rows := a.paletteRows()
	if len(rows) != 1 || rows[0].session.ID != "chat-2" {
		t.Fatalf("cross-field word matching: %+v", rows)
	}
	session := contracts.Session{ID: "archived-search", Title: "needle", ProjectID: a.projectID, Archived: true}
	a.state.Sessions = append(a.state.Sessions, session)
	a.paletteQuery = "needle"
	rows = a.paletteRows()
	if len(rows) != 1 || rows[0].session.ID != session.ID {
		t.Fatal("archived thread absent from global search")
	}
	a.paletteQuery = "a"
	a.paletteOpen = true
	a.sessionID = "chat-0"
	a.refreshPaletteSearch()
	if len(a.commands.result.Files) > 0 || len(a.commands.result.Content) > 0 {
		t.Fatal("single-character palette searched workspace")
	}
}
func TestCommandPaletteEvidence(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, tab := range []int{0, 1, 2, 3} {
			a := testApp(t, "default")
			a.selectSession("chat-0")
			a.state.Settings.Appearance.Mode = "light"
			if dark {
				a.state.Settings.Appearance.Mode = "dark"
			}
			a.refreshBackground()
			a.toggleCommandPalette()
			a.commands.tab = tab
			if tab == 3 {
				a.paletteQuery = "layout"
				a.refreshPaletteSearch()
			}
			tt := ui.NewTester(a.View, 1320, 880)
			name := "light"
			if dark {
				name = "dark"
			}
			captureWorkbenchAudit(t, tt, "palette-"+name+"-"+strings.ToLower(paletteTabs[tab]))
		}
	}
}
