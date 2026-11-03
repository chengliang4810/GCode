package desktop

import (
	"errors"
	"testing"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func TestPluginSourceAddSingleInputDropAndOperationLifecycle(t *testing.T) {
	app, service := pluginUIFixture(t)
	app.section = "Marketplace"
	app.openPluginSourceAdd()
	tt := pluginUITester(app)
	for _, removed := range []string{"Marketplace name", "Source type", "Git reference", "Save", "Cancel"} {
		if _, ok := tt.Find(removed); ok {
			t.Fatalf("add dialog retained an advanced field or action: %s", removed)
		}
	}
	if _, ok := tt.Find("Choose directory"); !ok {
		t.Fatal("directory chooser missing")
	}
	tt.Key(0, ui.KeyEnter)
	if len(service.commands) != 0 {
		t.Fatal("empty input submitted")
	}
	input, ok := tt.Find("Marketplace source")
	if !ok {
		t.Fatal("single source input missing")
	}
	path := t.TempDir()
	if !tt.DropFiles(input.X+10, input.Y+10, path) || app.capabilityUI.plugins.source.Source != path {
		t.Fatal("dropping a directory did not populate the input")
	}
	if err := tt.Click("Marketplace source"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyEnter)
	if len(service.commands) != 1 {
		t.Fatalf("expected one addition: %+v", service.commands)
	}
	command := service.commands[0]
	if command.Kind != "plugins.source.save" || command.RequestID == "" ||
		*command.Plugin.Marketplace != (contracts.PluginMarketplaceInput{Source: path}) {
		t.Fatalf("addition should contain only a location: %+v", command)
	}
	tt.Key(0, ui.KeyEnter)
	if len(service.commands) != 1 || !app.capabilityUI.plugins.sourcesOpen {
		t.Fatal("pending addition submitted twice or closed before completion")
	}
	app.state.Plugins.Operations = []contracts.PluginOperation{{ID: command.RequestID, State: "failed", Error: "Invalid marketplace fixture"}}
	app.syncPluginSourceOperation()
	tt.Frame()
	if !tt.HasText("Invalid marketplace fixture") || app.capabilityUI.plugins.source.Source != path {
		t.Fatal("failed addition lost its input or inline error")
	}
	if err := tt.Click("Marketplace source"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyEnter)
	command = service.commands[len(service.commands)-1]
	app.state.Plugins.Operations = []contracts.PluginOperation{{ID: command.RequestID, State: "completed"}}
	app.syncPluginSourceOperation()
	tt.Frame()
	if app.capabilityUI.plugins.sourcesOpen || app.capabilityUI.plugins.source.Source != "" {
		t.Fatal("successful addition did not close and clear the dialog")
	}
}

func TestPluginSourceAddRejectsMultipleAndRemoteFileDrops(t *testing.T) {
	app, service := pluginUIFixture(t)
	app.section = "Marketplace"
	app.openPluginSourceAdd()
	tt := pluginUITester(app)
	input, _ := tt.Find("Marketplace source")
	if !tt.DropFiles(input.X+10, input.Y+10, "/first", "/second") ||
		!tt.HasText("Add one marketplace at a time.") || app.capabilityUI.plugins.source.Source != "" {
		t.Fatal("multiple drops were silently accepted")
	}
	app.state.Runtime.Remote = true
	tt.Frame()
	if tt.DropFiles(input.X+10, input.Y+10, "/client-only") {
		t.Fatal("remote market accepted a client filesystem drop")
	}
	if !tt.HasText("Marketplace paths are on the remote host.") || len(service.commands) != 0 {
		t.Fatal("remote path boundary missing")
	}
}

func TestPluginSourceAddIgnoresLateCompletionAfterReopening(t *testing.T) {
	app, _ := pluginUIFixture(t)
	app.openPluginSourceAdd()
	app.capabilityUI.plugins.source.Source = "owner/repository"
	app.addPluginMarketplace()
	old := app.capabilityUI.plugins.sourcePending
	app.openPluginSourceAdd()
	app.capabilityUI.plugins.source.Source = "another/repository"
	app.pluginSourceCommandFinished(contracts.Command{Kind: "plugins.source.save", RequestID: old}, errors.New("late error"))
	app.state.Plugins.Operations = []contracts.PluginOperation{{ID: old, State: "completed"}}
	app.syncPluginSourceOperation()
	if !app.capabilityUI.plugins.sourcesOpen || app.capabilityUI.plugins.source.Source != "another/repository" ||
		app.capabilityUI.plugins.sourceError != "" {
		t.Fatal("old operation altered a new draft")
	}
}
