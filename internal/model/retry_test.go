package model

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

type retryTransport func(*http.Request) (*http.Response, error)

func (f retryTransport) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func retryGateway(t *testing.T, transport http.RoundTripper) *Gateway {
	t.Helper()
	client := newTestClient()
	client.HTTP = &http.Client{Transport: transport}
	provider := Provider{ID: "p", Name: "P", Protocol: Chat, BaseURL: "https://fixture.invalid",
		APIKey: "private-secret", Models: []Model{{ID: "m"}}}
	gateway, err := client.Bind(provider, provider.Models[0])
	if err != nil {
		t.Fatal(err)
	}
	return gateway
}

func retryResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}

func TestRetryMixedFailuresShareOneRequestAndBudget(t *testing.T) {
	var payloads []string
	gateway := retryGateway(t, retryTransport(func(request *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(request.Body)
		payloads = append(payloads, string(body))
		switch len(payloads) {
		case 1:
			return nil, errors.New("stream error: stream ID 21; INTERNAL_ERROR; received from peer")
		case 2:
			response := retryResponse(503, `{"error":{"message":"busy"}}`)
			response.Header.Set("Retry-After", "0")
			return response, nil
		case 3:
			return retryResponse(200, "data: {\"choices\":[{\"delta\":{\"content\":\"discard this\",\"tool_calls\":[{\"index\":0,\"id\":\"incomplete\",\"function\":{\"name\":\"Write\",\"arguments\":\"{}\"}}]}}],\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":2}}\n\n"), nil
		default:
			return retryResponse(200, "data: {\"choices\":[{\"delta\":{\"content\":\"complete\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"), nil
		}
	}))
	var states []RetryStatus
	var attempts []Result
	var traces []Invocation
	gateway.Observe = func(trace Invocation) {
		if trace.CompletedAt != nil {
			traces = append(traces, trace)
		}
	}
	result, err := gateway.StreamWithAttempts(t.Context(), Request{History: []Message{{Role: "user", Text: "original"}}},
		func(delta Delta) error {
			if delta.Retry != nil {
				states = append(states, *delta.Retry)
			}
			return nil
		}, func(result Result) error { attempts = append(attempts, result); return nil })
	if err != nil || result.Message.Text != "complete" || len(result.Message.Calls) != 0 ||
		len(payloads) != 4 || len(attempts) != 4 || len(states) != 3 || len(traces) != 4 {
		t.Fatalf("mixed recovery failed: result=%+v err=%v attempts=%d states=%+v traces=%d", result, err, len(attempts), states, len(traces))
	}
	for i, state := range states {
		if state.Attempt != i+1 || state.MaxRetries != 10 || traces[i].ID != traces[3].ID || traces[i].Attempt != i+1 {
			t.Fatalf("budget/trajectory split: states=%+v traces=%+v", states, traces)
		}
	}
	if attempts[2].Usage.Input != 7 || attempts[2].Usage.Output != 2 {
		t.Fatal("discarded generation lost its observed billable usage")
	}
	for _, payload := range payloads[1:] {
		if payload != payloads[0] {
			t.Fatal("retry changed the accepted request or committed incomplete output")
		}
	}
}

