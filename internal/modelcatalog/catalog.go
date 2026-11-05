// Package modelcatalog maintains the host-owned models.dev file cache.
package modelcatalog

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"gcode/internal/storage"
)

const Source = "https://models.dev/api.json"
const RefreshInterval = 3 * time.Hour
const maxBytes = 24 << 20

//go:embed models-dev.json
var seed []byte

type Query struct {
	ID      string `json:"id"`
	BaseURL string `json:"baseUrl"`
}

type Metadata struct {
	Found           bool     `json:"found"`
	Provider        string   `json:"provider,omitempty"`
	ContextWindow   int      `json:"contextWindow"`
	MaxOutput       int      `json:"maxOutput"`
	InputTypes      []string `json:"inputTypes"`
	ReasoningLevels []string `json:"reasoningLevels"`
}

type provider struct {
	API    string           `json:"api"`
	Models map[string]entry `json:"models"`
}

type entry struct {
	ID        string `json:"id"`
	Canonical string `json:"canonical_model_id"`
	Limit     struct {
		Context int `json:"context"`
		Output  int `json:"output"`
	} `json:"limit"`
	Modalities struct {
		Input []string `json:"input"`
	} `json:"modalities"`
	ReasoningOptions []struct {
		Type   string   `json:"type"`
		Values []string `json:"values"`
	} `json:"reasoning_options"`
}

type cacheState struct {
	CheckedAt    time.Time `json:"checkedAt"`
	ETag         string    `json:"etag,omitempty"`
	LastModified string    `json:"lastModified,omitempty"`
}

type Options struct {
	Directory string
	URL       string
	Client    *http.Client
}

type Cache struct {
	mu      sync.RWMutex
	refresh sync.Mutex
	path    string
	state   cacheState
	data    map[string]provider
	source  string
	client  *http.Client
}

