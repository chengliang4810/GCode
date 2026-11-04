package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/storage"
)

func TestModelCheckUsesOnlySelectedModelAcrossProtocols(t *testing.T) {
	for _, protocol := range []model.Protocol{model.Chat, model.Responses, model.Messages} {
		t.Run(string(protocol), func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				body := subagentRequest(t, r)
				if body["model"] != "selected/model" {
					t.Errorf("tested model = %v", body["model"])
				}
				if tools, ok := body["tools"].([]any); ok && len(tools) != 0 {
					t.Error("connection test exposed execution tools")
				}
				limitKey, path := "max_completion_tokens", "/chat/completions"
				switch protocol {
				case model.Responses:
					limitKey, path = "max_output_tokens", "/responses"
				case model.Messages:
					limitKey, path = "max_tokens", "/messages"
				}
				if body[limitKey] != float64(64) || r.URL.Path != "/v1"+path {
					t.Errorf("diagnostic request was not bounded or used wrong path: %+v %s", body, r.URL.Path)
				}
				if (protocol == model.Messages && r.Header.Get("x-api-key") != "check-secret") ||
					(protocol != model.Messages && r.Header.Get("Authorization") != "Bearer check-secret") {
					t.Error("test did not use selected provider credentials")
				}
				goalResponse(w, protocol, "OK", nil, true)
			}))
			t.Cleanup(server.Close)
			core := modelCheckCore(t, protocol, server.URL)
			if requests.Load() != 0 {
				t.Fatal("saving a model made an unsolicited request")
			}
			if err := core.Execute(context.Background(), contracts.Command{Kind: "provider.test", ID: "checks/selected/model"}); err != nil {
				t.Fatal(err)
			}
			state := waitSubagentState(t, core, func(state contracts.Snapshot) bool {
				return len(state.CustomProviders[0].ModelChecks) == 1 &&
					state.CustomProviders[0].ModelChecks[0].State != "testing"
			})
			check := state.CustomProviders[0].ModelChecks[0]
			if requests.Load() != 1 || check.State != "ready" || check.FinishedAt == nil ||
				check.Usage == nil || !check.Usage.Known || check.Usage.Input != 5 || check.Usage.Output != 3 {
				t.Fatalf("missing real protocol test result: %+v, requests=%d", check, requests.Load())
			}
			if len(state.Sessions) != 0 || len(state.Runs) != 0 || len(state.Queue) != 0 {
				t.Fatal("model diagnostic created Agent execution state")
			}
			check.Usage.Input = 900
			*check.FinishedAt = time.Time{}
			current, _ := core.Snapshot(context.Background())
			if current.CustomProviders[0].ModelChecks[0].Usage.Input != 5 ||
				current.CustomProviders[0].ModelChecks[0].FinishedAt.IsZero() {
				t.Fatal("model check snapshot shares mutable result pointers")
			}
		})
	}
}

func TestModelCheckRejectsDisabledEntriesWithoutCallingProvider(t *testing.T) {
	for _, kind := range []string{"provider", "model"} {
		t.Run(kind, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				goalResponse(w, model.Chat, "OK", nil, true)
			}))
			t.Cleanup(server.Close)
			core := modelCheckCore(t, model.Chat, server.URL)
			config := modelCheckConfig(model.Chat, server.URL)
			if kind == "provider" {
				config.Disabled = true
			} else {
				config.Models[1].Disabled = true
			}
			saveModelCheckConfig(t, core, config)
			if err := core.Execute(context.Background(), contracts.Command{Kind: "provider.test", ID: "checks/selected/model"}); err == nil {
				t.Fatal("disabled model test was accepted")
			}
			state, _ := core.Snapshot(context.Background())
			for _, entry := range state.Models {
				if entry.ID == "checks/selected/model" {
					t.Fatal("disabled model remained in execution catalog")
				}
			}
			if requests.Load() != 0 {
				t.Fatal("disabled model received a request")
			}
		})
	}
}

