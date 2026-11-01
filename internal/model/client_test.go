package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGatewayMapsUniformInputToThreeProtocols(t *testing.T) {
	for _, protocol := range []Protocol{Chat, Responses, Messages} {
		t.Run(string(protocol), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				encoded, _ := json.Marshal(body)
				for _, value := range []string{"uniform-system", "look at image", "aGVsbG8=", "inspect", "A tool"} {
					if !strings.Contains(string(encoded), value) {
						t.Errorf("uniform content %q missing: %s", value, encoded)
					}
				}
				for _, field := range []string{"Provider", "Protocol", "BaseURL", "APIKey"} {
					if _, exists := body[field]; exists {
						t.Errorf("configuration leaked into model input: %s", field)
					}
				}
				switch protocol {
				case Chat:
					if r.URL.Path != "/custom/v1/chat/completions" ||
						r.Header.Get("Authorization") != "Bearer configured-key" ||
						body["reasoning_effort"] == nil || body["max_completion_tokens"] == nil {
						t.Error("chat adapter lost endpoint, credentials or capability mapping")
					}
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
				case Responses:
					if r.URL.Path != "/custom/v1/responses" ||
						r.Header.Get("Authorization") != "Bearer configured-key" ||
						body["reasoning"] == nil || body["max_output_tokens"] == nil {
						t.Error("responses adapter lost endpoint, credentials or capability mapping")
					}
					fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"ok\"}]}]}}\n\n")
				case Messages:
					if r.URL.Path != "/custom/v1/messages" ||
						r.Header.Get("x-api-key") != "configured-key" ||
						r.Header.Get("anthropic-version") != "2023-06-01" ||
						body["thinking"] == nil || body["max_tokens"] == nil {
						t.Error("messages adapter lost endpoint, credentials or capability mapping")
					}
					fmt.Fprint(w, "data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"ok\"}}\n\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n\ndata: {\"type\":\"message_stop\"}\n\n")
				}
			}))
			defer server.Close()
			provider := Provider{
				ID: "custom", Name: "User supplied", Protocol: protocol,
				BaseURL: server.URL + "/custom/v1/", APIKey: "configured-key",
				Models: []Model{{ID: "user-model", ReasoningLevels: []string{"high"}, MaxOutput: 8192}},
			}
			gateway, err := NewClient().Bind(provider, provider.Models[0])
			if err != nil {
				t.Fatal(err)
			}
			// A bound model uses its frozen configuration even if the UI edits
			// the original channel while this run is active.
			provider.APIKey = "edited-key"
			provider.Models[0].ID = "edited-model"
			provider.Models[0].ReasoningLevels[0] = "low"
			result, err := gateway.Stream(context.Background(), Request{
				System: "uniform-system", Effort: "high",
				History: []Message{{
					Role: "user", Text: "look at image",
					Images: []Image{{MIME: "image/png", Data: "aGVsbG8="}},
				}},
				Tools: []Tool{{
					Name: "inspect", Description: "A tool",
					Parameters: map[string]any{"type": "object", "properties": map[string]any{}},
				}},
			}, nil)
			if err != nil || result.Message.Text != "ok" || result.Stop != StopComplete {
				t.Fatalf("uniform output missing: %+v %v", result, err)
			}
		})
	}
}

func TestBindRejectsDisabledConfiguration(t *testing.T) {
	for _, kind := range []string{"provider", "model", "stale enabled model"} {
		t.Run(kind, func(t *testing.T) {
			provider := Provider{
				ID: "p", Name: "P", Protocol: Chat, BaseURL: "https://example.test/v1",
				Models: []Model{{ID: "m"}},
			}
			selected := provider.Models[0]
			if kind == "provider" {
				provider.Disabled = true
			} else {
				provider.Models[0].Disabled = true
				if kind == "model" {
					selected = provider.Models[0]
				}
			}
			if _, err := NewClient().Bind(provider, selected); err == nil {
				t.Fatal("disabled configuration was accepted")
			}
		})
	}
}

func TestBindRejectsInvalidConfigurationWithoutDefaultFallback(t *testing.T) {
	for _, name := range []string{"protocol", "endpoint", "missing endpoint", "no models", "unconfigured model", "different capabilities"} {
		t.Run(name, func(t *testing.T) {
			provider := Provider{
				ID: "p", Name: "Custom", Protocol: Chat, BaseURL: "https://custom.example/v1",
				Models: []Model{{ID: "m"}},
			}
			selected := provider.Models[0]
			switch name {
			case "protocol":
				provider.Protocol = "unknown"
			case "endpoint":
				provider.BaseURL = "not a URL"
			case "missing endpoint":
				provider.BaseURL = ""
			case "no models":
				provider.Models = nil
			case "unconfigured model":
				selected.ID = "missing"
			case "different capabilities":
				selected.ReasoningLevels = []string{"high"}
			}
			if _, err := NewClient().Bind(provider, selected); err == nil {
				t.Fatal("invalid configuration silently selected a default")
			}
		})
	}
}

func TestGatewayCancelsAnInFlightStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	provider := Provider{
		ID: "p", Name: "Custom", Protocol: Chat, BaseURL: server.URL, Models: []Model{{ID: "m"}},
	}
	gateway, err := NewClient().Bind(provider, provider.Models[0])
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result, err := gateway.Stream(ctx, Request{}, func(Delta) error {
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) || len(result.Message.Calls) != 0 {
		t.Fatalf("canceled stream completed or exposed calls: %+v %v", result, err)
	}
}
