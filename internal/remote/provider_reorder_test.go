package remote

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/storage"
)

func TestProviderReorderUsesTypedRemoteCommands(t *testing.T) {
	store, err := storage.Open(storage.Options{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	core, err := engine.Open(engine.Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	token := strings.Repeat("reorder-test-", 3)
	handler, err := (Server{Core: core, Token: token}).Handler()
	if err != nil {
		t.Fatal(err)
	}
	host := httptest.NewServer(handler)
	defer host.Close()
	client, err := NewClient(host.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx := context.Background()
	for _, id := range []string{"first", "second"} {
		if err := client.Execute(ctx, contracts.Command{Kind: "provider.save",
			Provider: &contracts.ProviderInput{Provider: model.Provider{
				ID: id, Name: id, Protocol: model.Chat, BaseURL: "http://localhost:2345/v1",
				APIKey: "private-reorder-fixture", Models: []model.Model{{ID: "a"}, {ID: "b/variant"}},
			}}}); err != nil {
			t.Fatal(err)
		}
	}
	for _, command := range []contracts.Command{
		{Kind: "provider.reorder", Reorder: &contracts.ReorderInput{SourceID: "second", TargetID: "first"}},
		{Kind: "provider.models.reorder", Reorder: &contracts.ReorderInput{
			ProviderID: "second", SourceID: "b/variant", TargetID: "a",
		}},
	} {
		if err := client.Execute(ctx, command); err != nil {
			t.Fatal(err)
		}
	}
	state, err := client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.CustomProviders[0].Provider.ID != "second" || state.Models[0].ID != "second/b/variant" {
		t.Fatal("remote host lost relative ordering")
	}
	if state.CustomProviders[0].Provider.APIKey != "" || !state.CustomProviders[0].HasAPIKey {
		t.Fatal("remote reorder exposed credentials")
	}
}
