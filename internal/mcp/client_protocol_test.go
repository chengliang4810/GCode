package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func fixtureResult(w http.ResponseWriter, request envelope, result any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
}
func fixtureRequest(t *testing.T, r *http.Request) envelope {
	t.Helper()
	var request envelope
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		t.Error(err)
	}
	return request
}
func fixtureTool() map[string]any {
	return map[string]any{"name": "echo", "inputSchema": map[string]any{"type": "object"}}
}

func TestHTTPNegotiationPaginationResourcesPromptsAndNotifications(t *testing.T) {
	var deleted atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleted.Store(true)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		request := fixtureRequest(t, r)
		if request.Method != "initialize" && r.Header.Get("MCP-Protocol-Version") != "2024-11-05" {
			t.Errorf("negotiated revision not sent: %q", r.Header.Get("MCP-Protocol-Version"))
		}
		switch request.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "fixture-session")
			fixtureResult(w, request, map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{"resources": map[string]any{}, "prompts": map[string]any{}}})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			params := request.Params.(map[string]any)
			if params["cursor"] == nil {
				fixtureResult(w, request, map[string]any{"tools": []any{fixtureTool()}, "nextCursor": "page-2"})
				return
			}
			if params["cursor"] != "page-2" {
				t.Error("cursor not relayed")
			}
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, ": keepalive\n\nevent: message\ndata: {\"jsonrpc\":\"2.0\",\ndata: \"method\":\"notifications/tools/list_changed\",\"params\":{}}\n\n")
			fmt.Fprintf(w, "data: {\"jsonrpc\":\"2.0\",\"id\":%d,\n", request.ID)
			fmt.Fprint(w, "data: \"result\":{\"tools\":[]}}\n\n")
		case "resources/list":
			fixtureResult(w, request, map[string]any{"resources": []any{map[string]any{"name": "readme", "uri": "fixture://readme", "mimeType": "text/plain"}}})
		case "resources/templates/list":
			fixtureResult(w, request, map[string]any{"resourceTemplates": []any{map[string]any{"name": "files", "uriTemplate": "fixture://files/{name}"}}})
		case "resources/read":
			fixtureResult(w, request, map[string]any{"contents": []any{map[string]any{"uri": "fixture://readme", "mimeType": "text/plain", "text": "resource text"}}})
		case "prompts/list":
			fixtureResult(w, request, map[string]any{"prompts": []any{map[string]any{"name": "explain", "arguments": []any{map[string]any{"name": "subject", "required": true}}}}})
		case "prompts/get":
			fixtureResult(w, request, map[string]any{"description": "fixture prompt", "messages": []any{map[string]any{"role": "user", "content": map[string]any{"type": "text", "text": "explain the fixture"}}}})
		default:
			t.Errorf("unexpected method %s", request.Method)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()
	client, err := Connect(context.Background(), Config{Name: "fixture", Transport: "http", URL: server.URL}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if client.ProtocolVersion() != "2024-11-05" {
		t.Fatal(client.ProtocolVersion())
	}
	tools, err := client.Tools(context.Background())
	if err != nil || len(tools) != 1 || tools[0].Definition().Name != "mcp__fixture__echo" {
		t.Fatalf("tools=%v error=%v", tools, err)
	}
	select {
	case event := <-client.Notifications():
		if event.Method != "notifications/tools/list_changed" {
			t.Fatal(event)
		}
	case <-time.After(time.Second):
		t.Fatal("server notification lost")
	}
	resources, err := client.Resources(context.Background())
	if err != nil || len(resources) != 1 || resources[0].URI != "fixture://readme" {
		t.Fatalf("%v %v", resources, err)
	}
	templates, err := client.ResourceTemplates(context.Background())
	if err != nil || len(templates) != 1 || templates[0].URITemplate != "fixture://files/{name}" {
		t.Fatalf("%v %v", templates, err)
	}
	contents, err := client.ReadResource(context.Background(), resources[0].URI)
	if err != nil || len(contents) != 1 || contents[0].Text != "resource text" {
		t.Fatalf("%v %v", contents, err)
	}
	prompts, err := client.Prompts(context.Background())
	if err != nil || len(prompts) != 1 || len(prompts[0].Arguments) != 1 {
		t.Fatalf("%v %v", prompts, err)
	}
	prompt, err := client.GetPrompt(context.Background(), "explain", map[string]string{"subject": "fixture"})
	if err != nil || len(prompt.Messages) != 1 || prompt.Messages[0].Content["text"] != "explain the fixture" {
		t.Fatalf("%v %v", prompt, err)
	}
	if err = client.Close(); err != nil || !deleted.Load() {
		t.Fatalf("session was not terminated: %v", err)
	}
}

