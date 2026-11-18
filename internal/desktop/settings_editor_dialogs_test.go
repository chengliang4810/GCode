package desktop

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/design"
	localize "gcode/internal/i18n/view"
	"github.com/egoist/mygo/ui"
)

func TestEditorModalsDismissScrollAndKeepActionsVisible(t *testing.T) {
	for _, language := range []string{"en", "zh-CN"} {
		for _, dark := range []bool{false, true} {
			for _, size := range []struct{ w, h int }{{900, 600}, {1320, 880}} {
				for _, page := range []struct {
					name, title string
					open        func(*App)
					draw        func(*App, *ui.Context)
					isOpen      func(*App) bool
				}{
					{"automation", "New automation", func(a *App) { a.editAutomation(nil) }, (*App).automationPage, func(a *App) bool { return a.automationUI.Open }},
					{"mcp", "Add MCP server", func(a *App) { a.editMCP(nil) }, (*App).mcpSettings, func(a *App) bool { return a.capabilityUI.mcpOpen }},
					{"command", "Add command", func(a *App) { a.editCommand(nil) }, (*App).commandSettings, func(a *App) bool { return a.capabilityUI.commandOpen }},
					{"hook", "Add hook", func(a *App) { a.editHook(nil) }, (*App).hookSettings, func(a *App) bool { return a.capabilityUI.hookOpen }},
					{"subagent", "Add subagent", func(a *App) { a.editSubagent(nil) }, (*App).subagentSettings, func(a *App) bool { return a.runtimeUI.subagentOpen }},
				} {
					t.Run(fmt.Sprintf("%s-%s-%t-%d", page.name, language, dark, size.w), func(t *testing.T) {
						app, _ := settingsTestApp(t)
						app.state.Settings.General.Language = language
						theme := "rcode-light"
						if dark {
							theme = "rcode-dark"
						}
						app.palette = design.Palette(theme, dark)
						page.open(app)
						tt := ui.NewTester(func(c *ui.Context) { localize.Set(c, language); c.SetTheme(app.palette.UI()); page.draw(app, c) }, size.w, size.h)
						modal, ok := tt.AccessByRole(ui.RoleDialog, app.t(page.title))
						if !ok {
							t.Fatal("missing modal")
						}
						if modal.Bounds.X < 20 || modal.Bounds.Y < 20 || modal.Bounds.X+modal.Bounds.W > float32(size.w)-19 || modal.Bounds.Y+modal.Bounds.H > float32(size.h)-19 {
							t.Fatalf("modal escapes viewport: %+v", modal.Bounds)
						}
						for _, label := range []string{"Save", "Cancel", "Close dialog"} {
							action, ok := tt.AccessByRole(ui.RoleButton, app.t(label))
							if !ok || action.Bounds.H <= 0 || action.Bounds.Y < modal.Bounds.Y || action.Bounds.Y+action.Bounds.H > modal.Bounds.Y+modal.Bounds.H {
								t.Fatalf("%s not visible inside modal: %+v", label, action.Bounds)
							}
						}
						// Scroll long content; footer actions stay visible in the same place.
						save, _ := tt.Find(app.t("Save"))
						tt.Scroll(modal.Bounds.X+modal.Bounds.W/2, modal.Bounds.Y+modal.Bounds.H/2, 0, 10000)
						tt.Frame()
						after, _ := tt.Find(app.t("Save"))
						if save != after {
							t.Fatalf("scroll moved Save footer: before=%+v after=%+v", save, after)
						}
						if dir := os.Getenv("RCODE_EDITOR_MODAL_EVIDENCE"); dir != "" {
							if err := os.MkdirAll(dir, 0755); err != nil {
								t.Fatal(err)
							}
							path := filepath.Join(dir, fmt.Sprintf("%s-%s-%t-%d.png", page.name, language, dark, size.w))
							file, err := os.Create(path)
							if err != nil {
								t.Fatal(err)
							}
							err = png.Encode(file, tt.Image())
							closeErr := file.Close()
							if err != nil || closeErr != nil {
								t.Fatalf("render: %v %v", err, closeErr)
							}
						}
						tt.Key(0, ui.KeyEscape)
						tt.Frame()
						if page.isOpen(app) {
							t.Fatal("Escape did not dismiss editor")
						}
						page.open(app)
						tt.Frame()
						tt.ClickAt(4, 4)
						tt.Frame()
						if page.isOpen(app) {
							t.Fatal("backdrop did not dismiss editor")
						}
					})
				}
			}
		}
	}
}

func TestEditorModalCannotDismissOrResubmitDuringSave(t *testing.T) {
	app, _ := settingsTestApp(t)
	app.editMCP(nil)
	app.capabilityUI.mcp.Name = "private-server"
	app.capabilityUI.mcp.Command = "node"
	app.capabilityUI.mcpEnv = `{"TOKEN":"retained-private-value"}`
	frozen := contracts.Command{Kind: "resource.mcp.save", Resource: &contracts.ResourceInput{MCP: &app.capabilityUI.mcp}}
	app.capabilityUI.editorPending = &frozen
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(app.palette.UI()); app.mcpSettings(c) }, 900, 600)
	for _, label := range []string{"Save", "Cancel", "Close dialog"} {
		node, ok := tt.AccessByRole(ui.RoleButton, label)
		if !ok || !node.Disabled {
			t.Fatalf("%s remains enabled while saving", label)
		}
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	tt.ClickAt(4, 4)
	tt.Frame()
	if !app.capabilityUI.mcpOpen || app.capabilityUI.mcpEnv != `{"TOKEN":"retained-private-value"}` {
		t.Fatal("pending save discarded draft")
	}
	app.capabilityUI.editorPending = nil
	tt.Frame()
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if app.capabilityUI.mcpOpen {
		t.Fatal("Cancel failed after pending save settled")
	}
}
