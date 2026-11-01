package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestContinuationPersistsAndOnlyReplaysToItsTarget(t *testing.T) {
	fixtures := []struct {
		name     string
		protocol Protocol
		stream   string
		sealed   string
	}{
		{
			name: "responses", protocol: Responses, sealed: "sealed-reasoning",
			stream: `data: {"type":"response.completed","response":{"status":"completed","output":[{"type":"reasoning","id":"reason-1","encrypted_content":"sealed-reasoning"},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"visible"}]},{"type":"function_call","call_id":"call-1","name":"read","arguments":"{\"path\":\"proof.txt\"}"}]}}` + "\n\n",
		},
		{
			name: "messages", protocol: Messages, sealed: "sealed-signature",
			stream: `data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"first reasoning","signature":"sealed-signature-one"}}` + "\n\n" +
				`data: {"type":"content_block_start","index":1,"content_block":{"type":"redacted_thinking","data":"redacted-payload"}}` + "\n\n" +
				`data: {"type":"content_block_start","index":2,"content_block":{"type":"thinking","thinking":"second reasoning"}}` + "\n\n" +
				`data: {"type":"content_block_delta","index":2,"delta":{"type":"signature_delta","signature":"sealed-signature-two"}}` + "\n\n" +
				`data: {"type":"content_block_start","index":3,"content_block":{"type":"text","text":"visible"}}` + "\n\n" +
				`data: {"type":"content_block_start","index":4,"content_block":{"type":"tool_use","id":"call-1","name":"read","input":{"path":"proof.txt"}}}` + "\n\n" +
				`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"}}` + "\n\n" +
				`data: {"type":"message_stop"}` + "\n\n",
		},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			requests := make(chan []byte, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				requests <- body
				if strings.HasSuffix(r.URL.Path, "/chat/completions") {
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
				} else {
					fmt.Fprint(w, fixture.stream)
				}
			}))
			defer server.Close()
			provider := Provider{
				ID: "original", Name: "Custom", Protocol: fixture.protocol, BaseURL: server.URL,
				APIKey: "channel-credential", Models: []Model{{ID: "model"}},
			}
			first, err := NewClient().Bind(provider, provider.Models[0])
			if err != nil {
				t.Fatal(err)
			}
			result, err := first.Stream(context.Background(), Request{}, nil)
			<-requests
			if err != nil || result.Stop != StopTools || result.Message.Continuation == nil {
				t.Fatalf("missing complete tool result/continuation: %+v %v", result, err)
			}
			disk, err := json.Marshal(result.Message)
			if err != nil || bytes.Contains(disk, []byte(provider.APIKey)) {
				t.Fatalf("cannot safely persist continuation: %s %v", disk, err)
			}
			var restored Message
			if err := json.Unmarshal(disk, &restored); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"same target", "provider", "model", "protocol", "endpoint", "credential"} {
				t.Run(name, func(t *testing.T) {
					target := provider
					target.Models = []Model{{ID: "model"}}
					switch name {
					case "provider":
						target.ID = "other-provider"
					case "model":
						target.Models[0].ID = "other-model"
					case "protocol":
						target.Protocol = Chat
					case "endpoint":
						target.BaseURL += "/another-endpoint"
					case "credential":
						target.APIKey = "another-credential"
					}
					gateway, err := NewClient().Bind(target, target.Models[0])
					if err != nil {
						t.Fatal(err)
					}
					history := CloneMessages([]Message{
						restored, {Role: "tool", CallID: "call-1", Text: "tool output"},
					})
					_, err = gateway.Stream(context.Background(), Request{History: history}, nil)
					body := <-requests
					if err != nil {
						t.Fatal(err)
					}
					same := name == "same target"
					if bytes.Contains(body, []byte(fixture.sealed)) != same {
						t.Fatalf("replay target isolation failed: %s", body)
					}
					if same && fixture.protocol == Messages {
						first := bytes.Index(body, []byte("sealed-signature-one"))
						redacted := bytes.Index(body, []byte("redacted-payload"))
						second := bytes.Index(body, []byte("sealed-signature-two"))
						if first < 0 || redacted <= first || second <= redacted {
							t.Fatalf("reasoning block order/signatures changed: %s", body)
						}
					}
					if bytes.Count(body, []byte(`"call-1"`)) != 2 ||
						bytes.Count(body, []byte(`"visible"`)) != 1 {
						t.Fatalf("message or tool identity was duplicated/lost: %s", body)
					}
					for _, text := range []string{"visible", "call-1", "proof.txt", "tool output"} {
						if !bytes.Contains(body, []byte(text)) {
							t.Fatalf("target switch lost execution history %q: %s", text, body)
						}
					}
				})
			}
		})
	}
}

func TestCloneMessagesOwnsContinuationAndSlices(t *testing.T) {
	original := []Message{{
		Calls: []Call{{ID: "call"}}, Images: []Image{{Data: "original"}},
		Continuation: &Continuation{scope: "scope", data: json.RawMessage(`["original"]`)},
	}}
	cloned := CloneMessages(original)
	cloned[0].Calls[0].ID = "changed"
	cloned[0].Images[0].Data = "changed"
	cloned[0].Continuation.data[2] = 'X'
	if original[0].Calls[0].ID != "call" || original[0].Images[0].Data != "original" ||
		string(original[0].Continuation.data) != `["original"]` {
		t.Fatal("worker mutation changed the persisted source history")
	}
}
