package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestProviderHTTPErrorPreservesOriginalBody(t *testing.T) {
	for _, protocol := range []Protocol{Chat, Responses, Messages} {
		for _, body := range []string{
			`{"code":503,"message":"渠道凭证不可用 secret-key","stdCode":50301}`,
			"upstream authorization failed\nrequest_id: req-42\nBearer secret-key",
			"",
		} {
			t.Run(string(protocol)+"/"+fmt.Sprint(len(body)), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusUnauthorized)
					fmt.Fprint(w, body)
				}))
				defer server.Close()
				provider := Provider{ID: "errors", Name: "Errors", Protocol: protocol, BaseURL: server.URL, APIKey: "secret-key",
					Models: []Model{{ID: "model"}}}
				gateway, err := NewClient().Bind(provider, provider.Models[0])
				if err != nil {
					t.Fatal(err)
				}
				var completed Invocation
				gateway.Observe = func(call Invocation) {
					if call.CompletedAt != nil {
						completed = call
					}
				}
				_, err = gateway.Stream(t.Context(), Request{}, nil)
				var providerErr *ProviderError
				if !errors.As(err, &providerErr) || providerErr.HTTPStatus != 401 {
					t.Fatalf("missing typed HTTP error: %v", err)
				}
				want := "model request failed (HTTP 401)"
				if body != "" {
					want += "\n" + strings.ReplaceAll(body, "secret-key", "[redacted]")
				}
				if err.Error() != want || completed.Error != want {
					t.Fatalf("original body lost: error=%v trajectory=%q want=%q", err, completed.Error, want)
				}
				_, err = NewClient().ListModels(t.Context(), provider)
				if !errors.As(err, &providerErr) || strings.Contains(err.Error(), "secret-key") ||
					!strings.Contains(err.Error(), strings.ReplaceAll(body, "secret-key", "[redacted]")) {
					t.Fatalf("model list lost safe provider error: %v", err)
				}
			})
		}
	}
}

func TestProviderStreamingErrorsPreserveDetailsAndPartialText(t *testing.T) {
	for _, fixture := range []struct {
		protocol Protocol
		prefix   string
		failure  string
	}{
		{Chat, `{"choices":[{"delta":{"content":"partial"}}]}`, `{"error":{"message":"capacity secret-key","code":"overloaded"}}`},
		{Responses, `{"type":"response.output_text.delta","delta":"partial"}`, `{"type":"response.failed","response":{"error":{"code":"server_error","message":"capacity secret-key"}}}`},
		{Messages, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"partial"}}`, `{"type":"error","error":{"type":"overloaded_error","message":"capacity secret-key"}}`},
	} {
		t.Run(string(fixture.protocol), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintf(w, "data: %s\n\ndata: %s\n\n", fixture.prefix, fixture.failure)
			}))
			defer server.Close()
			provider := Provider{ID: "errors", Name: "Errors", Protocol: fixture.protocol, BaseURL: server.URL, APIKey: "secret-key",
				Models: []Model{{ID: "model"}}}
			gateway, err := NewClient().Bind(provider, provider.Models[0])
			if err != nil {
				t.Fatal(err)
			}
			result, err := gateway.Stream(t.Context(), Request{}, nil)
			var providerErr *ProviderError
			if !errors.As(err, &providerErr) || providerErr.HTTPStatus != 200 ||
				!strings.Contains(err.Error(), strings.ReplaceAll(fixture.failure, "secret-key", "[redacted]")) {
				t.Fatalf("original streaming error lost: %v", err)
			}
			if result.Message.Text != "partial" || len(result.Message.Calls) != 0 || result.Message.Continuation != nil {
				t.Fatalf("failed stream lost partial output or published execution state: %+v", result)
			}
		})
	}
}

func TestProviderErrorBoundsAndRedaction(t *testing.T) {
	secret := `secret+"quoted`
	encoded, _ := json.Marshal(secret)
	detail, truncated := providerErrorDetail("original\n"+secret+"\n"+string(encoded), secret)
	if truncated || strings.Contains(detail, "secret") || !strings.Contains(detail, "[redacted]") {
		t.Fatal(detail)
	}
	body := strings.Repeat("字", maxProviderErrorBytes) + secret
	response := &http.Response{StatusCode: 502, Body: io.NopCloser(strings.NewReader(body))}
	err := providerHTTPError(response, "model request failed", secret)
	if strings.Contains(err.Error(), secret) || len(err.Error()) > maxProviderErrorBytes+100 ||
		!utf8.ValidString(err.Error()) || !strings.Contains(err.Error(), "[provider error truncated]") {
		t.Fatal("unbounded, invalid or unredacted provider error")
	}
	// A credential straddling the visible limit must be removed as a unit.
	response.Body = io.NopCloser(strings.NewReader(strings.Repeat("x", maxProviderErrorBytes-3) + secret))
	err = providerHTTPError(response, "model request failed", secret)
	if strings.Contains(err.Error(), "sec") {
		t.Fatal("credential prefix leaked at the read boundary")
	}
	// Escaped credentials may be longer than their configured representation.
	response.Body = io.NopCloser(strings.NewReader(strings.Repeat("x", maxProviderErrorBytes-3) + string(encoded[1:len(encoded)-1])))
	err = providerHTTPError(response, "model request failed", secret)
	if strings.Contains(err.Error(), "sec") {
		t.Fatal("escaped credential prefix leaked at the read boundary")
	}
}

