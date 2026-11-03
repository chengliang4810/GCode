package desktop

import (
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestSettingsGlobalInstructionsSaveOnBlurAndNavigation(t *testing.T) {
	app, _ := runtimeSettingsTestApp(t)
	app.section = "Personalization"
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		app.personalizationSettings(c)
		textButton(c.Key("other"), "Other")
	}, 900, 800)
	if err := tt.Click("AGENTS.md"); err != nil {
		t.Fatal(err)
	}
	tt.Type("local test instructions")
	if _, err := os.Stat(app.globalAgentsPath()); !os.IsNotExist(err) {
		t.Fatal("instructions were saved before blur")
	}
	if err := tt.Click("Other"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(app.globalAgentsPath())
	if err != nil || string(data) != "local test instructions" {
		t.Fatalf("instructions did not save on blur: %v", err)
	}
	if err := tt.Click("AGENTS.md"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("instructions after navigation")
	app.selectSettingsSection("General")
	data, err = os.ReadFile(app.globalAgentsPath())
	if err != nil || string(data) != "instructions after navigation" {
		t.Fatalf("navigation lost the focused draft: %v", err)
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
