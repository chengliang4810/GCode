package model

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"gcode/internal/privacy"
	"gcode/third_party/magpie/redact"
)

func privacyEvent(w io.Writer, value any) {
	data, _ := json.Marshal(value)
	fmt.Fprintf(w, "data: %s\n\n", data)
}

func privacyAnswer(w http.ResponseWriter, protocol Protocol, text, arguments string) {
	w.Header().Set("Content-Type", "text/event-stream")
	pieces := []string{text[:len(text)/2], text[len(text)/2:]}
	switch protocol {
	case Chat:
		for _, part := range pieces {
			privacyEvent(w, map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"content": part}}}})
		}
		privacyEvent(w, map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{
			map[string]any{"index": 0, "id": "call-private", "function": map[string]any{"name": "save", "arguments": arguments}},
		}}, "finish_reason": "tool_calls"}}})
		fmt.Fprint(w, "data: [DONE]\n\n")
	case Responses:
		for _, part := range pieces {
			privacyEvent(w, map[string]any{"type": "response.output_text.delta", "delta": part})
		}
		privacyEvent(w, map[string]any{"type": "response.output_item.added", "output_index": 1,
			"item": map[string]any{"type": "function_call", "call_id": "call-private", "name": "save"}})
		privacyEvent(w, map[string]any{"type": "response.function_call_arguments.delta", "output_index": 1, "delta": arguments})
		privacyEvent(w, map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed"}})
	case Messages:
		privacyEvent(w, map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "thinking", "thinking": text, "signature": "unchanged-signature"}})
		privacyEvent(w, map[string]any{"type": "content_block_start", "index": 1, "content_block": map[string]any{"type": "text", "text": ""}})
		for _, part := range pieces {
			privacyEvent(w, map[string]any{"type": "content_block_delta", "index": 1, "delta": map[string]any{"type": "text_delta", "text": part}})
		}
		privacyEvent(w, map[string]any{"type": "content_block_start", "index": 2, "content_block": map[string]any{"type": "tool_use", "id": "call-private", "name": "save", "input": map[string]any{}}})
		privacyEvent(w, map[string]any{"type": "content_block_delta", "index": 2, "delta": map[string]any{"type": "input_json_delta", "partial_json": arguments}})
		privacyEvent(w, map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "tool_use"}})
		privacyEvent(w, map[string]any{"type": "message_stop"})
	}
}

func TestPrivacyThreeProtocolsMaskRestoreAndReplay(t *testing.T) {
	for _, protocol := range []Protocol{Chat, Responses, Messages} {
		t.Run(string(protocol), func(t *testing.T) {
			secret := "private-value-" + string(protocol) + `-"quoted"\path`
			var calls int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				body, _ := io.ReadAll(r.Body)
				var value any
				_ = json.Unmarshal(body, &value)
				all, _ := json.Marshal(value)
				encodedSecret, _ := json.Marshal(secret)
				if strings.Contains(string(all), string(encodedSecret[1:len(encodedSecret)-1])) {
					t.Error("original sensitive text reached the provider")
				}
				if !strings.Contains(string(body), `"save"`) || !strings.Contains(string(body), `"model"`) {
					t.Error("protocol/tool identifiers were changed")
				}
				placeholder := regexp.MustCompile(`\{\{TERM_[a-z2-7]{8}\}\}`).FindString(string(body))
				if placeholder == "" {
					t.Error("masked request has no placeholder")
					http.Error(w, "missing placeholder", 400)
					return
				}
				if protocol == Messages && calls == 2 && !strings.Contains(string(body), "unchanged-signature") {
					t.Error("signed reasoning lost on replay")
				}
				args, _ := json.Marshal(map[string]string{"path": placeholder})
				privacyAnswer(w, protocol, placeholder, string(args))
			}))
			defer server.Close()
			provider := Provider{ID: "private", Name: "Private", Protocol: protocol, BaseURL: server.URL, APIKey: "transport-auth", Models: []Model{{ID: "model"}}}
			gateway, err := newTestClient().Bind(provider, provider.Models[0])
			if err != nil {
				t.Fatal(err)
			}
			gateway.SetPrivacy(privacy.Settings{Enabled: true, Words: []string{secret}})
			request := Request{System: "Discuss " + secret, History: []Message{{Role: "user", Text: secret}},
				Tools: []Tool{{Name: "save", Description: "Save " + secret, Parameters: map[string]any{"type": "object"}}}}
			var streamed strings.Builder
			result, err := gateway.Stream(t.Context(), request, func(delta Delta) error {
				streamed.WriteString(delta.Text)
				return nil
			})
			if err != nil || result.Message.Text != secret || streamed.String() != secret {
				t.Fatalf("restoration result=%q stream=%q err=%v", result.Message.Text, streamed.String(), err)
			}
			if len(result.Message.Calls) != 1 {
				t.Fatalf("missing restored call: %+v", result)
			}
			var args map[string]string
			if err := json.Unmarshal([]byte(result.Message.Calls[0].Arguments), &args); err != nil || args["path"] != secret {
				t.Fatalf("tool arguments were not safely restored: %s %v", result.Message.Calls[0].Arguments, err)
			}
			if protocol == Messages && (result.Message.Reasoning != secret || !strings.Contains(string(result.Message.Continuation.data), "{{TERM_")) {
				t.Fatal("visible reasoning and signed replay do not have separate ownership")
			}
			request.History = append(request.History, result.Message, Message{Role: "tool", CallID: "call-private", Text: secret})
			if _, err := gateway.Stream(t.Context(), request, nil); err != nil {
				t.Fatal("replay failed:", err)
			}
			if request.History[0].Text != secret {
				t.Fatal("masking mutated caller-owned history")
			}
		})
	}
}

