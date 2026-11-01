package model

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func TestProviderDraftValidationAndReadiness(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		baseURL  string
		models   []Model
		disabled bool
		valid    bool
		ready    bool
	}{
		{name: "new draft", valid: true},
		{name: "endpoint only", baseURL: "http://localhost:11434/v1", valid: true},
		{name: "model only", models: []Model{{ID: "local"}}, valid: true},
		{name: "configured without credentials", baseURL: "http://localhost:11434/v1", models: []Model{{ID: "local"}}, valid: true, ready: true},
		{name: "disabled provider", baseURL: "https://example.test/v1", models: []Model{{ID: "m"}}, disabled: true, valid: true},
		{name: "disabled last model", baseURL: "https://example.test/v1", models: []Model{{ID: "m", Disabled: true}}, valid: true},
		{name: "invalid nonempty endpoint", baseURL: "not a URL"},
		{name: "endpoint credentials", baseURL: "https://secret@example.test/v1"},
		{name: "endpoint query", baseURL: "https://example.test/v1?secret=value"},
		{name: "empty model ID", models: []Model{{ID: ""}}},
		{name: "duplicate model", models: []Model{{ID: "m"}, {ID: "m"}}},
		{name: "negative limit", models: []Model{{ID: "m", MaxOutput: -1}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := Provider{
				ID: "p", Name: "Custom", Protocol: Chat,
				BaseURL: tt.baseURL, Models: tt.models, Disabled: tt.disabled,
			}
			if err := provider.Validate(); (err == nil) != tt.valid {
				t.Fatalf("Validate() = %v, want valid=%t", err, tt.valid)
			}
			if ready := provider.Ready(); ready != tt.ready {
				t.Fatalf("Ready() = %t, want %t", ready, tt.ready)
			}
		})
	}
}

func TestProviderRejectsInvalidModelPrices(t *testing.T) {
	for _, value := range []struct {
		name  string
		price float64
	}{
		{name: "negative", price: -0.1},
		{name: "nan", price: math.NaN()},
		{name: "positive infinity", price: math.Inf(1)},
		{name: "negative infinity", price: math.Inf(-1)},
	} {
		t.Run(value.name, func(t *testing.T) {
			for _, output := range []bool{false, true} {
				provider := Provider{
					ID: "p", Name: "P", Protocol: Chat, BaseURL: "https://example.test/v1",
					Models: []Model{{ID: "m"}},
				}
				if output {
					provider.Models[0].OutputPrice = value.price
				} else {
					provider.Models[0].InputPrice = value.price
				}
				if err := provider.Validate(); err == nil {
					t.Errorf("invalid price %v was accepted (output=%v)", value.price, output)
				}
			}
		})
	}
}

func TestModelResolveEffort(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		levels []string
		value  string
		want   string
		err    bool
	}{
		{name: "no capability", value: "", want: ""},
		{name: "undeclared display label", value: "Off", err: true},
		{name: "unsupported capability", value: "high", err: true},
		{name: "none requires capability", value: "none", err: true},
		{name: "configured order defines default", levels: []string{"high", "low"}, want: "low"},
		{name: "off does not select configured default", levels: []string{"low", "max"}, value: "Off", err: true},
		{name: "label", levels: []string{"low", "xhigh"}, value: " Extra high ", want: "xhigh"},
		{name: "minimal", levels: []string{"minimal", "high"}, value: "Minimal", want: "minimal"},
		{name: "explicit off level", levels: []string{"none", "high"}, value: "None", want: "none"},
		{name: "explicit max", levels: []string{"high", "max"}, value: "max", want: "max"},
		{name: "configured subset", levels: []string{"high"}, value: "low", err: true},
		{name: "unknown input is not default", levels: []string{"high"}, value: "invalid", err: true},
		{name: "invalid configuration", levels: []string{"HIGH"}, value: "", err: true},
		{name: "duplicate configuration", levels: []string{"high", "high"}, value: "", err: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selected := Model{ID: "custom", ReasoningLevels: tt.levels}
			got, err := selected.ResolveEffort(tt.value)
			if (err != nil) != tt.err || got != tt.want {
				t.Fatalf("ResolveEffort(%q) = %q, %v; want %q, error=%t", tt.value, got, err, tt.want, tt.err)
			}
		})
	}
}

func TestReasoningLabelsPreserveLevels(t *testing.T) {
	t.Parallel()
	for _, level := range []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"} {
		t.Run(level, func(t *testing.T) {
			if got := EffortID(ReasoningLabel(level)); got != level {
				t.Fatalf("display round trip = %q, want %q", got, level)
			}
		})
	}
	if label := ReasoningLabel("xhigh"); label != "Extra high" {
		t.Fatalf("xhigh label = %q", label)
	}
	if got := EffortID("unsupported"); got != "unsupported" {
		t.Fatalf("unknown level silently disappeared: %q", got)
	}
}

