package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestModernStdioDiscoveryAndToolCall(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	client, err := ConnectWithOptions(context.Background(), Config{Name: "child", Command: exe, Args: []string{"-test.run=^TestMCPProtocolChild$", "--", "modern"}, Env: map[string]string{"GCODE_MCP_PROTOCOL_FIXTURE": "enabled-protocol-fixture"}}, t.TempDir(), Options{ProtocolVersion: ModernProtocolVersion})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if client.ProtocolVersion() != ModernProtocolVersion {
		t.Fatal(client.ProtocolVersion())
	}
	tools, err := client.Tools(context.Background())
	if err != nil || len(tools) != 1 {
		t.Fatalf("%v %v", tools, err)
	}
	output, err := tools[0].Execute(context.Background(), "", json.RawMessage(`{}`))
	if err != nil || output.Text != "child result\n" {
		t.Fatalf("%v %v", output, err)
	}
}
func TestStdioCancellationNotifiesServerAndConnectionRemainsUsable(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	client, err := Connect(context.Background(), Config{Name: "child", Command: exe, Args: []string{"-test.run=^TestMCPProtocolChild$", "--", "cancel"}, Env: map[string]string{"GCODE_MCP_PROTOCOL_FIXTURE": "enabled-protocol-fixture"}}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	tools, err := client.Tools(context.Background())
	if err != nil || len(tools) != 1 {
		t.Fatalf("%v %v", tools, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { _, err := tools[0].Execute(ctx, "", json.RawMessage(`{}`)); result <- err }()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	select {
	case event := <-client.Notifications():
		if event.Method != "fixture/tool_started" {
			t.Fatal(event)
		}
	case <-deadline.C:
		t.Fatal("stdio request did not reach child")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("stdio request was not cancelled")
	}
	select {
	case event := <-client.Notifications():
		if event.Method != "fixture/cancellation_received" {
			t.Fatal(event)
		}
	case <-time.After(time.Second):
		t.Fatal("child did not receive notifications/cancelled")
	}
	if _, err = client.Tools(context.Background()); err != nil {
		t.Fatalf("cancelling one request killed its server: %v", err)
	}
	if err = client.Close(); err != nil {
		t.Fatal(err)
	}
	if _, open := <-client.Notifications(); open {
		t.Fatal("notification stream left open after Close")
	}
}

func TestStdioHostScratchPathsRemainUsableWhileExplicitCredentialsStayPrivate(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	scratch := t.TempDir()
	artifact := filepath.Join(scratch, "artifact.json")
	artifactURI := (&url.URL{Scheme: "file", Path: "/" + strings.TrimPrefix(filepath.ToSlash(artifact), "/")}).String()
	config := Config{
		Name:    "scratch",
		Command: exe,
		Args:    []string{"-test.run=^TestMCPProtocolChild$", "--", "scratch"},
		ProcessEnv: map[string]string{
			"TMPDIR": scratch,
			"TEMP":   scratch,
			"TMP":    scratch,
		},
		Env: map[string]string{
			"GCODE_MCP_PROTOCOL_FIXTURE": "enabled-protocol-fixture",
			"TEMP":                       "explicit-temp-override",
			"TOKEN":                      "saved-token-private",
			"LIST_MEMBERS":               `["list-member-private","another-member-private"]`,
		},
		SensitiveValues: []string{"list-member-private", "another-member-private"},
	}
	client, err := Connect(context.Background(), config, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	config.ProcessEnv["TMPDIR"] = "caller-mutated"
	if client.config.ProcessEnv["TMPDIR"] != scratch {
		t.Fatal("connection retained caller-owned process environment")
	}
	tools, err := client.Tools(context.Background())
	if err != nil || len(tools) != 1 {
		t.Fatalf("host scratch path in schema must remain usable: %v %v", tools, err)
	}
	properties, _ := tools[0].Definition().Parameters["properties"].(map[string]any)
	pathProperty, _ := properties["path"].(map[string]any)
	if pathProperty["default"] != artifact {
		t.Fatalf("schema lost scratch path: %v", pathProperty)
	}
	output, err := tools[0].Execute(context.Background(), "", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(output.Data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.Text, artifact) || !strings.Contains(output.Text, artifactURI) {
		t.Fatalf("text lost usable artifact path or URI: %s", output.Text)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	structured, _ := result["structuredContent"].(map[string]any)
	if structured["path"] != artifact || structured["uri"] != artifactURI || structured["tmp"] != scratch {
		t.Fatalf("structured artifact references changed: %s", data)
	}
	if structured["temp"] != "[redacted]" || structured["token"] != "[redacted]" || structured["member"] != "[redacted]" {
		t.Fatalf("explicit environment priority or credential redaction changed: %s", data)
	}
	for _, secret := range []string{"saved-token-private", "list-member-private", "explicit-temp-override"} {
		if strings.Contains(output.Text, secret) || strings.Contains(string(data), secret) {
			t.Fatalf("explicit credential leaked: %s %s", output.Text, data)
		}
	}
	body, err := os.ReadFile(artifact)
	if err != nil || string(body) != "{}" {
		t.Fatalf("child did not create its artifact in host scratch: %s %v", body, err)
	}
}

func TestMCPProtocolChild(t *testing.T) {
	if os.Getenv("GCODE_MCP_PROTOCOL_FIXTURE") != "enabled-protocol-fixture" {
		return
	}
	mode := os.Args[len(os.Args)-1]
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var request envelope
		if json.Unmarshal(scanner.Bytes(), &request) != nil {
			os.Exit(3)
		}
		if request.Method == "notifications/cancelled" {
			_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "method": "fixture/cancellation_received", "params": map[string]any{}})
			continue
		}
		if request.ID == 0 {
			continue
		}
		var result any
		switch request.Method {
		case "server/discover":
			params, _ := request.Params.(map[string]any)
			meta, _ := params["_meta"].(map[string]any)
			if meta["io.modelcontextprotocol/protocolVersion"] != ModernProtocolVersion {
				os.Exit(4)
			}
			result = map[string]any{"supportedVersions": []string{ModernProtocolVersion}, "capabilities": map[string]any{"tools": map[string]any{}}, "resultType": "complete"}
		case "initialize":
			if mode == "modern" {
				os.Exit(5)
			}
			result = map[string]any{"protocolVersion": LatestHandshakeVersion}
		case "tools/list":
			schema := map[string]any{"type": "object"}
			if mode == "scratch" {
				schema["properties"] = map[string]any{"path": map[string]any{"type": "string", "default": filepath.Join(os.Getenv("TMPDIR"), "artifact.json")}}
			}
			result = map[string]any{"tools": []any{map[string]any{"name": "echo", "inputSchema": schema}}}
		case "tools/call":
			if mode == "cancel" {
				_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "method": "fixture/tool_started", "params": map[string]any{}})
				continue
			}
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": "child result"}}}
			if mode == "scratch" {
				artifact := filepath.Join(os.Getenv("TMPDIR"), "artifact.json")
				if err := os.WriteFile(artifact, []byte("{}"), 0600); err != nil {
					os.Exit(7)
				}
				artifactURI := (&url.URL{Scheme: "file", Path: "/" + strings.TrimPrefix(filepath.ToSlash(artifact), "/")}).String()
				var members []string
				if err := json.Unmarshal([]byte(os.Getenv("LIST_MEMBERS")), &members); err != nil || len(members) == 0 {
					os.Exit(8)
				}
				structured := map[string]any{"path": artifact, "uri": artifactURI, "tmp": os.Getenv("TMP"), "token": os.Getenv("TOKEN"), "member": members[0], "temp": os.Getenv("TEMP")}
				result = map[string]any{
					"content":           []any{map[string]any{"type": "text", "text": strings.Join([]string{artifact, artifactURI, os.Getenv("TOKEN"), members[0], os.Getenv("TEMP")}, "\n")}},
					"structuredContent": structured,
				}
			}
		default:
			os.Exit(6)
		}
		raw, _ := json.Marshal(result)
		_ = encoder.Encode(envelope{JSONRPC: "2.0", ID: request.ID, Result: raw})
	}
	os.Exit(0)
}
