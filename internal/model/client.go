package model

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"time"
)

// Client owns the shared outbound HTTP transport, not Agent execution state.
type Client struct {
	HTTP        *http.Client
	RetryPolicy *RetryPolicy
}

func NewClient() *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 300 * time.Second
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
	Observe  func(Invocation)
	http     *http.Client
	provider Provider
	target   adapterTarget
	adapter  adapter
	retry    RetryPolicy
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
	retry := DefaultRetryPolicy()
	if c.RetryPolicy != nil {
		retry = *c.RetryPolicy
	}
	if retry.MaxRetries < 0 || retry.BaseDelay < 0 || retry.MaxDelay < 0 {
		return nil, errors.New("retry limits and delays must be nonnegative")
	}
	return &Gateway{
		http: c.HTTP, provider: provider, adapter: wire,
		target: adapterTarget{model: selected, scope: continuationScope(provider, selected)},
		retry:  retry,
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

// The configured URL is a service root, optionally with a gateway prefix.
// An explicit trailing /v1 already supplies the protocol's version segment.
func providerEndpoint(baseURL, resource string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	if !strings.HasSuffix(strings.TrimRight(base.Path, "/"), "/v1") {
		resource = "/v1" + resource
	}
	return url.JoinPath(baseURL, resource)
}

// maxModelListBytes bounds a provider models response independently of streams.
const maxModelListBytes = 8 << 20

// ListModels requests the configured provider's own models endpoint and
// returns the deduplicated, sorted model IDs it reports. All three protocols
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
	endpoint, err := providerEndpoint(provider.BaseURL, "/models")
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
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
		return nil, providerConnectionError("provider connection failed", err, provider.APIKey)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, providerHTTPError(response, "model list request failed", provider.APIKey)
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

func (g *Gateway) Stream(ctx context.Context, input Request, emit func(Delta) error) (result Result, callErr error) {
	return g.StreamWithAttempts(ctx, input, emit, nil)
}

// StreamWithAttempts observes each physical request for accounting. The retry
// loop and request identity are the same as Stream; observers cannot add retries.
func (g *Gateway) StreamWithAttempts(ctx context.Context, input Request, emit func(Delta) error, record func(Result) error) (result Result, callErr error) {
	started := time.Now()
	defer func() { result.RequestDuration = time.Since(started) }()
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
	endpoint, err := providerEndpoint(g.provider.BaseURL, g.adapter.path())
	if err != nil {
		return Result{}, err
	}
	requestID := rand.Text()
	var diagnostics RequestDiagnostics
	if g.Observe != nil {
		diagnostics = requestDiagnostics(payload, body, input)
		diagnostics.TargetHash = g.target.scope
	}
	// One budget covers transport, HTTP status and incomplete streams. Only a
	// complete response is returned to the Agent, which owns tool execution.
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		var consumerErr error
		deliver := func(delta Delta) error {
			if emit != nil {
				consumerErr = emit(delta)
			}
			return consumerErr
		}
		result, _, err = g.streamAttempt(ctx, input, deliver, endpoint, body, requestID, attempt+1, diagnostics)
		if record != nil {
			if recordErr := record(result); recordErr != nil {
				return result, errors.Join(err, recordErr)
			}
		}
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		if consumerErr != nil {
			return result, consumerErr
		}
		failure := ClassifyFailure(err)
		if err == nil || !failure.Retryable || attempt >= g.retry.MaxRetries {
			return result, err
		}
		if emit != nil {
			state := &RetryStatus{Attempt: attempt + 1, MaxRetries: g.retry.MaxRetries, Reason: failure.Reason, HTTPStatus: failure.HTTPStatus}
			if emitErr := emit(Delta{Retry: state}); emitErr != nil {
				return result, emitErr
			}
		}
		if waitErr := g.retry.wait(ctx, g.retry.delay(attempt+1, failure.RetryAfter)); waitErr != nil {
			return result, waitErr
		}
	}
}

func (g *Gateway) streamAttempt(ctx context.Context, input Request, emit func(Delta) error, endpoint string, body []byte,
	requestID string, attempt int, diagnostics RequestDiagnostics) (result Result, status int, callErr error) {
	started := time.Now()
	var uploaded atomic.Int64
	uploaded.Store(-1)
	var headers *int64
	var providerRequestID string
	if g.Observe != nil {
		trace := g.invocation(requestID, attempt, input, time.Now())
		trace.Request = diagnostics
		g.Observe(trace)
		defer func() {
			now := time.Now()
			trace.CompletedAt, trace.DurationMS, trace.HTTPStatus = &now, now.Sub(trace.StartedAt).Milliseconds(), status
			if elapsed := uploaded.Load(); elapsed >= 0 {
				trace.Transport.UploadCompleteMS = &elapsed
			}
			trace.Transport.ResponseHeadersMS = headers
			trace.Transport.RequestID = traceText(providerRequestID, g.provider.APIKey, &trace.Truncated)
			response := result
			response.Message = traceMessages([]Message{result.Message}, g.provider.APIKey, &trace.Truncated)[0]
			if status >= 200 && status < 300 {
				trace.Response = &response
			}
			if callErr != nil {
				trace.Error = traceText(callErr.Error(), g.provider.APIKey, &trace.Truncated)
				trace.InvalidToolCall = traceInvalidToolCall(callErr, g.provider.APIKey, &trace.Truncated)
			}
			g.Observe(trace)
		}()
	}
	defer func() { result.RequestDuration = time.Since(started) }()
	if g.Observe != nil {
		ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{WroteRequest: func(info httptrace.WroteRequestInfo) {
			if info.Err == nil {
				uploaded.Store(time.Since(started).Milliseconds())
			}
		}})
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return result, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	g.adapter.headers(req.Header, g.provider.APIKey)
	response, err := g.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return result, 0, ctx.Err()
		}
		return result, 0, providerConnectionError("model connection failed", err, g.provider.APIKey)
	}
	defer response.Body.Close()
	elapsed := time.Since(started).Milliseconds()
	headers = &elapsed
	providerRequestID = response.Header.Get("x-request-id")
	if providerRequestID == "" {
		providerRequestID = response.Header.Get("request-id")
	}
	status = response.StatusCode
	if status < 200 || status >= 300 {
		return result, status, providerHTTPError(response, "model request failed", g.provider.APIKey)
	}
	streamStarted := time.Now()
	result, callErr = decodeStream(response.Body, g.adapter.decoder(), emit)
	if result.FirstTokenObserved {
		result.FirstTokenLatency += streamStarted.Sub(started)
	}
	if ctx.Err() != nil {
		return result, status, ctx.Err()
	}
	var providerErr *streamProviderError
	if errors.As(callErr, &providerErr) {
		detail, truncated := providerErrorDetail(providerErr.payload, g.provider.APIKey)
		callErr = &ProviderError{HTTPStatus: status, operation: "model reported a streaming error", detail: detail, truncated: truncated,
			retryAfter: parseRetryAfter(response.Header.Get("Retry-After"), time.Now())}
	} else if callErr != nil && !errors.Is(callErr, ErrStreamInterrupted) {
		// Preserve transport error identity without treating protocol validation
		// or consumer callback failures as connection failures.
		var read *streamReadError
		if errors.As(callErr, &read) {
			callErr = providerConnectionError("model stream connection failed", callErr, g.provider.APIKey)
		}
	}
	if callErr == nil && result.Message.Continuation != nil {
		result.Message.Continuation.scope = g.target.scope
	}
	return result, status, callErr
}
