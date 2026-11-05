package engine

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/storage"
)

func TestReadImagesReachEveryProtocolAndSurviveRestart(t *testing.T) {
	for _, protocol := range []model.Protocol{model.Chat, model.Responses, model.Messages} {
		t.Run(string(protocol), func(t *testing.T) {
			var requests atomic.Int32
			bodies := make(chan []byte, 4)
			fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				bodies <- body
				w.Header().Set("Content-Type", "text/event-stream")
				if requests.Add(1) == 1 {
					sendImageReadCalls(w, protocol)
				} else {
					sendImageReadAnswer(w, protocol)
				}
			}))
			t.Cleanup(fixture.Close)
			home, workspace := t.TempDir(), t.TempDir()
			var encoded bytes.Buffer
			pixels := image.NewNRGBA(image.Rect(0, 0, 3, 2))
			pixels.SetNRGBA(1, 1, color.NRGBA{G: 210, B: 50, A: 160})
			if err := png.Encode(&encoded, pixels); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(workspace, "photo.png")
			if err := os.WriteFile(path, encoded.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(workspace, "note.txt"), []byte("expected text"), 0600); err != nil {
				t.Fatal(err)
			}
			store, err := storage.Open(storage.Options{Home: home})
			if err != nil {
				t.Fatal(err)
			}
			core, err := openExecutionFixture(t, Options{Storage: store})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = core.Close() })
			provider := model.Provider{
				ID: "images", Name: "Image fixture", Protocol: protocol, BaseURL: fixture.URL,
				Models: []model.Model{{ID: "vision"}},
			}
			ctx := context.Background()
			if err := core.Execute(ctx, contracts.Command{
				Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider},
			}); err != nil {
				t.Fatal(err)
			}
			if err := core.Execute(ctx, contracts.Command{
				Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: workspace, Name: "Images"},
			}); err != nil {
				t.Fatal(err)
			}
			before, _ := core.Snapshot(ctx)
			if err := core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
				ProjectID: before.Projects[0].ID, ModelID: "images/vision", Text: "Read the local photo and note",
			}}); err != nil {
				t.Fatal(err)
			}
			state := waitIdle(t, core)
			if state.Sessions[0].LastOutcome != "completed" || requests.Load() != 2 {
				t.Fatalf("image tool loop did not complete: %+v", state.Runs)
			}
			<-bodies
			body := <-bodies
			assertImageToolRequest(t, protocol, body, encoded.Bytes())
			sid := state.Sessions[0].ID
			if len(core.history[sid]) != 5 || len(core.history[sid][2].Images) != 1 ||
				core.history[sid][2].CallID != "photo-call" || core.history[sid][3].CallID != "text-call" {
				t.Fatalf("typed image result or paired tool identity was lost: %+v", core.history[sid])
			}
			snapshotJSON, _ := json.Marshal(state)
			if bytes.Contains(snapshotJSON, []byte(base64.StdEncoding.EncodeToString(encoded.Bytes()))) {
				t.Fatal("model image payload leaked into UI snapshot text")
			}
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}
			// The next request must use persisted image bytes, not re-read a path.
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			store, err = storage.Open(storage.Options{Home: home})
			if err != nil {
				t.Fatal(err)
			}
			core, err = openExecutionFixture(t, Options{Storage: store})
			if err != nil {
				t.Fatal(err)
			}
			if err := core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
				SessionID: sid, Text: "Look at that image again",
			}}); err != nil {
				t.Fatal(err)
			}
			state = waitIdle(t, core)
			if state.Sessions[0].LastOutcome != "completed" {
				t.Fatalf("restarted image history did not continue: %+v", state.Runs)
			}
			assertImageToolRequest(t, protocol, <-bodies, encoded.Bytes())
		})
	}
}

func sendImageReadCalls(w http.ResponseWriter, protocol model.Protocol) {
	calls := []model.Call{
		{ID: "photo-call", Name: "Read", Arguments: `{"file_path":"photo.png"}`},
		{ID: "text-call", Name: "Read", Arguments: `{"file_path":"note.txt"}`},
	}
	switch protocol {
	case model.Chat:
		wireCalls := []any{}
		for index, call := range calls {
			wireCalls = append(wireCalls, map[string]any{
				"index": index, "id": call.ID,
				"function": map[string]any{"name": call.Name, "arguments": call.Arguments},
			})
		}
		sendSSE(w, map[string]any{"choices": []any{map[string]any{
			"delta": map[string]any{"tool_calls": wireCalls}, "finish_reason": "tool_calls",
		}}})
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	case model.Responses:
		output := []any{}
		for _, call := range calls {
			output = append(output, map[string]any{
				"type": "function_call", "call_id": call.ID, "name": call.Name, "arguments": call.Arguments,
			})
		}
		sendSSE(w, map[string]any{
			"type": "response.completed", "response": map[string]any{"status": "completed", "output": output},
		})
	case model.Messages:
		for index, call := range calls {
			sendSSE(w, map[string]any{
				"type": "content_block_start", "index": index,
				"content_block": map[string]any{
					"type": "tool_use", "id": call.ID, "name": call.Name, "input": json.RawMessage(call.Arguments),
				},
			})
		}
		sendSSE(w, map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "tool_use"}})
		sendSSE(w, map[string]any{"type": "message_stop"})
	}
}