func TestRetryInvalidToolCallWithoutPublishingAnyCalls(t *testing.T) {
	var payloads []string
	gateway := retryGateway(t, retryTransport(func(request *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(request.Body)
		payloads = append(payloads, string(body))
		if len(payloads) == 1 {
			return retryResponse(200, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"valid","function":{"name":"Write","arguments":"{}"}},{"index":1,"id":"invalid","function":{"name":"Write","arguments":"{"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":12,"completion_tokens":7}}`+"\n\ndata: [DONE]\n\n"), nil
		}
		return retryResponse(200, "data: {\"choices\":[{\"delta\":{\"content\":\"recovered\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"), nil
	}))
	var retries []RetryStatus
	var attempts []Result
	result, err := gateway.StreamWithAttempts(t.Context(), Request{History: []Message{{Role: "user", Text: "original"}}},
		func(delta Delta) error {
			if delta.Retry != nil {
				retries = append(retries, *delta.Retry)
			}
			return nil
		}, func(result Result) error { attempts = append(attempts, result); return nil })
	if err != nil || result.Message.Text != "recovered" || len(payloads) != 2 || len(attempts) != 2 {
		t.Fatalf("invalid call was not recovered: result=%+v err=%v requests=%d", result, err, len(payloads))
	}
	if len(attempts[0].Message.Calls) != 0 || attempts[0].Usage.Output != 7 {
		t.Fatal("failed batch published executable calls or lost usage")
	}
	if payloads[0] != payloads[1] || len(retries) != 1 || retries[0].Reason != "invalid_tool_call" {
		t.Fatalf("retry changed request or classification: %+v", retries)
	}
}

func TestRetryBudgetIsTenAdditionalAttemptsAndResetsPerRequest(t *testing.T) {
	calls := 0
	gateway := retryGateway(t, retryTransport(func(*http.Request) (*http.Response, error) {
		calls++
		return retryResponse(503, fmt.Sprintf(`{"attempt":%d}`, calls)), nil
	}))
	for request := 1; request <= 2; request++ {
		states := 0
		_, err := gateway.Stream(t.Context(), Request{}, func(delta Delta) error {
			if delta.Retry != nil {
				states++
				if delta.Retry.Attempt != states {
					t.Fatal("budget leaked from an earlier request")
				}
			}
			return nil
		})
		if err == nil || calls != request*11 || states != 10 {
			t.Fatalf("request %d: calls=%d retries=%d err=%v", request, calls, states, err)
		}
	}
}

func TestRetryFailureClassification(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		reason string
		retry  bool
	}{
		{"cancel", context.Canceled, "cancelled", false},
		{"transport deadline", context.DeadlineExceeded, "timeout", true},
		{"certificate", providerConnectionError("connect", x509.UnknownAuthorityError{}, ""), "tls", false},
		{"auth", &ProviderError{HTTPStatus: 401}, "authentication", false},
		{"bad parameters", &ProviderError{HTTPStatus: 400}, "invalid_request", false},
		{"quota before rate limit", &ProviderError{HTTPStatus: 429, detail: `{"error":{"code":"insufficient_quota"}}`}, "quota", false},
		{"rate limit", &ProviderError{HTTPStatus: 429}, "rate_limited", true},
		{"http timeout", &ProviderError{HTTPStatus: 408}, "timeout", true},
		{"server", &ProviderError{HTTPStatus: 503}, "server", true},
		{"sse overloaded", &ProviderError{HTTPStatus: 200, detail: `{"error":{"type":"overloaded_error"}}`}, "server", true},
		{"incomplete stream", ErrStreamInterrupted, "stream_interrupted", true},
		{"http2 reset before headers", providerConnectionError("connect", &url.Error{Op: "Post", URL: "https://fixture.invalid", Err: errors.New("stream error: stream ID 21; INTERNAL_ERROR; received from peer")}, ""), "network", true},
		{"http2 reset in stream", providerConnectionError("read", &streamReadError{cause: errors.New("http2 reset")}, ""), "network", true},
		{"protocol violation", errors.New("model returned duplicate tool call IDs"), "unknown", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := ClassifyFailure(test.err)
			if got.Reason != test.reason || got.Retryable != test.retry {
				t.Fatalf("classification=%+v want=%s retry=%v", got, test.reason, test.retry)
			}
		})
	}
}