func TestProviderValidatesReasoningLevels(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		protocol Protocol
		levels   []string
		wantErr  bool
	}{
		{name: "chat all", protocol: Chat, levels: []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"}},
		{name: "responses all", protocol: Responses, levels: []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"}},
		{name: "messages adaptive", protocol: Messages, levels: []string{"none", "low", "medium", "high", "xhigh", "max"}},
		{name: "messages minimal", protocol: Messages, levels: []string{"minimal"}, wantErr: true},
		{name: "uppercase", protocol: Chat, levels: []string{"High"}, wantErr: true},
		{name: "unknown", protocol: Chat, levels: []string{"ultra"}, wantErr: true},
		{name: "duplicate", protocol: Chat, levels: []string{"low", "low"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := Provider{
				ID: "p", Name: "P", Protocol: tt.protocol, BaseURL: "https://model.example/v1",
				Models: []Model{{ID: "m", ReasoningLevels: tt.levels}},
			}
			if err := provider.Validate(); (err != nil) != tt.wantErr {
				t.Fatalf("Validate() = %v, want error=%t", err, tt.wantErr)
			}
		})
	}
}

func TestCloneProviderOwnsModelCapabilities(t *testing.T) {
	t.Parallel()
	original := Provider{Models: []Model{{ID: "m", ReasoningLevels: []string{"low", "max"}}}}
	cloned := CloneProvider(original)
	original.Models[0].ID = "edited"
	original.Models[0].ReasoningLevels[0] = "high"
	if cloned.Models[0].ID != "m" || !slices.Equal(cloned.Models[0].ReasoningLevels, []string{"low", "max"}) {
		t.Fatalf("clone retained caller-owned state: %+v", cloned.Models)
	}
	cloned.Models[0].ReasoningLevels[1] = "medium"
	if original.Models[0].ReasoningLevels[1] != "max" {
		t.Fatal("clone capability edits changed the source")
	}
}

func TestAdaptersEncodeConfiguredEffort(t *testing.T) {
	t.Parallel()
	for _, protocol := range []Protocol{Chat, Responses, Messages} {
		t.Run(string(protocol), func(t *testing.T) {
			levels := []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"}
			if protocol == Messages {
				levels = []string{"none", "low", "medium", "high", "xhigh", "max"}
			}
			selected := Model{ID: "user-model", ReasoningLevels: levels, MaxOutput: 1024}
			for _, level := range append(slices.Clone(levels), "") {
				name := level
				if name == "" {
					name = "default"
				}
				t.Run(name, func(t *testing.T) {
					body := requestBody(t, protocol, selected, Request{Effort: level})
					want := level
					if want == "" {
						want = "max"
					}
					switch protocol {
					case Chat:
						if body["reasoning_effort"] != want {
							t.Fatalf("chat effort = %v, want %s", body["reasoning_effort"], want)
						}
					case Responses:
						if body["reasoning"].(map[string]any)["effort"] != want {
							t.Fatalf("responses effort = %v, want %s", body["reasoning"], want)
						}
					case Messages:
						thinking := body["thinking"].(map[string]any)
						if want == "none" {
							if thinking["type"] != "disabled" || body["output_config"] != nil {
								t.Fatalf("explicit no thinking = %+v", body)
							}
							return
						}
						output := body["output_config"].(map[string]any)
						if thinking["type"] != "adaptive" || thinking["budget_tokens"] != nil || output["effort"] != want {
							t.Fatalf("messages adaptive effort = %+v", body)
						}
					}
				})
			}
		})
	}
}

func TestAdaptersRejectUnconfiguredEffort(t *testing.T) {
	t.Parallel()
	for _, wire := range []adapter{chatAdapter{}, responsesAdapter{}, messagesAdapter{}} {
		t.Run(wire.path(), func(t *testing.T) {
			target := adapterTarget{model: Model{ID: "m", ReasoningLevels: []string{"high"}}}
			for _, value := range []string{"max", "invalid"} {
				if _, err := wire.encode(Request{Effort: value}, target); err == nil {
					t.Fatalf("unconfigured effort %q was silently accepted", value)
				}
			}
		})
	}
}

func TestThreeProtocolsBuildAndStream(t *testing.T) {
	for _, protocol := range []Protocol{Chat, Responses, Messages} {
		t.Run(string(protocol), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request map[string]any
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				if request["model"] != "custom-model" || request["stream"] != true {
					t.Error("invalid request")
				}
				if protocol == Messages {
					if r.URL.Path != "/v1/messages" || r.Header.Get("x-api-key") != "local-test" {
						t.Error("Messages endpoint or credentials")
					}
				} else if r.Header.Get("Authorization") != "Bearer local-test" {
					t.Error("bearer credentials")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				switch protocol {
				case Chat:
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"你好\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":2}}\n\ndata: [DONE]\n\n")
				case Responses:
					fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"你好\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":12,\"output_tokens\":2}}}\n\n")
				case Messages:
					fmt.Fprint(w, "data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":12}}}\n\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"你好\"}}\n\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}\n\ndata: {\"type\":\"message_stop\"}\n\n")
				}
			}))
			defer server.Close()
			var streamed strings.Builder
			provider := Provider{
				ID: "custom", Name: "Custom", Protocol: protocol, BaseURL: server.URL + "/v1",
				APIKey: "local-test", Models: []Model{{ID: "custom-model"}},
			}
			gateway, err := NewClient().Bind(provider, provider.Models[0])
			if err != nil {
				t.Fatal(err)
			}
			result, err := gateway.Stream(
				context.Background(),
				Request{History: []Message{{Role: "user", Text: "你好"}}},
				func(d Delta) error { streamed.WriteString(d.Text); return nil },
			)
			if err != nil || result.Message.Text != "你好" || streamed.String() != "你好" || result.Usage.Input != 12 || result.Usage.Output != 2 {
				t.Fatalf("result=%+v streamed=%s err=%v", result, streamed.String(), err)
			}
		})
	}
}