func TestModelCheckConfigurationChangesCancelAndDiscardResults(t *testing.T) {
	for _, change := range []string{"save", "delete"} {
		t.Run(change, func(t *testing.T) {
			started, canceled := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = subagentRequest(t, r)
				close(started)
				<-r.Context().Done()
				close(canceled)
			}))
			t.Cleanup(server.Close)
			core := modelCheckCore(t, model.Chat, server.URL)
			if err := core.Execute(context.Background(), contracts.Command{Kind: "provider.test", ID: "checks/selected/model"}); err != nil {
				t.Fatal(err)
			}
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("test request did not start")
			}
			if err := core.Execute(context.Background(), contracts.Command{Kind: "provider.test", ID: "checks/selected/model"}); err == nil {
				t.Fatal("duplicate simultaneous test was accepted")
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
				t.Fatal("configuration change did not cancel in-flight request")
			}
			core.wg.Wait()
			state, _ := core.Snapshot(context.Background())
			for _, provider := range state.CustomProviders {
				if len(provider.ModelChecks) != 0 {
					t.Fatalf("old configuration test overwrote new configuration: %+v", provider.ModelChecks)
				}
			}
		})
	}
}

func TestModelCheckShutdownCancelsAndJoinsRequest(t *testing.T) {
	started, canceled := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = subagentRequest(t, r)
		close(started)
		<-r.Context().Done()
		close(canceled)
	}))
	t.Cleanup(server.Close)
	core := modelCheckCore(t, model.Chat, server.URL)
	if err := core.Execute(context.Background(), contracts.Command{Kind: "provider.test", ID: "checks/selected/model"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("test request did not start")
	}
	done := make(chan error, 1)
	go func() { done <- core.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("engine shutdown did not join model test")
	}
	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("HTTP request survived engine shutdown")
	}
}

func TestModelCheckErrorsDoNotPublishCredentials(t *testing.T) {
	for _, failure := range []string{"http", "stream"} {
		t.Run(failure, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if failure == "http" {
					w.WriteHeader(http.StatusUnauthorized)
				}
				fmt.Fprint(w, `data: {"error":{"message":"check-secret","code":"upstream_capacity","request_id":"req-42"}}`+"\n\n")
			}))
			t.Cleanup(server.Close)
			core := modelCheckCore(t, model.Chat, server.URL)
			if err := core.Execute(context.Background(), contracts.Command{Kind: "provider.test", ID: "checks/selected/model"}); err != nil {
				t.Fatal(err)
			}
			state := waitSubagentState(t, core, func(state contracts.Snapshot) bool {
				return len(state.CustomProviders[0].ModelChecks) == 1 &&
					state.CustomProviders[0].ModelChecks[0].State == "error"
			})
			encoded, _ := json.Marshal(state)
			if strings.Contains(string(encoded), "check-secret") {
				t.Fatal("model test snapshot leaked provider credentials")
			}
			if failure == "http" && !strings.Contains(state.CustomProviders[0].ModelChecks[0].Error, "401") {
				t.Fatal("HTTP diagnostic omitted its safe status")
			}
			if detail := state.CustomProviders[0].ModelChecks[0].Error; !strings.Contains(detail, `"code":"upstream_capacity"`) ||
				!strings.Contains(detail, `"request_id":"req-42"`) || !strings.Contains(detail, "[redacted]") {
				t.Fatalf("model test discarded original provider details: %s", detail)
			}
		})
	}
	if strings.Contains(modelCheckError(errors.New("credential check-secret echoed by provider")), "check-secret") {
		t.Fatal("raw untrusted model error reached UI")
	}
	if modelCheckError(context.DeadlineExceeded) != "Model test timed out." {
		t.Fatal("timeout category lost")
	}
}

func modelCheckConfig(protocol model.Protocol, baseURL string) model.Provider {
	return model.Provider{
		ID: "checks", Name: "Check fixture", Protocol: protocol, BaseURL: baseURL,
		APIKey: "check-secret",
		Models: []model.Model{
			{ID: "unselected"},
			{ID: "selected/model", MaxOutput: 8192, ReasoningLevels: []string{"none", "max"}},
		},
	}
}

func saveModelCheckConfig(t *testing.T, core *Engine, config model.Provider) {
	t.Helper()
	if err := core.Execute(context.Background(), contracts.Command{
		Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: config},
	}); err != nil {
		t.Fatal(err)
	}
}

func modelCheckCore(t *testing.T, protocol model.Protocol, baseURL string) *Engine {
	t.Helper()
	store, err := storage.Open(storage.Options{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	core, err := openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = core.Close() })
	saveModelCheckConfig(t, core, modelCheckConfig(protocol, baseURL))
	return core
}
