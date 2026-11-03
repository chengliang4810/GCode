package remote

import (
	"context"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/storage"
)

func TestProviderLogoRemoteFetchPersistenceAndAuthentication(t *testing.T) {
	website := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("remote credentials were forwarded to the website")
		}
		if r.URL.Path != "/favicon.png" {
			http.NotFound(w, r)
			return
		}
		_ = png.Encode(w, image.NewRGBA(image.Rect(0, 0, 24, 24)))
	}))
	defer website.Close()
	home := t.TempDir()
	token := strings.Repeat("provider-logo-", 3)
	var saved string
	for attempt := range 2 {
		func() {
			store, err := storage.Open(storage.Options{Home: home})
			if err != nil {
				t.Fatal(err)
			}
			core, err := engine.Open(engine.Options{Storage: store})
			if err != nil {
				t.Fatal(err)
			}
			defer core.Close()
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
			if attempt == 0 {
				response, err := http.Post(host.URL+"/v1/provider-logo", "application/json", strings.NewReader(`{"baseUrl":"`+website.URL+`/v1"}`))
				if err != nil {
					t.Fatal(err)
				}
				response.Body.Close()
				if response.StatusCode != http.StatusUnauthorized {
					t.Fatal("website logo endpoint bypassed host authentication")
				}
				saved, err = client.FetchProviderLogo(t.Context(), website.URL+"/v1")
				if err != nil || !strings.HasPrefix(saved, "data:image/png;base64,") {
					t.Fatalf("typed remote website fetch failed: %v", err)
				}
				err = client.Execute(t.Context(), contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: model.Provider{
					ID: "logo", Name: "Logo", Protocol: model.Chat, BaseURL: website.URL + "/v1", APIKey: "private-logo-key",
					Logo: saved, Models: []model.Model{{ID: "model"}},
				}}})
				if err != nil {
					t.Fatal(err)
				}
			}
			snapshot, err := client.Snapshot(context.Background())
			if err != nil || snapshot.Providers[0].Logo != saved || snapshot.CustomProviders[0].Provider.Logo != saved ||
				snapshot.CustomProviders[0].Provider.APIKey != "" || !snapshot.CustomProviders[0].HasAPIKey {
				t.Fatalf("logo or credentials changed after restart %d: %v", attempt, err)
			}
		}()
	}
}
