package desktop

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/remote"
	"github.com/egoist/mygo/ui"
)

type instructionsReadCounter struct {
	contracts.Service
	contracts.GlobalInstructionsService
	reads int
}

func (s *instructionsReadCounter) ReadGlobalInstructions(ctx context.Context) (string, error) {
	s.reads++
	return s.GlobalInstructionsService.ReadGlobalInstructions(ctx)
}

func TestSettingsGlobalInstructionsLoadBelongsToNavigation(t *testing.T) {
	app, core := runtimeSettingsTestApp(t)
	service := &instructionsReadCounter{Service: core, GlobalInstructionsService: core}
	app.service = service
	app.selectSettingsSection("Personalization")
	if service.reads != 1 {
		t.Fatal("navigation did not prepare instructions")
	}
	tester := ui.NewTester(func(c *ui.Context) { app.personalizationSettings(c) }, 900, 800)
	for range 4 {
		tester.Frame()
	}
	if service.reads != 1 {
		t.Fatal("rendering restarted instruction I/O")
	}
}

func TestSettingsGlobalInstructionsRemoteHostDoesNotTouchLocalPath(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, width := range []int{600, 1320} {
			t.Run(fmt.Sprintf("dark=%v/width=%d", dark, width), func(t *testing.T) {
				app, core := runtimeSettingsTestApp(t)
				t.Cleanup(func() { _ = core.Close() })
				token := strings.Repeat("x", 24)
				handler, err := (remote.Server{Core: core, Token: token}).Handler()
				if err != nil {
					t.Fatal(err)
				}
				host := httptest.NewServer(handler)
				defer host.Close()
				client, err := remote.NewClient(host.URL, token)
				if err != nil {
					t.Fatal(err)
				}
				defer client.Close()
				app.service = client
				local := t.TempDir()
				path := filepath.Join(local, "AGENTS.md")
				if err := os.WriteFile(path, []byte("local sentinel"), 0600); err != nil {
					t.Fatal(err)
				}
				app.state.Runtime.ConfigDirectory = local
				app.state.Runtime.Remote = true
				if err := core.WriteGlobalInstructions(t.Context(), "host original"); err != nil {
					t.Fatal(err)
				}
				app.selectSettingsSection("Personalization")
				theme := "rcode-light"
				if dark {
					theme = "rcode-dark"
				}
				app.palette = design.Palette(theme, dark)
				tt := ui.NewTester(func(c *ui.Context) {
					c.SetTheme(app.palette.UI())
					app.personalizationSettings(c)
					textButton(c.Key("other"), "Other")
				}, width, 800)
				if app.agentsDraft != "host original" {
					t.Fatalf("loaded local path instead of host: %q", app.agentsDraft)
				}
				if err := tt.Click("AGENTS.md"); err != nil {
					t.Fatal(err)
				}
				tt.Key(ui.Cmd, ui.KeyA)
				tt.Type("remote edit")
				if err := tt.Click("Other"); err != nil {
					t.Fatal(err)
				}
				text, err := core.ReadGlobalInstructions(t.Context())
				if err != nil || text != "remote edit" {
					t.Fatalf("host edit missing: %q %v", text, err)
				}
				data, err := os.ReadFile(path)
				if err != nil || string(data) != "local sentinel" {
					t.Fatalf("client wrote host path locally: %q %v", data, err)
				}
			})
		}
	}
}

func TestSettingsGlobalInstructionsSaveOnBlurAndNavigation(t *testing.T) {
	app, _ := runtimeSettingsTestApp(t)
	path := filepath.Join(app.state.Runtime.ConfigDirectory, "AGENTS.md")
	app.selectSettingsSection("Personalization")
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.personalizationSettings(c)
		textButton(c.Key("other"), "Other")
	}, 900, 800)
	if err := tt.Click("AGENTS.md"); err != nil {
		t.Fatal(err)
	}
	tt.Type("local test instructions")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("instructions were saved before blur")
	}
	if err := tt.Click("Other"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "local test instructions" {
		t.Fatalf("instructions did not save on blur: %v", err)
	}
	if err := tt.Click("AGENTS.md"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("instructions after navigation")
	app.selectSettingsSection("General")
	data, err = os.ReadFile(path)
	if err != nil || string(data) != "instructions after navigation" {
		t.Fatalf("navigation lost the focused draft: %v", err)
	}
}

func TestSettingsGlobalInstructionsCloseFlushesDraft(t *testing.T) {
	app, _ := runtimeSettingsTestApp(t)
	path := filepath.Join(app.state.Runtime.ConfigDirectory, "AGENTS.md")
	app.agentsDraft = "最后一次编辑"
	app.Close()
	data, err := os.ReadFile(path)
	if err != nil || string(data) != app.agentsDraft {
		t.Fatalf("close lost instruction draft: %q %v", data, err)
	}
}

func TestSettingsInputCommitOnlyOnChangedBlur(t *testing.T) {
	for _, multiline := range []bool{false, true} {
		t.Run(map[bool]string{false: "input", true: "textarea"}[multiline], func(t *testing.T) {
			value, saved := "initial", ""
			count := 0
			tt := ui.NewTester(func(c *ui.Context) {
				var input ui.Element
				if multiline {
					input = controlTextArea(c.Key("multiline"), &value).Height(100)
				} else {
					input = controlTextInput(c.Key("field"), &value)
				}
				settingsInputCommit(input.Label("Field"), &value, func() {
					saved = value
					count++
				})
				textButton(c.Key("other"), "Other")
			}, 600, 400)
			if err := tt.Click("Field"); err != nil {
				t.Fatal(err)
			}
			tt.Key(ui.Cmd, ui.KeyA)
			tt.Type("edited")
			tt.Advance(time.Second)
			if count != 0 {
				t.Fatal("saved while focused")
			}
			if err := tt.Click("Other"); err != nil {
				t.Fatal(err)
			}
			if saved != "edited" || count != 1 {
				t.Fatalf("blur save = %q, count = %d", saved, count)
			}
			tt.Frame()
			if err := tt.Click("Field"); err != nil {
				t.Fatal(err)
			}
			if err := tt.Click("Other"); err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatal("unchanged focus cycle saved again")
			}
		})
	}
}
