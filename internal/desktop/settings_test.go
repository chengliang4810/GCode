package desktop

import (
	"context"
	"strings"
	"testing"
	"time"

	"gcode/internal/assets"
	"gcode/internal/design"
	"gcode/internal/mock"
	"github.com/egoist/mygo/ui"
)

func settingsTestApp(t *testing.T) (*App, *mock.Server) {
	t.Helper()
	if err := assets.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	backend, err := mock.New(mock.Options{Instant: true})
	if err != nil {
		t.Fatal(err)
	}
	app, err := New(backend)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	return app, backend
}

func TestSettingsPagesRenderWithoutVoiceOrHorizontalOverflow(t *testing.T) {
	app, _ := settingsTestApp(t)
	for _, section := range app.sections() {
		t.Run(section, func(t *testing.T) {
			app.section = section
			app.palette = design.Palette("zeron-dark", true)
			tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(app.palette.UI()); app.settingsView(c) }, 900, 600)
			if !tt.HasText(section) {
				t.Fatal("missing page heading")
			}
			for _, text := range tt.Texts() {
				if strings.EqualFold(text, "Voice") || strings.Contains(strings.ToLower(text), "dictation") {
					t.Fatalf("voice leaked into %s", section)
				}
				r, ok := tt.Find(text)
				if ok && r.W > 0 && (r.X < -1 || r.X+r.W > 901) {
					t.Fatalf("horizontal overflow for %q: %+v", text, r)
				}
			}
			tt.SetSize(1320, 880)
			tt.Frame()
			if !tt.HasText(section) {
				t.Fatal("page missing after resize")
			}
		})
	}
}

func TestSettingsAppearanceSupportsAllOriginalPalettes(t *testing.T) {
	app, _ := settingsTestApp(t)
	app.section = "Appearance"
	for _, variant := range design.Themes() {
		t.Run(variant.ID, func(t *testing.T) {
			app.palette = design.Palette(variant.ID, variant.Appearance == "dark")
			tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(app.palette.UI()); app.settingsView(c) }, 900, 600)
			if !tt.HasText("Light theme") || !tt.HasText("Dark theme") {
				t.Fatal("theme selectors missing")
			}
			r, ok := tt.Find("Appearance Dark")
			if !ok || r.W <= 0 || r.X < 0 || r.X+r.W > 900 {
				t.Fatalf("appearance control bounds: %+v", r)
			}
		})
	}
}

func TestSettingsTogglePersistsThroughBackend(t *testing.T) {
	app, backend := settingsTestApp(t)
	app.section = "General"
	app.palette = design.Palette("zeron-dark", true)
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(app.palette.UI()); app.settingsView(c) }, 900, 600)
	before := app.state.Settings.General.CompactMode
	if err := tt.Click("Toggle Compact mode"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	snapshot, err := backend.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Settings.General.CompactMode == before {
		t.Fatal("switch did not update typed backend settings")
	}
	app.state = snapshot
	tt.Frame()
	if app.state.Settings.General.CompactMode == before {
		t.Fatal("backend refresh reverted edit")
	}
}

func TestAppearanceSelectionChangesModeAndClearsOverride(t *testing.T) {
	app, backend := settingsTestApp(t)
	app.section = "Appearance"
	app.palette = design.Palette("zeron-dark", true)
	app.state.Settings.Appearance.Mode = "dark"
	app.state.Settings.Appearance.ThemeID = "zeron-dark"
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(app.palette.UI()); app.settingsView(c) }, 900, 600)
	if err := tt.Click("Appearance Light"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	snapshot, err := backend.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Settings.Appearance.Mode != "light" || snapshot.Settings.Appearance.ThemeID != "" {
		t.Fatalf("mode selection not persisted: %+v", snapshot.Settings.Appearance)
	}
}

func TestSettingsSearchNavigatesToAppearance(t *testing.T) {
	app, _ := settingsTestApp(t)
	app.palette = design.Palette("zeron-dark", true)
	app.section = "General"
	app.settingsSearch = "theme"
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(app.palette.UI()); app.settingsNavigation(c) }, 256, 880)
	if !tt.HasText("Light theme") || !tt.HasText("Theme library") {
		t.Fatal("settings search omitted relevant rows")
	}
	if err := tt.Click("Light theme"); err != nil {
		t.Fatal(err)
	}
	if app.section != "Appearance" {
		t.Fatalf("landed in %s", app.section)
	}
	app.settingsSearch = "zzzz-no-setting"
	tt.Frame()
	if !tt.HasText("No settings found") {
		t.Fatal("missing empty search state")
	}
}

func TestSettingsSearchRanksAndCorrectsTypos(t *testing.T) {
	app, _ := settingsTestApp(t)
	hits := settingsSearchHits(app.settingLinks(), "thmee")
	if len(hits) == 0 {
		t.Fatal("adjacent typo lost theme results")
	}
	found := false
	for _, hit := range hits {
		if hit.title == "Light theme" {
			found = true
		}
	}
	if !found {
		t.Fatal("theme typo omitted Light theme")
	}
	hits = settingsSearchHits(app.settingLinks(), "light theme")
	if len(hits) == 0 || hits[0].title != "Light theme" {
		t.Fatal("exact title should rank first")
	}
	if len(settingsSearchHits(app.settingLinks(), "theme nonexistentword")) != 0 {
		t.Fatal("all query words must match")
	}
}

