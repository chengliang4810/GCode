package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/storage"
)

func TestProviderDraftPersistsAndBecomesRunnableAcrossProtocols(t *testing.T) {
	for _, protocol := range []model.Protocol{model.Chat, model.Responses, model.Messages} {
		t.Run(string(protocol), func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if body := subagentRequest(t, r); body["model"] != "selected" {
					t.Errorf("request model = %v, want selected", body["model"])
				}
				goalResponse(w, protocol, "OK", nil, true)
			}))
			t.Cleanup(server.Close)
			home := t.TempDir()
			openCore := func() *Engine {
				store, err := storage.Open(storage.Options{Home: home})
				if err != nil {
					t.Fatal(err)
				}
				core, err := openExecutionFixture(t, Options{Storage: store})
				if err != nil {
					_ = store.Close()
					t.Fatal(err)
				}
				return core
			}
			core := openCore()
			t.Cleanup(func() { _ = core.Close() })
			ctx := context.Background()
			draft := model.Provider{ID: "draft", Name: "New provider", Protocol: protocol}
			saveModelCheckConfig(t, core, draft)
			assertProviderDraft(t, core, true)
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}
			core = openCore()
			assertProviderDraft(t, core, true)

			// Models can be maintained before the endpoint is filled in. Neither
			// direct execution nor a diagnostic may bypass the incomplete state.
			draft.Models = []model.Model{{ID: "selected"}}
			saveModelCheckConfig(t, core, draft)
			assertProviderDraft(t, core, true)
			if err := core.Execute(ctx, contracts.Command{Kind: "provider.test", ID: "draft/selected"}); err == nil {
				t.Fatal("endpoint-free model diagnostic was accepted")
			}
			if err := core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
				Text: "must not run", ModelID: "draft/selected",
			}}); err == nil {
				t.Fatal("endpoint-free model execution was accepted")
			}
			state, _ := core.Snapshot(ctx)
			if requests.Load() != 0 || len(state.Sessions) != 0 || len(state.Runs) != 0 {
				t.Fatal("draft caused execution or created conversation state")
			}

			draft.BaseURL = server.URL + "/v1"
			saveModelCheckConfig(t, core, draft)
			assertProviderDraft(t, core, false)
			if err := core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
				Text: "run after configuration", ModelID: "draft/selected",
			}}); err != nil {
				t.Fatal(err)
			}
			state = waitIdle(t, core)
			if requests.Load() != 1 || state.Sessions[0].LastOutcome != "completed" {
				t.Fatalf("completed provider failed execution: requests=%d, runs=%+v", requests.Load(), state.Runs)
			}

			// Deleting the final model keeps the named provider as a draft.
			draft.Models = nil
			saveModelCheckConfig(t, core, draft)
			assertProviderDraft(t, core, true)
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}
			core = openCore()
			assertProviderDraft(t, core, true)
			if requests.Load() != 1 {
				t.Fatal("saving or reopening a draft made an unsolicited request")
			}
		})
	}
}

func assertProviderDraft(t *testing.T, core *Engine, incomplete bool) {
	t.Helper()
	state, err := core.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(state.CustomProviders) != 1 || state.CustomProviders[0].Provider.ID != "draft" {
		t.Fatalf("provider draft disappeared: %+v", state.CustomProviders)
	}
	status := "ready"
	modelCount := 1
	if incomplete {
		status, modelCount = "incomplete", 0
	}
	if len(state.Providers) != 1 || state.Providers[0].Status != status ||
		len(state.Models) != modelCount || len(state.Providers[0].Models) != modelCount {
		t.Fatalf("incorrect draft catalog: providers=%+v models=%+v", state.Providers, state.Models)
	}
	if incomplete {
		if _, _, err := core.resolveModel("draft/selected"); err == nil {
			t.Fatal("unfinished provider resolved to a runnable model")
		}
	}
}

func TestIncompleteProviderSelectionDoesNotFallBackToReadyProvider(t *testing.T) {
	providers := []model.Provider{
		{ID: "ready", Name: "Ready", Protocol: model.Chat, BaseURL: "https://example.test/v1", Models: []model.Model{{ID: "selected"}}},
		{ID: "draft", Name: "Draft", Protocol: model.Chat, Models: []model.Model{{ID: "selected"}}},
	}
	if _, _, err := resolveConfiguredModel(providers, "draft/selected"); err == nil {
		t.Fatal("unfinished selection fell back to another provider")
	}
	provider, selected, err := resolveConfiguredModel(providers, "ready/selected")
	if err != nil || provider.ID != "ready" || selected.ID != "selected" {
		t.Fatalf("ready selection unavailable: %+v %+v %v", provider, selected, err)
	}
}
