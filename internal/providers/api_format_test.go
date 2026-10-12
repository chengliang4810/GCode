package providers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"gcode/internal/config"
	"gcode/internal/zeroruntime"
)

func TestNewRoutesConfiguredAPIFormats(t *testing.T) {
	tests := []struct {
		name        string
		kind        config.ProviderKind
		format      string
		wantPath    string
		headerName  string
		headerValue string
		response    string
	}{
		{
			name:        "openai responses",
			kind:        config.ProviderKindOpenAI,
			format:      "responses",
			wantPath:    "/responses",
			headerName:  "Authorization",
			headerValue: "Bearer key-responses",
			response:    `data: {"type":"response.completed","response":{"id":"r","status":"completed"}}` + "\n\n",
		},
		{
			name:        "openai chat completions",
			kind:        config.ProviderKindOpenAI,
			format:      "chat-completions",
			wantPath:    "/chat/completions",
			headerName:  "Authorization",
			headerValue: "Bearer key-chat",
			response:    `data: {"choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}]}` + "\n\n",
		},
		{
			name:        "anthropic messages",
			kind:        config.ProviderKindAnthropic,
			format:      "anthropic",
			wantPath:    "/v1/messages",
			headerName:  "x-api-key",
			headerValue: "key-anthropic",
			response:    "data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":1}}}\n\n" + "data: {\"type\":\"message_stop\"}\n\n",
		},
		{
			name:        "gemini generate content",
			kind:        config.ProviderKindGoogle,
			format:      "gemini",
			wantPath:    "/v1beta/models/test-model:streamGenerateContent",
			headerName:  "x-goog-api-key",
			headerValue: "key-gemini",
			response:    `data: {"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}` + "\n\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var mu sync.Mutex
			var gotPath, gotHeader string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				gotPath = r.URL.Path
				gotHeader = r.Header.Get(test.headerName)
				mu.Unlock()
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte(test.response))
			}))
			defer server.Close()

			key := strings.TrimPrefix(test.headerValue, "Bearer ")
			provider, err := New(config.ProviderProfile{
				Name:         test.name,
				ProviderKind: test.kind,
				APIFormat:    test.format,
				BaseURL:      server.URL,
				APIKey:       key,
				Model:        "test-model",
			}, Options{})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			stream, err := provider.StreamCompletion(context.Background(), zeroruntime.CompletionRequest{
				Messages: []zeroruntime.Message{{Role: zeroruntime.MessageRoleUser, Content: "hello"}},
			})
			if err != nil {
				t.Fatalf("StreamCompletion: %v", err)
			}
			for event := range stream {
				if event.Type == zeroruntime.StreamEventError {
					t.Fatalf("stream error: %s", event.Error)
				}
			}
			mu.Lock()
			defer mu.Unlock()
			if gotPath != test.wantPath {
				t.Fatalf("path = %q, want %q", gotPath, test.wantPath)
			}
			if gotHeader != test.headerValue {
				t.Fatalf("auth header = %q, want %q", gotHeader, test.headerValue)
			}
		})
	}
}

func TestNewRejectsIncompatibleAPIFormat(t *testing.T) {
	_, err := New(config.ProviderProfile{
		Name:         "wrong-format",
		ProviderKind: config.ProviderKindAnthropic,
		APIFormat:    "responses",
		Model:        "test-model",
	}, Options{})
	if err == nil {
		t.Fatal("New returned nil error for incompatible API format")
	}
	if !strings.Contains(err.Error(), `API format "responses" is incompatible`) {
		t.Fatalf("error = %q, want API format mismatch", err)
	}
}