func TestLegacySSEActualConnectionAndCleanup(t *testing.T) {
	events := make(chan string, 16)
	streamClosed := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			defer close(streamClosed)
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "event: endpoint\ndata: /messages?session=fixture-session-id\n\n")
			w.(http.Flusher).Flush()
			for {
				select {
				case data := <-events:
					fmt.Fprintf(w, "event: message\ndata: %s\n\n", data)
					w.(http.Flusher).Flush()
				case <-r.Context().Done():
					return
				}
			}
		}
		if r.URL.Path != "/messages" || r.URL.Query().Get("session") != "fixture-session-id" {
			t.Error("POST did not use SSE endpoint event")
		}
		request := fixtureRequest(t, r)
		w.WriteHeader(http.StatusAccepted)
		if request.ID == 0 {
			return
		}
		var result any
		switch request.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2024-11-05"}
		case "tools/list":
			result = map[string]any{"tools": []any{fixtureTool()}}
		case "tools/call":
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": "SSE actual result"}}}
		default:
			t.Error(request.Method)
		}
		data, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
		events <- string(data)
	}))
	defer server.Close()
	client, err := Connect(context.Background(), Config{Name: "sse", Transport: "sse", URL: server.URL + "/sse"}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tools, err := client.Tools(context.Background())
	if err != nil || len(tools) != 1 {
		t.Fatalf("%v %v", tools, err)
	}
	result, err := tools[0].Execute(context.Background(), "", json.RawMessage(`{}`))
	if err != nil || result.Text != "SSE actual result\n" {
		t.Fatalf("%v %v", result, err)
	}
	if err = client.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-streamClosed:
	case <-time.After(time.Second):
		t.Fatal("legacy SSE stream leaked after Close")
	}
}

func TestLegacySSERejectsCrossOriginEndpointWithoutSendingCredentials(t *testing.T) {
	var forwarded atomic.Int64
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded.Add(1)
		t.Error("cross-origin endpoint was contacted")
	}))
	defer other.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "event: endpoint\ndata: %s/messages\n\n", other.URL)
	}))
	defer server.Close()
	client, err := Connect(context.Background(), Config{Name: "bad-endpoint", Transport: "sse", URL: server.URL, Headers: map[string]string{"Authorization": "Bearer never-forward-this"}}, t.TempDir())
	if client != nil || err == nil || forwarded.Load() != 0 {
		t.Fatalf("client=%v error=%v forwarded=%d", client, err, forwarded.Load())
	}
}

func TestHTTPToolCancellationClosesResponseAndSignalsServer(t *testing.T) {
	started := make(chan struct{})
	serverCanceled := make(chan struct{})
	cancelNotice := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := fixtureRequest(t, r)
		switch request.Method {
		case "initialize":
			fixtureResult(w, request, map[string]any{"protocolVersion": LatestHandshakeVersion})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			fixtureResult(w, request, map[string]any{"tools": []any{fixtureTool()}})
		case "tools/call":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, ": waiting\n\n")
			w.(http.Flusher).Flush()
			close(started)
			<-r.Context().Done()
			close(serverCanceled)
		case "notifications/cancelled":
			cancelNotice <- struct{}{}
			w.WriteHeader(http.StatusAccepted)
		default:
			t.Error(request.Method)
		}
	}))
	defer server.Close()
	client, err := Connect(context.Background(), Config{Name: "cancel", URL: server.URL}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	tools, err := client.Tools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := tools[0].Execute(ctx, "", json.RawMessage(`{}`)); done <- err }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("tool did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("lost cancellation identity: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("tool cancellation hung")
	}
	select {
	case <-serverCanceled:
	case <-time.After(time.Second):
		t.Fatal("HTTP response body not closed")
	}
	select {
	case <-cancelNotice:
	case <-time.After(time.Second):
		t.Fatal("handshake cancellation notification missing")
	}
}

