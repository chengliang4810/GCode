package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/modelcatalog"
	"gcode/internal/storage"
)

// modelCatalogPayload is the smallest catalog a refresh accepts.
const modelCatalogPayload = `{"test":{"api":"https://example.com/v1","models":{"test-model":{"id":"test-model","limit":{"context":1000,"output":100}}}}}`

// modelCatalogServer serves the upstream catalog. Every response waits for a token,
// so a test can hold a refresh in flight and observe the loading state.
type modelCatalogServer struct {
	server *httptest.Server
	tokens chan struct{}
	fail   atomic.Bool
	calls  atomic.Int32
}

func newModelCatalogServer(t *testing.T) *modelCatalogServer {
	t.Helper()
	c := &modelCatalogServer{tokens: make(chan struct{}, 8)}
	c.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.calls.Add(1)
		select {
		case <-c.tokens:
		case <-r.Context().Done():
			return
		}
		if c.fail.Swap(false) {
			http.Error(w, "upstream unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(modelCatalogPayload))
	}))
	t.Cleanup(c.server.Close)
	return c
}

// arm releases the next response. Without a token the refresh stays in flight.
func (c *modelCatalogServer) arm() { c.tokens <- struct{}{} }

// failNext makes the next released response fail.
func (c *modelCatalogServer) failNext() { c.fail.Store(true) }

// modelCatalogFixture opens an engine whose remote catalog is due for a check, so the
// host starts its first refresh immediately.
func modelCatalogFixture(t *testing.T, upstream *modelCatalogServer) *Engine {
	t.Helper()
	store, err := storage.Open(storage.Options{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	// A stored check time from the past makes the remote catalog due.
	if err := storage.WriteJSON(filepath.Join(directory, "models-dev-state.json"), map[string]any{"checkedAt": time.Now().Add(-24 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	cache, err := modelcatalog.Open(modelcatalog.Options{Directory: directory, URL: upstream.server.URL, Client: upstream.server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if !cache.NeedsRefresh() {
		t.Fatal("an expired catalog must be due for its first remote check")
	}
	core, err := Open(Options{Storage: store, Catalog: cache})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = core.Close() })
	// One configured model keeps the settled state "ready" instead of "empty".
	provider := &contracts.ProviderInput{Provider: model.Provider{
		ID: "test", Name: "Test", Protocol: model.Chat, BaseURL: "https://example.com/v1",
		Models: []model.Model{{ID: "test-model", ContextWindow: 1000, MaxOutput: 100}},
	}}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "provider.save", Provider: provider}); err != nil {
		t.Fatal(err)
	}
	return core
}

func modelCatalogHostState(t *testing.T, core *Engine) contracts.ModelCatalogState {
	t.Helper()
	snapshot, err := core.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return snapshot.ModelCatalog
}

// waitForModelCatalogState polls the host-visible state until want appears.
func waitForModelCatalogState(t *testing.T, core *Engine, want contracts.ModelCatalogStatus) contracts.ModelCatalogState {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var state contracts.ModelCatalogState
	for time.Now().Before(deadline) {
		state = modelCatalogHostState(t, core)
		if state.State == want {
			return state
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("catalog state stayed %+v, want %s", state, want)
	return state
}

func TestModelCatalogReportsLoadingBeforeFirstRefresh(t *testing.T) {
	upstream := newModelCatalogServer(t)
	core := modelCatalogFixture(t, upstream)
	if state := modelCatalogHostState(t, core); state.State != contracts.CatalogLoading {
		t.Fatalf("first snapshot catalog state = %q, want loading", state.State)
	}
	events, err := core.Subscribe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	upstream.arm()
	settled := waitForModelCatalogState(t, core, contracts.CatalogReady)
	if settled.Error != "" {
		t.Fatalf("a settled catalog must not keep an error: %+v", settled)
	}
	if upstream.calls.Load() == 0 {
		t.Fatal("the catalog settled without contacting the upstream")
	}
	// The settled edge reaches subscribers as a published snapshot event.
	deadline := time.After(10 * time.Second)
	for {
		select {
		case event := <-events:
			if event.Snapshot != nil && event.Snapshot.ModelCatalog.State == contracts.CatalogReady {
				return
			}
		case <-deadline:
			t.Fatal("the settled catalog state was not published")
		}
	}
}

func TestModelCatalogReportsRefreshFailureUntilFirstSuccess(t *testing.T) {
	upstream := newModelCatalogServer(t)
	core := modelCatalogFixture(t, upstream)
	upstream.failNext()
	upstream.arm()
	failed := waitForModelCatalogState(t, core, contracts.CatalogError)
	if failed.Error == "" {
		t.Fatal("a failed refresh must explain itself")
	}
}

func TestModelCatalogKeepsLastStateAfterLaterFailure(t *testing.T) {
	upstream := newModelCatalogServer(t)
	core := modelCatalogFixture(t, upstream)
	upstream.arm()
	waitForModelCatalogState(t, core, contracts.CatalogReady)
	// A later attempt fails: the last usable state stays visible instead of
	// falling back to an error the user cannot act on.
	upstream.failNext()
	upstream.arm()
	if err := core.Execute(context.Background(), contracts.Command{Kind: "catalog.retry"}); err != nil {
		t.Fatal(err)
	}
	waitForModelCatalogState(t, core, contracts.CatalogReady)
}

func TestModelCatalogRetryReportsLoadingFirst(t *testing.T) {
	upstream := newModelCatalogServer(t)
	core := modelCatalogFixture(t, upstream)
	upstream.arm()
	waitForModelCatalogState(t, core, contracts.CatalogReady)
	if err := core.Execute(context.Background(), contracts.Command{Kind: "catalog.retry"}); err != nil {
		t.Fatal(err)
	}
	if state := modelCatalogHostState(t, core); state.State != contracts.CatalogLoading {
		t.Fatalf("retry did not enter loading first: %+v", state)
	}
	upstream.arm()
	waitForModelCatalogState(t, core, contracts.CatalogReady)
}