func TestResolveAPIFormatDefaultsMatchProviderKinds(t *testing.T) {
	tests := []struct {
		name string
		kind config.ProviderKind
		want string
	}{
		{name: "openai", kind: config.ProviderKindOpenAI, want: "chat-completions"},
		{name: "openai-compatible", kind: config.ProviderKindOpenAICompatible, want: "chat-completions"},
		{name: "anthropic", kind: config.ProviderKindAnthropic, want: "anthropic"},
		{name: "anthropic-compatible", kind: config.ProviderKindAnthropicCompat, want: "anthropic"},
		{name: "google", kind: config.ProviderKindGoogle, want: "gemini"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := resolveAPIFormat(config.ProviderProfile{}, test.kind)
			if err != nil {
				t.Fatalf("resolveAPIFormat() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("resolveAPIFormat() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestFactoryUsesKindInsteadOfDisplayNameForCustomModels(t *testing.T) {
	metadata, err := ResolveRuntimeMetadata(config.ProviderProfile{
		Name:         "anthropic", // display name only; it must not select Anthropic transport
		ProviderKind: config.ProviderKindOpenAICompatible,
		BaseURL:      "https://gateway.example/v1",
		Model:        "vendor/third-party-model",
	}, Options{})
	if err != nil {
		t.Fatalf("ResolveRuntimeMetadata() error = %v", err)
	}
	if metadata.ProviderKind != config.ProviderKindOpenAICompatible {
		t.Fatalf("ProviderKind = %q, want %q", metadata.ProviderKind, config.ProviderKindOpenAICompatible)
	}
	if metadata.APIModel != "vendor/third-party-model" {
		t.Fatalf("APIModel = %q, want custom model passed through", metadata.APIModel)
	}
}

func TestFactoryDoesNotUseDisplayNameForProviderModelAllowlist(t *testing.T) {
	_, err := ResolveRuntimeMetadata(config.ProviderProfile{
		Name:         "opencode-go-anthropic-compatible", // display name only; no catalog identity
		ProviderKind: config.ProviderKindOpenAICompatible,
		BaseURL:      "https://gateway.example/v1",
		Model:        "vendor/third-party-model",
	}, Options{})
	if err != nil {
		t.Fatalf("ResolveRuntimeMetadata() error = %v, want display name ignored", err)
	}
}

func TestFactoryMapsAPIFormatWithoutUsingDisplayName(t *testing.T) {
	tests := []struct {
		name        string
		format      string
		baseURL     string
		wantKind    config.ProviderKind
		wantAPIName string
	}{
		{
			name:        "openai",
			format:      "responses",
			baseURL:     config.OpenAIBaseURL,
			wantKind:    config.ProviderKindOpenAI,
			wantAPIName: "responses",
		},
		{
			name:        "anthropic",
			format:      "messages",
			baseURL:     config.AnthropicBaseURL,
			wantKind:    config.ProviderKindAnthropic,
			wantAPIName: "anthropic",
		},
		{
			name:        "google",
			format:      "generate-content",
			baseURL:     config.GoogleBaseURL,
			wantKind:    config.ProviderKindGoogle,
			wantAPIName: "gemini",
		},
		{
			name:        "custom anthropic gateway",
			format:      "anthropic",
			baseURL:     "https://gateway.example/anthropic",
			wantKind:    config.ProviderKindAnthropicCompat,
			wantAPIName: "third-party-model",
		},
		{
			name:        "custom openai gateway",
			format:      "chat-completions",
			baseURL:     "https://gateway.example/v1",
			wantKind:    config.ProviderKindOpenAICompatible,
			wantAPIName: "third-party-model",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			metadata, err := ResolveRuntimeMetadata(config.ProviderProfile{
				Name:      "display-name-that-must-be-ignored",
				APIFormat: test.format,
				BaseURL:   test.baseURL,
				Model:     test.wantAPIName,
			}, Options{})
			if err != nil {
				t.Fatalf("ResolveRuntimeMetadata() error = %v", err)
			}
			if metadata.ProviderKind != test.wantKind {
				t.Fatalf("ProviderKind = %q, want %q", metadata.ProviderKind, test.wantKind)
			}
			if metadata.APIModel != test.wantAPIName {
				t.Fatalf("APIModel = %q, want %q", metadata.APIModel, test.wantAPIName)
			}
		})
	}
}

func TestResolveRuntimeMetadataLeavesProviderKindStableAcrossFormats(t *testing.T) {
	for _, format := range []string{"responses", "chat-completions"} {
		metadata, err := ResolveRuntimeMetadata(config.ProviderProfile{
			Name:         fmt.Sprintf("openai-%s", format),
			ProviderKind: config.ProviderKindOpenAI,
			APIFormat:    format,
			Model:        "test-model",
		}, Options{})
		if err != nil {
			t.Fatalf("ResolveRuntimeMetadata(%s): %v", format, err)
		}
		if metadata.ProviderKind != config.ProviderKindOpenAI || metadata.APIModel != "test-model" {
			t.Fatalf("metadata(%s) = %#v", format, metadata)
		}
	}
}
