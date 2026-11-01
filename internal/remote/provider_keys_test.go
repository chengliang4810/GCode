package remote

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/storage"
)

func TestProviderKeyRevealRequiresAuthenticationAndDoesNotPublishSecrets(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(storage.Options{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	core, err := engine.Open(engine.Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	provider := model.Provider{ID: "saved", Name: "Saved", Protocol: model.Chat,
		BaseURL: "https://example.com/v1", APIKey: "private-saved-key"}
	if err := core.Execute(ctx, contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider}}); err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("test", 8)
	handler, err := (Server{Core: core, Token: token}).Handler()
	if err != nil {
		t.Fatal(err)
	}
	host := httptest.NewServer(handler)
	defer host.Close()
	response, err := http.Get(host.URL + "/v1/provider-key?id=saved")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatal("unauthenticated key reveal accepted")
	}
	client, err := NewClient(host.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	key, err := client.ReadProviderKey(ctx, provider.ID)
	if err != nil || key != provider.APIKey {
		t.Fatal("authenticated client could not reveal the saved key")
	}
	response, err = client.request(ctx, http.MethodGet, "/v1/provider-key?id=saved", nil)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("credential response permits caching")
	}
	if _, err := client.ReadProviderKey(ctx, "missing"); err == nil {
		t.Fatal("missing provider returned a successful key")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := core.ReadProviderKey(canceled, provider.ID); err == nil {
		t.Fatal("canceled key read succeeded")
	}
	snapshot, err := client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(snapshot)
	if err != nil || strings.Contains(string(data), provider.APIKey) || !snapshot.CustomProviders[0].HasAPIKey {
		t.Fatal("key reveal leaked the credential into a snapshot")
	}
	streamCtx, stop := context.WithCancel(ctx)
	defer stop()
	events, err := client.Subscribe(streamCtx)
	if err != nil {
		t.Fatal(err)
	}
	data, err = json.Marshal(<-events)
	if err != nil || strings.Contains(string(data), provider.APIKey) {
		t.Fatal("key reveal leaked the credential into events")
	}
}