func TestRetryDelayMatchesReferenceAndHonorsRetryAfter(t *testing.T) {
	policy := DefaultRetryPolicy()
	policy.Jitter = false
	var delays []time.Duration
	for retry := 1; retry <= 7; retry++ {
		delays = append(delays, policy.delay(retry, nil))
	}
	if !reflect.DeepEqual(delays, []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 32 * time.Second, time.Minute, time.Minute}) {
		t.Fatal(delays)
	}
	after := 2 * time.Minute
	if policy.delay(1, &after) != after {
		t.Fatal("Retry-After was capped to the local 60-second limit")
	}
	policy.Jitter = true
	for i := 0; i < 100; i++ {
		if delay := policy.delay(1, nil); delay < time.Second || delay > 2*time.Second {
			t.Fatal(delay)
		}
		if delay := policy.delay(1, &after); delay < after || delay > after+5*time.Second {
			t.Fatal(delay)
		}
	}
	now := time.Date(2026, 11, 4, 7, 0, 0, 0, time.UTC)
	for _, value := range []string{"90", now.Add(90 * time.Second).Format(http.TimeFormat)} {
		if got := parseRetryAfter(value, now); got == nil || *got != 90*time.Second {
			t.Fatalf("%q = %v", value, got)
		}
	}
}

func TestRetryCancellationAndConsumerFailureStopImmediately(t *testing.T) {
	for _, consumer := range []bool{false, true} {
		t.Run(fmt.Sprint(consumer), func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			calls := 0
			gateway := retryGateway(t, retryTransport(func(*http.Request) (*http.Response, error) { calls++; return retryResponse(503, "busy"), nil }))
			if !consumer {
				gateway.retry.Wait = func(ctx context.Context, _ time.Duration) error { cancel(); <-ctx.Done(); return ctx.Err() }
			}
			sentinel := errors.New("cannot publish")
			_, err := gateway.Stream(ctx, Request{}, func(delta Delta) error {
				if consumer && delta.Retry != nil {
					return sentinel
				}
				return nil
			})
			want := error(context.Canceled)
			if consumer {
				want = sentinel
			}
			if calls != 1 || !errors.Is(err, want) {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
		})
	}
}

func TestRetryDoesNotRetryConsumerTimeout(t *testing.T) {
	calls := 0
	gateway := retryGateway(t, retryTransport(func(*http.Request) (*http.Response, error) {
		calls++
		return retryResponse(200, "data: {\"choices\":[{\"delta\":{\"content\":\"output\"}}]}\n\n"), nil
	}))
	_, err := gateway.Stream(t.Context(), Request{}, func(Delta) error { return context.DeadlineExceeded })
	if !errors.Is(err, context.DeadlineExceeded) || calls != 1 {
		t.Fatalf("consumer timeout was treated as network failure: calls=%d err=%v", calls, err)
	}
}

func TestRetrySharedAcrossAllProtocols(t *testing.T) {
	for _, test := range []struct {
		protocol Protocol
		complete string
	}{
		{Chat, "data: {\"choices\":[{\"delta\":{\"content\":\"complete\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"},
		{Responses, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"complete\"}]}]}}\n\n"},
		{Messages, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"complete\"}}\n\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n\ndata: {\"type\":\"message_stop\"}\n\n"},
	} {
		t.Run(string(test.protocol), func(t *testing.T) {
			calls, retries := 0, 0
			client := newTestClient()
			client.HTTP = &http.Client{Transport: retryTransport(func(*http.Request) (*http.Response, error) {
				calls++
				switch calls {
				case 1:
					return retryResponse(503, "busy"), nil
				case 2:
					return retryResponse(200, ""), nil // Missing completion marker.
				default:
					return retryResponse(200, test.complete), nil
				}
			})}
			provider := Provider{ID: "p", Name: "P", Protocol: test.protocol, BaseURL: "https://fixture.invalid", Models: []Model{{ID: "m"}}}
			gateway, err := client.Bind(provider, provider.Models[0])
			if err != nil {
				t.Fatal(err)
			}
			result, err := gateway.Stream(t.Context(), Request{}, func(delta Delta) error {
				if delta.Retry != nil {
					retries++
					if delta.Retry.Attempt != retries {
						t.Fatal("HTTP and stream failures used separate budgets")
					}
				}
				return nil
			})
			if err != nil || calls != 3 || retries != 2 || result.Message.Text != "complete" {
				t.Fatalf("calls=%d retries=%d result=%+v error=%v", calls, retries, result, err)
			}
		})
	}
}
