package engine

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/storage"
)

func TestProviderReorderPreservesConfigurationAndPersistsPickerOrder(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	store, err := storage.Open(storage.Options{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	core, err := openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = core.Close() }()
	configs := []model.Provider{
		{ID: "z", Name: "Zed", Protocol: model.Responses, BaseURL: "http://localhost:2345/v1",
			APIKey: "fixture-z-secret", Models: []model.Model{
				{ID: "first", MaxOutput: 500, ContextWindow: 1000, ReasoningLevels: []string{"low", "high"}},
				{ID: "disabled", Disabled: true, InputPrice: 1.5},
				{ID: "last/variant", InputPrice: 2.5, OutputPrice: 5},
			}},
		{ID: "a", Name: "Alpha", Protocol: model.Chat, BaseURL: "http://localhost:2345/v1",
			APIKey: "fixture-a-secret", Models: []model.Model{{ID: "one"}}},
		{ID: "draft", Name: "Draft", Protocol: model.Messages, APIKey: "fixture-draft-secret"},
	}
	for _, config := range configs {
		if err := core.Execute(ctx, contracts.Command{Kind: "provider.save",
			Provider: &contracts.ProviderInput{Provider: config}}); err != nil {
			t.Fatal(err)
		}
	}
	execute := func(kind string, input contracts.ReorderInput) {
		t.Helper()
		if err := core.Execute(ctx, contracts.Command{Kind: kind, Reorder: &input}); err != nil {
			t.Fatal(err)
		}
	}
	execute("provider.reorder", contracts.ReorderInput{SourceID: "draft", TargetID: "z"})
	execute("provider.reorder", contracts.ReorderInput{SourceID: "a", TargetID: "draft", After: true})
	execute("provider.models.reorder", contracts.ReorderInput{
		ProviderID: "z", SourceID: "last/variant", TargetID: "first",
	})
	execute("provider.models.reorder", contracts.ReorderInput{
		ProviderID: "z", SourceID: "disabled", TargetID: "last/variant", After: true,
	})
	wantZ := model.CloneProvider(configs[0])
	wantZ.Models = []model.Model{configs[0].Models[2], configs[0].Models[1], configs[0].Models[0]}
	want := []model.Provider{configs[2], configs[1], wantZ}
	if !reflect.DeepEqual(core.providers, want) {
		t.Fatal("reordering lost credentials, drafts or model metadata")
	}
	assertOrder := func(core *Engine) {
		t.Helper()
		state, err := core.Snapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		ids, picker := []string{}, []string{}
		for _, config := range state.CustomProviders {
			ids = append(ids, config.Provider.ID)
			if config.Provider.APIKey != "" || !config.HasAPIKey {
				t.Fatal("public snapshot leaked credentials or lost saved key status")
			}
		}
		for _, config := range state.Models {
			picker = append(picker, config.ID)
		}
		if !slices.Equal(ids, []string{"draft", "a", "z"}) ||
			!slices.Equal(picker, []string{"a/one", "z/last/variant", "z/first"}) {
			t.Fatalf("configuration and picker order disagree: providers %v, picker %v", ids, picker)
		}
	}
	assertOrder(core)
	// Current configuration updates remain authoritative; a later relative move
	// cannot overwrite a value from an earlier UI snapshot.
	edit := model.CloneProvider(core.providers[2])
	edit.Name = "Renamed while dragging"
	edit.Models[2].MaxOutput = 750
	if err := core.Execute(ctx, contracts.Command{Kind: "provider.save",
		Provider: &contracts.ProviderInput{Provider: edit, PreserveKey: true}}); err != nil {
		t.Fatal(err)
	}
	execute("provider.reorder", contracts.ReorderInput{SourceID: "a", TargetID: "draft", After: true})
	if core.providers[2].Name != edit.Name || core.providers[2].Models[2].MaxOutput != 750 {
		t.Fatal("move overwrote a newer configuration edit")
	}
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = storage.Open(storage.Options{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	core, err = openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	assertOrder(core)
	if core.providers[2].APIKey != "fixture-z-secret" {
		t.Fatal("restart lost saved credentials")
	}
}

func TestProviderReorderInvalidIDsDoNotChangeStateOrDisk(t *testing.T) {
	core := modelCheckCore(t, model.Chat, "http://localhost:2345/v1")
	ctx := context.Background()
	state, _ := core.Snapshot(ctx)
	file := filepath.Join(core.storage.Directory(), "engine.json")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, kind string
		input      contracts.ReorderInput
	}{
		{name: "missing provider source", kind: "provider.reorder", input: contracts.ReorderInput{
			SourceID: "gone", TargetID: "checks",
		}},
		{name: "missing provider target", kind: "provider.reorder", input: contracts.ReorderInput{
			SourceID: "checks", TargetID: "gone",
		}},
		{name: "missing model provider", kind: "provider.models.reorder", input: contracts.ReorderInput{
			ProviderID: "gone", SourceID: "unselected", TargetID: "selected/model",
		}},
		{name: "missing model source", kind: "provider.models.reorder", input: contracts.ReorderInput{
			ProviderID: "checks", SourceID: "gone", TargetID: "selected/model",
		}},
		{name: "missing model target", kind: "provider.models.reorder", input: contracts.ReorderInput{
			ProviderID: "checks", SourceID: "unselected", TargetID: "gone",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := core.Execute(ctx, contracts.Command{Kind: tc.kind, Reorder: &tc.input}); err == nil {
				t.Fatal("accepted obsolete reorder IDs")
			}
			got, err := core.Snapshot(ctx)
			if err != nil || !reflect.DeepEqual(state, got) {
				t.Fatal("invalid move changed state or revision")
			}
			actual, err := os.ReadFile(file)
			if err != nil || !bytes.Equal(data, actual) {
				t.Fatal("invalid move changed persisted configuration")
			}
		})
	}
}
