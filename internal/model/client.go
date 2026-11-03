package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"
)

// Client owns the shared outbound HTTP transport, not Agent execution state.
type Client struct {
	HTTP *http.Client
}

func NewClient() *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 45 * time.Second
	return &Client{HTTP: &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
}

type adapterTarget struct {
	model Model
	scope string
}

// adapter is the complete wire boundary. Adding a protocol does not change the
// Agent, shared transport, or SSE framing.
type adapter interface {
	path() string
	headers(http.Header, string)
	encode(Request, adapterTarget) (map[string]any, error)
	decoder() streamDecoder
}

// Gateway binds a frozen channel/model configuration to one protocol adapter.
// Its Stream input and output contain no endpoint, credentials or protocol choice.
type Gateway struct {
	http     *http.Client
	provider Provider
	target   adapterTarget
	adapter  adapter
}

func (c *Client) Bind(provider Provider, selected Model) (*Gateway, error) {
	provider = CloneProvider(provider)
	if err := provider.Validate(); err != nil {
		return nil, err
	}
	if provider.Disabled {
		return nil, errors.New("provider is disabled")
	}
	if provider.BaseURL == "" {
		return nil, errors.New("configure the provider base URL before using its models")
	}
	found := false
	for _, configured := range provider.Models {
		if equalModel(configured, selected) {
			selected = configured
			found = true
			break
		}
	}
	if !found {
		return nil, errors.New("selected model is not configured in this provider")
	}
	if selected.Disabled {
		return nil, errors.New("model is disabled")
	}
	wire, err := adapterFor(provider.Protocol)
	if err != nil {
		return nil, err
	}
	return &Gateway{
		http: c.HTTP, provider: provider, adapter: wire,
		target: adapterTarget{model: selected, scope: continuationScope(provider, selected)},
	}, nil
}

func equalModel(left, right Model) bool {
	sameIdentity := left.ID == right.ID && left.Disabled == right.Disabled
	sameLimits := left.ContextWindow == right.ContextWindow && left.MaxOutput == right.MaxOutput
	samePrices := left.InputPrice == right.InputPrice && left.OutputPrice == right.OutputPrice
	return sameIdentity && sameLimits && samePrices && slices.Equal(left.ReasoningLevels, right.ReasoningLevels) && slices.Equal(left.InputTypes, right.InputTypes)
}

// adapterFor resolves one protocol to its complete wire boundary.
func adapterFor(protocol Protocol) (adapter, error) {
	switch protocol {
	case Chat:
		return chatAdapter{}, nil
	case Responses:
		return responsesAdapter{}, nil
	case Messages:
		return messagesAdapter{}, nil
	default:
		return nil, errors.New("unsupported model protocol")
	}
}

// maxModelListBytes bounds a provider models response independently of streams.
const maxModelListBytes = 8 << 20

// ListModels requests the configured provider's own models endpoint and
// returns the deduplicated, sorted model IDs it reports. Both wire protocols
// answer {"data":[{"id":...}]}.
func (c *Client) ListModels(ctx context.Context, provider Provider) ([]string, error) {
	provider = CloneProvider(provider)
	if err := provider.Validate(); err != nil {
		return nil, err
	}
	if provider.Disabled {
		return nil, errors.New("provider is disabled")
	}
	if provider.BaseURL == "" {
		return nil, errors.New("configure the provider base URL before listing its models")
	}
	wire, err := adapterFor(provider.Protocol)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(provider.BaseURL, "/")+"/models", nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	wire.headers(request.Header, provider.APIKey)
	response, err := c.HTTP.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("provider connection failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("model list request failed (HTTP %d)", response.StatusCode)
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxModelListBytes)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("parse model list: %w", err)
	}
	ids := make([]string, 0, len(payload.Data))
	seen := make(map[string]bool, len(payload.Data))
	for _, entry := range payload.Data {
		id := strings.TrimSpace(entry.ID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids, nil
}

func (g *Gateway) Stream(ctx context.Context, input Request, emit func(Delta) error) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	payload, err := g.adapter.encode(input, g.target)
	if err != nil {
		return Result{}, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Result{}, err
	}
	endpoint := strings.TrimRight(g.provider.BaseURL, "/") + g.adapter.path()
	// HTTP retries only precede a response stream; every attempt uses identical bytes.
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
		if err != nil {
			return Result{}, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")
		g.adapter.headers(req.Header, g.provider.APIKey)
		response, err := g.http.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return Result{}, ctx.Err()
			}
			return Result{}, errors.New("model connection failed")
		}
		if (response.StatusCode == 429 || response.StatusCode >= 500) && attempt < 2 {
			_ = response.Body.Close()
			select {
			case <-ctx.Done():
				return Result{}, ctx.Err()
			case <-time.After(time.Second * time.Duration(1<<attempt)):
			}
			continue
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			_ = response.Body.Close()
			return Result{}, fmt.Errorf("model request failed (HTTP %d)", response.StatusCode)
		}
		result, streamErr := decodeStream(response.Body, g.adapter.decoder(), emit)
		_ = response.Body.Close()
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		if streamErr == nil && result.Message.Continuation != nil {
			result.Message.Continuation.scope = g.target.scope
		}
		return result, streamErr
	}
}
