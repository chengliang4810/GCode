package model

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListModelsReadsProviderEndpointPerProtocol(t *testing.T) {
	for _, tc := range []struct {
		protocol Protocol
		auth     func(r *http.Request) bool
	}{
		{Chat, func(r *http.Request) bool { return r.Header.Get("Authorization") == "Bearer list-secret" }},
		{Responses, func(r *http.Request) bool { return r.Header.Get("Authorization") == "Bearer list-secret" }},
		{Messages, func(r *http.Request) bool {
			return r.Header.Get("x-api-key") == "list-secret" && r.Header.Get("anthropic-version") == "2023-06-01"
		}},
	} {
		t.Run(string(tc.protocol), func(t *testing.T) {
			var path, method string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path, method = r.URL.Path, r.Method
				if !tc.auth(r) {
					t.Errorf("model list did not carry protocol credentials: %v", r.Header)
				}
				if r.Header.Get("Accept") != "application/json" {
					t.Error("model list did not request JSON")
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"data":[{"id":"gamma"},{"id":"beta"},{"id":"beta"},{"id":" "},{"note":"ignored"}]}`)
			}))
			t.Cleanup(server.Close)
			client := NewClient()
			ids, err := client.ListModels(context.Background(), Provider{
				ID: "p", Name: "Fixture", Protocol: tc.protocol, BaseURL: server.URL + "/v1", APIKey: "list-secret",
				Models: []Model{{ID: "seed"}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if method != http.MethodGet || path != "/v1/models" {
				t.Fatalf("unexpected request: %s %s", method, path)
			}
			if len(ids) != 2 || ids[0] != "beta" || ids[1] != "gamma" {
				t.Fatalf("model list was not deduplicated and sorted: %v", ids)
			}
		})
	}
}

func TestListModelsRejectsInvalidConfiguration(t *testing.T) {
	client := NewClient()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	t.Cleanup(server.Close)
	if _, err := client.ListModels(context.Background(), Provider{ID: "p", Name: "Fixture", Protocol: Chat, Disabled: true, BaseURL: server.URL, Models: []Model{{ID: "m"}}}); err == nil {
		t.Fatal("disabled provider listing was accepted")
	}
	if _, err := client.ListModels(context.Background(), Provider{ID: "p", Name: "Fixture", Protocol: Chat, Models: []Model{{ID: "m"}}}); err == nil {
		t.Fatal("missing base URL listing was accepted")
	}
	if _, err := client.ListModels(context.Background(), Provider{ID: "p", Name: "Fixture", Protocol: "bogus", BaseURL: server.URL, Models: []Model{{ID: "m"}}}); err == nil {
		t.Fatal("unknown protocol listing was accepted")
	}
	if calls != 0 {
		t.Fatal("rejected listing still called the endpoint")
	}
}

func TestListModelsSurfacesHTTPAndPayloadFailures(t *testing.T) {
	t.Run("status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `denied`)
		}))
		t.Cleanup(server.Close)
		_, err := NewClient().ListModels(context.Background(), Provider{ID: "p", Name: "Fixture", Protocol: Chat, BaseURL: server.URL + "/v1", Models: []Model{{ID: "m"}}})
		if err == nil || err.Error() != "model list request failed (HTTP 403)" {
			t.Fatalf("unexpected status error: %v", err)
		}
	})
	t.Run("payload", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `<html>not json</html>`)
		}))
		t.Cleanup(server.Close)
		if _, err := NewClient().ListModels(context.Background(), Provider{ID: "p", Name: "Fixture", Protocol: Chat, BaseURL: server.URL + "/v1", Models: []Model{{ID: "m"}}}); err == nil {
			t.Fatal("invalid payload listing was accepted")
		}
	})
	t.Run("trailing slash", func(t *testing.T) {
		var path string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path = r.URL.Path
			fmt.Fprint(w, `{"data":[]}`)
		}))
		t.Cleanup(server.Close)
		ids, err := NewClient().ListModels(context.Background(), Provider{ID: "p", Name: "Fixture", Protocol: Chat, BaseURL: server.URL + "/v1/", Models: []Model{{ID: "m"}}})
		if err != nil || len(ids) != 0 {
			t.Fatalf("empty listing failed: %v %v", ids, err)
		}
		if path != "/v1/models" {
			t.Fatalf("trailing slash produced wrong path: %s", path)
		}
	})
}
