package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
)

func TestModelFetchReadsConfiguredProviderEndpoint(t *testing.T) {
	for _, protocol := range []model.Protocol{model.Chat, model.Responses, model.Messages} {
		t.Run(string(protocol), func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Method != http.MethodGet || r.URL.Path != "/v1/models" {
					t.Errorf("model list used wrong endpoint: %s %s", r.Method, r.URL.Path)
				}
				if (protocol == model.Messages && r.Header.Get("x-api-key") != "check-secret") ||
					(protocol != model.Messages && r.Header.Get("Authorization") != "Bearer check-secret") {
					t.Error("model list did not use provider credentials")
				}
				if protocol == model.Messages && r.Header.Get("anthropic-version") == "" {
					t.Error("anthropic model list omitted its version header")
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"data":[{"id":"zeta"},{"id":"alpha"},{"id":"alpha"},{"id":"  "},{"object":"model"}]}`)
			}))
			t.Cleanup(server.Close)
			core := modelCheckCore(t, protocol, server.URL)
			if err := core.Execute(context.Background(), contracts.Command{Kind: "provider.models.fetch", ID: "checks"}); err != nil {
				t.Fatal(err)
			}
			state := waitSubagentState(t, core, func(state contracts.Snapshot) bool {
				fetch := state.CustomProviders[0].ModelFetch
				return fetch != nil && fetch.State == "ready"
			})
			fetch := state.CustomProviders[0].ModelFetch
			if requests.Load() != 1 || fetch.ProviderID != "checks" || fetch.FinishedAt == nil {
				t.Fatalf("missing fetch result: %+v requests=%d", fetch, requests.Load())
			}
			if len(fetch.Models) != 2 || fetch.Models[0] != "alpha" || fetch.Models[1] != "zeta" {
				t.Fatalf("model list was not deduplicated and sorted: %v", fetch.Models)
			}
			fetch.Models[0] = "mutated"
			current, _ := core.Snapshot(context.Background())
			if current.CustomProviders[0].ModelFetch.Models[0] != "alpha" {
				t.Fatal("model fetch snapshot shares mutable result state")
			}
		})
	}
}

func TestModelFetchSurfacesSanitizedErrors(t *testing.T) {
	t.Run("http", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"error":{"message":"check-secret","code":"invalid_api_key","request_id":"req-42"}}`)
		}))
		t.Cleanup(server.Close)
		core := modelCheckCore(t, model.Chat, server.URL)
		if err := core.Execute(context.Background(), contracts.Command{Kind: "provider.models.fetch", ID: "checks"}); err != nil {
			t.Fatal(err)
		}
		state := waitSubagentState(t, core, func(state contracts.Snapshot) bool {
			fetch := state.CustomProviders[0].ModelFetch
			return fetch != nil && fetch.State == "error"
		})
		encoded, _ := json.Marshal(state)
		if strings.Contains(string(encoded), "check-secret") {
			t.Fatal("model fetch snapshot leaked provider error payload")
		}
		if detail := state.CustomProviders[0].ModelFetch.Error; !strings.Contains(detail, "401") ||
			!strings.Contains(detail, `"code":"invalid_api_key"`) || !strings.Contains(detail, `"request_id":"req-42"`) ||
			!strings.Contains(detail, "[redacted]") {
			t.Fatalf("fetch diagnostic omitted original safe details: %q", detail)
		}
	})
	t.Run("payload", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `not json`)
		}))
		t.Cleanup(server.Close)
		core := modelCheckCore(t, model.Chat, server.URL)
		if err := core.Execute(context.Background(), contracts.Command{Kind: "provider.models.fetch", ID: "checks"}); err != nil {
			t.Fatal(err)
		}
		state := waitSubagentState(t, core, func(state contracts.Snapshot) bool {
			fetch := state.CustomProviders[0].ModelFetch
			return fetch != nil && fetch.State == "error"
		})
		if state.CustomProviders[0].ModelFetch.Error != "The provider returned an invalid model list." {
			t.Fatalf("unexpected payload error: %q", state.CustomProviders[0].ModelFetch.Error)
		}
	})
	if strings.Contains(modelFetchError(fmt.Errorf("provider echoed fetch-secret")), "fetch-secret") {
		t.Fatal("raw untrusted provider error reached UI")
	}
	if modelFetchError(context.DeadlineExceeded) != "Model list request timed out." {
		t.Fatal("timeout category lost")
	}
}

func TestModelFetchRejectsUnknownAndDisabledProviders(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
	}))
	t.Cleanup(server.Close)
	core := modelCheckCore(t, model.Chat, server.URL)
	if err := core.Execute(context.Background(), contracts.Command{Kind: "provider.models.fetch", ID: "missing"}); err == nil {
		t.Fatal("unknown provider fetch was accepted")
	}
	config := modelCheckConfig(model.Chat, server.URL)
	config.Disabled = true
	saveModelCheckConfig(t, core, config)
	if err := core.Execute(context.Background(), contracts.Command{Kind: "provider.models.fetch", ID: "checks"}); err == nil {
		t.Fatal("disabled provider fetch was accepted")
	}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "provider.models.fetch", ID: "checks"}); err == nil {
		t.Fatal("fetch without provider ID was accepted")
	}
	if requests.Load() != 0 {
		t.Fatal("rejected fetch still called the provider")
	}
}

func TestModelFetchConfigurationChangesCancelAndDiscardResults(t *testing.T) {
	for _, change := range []string{"save", "delete"} {
		t.Run(change, func(t *testing.T) {
			started, canceled := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(started)
				<-r.Context().Done()
				close(canceled)
			}))
			t.Cleanup(server.Close)
			core := modelCheckCore(t, model.Chat, server.URL)
			if err := core.Execute(context.Background(), contracts.Command{Kind: "provider.models.fetch", ID: "checks"}); err != nil {
				t.Fatal(err)
			}
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("fetch request did not start")
			}
			if err := core.Execute(context.Background(), contracts.Command{Kind: "provider.models.fetch", ID: "checks"}); err == nil {
				t.Fatal("duplicate simultaneous fetch was accepted")
			}
			if change == "save" {
				config := modelCheckConfig(model.Chat, server.URL)
				config.Name = "Changed provider"
				saveModelCheckConfig(t, core, config)
			} else if err := core.Execute(context.Background(), contracts.Command{Kind: "provider.delete", ID: "checks"}); err != nil {
				t.Fatal(err)
			}
			select {
			case <-canceled:
			case <-time.After(5 * time.Second):
				t.Fatal("configuration change did not cancel in-flight fetch")
			}
			core.wg.Wait()
			state, _ := core.Snapshot(context.Background())
			for _, provider := range state.CustomProviders {
				if provider.ModelFetch != nil {
					t.Fatalf("old configuration fetch overwrote new configuration: %+v", provider.ModelFetch)
				}
			}
		})
	}
}