func fixturePNG(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	img.Set(0, 0, color.RGBA{R: 230, A: 255})
	img.Set(1, 0, color.RGBA{B: 200, A: 255})
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(out.Bytes())
}
func TestModernHTTPMetadataHeadersNamesAndMultimodalContent(t *testing.T) {
	encodedImage := fixturePNG(t)
	var calls atomic.Int64
	secret := "configured-credential-for-fixture"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Error("modern client used a session method")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		request := fixtureRequest(t, r)
		if r.Header.Get("MCP-Protocol-Version") != ModernProtocolVersion || r.Header.Get("Mcp-Method") != request.Method || r.Header.Get("Mcp-Session-Id") != "" {
			t.Error("missing modern metadata headers or leaked legacy session")
		}
		params := request.Params.(map[string]any)
		meta, _ := params["_meta"].(map[string]any)
		if meta["io.modelcontextprotocol/protocolVersion"] != ModernProtocolVersion || meta["io.modelcontextprotocol/clientCapabilities"] == nil {
			t.Error("missing per-request metadata")
		}
		switch request.Method {
		case "server/discover":
			fixtureResult(w, request, map[string]any{"resultType": "complete", "supportedVersions": []string{ModernProtocolVersion}, "capabilities": map[string]any{"tools": map[string]any{}}})
		case "tools/list":
			fixtureResult(w, request, map[string]any{"resultType": "complete", "tools": []any{map[string]any{"name": "look.at", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"count": map[string]any{"type": "integer", "x-mcp-header": "Count"}, "nested": map[string]any{"type": "object", "properties": map[string]any{"project": map[string]any{"type": "string", "x-mcp-header": "Project"}}}}}}}})
		case "tools/call":
			calls.Add(1)
			if r.Header.Get("Mcp-Name") != "look.at" || r.Header.Get("Mcp-Param-Count") != "42" || r.Header.Get("Mcp-Param-Project") != headerValue("中文\nx") {
				t.Errorf("header mirroring failed: %v", r.Header)
			}
			fixtureResult(w, request, map[string]any{"resultType": "complete", "content": []any{map[string]any{"type": "text", "text": "answer " + secret}, map[string]any{"type": "image", "mimeType": "image/png", "data": encodedImage, "annotations": map[string]any{"audience": []string{"user"}}}, map[string]any{"type": "resource", "resource": map[string]any{"uri": "fixture://readme", "mimeType": "text/plain", "text": "embedded resource"}}, map[string]any{"type": "resource_link", "uri": "fixture://linked", "name": "linked"}, map[string]any{"type": "audio", "mimeType": "audio/wav", "data": base64.StdEncoding.EncodeToString([]byte("audio bytes"))}}, "structuredContent": map[string]any{"ok": true, "credential": secret}, "extensionField": map[string]any{"retained": true}})
		default:
			t.Error(request.Method)
		}
	}))
	defer server.Close()
	client, err := ConnectWithOptions(context.Background(), Config{Name: "fixture", Plugin: "my.plugin", URL: server.URL, Headers: map[string]string{"Authorization": "Bearer " + secret}}, t.TempDir(), Options{ProtocolVersion: ModernProtocolVersion})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	tools, err := client.Tools(context.Background())
	if err != nil || len(tools) != 1 {
		t.Fatalf("%v %v", tools, err)
	}
	if tools[0].Definition().Name != "mcp__plugin_my_plugin_fixture__look_at" {
		t.Fatal(tools[0].Definition().Name)
	}
	out, err := tools[0].Execute(context.Background(), "", json.RawMessage(`{"count":42,"nested":{"project":"中文\nx"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Images) != 1 || out.Images[0].Data != encodedImage || out.Images[0].Width != 2 || !strings.Contains(out.Text, "embedded resource") || !strings.Contains(out.Text, "audio content retained") {
		t.Fatalf("content lost: %+v", out)
	}
	data, ok := out.Data.(CallResult)
	if !ok || len(data.Content) != 5 || data.Raw["extensionField"] == nil {
		t.Fatalf("typed response lost: %#v", out.Data)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), secret) || !strings.Contains(string(raw), "[redacted]") {
		t.Fatalf("credential leaked or output not scrubbed: %s", raw)
	}
	if calls.Load() != 1 {
		t.Fatal("tool call retried")
	}
}

func TestToolNameLimitsAndNormalizationCollisions(t *testing.T) {
	name, err := QualifiedToolName(Config{Name: "space.server", Plugin: "my-plugin"}, "lookup.item")
	if err != nil || name != "mcp__plugin_my-plugin_space_server__lookup_item" {
		t.Fatalf("%s %v", name, err)
	}
	long, err := QualifiedToolName(Config{Name: strings.Repeat("s", 40)}, strings.Repeat("t", 30))
	if err != nil || len(long) != 77 {
		t.Fatalf("valid Claude tool between 65 and 128 characters rejected: %q %v", long, err)
	}
	boundary, err := QualifiedToolName(Config{Name: strings.Repeat("s", 40)}, strings.Repeat("t", 81))
	if err != nil || len(boundary) != 128 {
		t.Fatalf("128-character qualified name rejected: %q %v", boundary, err)
	}
	_, err = QualifiedToolName(Config{Name: strings.Repeat("s", 40)}, strings.Repeat("t", 82))
	var limit *ToolNameLimitError
	if !errors.As(err, &limit) || limit.Limit != 128 {
		t.Fatalf("no explicit namespace limit: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := fixtureRequest(t, r)
		switch request.Method {
		case "initialize":
			fixtureResult(w, request, map[string]any{"protocolVersion": LatestHandshakeVersion})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			fixtureResult(w, request, map[string]any{"tools": []any{map[string]any{"name": "read.file", "inputSchema": map[string]any{"type": "object"}}, map[string]any{"name": "read_file", "inputSchema": map[string]any{"type": "object"}}}})
		}
	}))
	defer server.Close()
	client, err := Connect(context.Background(), Config{Name: "collision", URL: server.URL}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err = client.Tools(context.Background()); err == nil || !strings.Contains(err.Error(), "colliding") {
		t.Fatalf("normalization collision silently aliased a tool: %v", err)
	}
}

func TestOverlongToolIsSkippedWithDiagnosticWithoutDisablingOtherTools(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := fixtureRequest(t, r)
		switch request.Method {
		case "initialize":
			fixtureResult(w, request, map[string]any{"protocolVersion": LatestHandshakeVersion})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			fixtureResult(w, request, map[string]any{"tools": []any{
				map[string]any{"name": strings.Repeat("x", 129), "inputSchema": map[string]any{"type": "object"}},
				map[string]any{"name": "echo", "inputSchema": map[string]any{"type": "object"}},
			}})
		case "tools/call":
			fixtureResult(w, request, map[string]any{"content": []any{map[string]any{"type": "text", "text": "still available"}}})
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()
	client, err := Connect(context.Background(), Config{Name: "names", URL: server.URL}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	tools, err := client.Tools(context.Background())
	if err != nil || len(tools) != 1 || tools[0].Definition().Name != "mcp__names__echo" {
		t.Fatalf("one long name disabled its server: %+v %v", tools, err)
	}
	diagnostics := client.ToolDiagnostics()
	if len(diagnostics) != 1 || diagnostics[0].Limit != 128 || diagnostics[0].Length != 141 {
		t.Fatalf("missing rejection evidence: %+v", diagnostics)
	}
	diagnostics[0].Message = "caller changed"
	if client.ToolDiagnostics()[0].Message == "caller changed" {
		t.Fatal("diagnostics retained caller mutation")
	}
	select {
	case event := <-client.Notifications():
		if event.Method != "gcode/tool_rejected" || !strings.Contains(string(event.Params), "128") {
			t.Fatalf("missing rejection notification: %+v", event)
		}
	default:
		t.Fatal("rejection was silently dropped")
	}
	out, err := tools[0].Execute(context.Background(), "", json.RawMessage(`{}`))
	if err != nil || out.Text != "still available\n" {
		t.Fatalf("valid server tool no longer callable: %+v %v", out, err)
	}
}

func TestToolPaginationRejectsRepeatedCursor(t *testing.T) {
	var pages atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := fixtureRequest(t, r)
		switch request.Method {
		case "initialize":
			fixtureResult(w, request, map[string]any{"protocolVersion": LatestHandshakeVersion})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			pages.Add(1)
			fixtureResult(w, request, map[string]any{"tools": []any{}, "nextCursor": "same-page"})
		}
	}))
	defer server.Close()
	client, err := Connect(context.Background(), Config{Name: "cursor", URL: server.URL}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err = client.Tools(context.Background()); err == nil || pages.Load() != 2 {
		t.Fatalf("cursor cycle not bounded: %v pages=%d", err, pages.Load())
	}
}

func TestModernListenStreamReceivesAcknowledgementAndStopsOnCancellation(t *testing.T) {
	var streamClosed = make(chan struct{})
	var canceledNotice atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := fixtureRequest(t, r)
		switch request.Method {
		case "server/discover":
			fixtureResult(w, request, map[string]any{"resultType": "complete", "supportedVersions": []string{ModernProtocolVersion}, "capabilities": map[string]any{"tools": map[string]any{"listChanged": true}}})
		case "subscriptions/listen":
			params := request.Params.(map[string]any)
			filter, _ := params["notifications"].(map[string]any)
			if filter["toolsListChanged"] != true {
				t.Error("notification opt-in missing")
			}
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintf(w, "data: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/subscriptions/acknowledged\",\"params\":{\"notifications\":{\"toolsListChanged\":true},\"_meta\":{\"io.modelcontextprotocol/subscriptionId\":%d}}}\n\n", request.ID)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			close(streamClosed)
		case "notifications/cancelled":
			canceledNotice.Add(1)
			w.WriteHeader(http.StatusAccepted)
		default:
			t.Error(request.Method)
		}
	}))
	defer server.Close()
	client, err := ConnectWithOptions(context.Background(), Config{Name: "listen", URL: server.URL}, t.TempDir(), Options{ProtocolVersion: ModernProtocolVersion})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- client.ListenNotifications(ctx, NotificationFilter{ToolsListChanged: true}) }()
	select {
	case event := <-client.Notifications():
		if event.Method != "notifications/subscriptions/acknowledged" {
			t.Fatal(event)
		}
	case <-time.After(time.Second):
		t.Fatal("listen acknowledgement missing")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("lost cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("listen stream did not cancel")
	}
	select {
	case <-streamClosed:
	case <-time.After(time.Second):
		t.Fatal("server stream did not close")
	}
	if canceledNotice.Load() != 0 {
		t.Fatal("modern HTTP incorrectly sent a cancellation notification")
	}
}

func TestModernProtocolProbeDoesNotTreatMalformedDiscoveryAsSuccessful(t *testing.T) {
	var initialized atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := fixtureRequest(t, r)
		switch request.Method {
		case "server/discover":
			fixtureResult(w, request, map[string]any{})
		case "initialize":
			initialized.Add(1)
			fixtureResult(w, request, map[string]any{"protocolVersion": "2025-06-18"})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		default:
			t.Error(request.Method)
		}
	}))
	defer server.Close()
	client, err := ConnectWithOptions(context.Background(), Config{Name: "probe", URL: server.URL}, t.TempDir(), Options{ProtocolVersion: ModernProtocolVersion})
	if err == nil || client != nil || initialized.Load() != 0 {
		t.Fatalf("explicit modern connection silently downgraded: %v %v", client, err)
	}
	client, err = ConnectWithOptions(context.Background(), Config{Name: "probe", URL: server.URL}, t.TempDir(), Options{ProtocolVersion: "auto"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if client.ProtocolVersion() != "2025-06-18" || initialized.Load() != 1 {
		t.Fatalf("automatic older-era connection failed: %s", client.ProtocolVersion())
	}
}
