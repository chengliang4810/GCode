package modelcatalog

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"gcode/internal/storage"
)

const fixture = `{"alpha":{"api":"https://alpha.example/v1","models":{"shared":{"id":"shared","limit":{"context":32000,"output":4000},"modalities":{"input":["text","image"]},"reasoning_options":[{"type":"effort","values":["high","low"]}]}}},"beta":{"api":"https://beta.example/v1","models":{"shared":{"id":"shared","limit":{"context":64000,"output":8000},"modalities":{"input":["text"]}}}}}`

func fixtureCache(t *testing.T, source string) *Cache {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "models-dev.json"), []byte(fixture), 0600); err != nil {
		t.Fatal(err)
	}
	cache, err := Open(Options{Directory: dir, URL: source})
	if err != nil {
		t.Fatal(err)
	}
	return cache
}

func TestLookupUsesLocalFileAndSelectsStableFirstMatch(t *testing.T) {
	var requests atomic.Int32
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "offline", http.StatusServiceUnavailable)
	}))
	defer host.Close()
	cache := fixtureCache(t, host.URL)
	for _, test := range []struct {
		name, id, base string
		found          bool
		context        int
	}{
		{"provider endpoint", "shared", "https://alpha.example/custom/v1", true, 32000},
		{"qualified id", "beta/shared", "", true, 64000},
		{"multiple matches choose first", "shared", "", true, 32000},
		{"unknown", "not-a-model", "", false, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := cache.Lookup(context.Background(), Query{ID: test.id, BaseURL: test.base})
			if err != nil || result.Found != test.found || result.ContextWindow != test.context {
				t.Fatalf("lookup=%+v error=%v", result, err)
			}
		})
	}
	for range 20 {
		result, err := cache.Lookup(context.Background(), Query{ID: "shared"})
		if err != nil || result.Provider != "alpha" || result.ContextWindow != 32000 {
			t.Fatalf("first candidate changed across lookups: %+v %v", result, err)
		}
	}
	if requests.Load() != 0 {
		t.Fatal("local lookup made an external request")
	}
}

func TestRefreshKeepsGoodFileAndPersistsThreeHourSchedule(t *testing.T) {
	var status atomic.Int32
	status.Store(http.StatusOK)
	var requests atomic.Int32
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if status.Load() == http.StatusNotModified {
			if r.Header.Get("If-None-Match") != `"catalog-1"` {
				t.Error("conditional validator was not reused")
			}
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"catalog-1"`)
		w.WriteHeader(int(status.Load()))
		if status.Load() == http.StatusOK {
			_, _ = w.Write([]byte(fixture))
		}
	}))
	defer host.Close()
	cache := fixtureCache(t, host.URL)
	for _, test := range []struct {
		name   string
		status int
		fail   bool
	}{
		{"valid response", 200, false},
		{"HTTP failure", 503, true},
		{"not modified", 304, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			status.Store(int32(test.status))
			before, _ := os.ReadFile(cache.path)
			err := cache.Refresh(context.Background())
			if (err != nil) != test.fail {
				t.Fatalf("refresh error=%v", err)
			}
			after, readErr := os.ReadFile(cache.path)
			if readErr != nil || !json.Valid(after) || test.fail && string(before) != string(after) {
				t.Fatal("failed refresh damaged the local file")
			}
			reopened, err := Open(Options{Directory: filepath.Dir(cache.path), URL: host.URL})
			if err != nil || time.Until(reopened.state.CheckedAt.Add(RefreshInterval)) < RefreshInterval-time.Minute {
				t.Fatalf("three-hour schedule lost on restart: %v", err)
			}
		})
	}
	if requests.Load() != 3 {
		t.Fatalf("unexpected refresh requests=%d", requests.Load())
	}
}

func TestRefreshRejectsInvalidJSONWithoutReplacingFile(t *testing.T) {
	for _, body := range []string{`{"broken"`, `{}`, `{"p":{"models":{"m":{"id":"m","limit":{"context":-1}}}}}`} {
		t.Run(body, func(t *testing.T) {
			host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer host.Close()
			cache := fixtureCache(t, host.URL)
			before, _ := os.ReadFile(cache.path)
			if err := cache.Refresh(context.Background()); err == nil {
				t.Fatal("invalid catalog accepted")
			}
			after, _ := os.ReadFile(cache.path)
			if string(before) != string(after) {
				t.Fatal("invalid response replaced good data")
			}
		})
	}
}

func TestLookupDoesNotWaitForNetworkAndRefreshCancels(t *testing.T) {
	started := make(chan struct{})
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer host.Close()
	cache := fixtureCache(t, host.URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- cache.Refresh(ctx) }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("refresh did not start")
	}
	lookupDone := make(chan error, 1)
	go func() {
		_, err := cache.Lookup(context.Background(), Query{ID: "alpha/shared"})
		lookupDone <- err
	}()
	select {
	case err := <-lookupDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("local lookup waited for the network")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled refresh succeeded")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("refresh did not stop")
	}
}

func TestBundledCatalogWorksOffline(t *testing.T) {
	dir := t.TempDir()
	cache, err := Open(Options{Directory: dir, URL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := cache.Lookup(context.Background(), Query{ID: "deepseek-v4-pro", BaseURL: "https://api.deepseek.com/v1"})
	if err != nil || !result.Found || result.ContextWindow <= 0 {
		t.Fatalf("offline seed lookup=%+v error=%v", result, err)
	}
	content, err := os.ReadFile(filepath.Join(dir, "models-dev.json"))
	if err != nil || !strings.Contains(string(content), "deepseek-v4-pro") {
		t.Fatal("bundled JSON was not cached")
	}
	if err := storage.WriteJSON(cache.statePath(), cacheState{CheckedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() { cache.Run(ctx); close(stopped) }()
	stop()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("idle refresh loop did not stop")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRefreshLoopRequestsOnceEveryThreeHours(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := fixtureCache(t, "https://catalog.example/api.json")
		cache.state.CheckedAt = time.Now()
		var requests atomic.Int32
		cache.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			requests.Add(1)
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(fixture))}, nil
		})}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go cache.Run(ctx)
		synctest.Wait()
		time.Sleep(RefreshInterval - time.Nanosecond)
		if requests.Load() != 0 {
			t.Fatal("refreshed before the three-hour deadline")
		}
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		if requests.Load() != 1 {
			t.Fatalf("first deadline: requests=%d", requests.Load())
		}
		for range 10 {
			if _, err := cache.Lookup(ctx, Query{ID: "alpha/shared"}); err != nil {
				t.Fatal(err)
			}
		}
		if requests.Load() != 1 {
			t.Fatal("lookups triggered extra refreshes")
		}
		time.Sleep(RefreshInterval)
		synctest.Wait()
		if requests.Load() != 2 {
			t.Fatalf("second deadline: requests=%d", requests.Load())
		}
		cancel()
		synctest.Wait()
	})
}