type providerErrorTransport struct{ failure error }

func (t providerErrorTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, t.failure
}

func TestProviderConnectionErrorsPreserveRedactedCause(t *testing.T) {
	cause := errors.New("TLS fixture failure reflecting secret-key")
	client := &Client{HTTP: &http.Client{Transport: providerErrorTransport{failure: cause}}}
	provider := Provider{ID: "errors", Name: "Errors", Protocol: Chat, BaseURL: "https://fixture.invalid",
		APIKey: "secret-key", Models: []Model{{ID: "model"}}}
	gateway, err := client.Bind(provider, provider.Models[0])
	if err != nil {
		t.Fatal(err)
	}
	_, streamErr := gateway.Stream(t.Context(), Request{}, nil)
	_, listErr := client.ListModels(t.Context(), provider)
	for _, err := range []error{streamErr, listErr} {
		var providerErr *ProviderError
		if !errors.As(err, &providerErr) || !errors.Is(err, cause) ||
			strings.Contains(err.Error(), "secret-key") ||
			!strings.Contains(err.Error(), "TLS fixture failure reflecting [redacted]") {
			t.Fatalf("connection diagnostic lost or exposed credentials: %v", err)
		}
	}
}

func TestProviderRetryRecordsEachOriginalError(t *testing.T) {
	attempt := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		w.WriteHeader(503)
		fmt.Fprintf(w, `{"message":"unavailable","attempt":%d}`, attempt)
	}))
	defer server.Close()
	provider := Provider{ID: "errors", Name: "Errors", Protocol: Chat, BaseURL: server.URL, Models: []Model{{ID: "model"}}}
	gateway, err := NewClient().Bind(provider, provider.Models[0])
	if err != nil {
		t.Fatal(err)
	}
	var calls []Invocation
	gateway.Observe = func(call Invocation) {
		if call.CompletedAt != nil {
			calls = append(calls, call)
		}
	}
	_, err = gateway.Stream(context.Background(), Request{}, nil)
	if len(calls) != 3 || err == nil || !strings.Contains(err.Error(), `"attempt":3`) {
		t.Fatalf("retry details lost: calls=%d error=%v", len(calls), err)
	}
	for i, call := range calls {
		if !strings.Contains(call.Error, fmt.Sprintf(`"attempt":%d`, i+1)) {
			t.Fatalf("attempt %d has wrong original body: %s", i+1, call.Error)
		}
	}
}
