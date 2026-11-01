package mock

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"gcode/internal/contracts"
)

func TestProviderReorderPersistsMockListsAndRejectsObsoleteIDs(t *testing.T) {
	ctx := context.Background()
	options := Options{Instant: true, StatePath: filepath.Join(t.TempDir(), "state.json")}
	server, err := New(options)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = server.Close() }()
	original, _ := server.Snapshot(ctx)
	if err := server.Execute(ctx, contracts.Command{Kind: "provider.reorder",
		Reorder: &contracts.ReorderInput{SourceID: "openai", TargetID: "claude"}}); err != nil {
		t.Fatal(err)
	}
	if err := server.Execute(ctx, contracts.Command{Kind: "provider.models.reorder",
		Reorder: &contracts.ReorderInput{
			ProviderID: "claude", SourceID: "claude-sonnet-4-6", TargetID: "claude-opus-4-6",
		}}); err != nil {
		t.Fatal(err)
	}
	state, _ := server.Snapshot(ctx)
	if state.Providers[0].ID != "openai" || state.Providers[1].ID != "claude" ||
		state.Providers[1].Models[0] != "claude-sonnet-4-6" {
		t.Fatal("mock lists did not reflect provider and model order")
	}
	if len(state.Models) != len(original.Models) || len(state.Providers) != len(original.Providers) {
		t.Fatal("move added or removed entries")
	}
	if err := server.Execute(ctx, contracts.Command{Kind: "provider.models.reorder",
		Reorder: &contracts.ReorderInput{
			ProviderID: "claude", SourceID: "gone", TargetID: "claude-opus-4-6",
		}}); err == nil {
		t.Fatal("mock accepted missing model")
	}
	unchanged, _ := server.Snapshot(ctx)
	if !reflect.DeepEqual(state, unchanged) {
		t.Fatal("failed move changed state")
	}
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := New(options)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	state, _ = restored.Snapshot(ctx)
	if state.Providers[0].ID != "openai" || state.Providers[1].Models[0] != "claude-sonnet-4-6" {
		t.Fatal("mock restart lost order")
	}
}
