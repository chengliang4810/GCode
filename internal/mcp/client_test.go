package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestStreamableHTTPTools(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Header.Get("Authorization") != "Bearer local-test" {
			t.Error("missing configured header")
		}
		var request envelope
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		if request.Method == "notifications/initialized" {
			w.WriteHeader(202)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Mcp-Session-Id", "local-session")
		var result any
		switch request.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]any{"name": "fixture", "version": "1"}}
		case "tools/list":
			result = map[string]any{"tools": []any{map[string]any{"name": "echo", "description": "Echo", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"text": map[string]string{"type": "string"}}}}}}
		case "tools/call":
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": "actual MCP result"}}}
		default:
			t.Error("unexpected method " + request.Method)
		}
		data, _ := json.Marshal(result)
		_ = json.NewEncoder(w).Encode(envelope{JSONRPC: "2.0", ID: request.ID, Result: data})
	}))
	defer server.Close()
	client, err := Connect(context.Background(), Config{Name: "fixture", URL: server.URL, Headers: map[string]string{"Authorization": "Bearer local-test"}}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	tools, err := client.Tools(context.Background())
	if err != nil || len(tools) != 1 {
		t.Fatalf("%+v %v", tools, err)
	}
	output, err := tools[0].Execute(context.Background(), "", json.RawMessage(`{"text":"hello"}`))
	if err != nil || output.Text != "actual MCP result\n" {
		t.Fatalf("%s %v", output.Text, err)
	}
}

func TestStdioMCPToolsAndCleanup(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	client, err := Connect(context.Background(), Config{Name: "stdio", Command: executable, Args: []string{"-test.run=^TestMCPHelper$"}, Env: map[string]string{"GCODE_TEST_MCP_HELPER": "1"}}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tools, err := client.Tools(context.Background())
	if err != nil || len(tools) != 1 {
		t.Fatalf("%+v %v", tools, err)
	}
	output, err := tools[0].Execute(context.Background(), "", json.RawMessage(`{}`))
	if err != nil || output.Text != "stdio result\n" {
		t.Fatalf("%s %v", output.Text, err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-client.processDone:
	default:
		t.Fatal("stdio child not reaped")
	}
}
func TestMCPHelper(t *testing.T) {
	if os.Getenv("GCODE_TEST_MCP_HELPER") != "1" {
		return
	}
	if proof := os.Getenv("GCODE_TEST_MCP_ENV_PROOF"); proof != "" {
		data, _ := json.Marshal(map[string]string{"TEMP": os.Getenv("TEMP"), "TMP": os.Getenv("TMP"), "TMPDIR": os.Getenv("TMPDIR")})
		if err := os.WriteFile(proof, data, 0600); err != nil {
			os.Exit(2)
		}
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var request envelope
		if json.Unmarshal(scanner.Bytes(), &request) != nil {
			continue
		}
		if request.ID == 0 {
			continue
		}
		var result any
		switch request.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "serverInfo": map[string]string{"name": "helper", "version": "1"}}
		case "tools/list":
			result = map[string]any{"tools": []any{map[string]any{"name": "echo", "inputSchema": map[string]any{"type": "object"}}}}
		case "tools/call":
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": "stdio result"}}}
		}
		raw, _ := json.Marshal(result)
		data, _ := json.Marshal(envelope{JSONRPC: "2.0", ID: request.ID, Result: raw})
		fmt.Println(string(data))
	}
	os.Exit(0)
}