// Open reads only local files. The bundled JSON seeds the first offline launch.
func Open(options Options) (*Cache, error) {
	if options.Directory == "" {
		return nil, errors.New("catalog cache directory is required")
	}
	c := &Cache{path: filepath.Join(options.Directory, "models-dev.json"), source: options.URL, client: options.Client}
	if c.source == "" {
		c.source = Source
	}
	if c.client == nil {
		c.client = &http.Client{Timeout: 30 * time.Second}
	}
	payload, err := os.ReadFile(c.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	seeded := len(payload) == 0
	recovered := seeded && err == nil
	if len(payload) == 0 {
		if recovered {
			slog.Warn("model catalog cache is empty; restoring bundled catalog", "path", c.path)
		}
		payload = seed
		if err := storage.WriteJSON(c.path, json.RawMessage(payload)); err != nil {
			return nil, err
		}
	}
	c.data, err = decode(payload)
	if err != nil {
		slog.Warn("model catalog cache is damaged; restoring bundled catalog", "path", c.path, "error", err)
		c.data, err = decode(seed)
		if err != nil {
			return nil, fmt.Errorf("read bundled model catalog: %w", err)
		}
		if err := storage.WriteJSON(c.path, json.RawMessage(seed)); err != nil {
			return nil, fmt.Errorf("restore model catalog cache: %w", err)
		}
		recovered = true
	}
	info, err := os.Stat(c.path)
	if err != nil {
		return nil, err
	}
	c.state.CheckedAt = info.ModTime()
	payload, err = os.ReadFile(c.statePath())
	if err == nil {
		if seeded {
			// A missing/empty catalog cannot use another file's validators.
			recovered = true
		}
		if err := json.Unmarshal(payload, &c.state); err != nil {
			slog.Warn("model catalog refresh state is damaged; scheduling a fresh download", "path", c.statePath(), "error", err)
			recovered = true
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if recovered {
		// Validators from a different catalog must not produce a 304 against
		// the bundled seed. A zero check time also retries promptly when online.
		c.state = cacheState{}
		if err := storage.WriteJSON(c.statePath(), c.state); err != nil {
			return nil, fmt.Errorf("restore model catalog refresh state: %w", err)
		}
	}
	return c, nil
}

func decode(payload []byte) (map[string]provider, error) {
	var data map[string]provider
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, err
	}
	count := 0
	for _, p := range data {
		for _, m := range p.Models {
			if m.ID == "" || m.Limit.Context < 0 || m.Limit.Output < 0 {
				return nil, errors.New("invalid model catalog entry")
			}
			count++
		}
	}
	if count == 0 {
		return nil, errors.New("model catalog is empty")
	}
	return data, nil
}

func (c *Cache) statePath() string {
	return filepath.Join(filepath.Dir(c.path), "models-dev-state.json")
}

// Run owns one refresh loop. Lookups never cause HTTP requests.
func (c *Cache) Run(ctx context.Context) {
	for {
		c.mu.Lock()
		wait := time.Until(c.state.CheckedAt.Add(RefreshInterval))
		c.mu.Unlock()
		timer := time.NewTimer(max(time.Duration(0), wait))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			// Refresh records failures as well, so an outage never creates a busy loop.
			if err := c.Refresh(ctx); err != nil && ctx.Err() == nil {
				slog.Warn("model catalog refresh failed; retaining local cache", "error", err)
			}
		}
	}
}

// Refresh validates an entire response before atomically replacing the local file.
// Conditional requests avoid rewriting unchanged files; errors retain the last good JSON.
func (c *Cache) Refresh(ctx context.Context) (resultErr error) {
	c.refresh.Lock()
	defer c.refresh.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.RLock()
	state := c.state
	c.mu.RUnlock()
	defer func() {
		state.CheckedAt = time.Now()
		resultErr = errors.Join(resultErr, storage.WriteJSON(c.statePath(), state))
		c.mu.Lock()
		c.state = state
		c.mu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.source, nil)
	if err != nil {
		return err
	}
	request.Header.Set("If-None-Match", state.ETag)
	request.Header.Set("If-Modified-Since", state.LastModified)
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotModified {
		return nil
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("model catalog HTTP %d", response.StatusCode)
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return err
	}
	if len(payload) > maxBytes {
		return errors.New("model catalog exceeds size limit")
	}
	data, err := decode(payload)
	if err != nil {
		return fmt.Errorf("validate model catalog: %w", err)
	}
	if err := storage.WriteJSON(c.path, json.RawMessage(payload)); err != nil {
		return err
	}
	c.mu.Lock()
	c.data = data
	c.mu.Unlock()
	state.ETag, state.LastModified = response.Header.Get("ETag"), response.Header.Get("Last-Modified")
	return nil
}

// Lookup uses the in-memory index of the local JSON, never the network.
// Equal-ranked candidates use provider ID and model key order deterministically.
func (c *Cache) Lookup(ctx context.Context, q Query) (Metadata, error) {
	if err := ctx.Err(); err != nil {
		return Metadata{}, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	bestRank := 0
	var result Metadata
	for _, pid := range slices.Sorted(maps.Keys(c.data)) {
		p := c.data[pid]
		for _, key := range slices.Sorted(maps.Keys(p.Models)) {
			m := p.Models[key]
			if q.ID == "" || (m.ID != q.ID && key != q.ID && pid+"/"+m.ID != q.ID && m.Canonical != q.ID) {
				continue
			}
			rank := 1
			if strings.HasPrefix(q.ID, pid+"/") {
				rank = 3
			}
			if sameHost(p.API, q.BaseURL) {
				rank = 4
			}
			if rank <= bestRank {
				continue
			}
			result, bestRank = metadata(pid, m), rank
		}
	}
	return result, nil
}

func sameHost(left, right string) bool {
	a, errA := url.Parse(left)
	b, errB := url.Parse(right)
	return errA == nil && errB == nil && a.Host != "" && strings.EqualFold(a.Host, b.Host)
}

func metadata(provider string, m entry) Metadata {
	result := Metadata{Found: true, Provider: provider, ContextWindow: m.Limit.Context, MaxOutput: m.Limit.Output}
	for _, input := range []string{"text", "image", "audio", "video", "pdf"} {
		if input == "text" || slices.Contains(m.Modalities.Input, input) {
			result.InputTypes = append(result.InputTypes, input)
		}
	}
	for _, level := range []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"} {
		for _, option := range m.ReasoningOptions {
			if option.Type == "effort" && slices.Contains(option.Values, level) {
				result.ReasoningLevels = append(result.ReasoningLevels, level)
				break
			}
		}
	}
	return result
}