func TestPrivacyOffWarnBlockAndInvalidConfiguration(t *testing.T) {
	for _, mode := range []string{"off", "warn", "block", "invalid"} {
		t.Run(mode, func(t *testing.T) {
			secret := "sk-proj-" + strings.Repeat("x7Qp2m", 6) + mode
			var requests atomic.Int32
			var report privacy.Report
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				body, _ := io.ReadAll(r.Body)
				if !strings.Contains(string(body), secret) {
					t.Error("disabled/warn policy changed the original value")
				}
				fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			}))
			defer server.Close()
			p := Provider{ID: "test", Name: "Test", Protocol: Chat, BaseURL: server.URL, Models: []Model{{ID: "model"}}}
			g, _ := newTestClient().Bind(p, p.Models[0])
			settings := privacy.Settings{Enabled: mode != "off", Secrets: true}
			if mode == "invalid" {
				settings.Rules = []redact.Rule{{Kind: "BAD", Regex: "["}}
			} else if mode != "off" {
				settings.Actions = map[string]string{"API_KEY": mode}
			}
			g.SetPrivacy(settings)
			g.PrivacyReport = func(value privacy.Report) { report = value }
			_, err := g.Stream(context.Background(), Request{History: []Message{{Role: "user", Text: secret}}}, nil)
			blocked := mode == "block" || mode == "invalid"
			if (err != nil) != blocked || (requests.Load() == 0) != blocked {
				t.Fatalf("policy=%s sent=%d err=%v", mode, requests.Load(), err)
			}
			if mode == "warn" && report.Warned == 0 || mode == "block" && report.Blocked == 0 {
				t.Fatalf("missing policy report: %+v", report)
			}
		})
	}
}

func TestPrivacyNamedToolSecretRespectsActionsAndAllowlist(t *testing.T) {
	for _, action := range []string{"mask", "warn", "block", "allow"} {
		t.Run(action, func(t *testing.T) {
			value := "tool-private-password-" + action
			body, _ := json.Marshal(map[string]any{"messages": []any{map[string]any{"role": "assistant", "content": []any{
				map[string]any{"type": "tool_use", "name": "login", "id": "c", "input": map[string]string{"password": value}},
			}}}})
			g := Gateway{}
			settings := privacy.Settings{Enabled: true, Secrets: true, Actions: map[string]string{"SECRET": action}}
			if action == "allow" {
				settings.Actions = nil
				settings.Allow = []string{value}
			}
			g.SetPrivacy(settings)
			out, err := g.maskRequest(t.Context(), body)
			if action == "block" {
				if err == nil {
					t.Fatal("named secret bypassed block policy")
				}
			} else if err != nil || strings.Contains(string(out), value) != (action == "allow" || action == "warn") {
				t.Fatalf("action=%s err=%v body=%s", action, err, out)
			}
		})
	}
}
