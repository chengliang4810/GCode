package desktop

import (
	"context"
	"errors"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestModelSettingsAutomaticSaveRetainsNewerKeyAndFailedDraft(t *testing.T) {
	app, core := runtimeSettingsTestApp(t)
	provider := model.Provider{ID: "auto-key", Name: "Auto key", Protocol: model.Chat}
	app.dispatch(contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider}})
	app.win = &mygo.Window{}
	t.Cleanup(func() { app.win = nil })
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).Fill().Children(func() { app.customProviderSettings(c) })
	}, 1000, 800)
	if err := tt.Click("API key"); err != nil {
		t.Fatal(err)
	}
	tt.Type("first")
	if err := tt.Click("Provider name"); err != nil {
		t.Fatal(err)
	}
	first := <-app.jobs
	if err := tt.Click("API key"); err != nil {
		t.Fatal(err)
	}
	tt.Type("-second")
	if err := tt.Click("Provider name"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-app.jobs:
		t.Fatal("a second save started before the first completed")
	default:
	}
	err := core.Execute(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	app.commandFinished(first, nil)
	snapshot, err := core.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	app.apply(snapshot)
	tt.Frame()
	next := <-app.jobs
	if next.Provider.Provider.APIKey != "first-second" {
		t.Fatal("automatic save discarded the newer key")
	}
	app.commandFinished(next, errors.New("credential write failed"))
	tt.Frame()
	tt.Advance(time.Second)
	if app.runtimeUI.key != "first-second" || !app.modelsUI.inlineDirty || !tt.HasText("credential write failed") {
		t.Fatal("failed automatic save lost the draft or error")
	}
	select {
	case <-app.jobs:
		t.Fatal("failed save retried without a new edit")
	default:
	}
}

func TestModelSettingsQueuedSaveKeepsNewerInputAndWaitsBeforeNavigation(t *testing.T) {
	app, core := runtimeSettingsTestApp(t)
	app.route, app.section = "settings", "Model settings"
	// Exercise the native command queue without attaching a graphics window.
	app.win = &mygo.Window{}
	t.Cleanup(func() { app.win = nil })
	app.addCustomProvider()
	id := app.modelsUI.selected
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).Fill().Padding(16).Children(func() { app.customProviderSettings(c) })
	}, 1000, 800)
	if id == "" || app.modelsUI.selected != id || !app.modelsUI.inlineSaving {
		t.Fatal("pending provider creation lost its selected draft")
	}
	complete := func(command contracts.Command) {
		t.Helper()
		err := core.Execute(context.Background(), command)
		app.commandFinished(command, err)
		if err != nil {
			t.Fatal(err)
		}
		snapshot, snapshotErr := core.Snapshot(context.Background())
		if snapshotErr != nil {
			t.Fatal(snapshotErr)
		}
		app.apply(snapshot)
		tt.Frame()
	}
	complete(<-app.jobs)
	if app.modelsUI.selected != id || !tt.HasText("Base URL") {
		t.Fatal("completed empty provider creation did not display its editor")
	}
	app.runtimeUI.provider.BaseURL = "https://first.example.com/v1"
	app.runtimeUI.key = "first-test-key"
	app.markInlineProviderDirty()
	app.saveInlineProvider()
	firstSave := <-app.jobs

	// Typing can continue while disk/network work is outstanding.
	app.runtimeUI.provider.BaseURL = "https://second.example.com/v1"
	app.runtimeUI.key = "second-test-key"
	app.markInlineProviderDirty()
	app.selectSettingsSection("Skills")
	complete(firstSave)
	if app.section != "Model settings" || !app.modelsUI.inlineDirty ||
		app.runtimeUI.key != "second-test-key" || app.runtimeUI.provider.BaseURL != "https://second.example.com/v1" {
		t.Fatal("an older save discarded newer input or navigated before saving it")
	}
	app.saveInlineProvider()
	complete(<-app.jobs)
	if app.section != "Skills" || app.runtimeUI.key != "" {
		t.Fatal("navigation did not resume after the current draft was saved")
	}
	snapshot, _ := core.Snapshot(context.Background())
	if len(snapshot.CustomProviders) != 1 || snapshot.CustomProviders[0].Provider.BaseURL != "https://second.example.com/v1" ||
		!snapshot.CustomProviders[0].HasAPIKey {
		t.Fatal("the newest queued draft did not reach the real core")
	}
	app.selectSettingsSection("Model settings")
	tt.Frame()
	app.runtimeUI.key = "key-before-leaving-settings"
	app.markInlineProviderDirty()
	app.OpenRoute("chat")
	if app.route != "settings" {
		t.Fatal("left settings before the key was saved")
	}
	complete(<-app.jobs)
	if app.route != "chat" {
		t.Fatal("did not leave settings after saving")
	}
	key, err := core.ReadProviderKey(context.Background(), id)
	if err != nil || key != "key-before-leaving-settings" {
		t.Fatal("leaving settings lost the focused key draft")
	}
}