func sendImageReadAnswer(w http.ResponseWriter, protocol model.Protocol) {
	switch protocol {
	case model.Chat:
		sendSSE(w, map[string]any{"choices": []any{map[string]any{
			"delta": map[string]any{"content": "Image received"}, "finish_reason": "stop",
		}}})
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	case model.Responses:
		sendSSE(w, map[string]any{"type": "response.completed", "response": map[string]any{
			"status": "completed", "output": []any{map[string]any{
				"type": "message", "role": "assistant",
				"content": []any{map[string]any{"type": "output_text", "text": "Image received"}},
			}},
		}})
	case model.Messages:
		sendSSE(w, map[string]any{
			"type": "content_block_start", "index": 0,
			"content_block": map[string]any{"type": "text", "text": "Image received"},
		})
		sendSSE(w, map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "end_turn"}})
		sendSSE(w, map[string]any{"type": "message_stop"})
	}
}

func assertImageToolRequest(t *testing.T, protocol model.Protocol, body, original []byte) {
	t.Helper()
	for _, text := range []string{"photo-call", "text-call", "expected text", "Original: 3x2"} {
		if !bytes.Contains(body, []byte(text)) {
			t.Fatalf("%s request lost %q: %s", protocol, text, body)
		}
	}
	var request struct {
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
		Input []struct {
			Type   string          `json:"type"`
			CallID string          `json:"call_id"`
			Output json.RawMessage `json:"output"`
		} `json:"input"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		t.Fatal(err)
	}
	var contents []json.RawMessage
	switch protocol {
	case model.Chat:
		for i, message := range request.Messages {
			if message.Role == "tool" && i > 0 && request.Messages[i-1].Role == "tool" {
				if i+1 >= len(request.Messages) || request.Messages[i+1].Role != "user" {
					t.Fatal("image user message must follow the complete pair of tool results")
				}
				contents = append(contents, request.Messages[i+1].Content)
				break
			}
		}
	case model.Responses:
		for _, item := range request.Input {
			if item.Type == "function_call_output" && item.CallID == "photo-call" {
				contents = append(contents, item.Output)
			}
		}
	case model.Messages:
		for _, message := range request.Messages {
			var blocks []struct {
				Type    string          `json:"type"`
				ToolID  string          `json:"tool_use_id"`
				Content json.RawMessage `json:"content"`
			}
			if json.Unmarshal(message.Content, &blocks) != nil {
				continue
			}
			for _, block := range blocks {
				if block.Type == "tool_result" && block.ToolID == "photo-call" {
					contents = append(contents, block.Content)
				}
			}
		}
	}
	found := false
	for _, raw := range contents {
		var blocks []struct {
			Type     string          `json:"type"`
			ImageURL json.RawMessage `json:"image_url"`
			Source   struct {
				Data string `json:"data"`
				MIME string `json:"media_type"`
			} `json:"source"`
		}
		if err := json.Unmarshal(raw, &blocks); err != nil {
			t.Fatal(err)
		}
		for _, block := range blocks {
			var encoded string
			switch block.Type {
			case "image":
				if block.Source.MIME != "image/png" {
					t.Fatal("wrong image MIME")
				}
				encoded = block.Source.Data
			case "image_url":
				var imageURL struct {
					URL string `json:"url"`
				}
				_ = json.Unmarshal(block.ImageURL, &imageURL)
				encoded = strings.TrimPrefix(imageURL.URL, "data:image/png;base64,")
			case "input_image":
				var imageURL string
				_ = json.Unmarshal(block.ImageURL, &imageURL)
				encoded = strings.TrimPrefix(imageURL, "data:image/png;base64,")
			default:
				continue
			}
			data, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil || !bytes.Equal(data, original) {
				t.Fatalf("read image bytes did not reach %s model request", protocol)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("%s model request has no image in its protocol-specific location", protocol)
	}
}
