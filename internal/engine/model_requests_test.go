package engine

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/storage"
)

func TestModelRequestsAccountRetriesToolsModelsAndRestart(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		switch calls.Add(1) {
		case 1:
			// Reported usage remains billable when this attempt is interrupted.
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"discarded attempt\"}}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":2}}\n\n")
		case 2:
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"read\",\"function\":{\"name\":\"Read\",\"arguments\":\"{\\\"file_path\\\":\\\"missing.txt\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":3}}\n\ndata: [DONE]\n\n")
		default:
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"finished\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":20,\"completion_tokens\":5,\"prompt_tokens_details\":{\"cached_tokens\":8}}}\n\ndata: [DONE]\n\n")
		}
	}))
	defer server.Close()
	directory := t.TempDir()
	open := func() *Engine {
		store, err := storage.Open(storage.Options{Directory: directory})
		if err != nil {
			t.Fatal(err)
		}
		core, err := openExecutionFixture(t, Options{Storage: store})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = core.Close() })
		return core
	}
	core := open()
	ctx := t.Context()
	for _, provider := range []string{"A", "B"} {
		if err := core.Execute(ctx, contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: model.Provider{
			ID: provider, Name: "Frozen " + provider, Protocol: model.Chat, BaseURL: server.URL, Models: []model.Model{{ID: "family/model"}},
		}}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := core.Execute(ctx, contracts.Command{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: t.TempDir(), Name: "Metering"}}); err != nil {
		t.Fatal(err)
	}
	state, _ := core.Snapshot(ctx)
	var sessionID string
	for _, provider := range []string{"A", "B"} {
		if err := core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
			ProjectID: state.Projects[0].ID, SessionID: sessionID, ModelID: provider + "/family/model", Text: "meter this turn",
		}}); err != nil {
			t.Fatal(err)
		}
		state = waitIdle(t, core)
		if state.Runs[len(state.Runs)-1].Status != "completed" {
			t.Fatalf("actual request failed: %+v", state.Runs)
		}
		sessionID = state.Sessions[0].ID
	}
	if len(state.ModelRequests) != 4 {
		t.Fatalf("lost retry or tool round: %+v", state.ModelRequests)
	}
	var total contracts.ModelRequestTotals
	for i, request := range state.ModelRequests {
		total.Add(request)
		want := "A"
		if i == 3 {
			want = "B"
		}
		if request.SessionID != sessionID || request.TurnID == "" || request.ProviderID != want || request.ProviderName != "Frozen "+want ||
			request.ModelID != "family/model" || !request.Usage.Known || request.DecodeDuration < 0 {
			t.Fatalf("wrong request ownership or identity: %+v", request)
		}
		if request.RequestDuration <= 0 || !request.FirstTokenObserved || request.FirstTokenLatency < 0 {
			t.Fatalf("actual model timing was not recorded: %+v", request)
		}
	}
	if total.Input+total.Output != 66 || total.Cached != 16 || total.UnknownUsage != 0 {
		t.Fatalf("actual attempts total=%+v", total)
	}
	if fraction, known := total.CacheHitFraction(); !known || fraction != .4 || total.CacheRequests != 2 {
		t.Fatalf("requests without cache fields diluted the persisted cache hit rate: %+v", total)
	}
	firstTurn := state.ModelRequests[0].TurnID
	if state.ModelRequests[1].TurnID != firstTurn || state.ModelRequests[2].TurnID != firstTurn || state.ModelRequests[3].TurnID == firstTurn {
		t.Fatal("tool steps/retries were not attached to the correct reply")
	}
	if err := core.Execute(ctx, contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: model.Provider{
		ID: "A", Name: "Renamed later", Protocol: model.Chat, BaseURL: server.URL, Models: []model.Model{{ID: "family/model"}},
	}}}); err != nil {
		t.Fatal(err)
	}
	// A fork inherits text, never the original conversation's billable requests.
	if err := core.Execute(ctx, contracts.Command{Kind: "session.fork", SessionID: sessionID}); err != nil {
		t.Fatal(err)
	}
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	restored := open()
	reloaded, err := restored.Snapshot(ctx)
	if err != nil || !reflect.DeepEqual(reloaded.ModelRequests, state.ModelRequests) {
		t.Fatalf("accounting changed on fork or restart: %v %+v", err, reloaded.ModelRequests)
	}
	reloaded.ModelRequests[0].Usage.Input = 99999
	fresh, _ := restored.Snapshot(ctx)
	if fresh.ModelRequests[0].Usage.Input != 1 {
		t.Fatal("consumer mutated engine accounting")
	}
}

type metricsFailureModel struct{}

func (metricsFailureModel) Stream(ctx context.Context, _ model.Request, _ func(model.Delta) error) (model.Result, error) {
	return model.Result{
		Usage:          model.Usage{OutputKnown: true, Output: 7},
		DecodeDuration: time.Second,
	}, context.Canceled
}

func TestModelRequestsRetainPartialUsageOnCancellation(t *testing.T) {
	store, err := storage.Open(storage.Options{Directory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	core, err := openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create", Name: "Partial"}); err != nil {
		t.Fatal(err)
	}
	state, _ := core.Snapshot(t.Context())
	meter := meteredModel{inner: metricsFailureModel{}, core: core, sessionID: state.Sessions[0].ID,
		turnID: "cancelled", provider: model.Provider{ID: "P", Name: "Original"}, modelID: "M"}
	_, _ = meter.Stream(t.Context(), model.Request{}, nil)
	state, _ = core.Snapshot(t.Context())
	if len(state.ModelRequests) != 1 || state.ModelRequests[0].Usage.Known || state.ModelRequests[0].Usage.Output != 7 ||
		state.ModelRequests[0].ProviderName != "Original" || state.ModelRequests[0].DecodeDuration != time.Second {
		t.Fatalf("cancellation lost partial request accounting: %+v", state.ModelRequests)
	}
}