func TestTruncatedAndIncompleteCallsNeverSucceed(t *testing.T) {
	for _, fixture := range []string{
		"data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n",
		"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call\",\"function\":{\"name\":\"write\",\"arguments\":\"{\"}}]},\"finish_reason\":\"length\"}]}\n\ndata: [DONE]\n\n",
		"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call\",\"function\":{\"name\":\"write\",\"arguments\":\"{\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n",
	} {
		result, err := decodeStream(strings.NewReader(fixture), chatAdapter{}.decoder(), nil)
		if err == nil || len(result.Message.Calls) != 0 {
			t.Fatalf("unsafe tool response accepted: %+v, %v", result, err)
		}
	}
}

func TestProtocolHistoryPreservesToolIdentities(t *testing.T) {
	history := []Message{{Role: "user", Text: "Read it"}, {Role: "assistant", Calls: []Call{{ID: "call-1", Name: "read", Arguments: `{"path":"one.txt"}`}}}, {Role: "tool", CallID: "call-1", Text: "complete result"}}
	for _, protocol := range []Protocol{Chat, Responses, Messages} {
		request := requestBody(t, protocol, Model{ID: "model"}, Request{History: history})
		data, err := json.Marshal(request)
		if err != nil || !strings.Contains(string(data), "call-1") || !strings.Contains(string(data), "complete result") {
			t.Fatalf("%s: %s %v", protocol, data, err)
		}
	}
}

func TestReasoningRequiresExplicitModelCapability(t *testing.T) {
	for _, protocol := range []Protocol{Chat, Responses, Messages} {
		body := requestBody(t, protocol, Model{ID: "m"}, Request{})
		for _, key := range []string{"reasoning_effort", "reasoning", "thinking", "output_config"} {
			if _, ok := body[key]; ok {
				t.Fatalf("%s sent unsupported reasoning", protocol)
			}
		}
		var wire adapter
		switch protocol {
		case Chat:
			wire = chatAdapter{}
		case Responses:
			wire = responsesAdapter{}
		case Messages:
			wire = messagesAdapter{}
		}
		if _, err := wire.encode(Request{Effort: "High"}, adapterTarget{model: Model{ID: "m"}}); err == nil {
			t.Fatalf("%s silently ignored unsupported effort", protocol)
		}
	}
}

func TestRedirectDoesNotForwardCredentials(t *testing.T) {
	called := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	provider := Provider{ID: "p", Name: "P", Protocol: Messages, BaseURL: source.URL, APIKey: "test-secret", Models: []Model{{ID: "m"}}}
	gateway, err := NewClient().Bind(provider, provider.Models[0])
	if err != nil {
		t.Fatal(err)
	}
	_, err = gateway.Stream(context.Background(), Request{}, nil)
	if err == nil || called {
		t.Fatal("redirect forwarded a credential-bearing request")
	}
}

func TestCompleteContentBlocksAndResponsesWithoutDeltas(t *testing.T) {
	anthropic := "data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"call\",\"name\":\"read\",\"input\":{\"path\":\"real.txt\"}}}\n\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"}}\n\ndata: {\"type\":\"message_stop\"}\n\n"
	result, err := decodeStream(strings.NewReader(anthropic), messagesAdapter{}.decoder(), nil)
	if err != nil || len(result.Message.Calls) != 1 || result.Message.Calls[0].Arguments != `{"path":"real.txt"}` {
		t.Fatalf("%+v %v", result, err)
	}
	responses := "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"final text\"}]}]}}\n\n"
	var displayed string
	result, err = decodeStream(
		strings.NewReader(responses), responsesAdapter{}.decoder(),
		func(d Delta) error { displayed += d.Text; return nil },
	)
	if err != nil || displayed != "final text" || result.Message.Text != "final text" {
		t.Fatalf("%+v %v", result, err)
	}
}

func requestBody(t *testing.T, protocol Protocol, selected Model, request Request) map[string]any {
	t.Helper()
	provider := Provider{
		ID: "p", Name: "Custom", Protocol: protocol,
		BaseURL: "https://model.example/v1", Models: []Model{selected},
	}
	gateway, err := NewClient().Bind(provider, selected)
	if err != nil {
		t.Fatal(err)
	}
	body, err := gateway.adapter.encode(request, gateway.target)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