func TestSettingsSearchRevealsTargetRowOnce(t *testing.T) {
	app, _ := settingsTestApp(t)
	app.section = "Appearance"
	app.palette = design.Palette("zeron-dark", true)
	app.expanded["settings-target:Terminal font"] = true
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(app.palette.UI()); app.settingsView(c) }, 900, 600)
	r, ok := tt.Find("Terminal font")
	if !ok || r.Y < 0 || r.Y+r.H > 600 {
		t.Fatalf("target was not revealed: %+v", r)
	}
	if app.expanded["settings-target:Terminal font"] {
		t.Fatal("reveal request not consumed")
	}
	tt.Advance(2 * time.Second)
	if app.expanded["settings-target:Terminal font"] {
		t.Fatal("highlight reissued a scroll request")
	}
}

func TestShortcutRecorderCapturesNativeKeysAndPersists(t *testing.T) {
	app, backend := settingsTestApp(t)
	app.section = "Shortcuts"
	app.palette = design.Palette("zeron-dark", true)
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(app.palette.UI()); app.settingsView(c) }, 900, 600)
	if err := tt.Click("Edit shortcut for New session"); err != nil {
		t.Fatal(err)
	}
	if !app.shortcutRecording() {
		t.Fatal("recorder did not activate")
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyP)
	tt.Frame()
	snapshot, err := backend.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Settings.Shortcuts[0].Keys != "Mod+Shift+P" {
		t.Fatalf("recorded %q", snapshot.Settings.Shortcuts[0].Keys)
	}
	if app.shortcutRecording() {
		t.Fatal("recorder did not settle")
	}
}

func TestShortcutRecorderRejectsDuplicatesAndEscapeCancels(t *testing.T) {
	app, _ := settingsTestApp(t)
	app.section = "Shortcuts"
	app.palette = design.Palette("zeron-dark", true)
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(app.palette.UI()); app.settingsView(c) }, 900, 600)
	if err := tt.Click("Edit shortcut for New session"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyK)
	if !app.shortcutRecording() || app.state.Settings.Shortcuts[0].Keys != "Mod+N" {
		t.Fatal("duplicate changed keybinding")
	}
	tt.Key(0, ui.KeyEscape)
	if app.shortcutRecording() {
		t.Fatal("Escape failed to cancel recording")
	}
}

func TestThemeImportDialogUsesBackendPreviewAndLibrary(t *testing.T) {
	app, backend := settingsTestApp(t)
	app.section = "Appearance"
	app.palette = design.Palette("zeron-dark", true)
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(app.palette.UI()); app.settingsView(c) }, 900, 1800)
	if err := tt.Click("Add theme"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Add a theme") {
		t.Fatal("import modal did not open")
	}
	if err := tt.Click("Theme source path"); err != nil {
		t.Fatal(err)
	}
	tt.Type("/themes/github.json")
	if err := tt.Click("Analyze theme"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Import selected") {
		t.Fatalf("analysis did not return preview: %v", tt.Texts())
	}
	if err := tt.Click("Import selected"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	snapshot, err := backend.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.ThemeLibrary) != 1 {
		t.Fatalf("library has %d entries", len(snapshot.ThemeLibrary))
	}
	if tt.HasText("Add a theme") {
		t.Fatal("successful import did not close modal")
	}
	if err := tt.Click("Remove theme " + snapshot.ThemeLibrary[0].ID); err != nil {
		t.Fatal(err)
	}
	snapshot, err = backend.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.ThemeLibrary) != 0 {
		t.Fatal("Remove did not update backend library")
	}
}

func TestPersistedShortcutIsUsedAndRecorderSuppressesGlobalActions(t *testing.T) {
	app, _ := settingsTestApp(t)
	app.section = "Shortcuts"
	app.route = "settings"
	app.palette = design.Palette("zeron-dark", true)
	app.state.Settings.Shortcuts[0].Keys = "Mod+Shift+P"
	app.saveSettings()
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.registerSettingsShortcuts(c)
		app.settingsView(c)
	}, 900, 600)
	tt.Key(ui.Cmd, ui.KeyN)
	if app.route != "settings" {
		t.Fatal("old default shortcut still active")
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyP)
	if app.route != "chat" {
		t.Fatal("persisted shortcut not active")
	}
	app.route = "settings"
	tt.Frame()
	if err := tt.Click("Edit shortcut for New session"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyK)
	if app.paletteOpen {
		t.Fatal("global command palette shortcut stole recorder input")
	}
}

