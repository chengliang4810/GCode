package model

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestDiagnosticsFingerprintActualWireAndPreserveRetries(t *testing.T) {
	for _, protocol := range []Protocol{Chat, Responses, Messages} {
		t.Run(string(protocol), func(t *testing.T) {
			provider := Provider{ID: "p", Name: "P", Protocol: protocol, APIKey: "secret", BaseURL: "http://fixture", Models: []Model{{ID: "m"}}}
			gateway, err := newTestClient().Bind(provider, provider.Models[0])
			if err != nil {
				t.Fatal(err)
			}
			request := Request{System: "stable system", Tools: []Tool{{Name: "Read", Parameters: map[string]any{"type": "object"}}}, History: []Message{
				{Role: "user", Text: "inspect", Images: []Image{{MIME: "image/png", Data: "image-one"}}},
			}}
			payload, err := gateway.adapter.encode(request, gateway.target)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := json.Marshal(payload)
			first := requestDiagnostics(payload, body, request)
			if first.Bytes != len(body) || first.SHA256 != diagnosticHash(body) || first.ImageBytes != len("image-one") || len(first.MessageHashes) == 0 {
				t.Fatal("wire metadata lost content or length")
			}
			request.History[0].Images[0].Data = "image-two"
			payload, _ = gateway.adapter.encode(request, gateway.target)
			body, _ = json.Marshal(payload)
			second := requestDiagnostics(payload, body, request)
			if first.SHA256 == second.SHA256 || first.ImageHashes[0] == second.ImageHashes[0] ||
				first.MessageHashes[len(first.MessageHashes)-1] == second.MessageHashes[len(second.MessageHashes)-1] {
				t.Fatal("changed image was invisible to diagnostics")
			}
			request.System += "changed"
			payload, _ = gateway.adapter.encode(request, gateway.target)
			body, _ = json.Marshal(payload)
			if first.SystemHash == requestDiagnostics(payload, body, request).SystemHash {
				t.Fatal("system changes were invisible")
			}
			encoded, _ := json.Marshal(second)
			if strings.Contains(string(encoded), "image-two") || strings.Contains(string(encoded), "secret") {
				t.Fatal("diagnostics duplicated raw payload or credentials")
			}
		})
	}
}

func TestTransportDiagnosticsDistinguishUploadHeadersAndRequestID(t *testing.T) {
	attempts := 0
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		var body json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, string(body))
		w.Header().Set("x-request-id", "upstream-request")
		if attempts == 1 {
			w.WriteHeader(503)
			return
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"done\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":2,\"prompt_cache_hit_tokens\":8}}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	provider := Provider{ID: "p", Name: "P", Protocol: Chat, BaseURL: server.URL, Models: []Model{{ID: "m"}}}
	gateway, err := newTestClient().Bind(provider, provider.Models[0])
	if err != nil {
		t.Fatal(err)
	}
	var records []Invocation
	gateway.Observe = func(invocation Invocation) { records = append(records, invocation) }
	_, err = gateway.Stream(context.Background(), Request{History: []Message{{Role: "user", Text: "task"}}}, nil)
	if err != nil || len(records) != 4 || len(bodies) != 2 || bodies[0] != bodies[1] {
		t.Fatalf("physical retry changed its body: %v records=%d", err, len(records))
	}
	first, second := records[1], records[3]
	if first.Request.SHA256 != second.Request.SHA256 || second.Request.SHA256 != diagnosticHash([]byte(bodies[1])) ||
		second.Transport.UploadCompleteMS == nil || second.Transport.ResponseHeadersMS == nil || second.Transport.RequestID != "upstream-request" {
		t.Fatalf("transport diagnostics incomplete: %+v", second.Transport)
	}
}