func TestFontPickerFiltersAndPersistsSelection(t *testing.T) {
	app, backend := settingsTestApp(t)
	app.section = "Appearance"
	app.palette = design.Palette("zeron-dark", true)
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(app.palette.UI()); app.settingsView(c) }, 900, 1800)
	if err := tt.Click("Choose Interface font"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Search fonts") {
		t.Fatal("font search did not open")
	}
	tt.Type("font-that-does-not-exist")
	if !tt.HasText("No matching fonts") {
		t.Fatal("font picker did not filter unavailable names")
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("Geist Mono")
	if err := tt.Click("Select Interface font Geist Mono"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := backend.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Settings.Appearance.Font != "Geist Mono" {
		t.Fatalf("selected font = %q", snapshot.Settings.Appearance.Font)
	}
	if tt.HasText("Search fonts") {
		t.Fatal("font popup did not close after selection")
	}
}

func TestNotificationChildrenFollowParentAvailability(t *testing.T) {
	app, backend := settingsTestApp(t)
	app.section = "Notifications"
	app.palette = design.Palette("zeron-dark", true)
	app.state.Settings.Notifications.Sound = false
	app.state.Settings.Notifications.Completion = true
	app.state.Settings.Notifications.Enabled = false
	app.state.Settings.Notifications.AgentUpdates = true
	app.saveSettings()
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(app.palette.UI()); app.settingsView(c) }, 900, 900)
	_ = tt.Click("Toggle Task completed")
	_ = tt.Click("Toggle Agent updates")
	if !app.state.Settings.Notifications.Completion || !app.state.Settings.Notifications.AgentUpdates {
		t.Fatal("disabled child changed its preference")
	}
	if err := tt.Click("Toggle Desktop notifications"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Toggle Agent updates"); err != nil {
		t.Fatal(err)
	}
	before := app.state.Settings.Notifications.WhenFocused
	if err := tt.Click("Toggle Only when in the background"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := backend.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.Settings.Notifications.Enabled || snapshot.Settings.Notifications.AgentUpdates || snapshot.Settings.Notifications.WhenFocused == before {
		t.Fatal("notification preferences did not persist independently")
	}
}

func TestFilesAutosaveOnlyShowsOriginalDelayChoices(t *testing.T) {
	app, backend := settingsTestApp(t)
	app.section = "Files"
	app.palette = design.Palette("zeron-dark", true)
	app.state.Settings.Files.Autosave = false
	app.saveSettings()
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(app.palette.UI()); app.settingsView(c) }, 900, 600)
	if tt.HasText("Autosave delay") || tt.HasText("Line numbers") {
		t.Fatal("unavailable or non-reference control is visible")
	}
	if err := tt.Click("Toggle Autosave"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Choose Autosave delay"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("1.5 s") || !tt.HasText("600 ms") || tt.HasText("10000 ms") {
		t.Fatal("autosave options differ from the reference")
	}
	if err := tt.Click("600 ms"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := backend.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Settings.Files.AutosaveDelay != 600 {
		t.Fatal("autosave selection did not persist")
	}
}

func TestSettingsSearchTargetsParentOfHiddenRow(t *testing.T) {
	app, _ := settingsTestApp(t)
	app.section = "General"
	app.settingsSearch = "autosave delay"
	app.state.Settings.Files.Autosave = false
	app.palette = design.Palette("zeron-dark", true)
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(app.palette.UI()); app.settingsNavigation(c) }, 256, 880)
	if err := tt.Click("Autosave delay"); err != nil {
		t.Fatal(err)
	}
	if app.section != "Files" || !app.expanded["settings-target:Autosave"] || app.expanded["settings-target:Autosave delay"] {
		t.Fatal("hidden row did not land at its parent control")
	}
}

func TestAppshotsUsesNativeRecordingAndBackendPermissionStates(t *testing.T) {
	if err := assets.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	backend, err := mock.New(mock.Options{Instant: true, Platform: "darwin"})
	if err != nil {
		t.Fatal(err)
	}
	app, err := New(backend)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	app.palette = design.Palette("zeron-dark", true)
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Scroll(c).FillWidth().FillHeight().Children(func() {
			ui.Column(c).Padding(20).Gap(16).Children(func() { app.appshotSettings(c) })
		})
	}, 900, 900)
	for _, title := range []string{"Capture Appshots", "Capture sound", "Global shortcut", "Destination", "Window capture", "Application text"} {
		if !tt.HasText(title) {
			t.Fatalf("missing source row %q", title)
		}
	}
	if tt.HasText("Include cursor") || tt.HasText("Allow window capture") {
		t.Fatal("unexpected source row or permission action while Appshots are off")
	}
	if err := tt.Click("Toggle Capture Appshots"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Allow window capture"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Enable text capture"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Edit shortcut for Global shortcut"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd|ui.Alt|ui.Shift, ui.KeyY)
	if err := tt.Click("Choose Appshot destination"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("New session"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := backend.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.AppshotCapabilities.WindowCapture != "ready" || snapshot.AppshotCapabilities.ApplicationText != "ready" || snapshot.Settings.Appshots.Shortcut != "Mod+Alt+Shift+Y" || snapshot.Settings.Appshots.Destination != "newSession" {
		t.Fatal("Appshots preferences or permission responses did not reach the backend")
	}
}
