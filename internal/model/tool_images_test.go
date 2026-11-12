package model

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"slices"
	"strings"
	"testing"
)

type toolImageWirePart struct {
	Type     string          `json:"type"`
	Text     string          `json:"text"`
	ImageURL json.RawMessage `json:"image_url"`
	Source   struct {
		Type      string `json:"type"`
		MediaType string `json:"media_type"`
		Data      string `json:"data"`
	} `json:"source"`
}

func TestChatToolImagesFollowCompleteToolBatch(t *testing.T) {
	t.Parallel()
	history := persistedToolImageHistory(t)
	data := toolImageRequestJSON(t, Chat, history)
	var body struct {
		Messages []struct {
			Role       string          `json:"role"`
			Content    json.RawMessage `json:"content"`
			ToolCallID string          `json:"tool_call_id"`
			Calls      []struct {
				ID string `json:"id"`
			} `json:"tool_calls"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Messages) != 9 {
		t.Fatalf("unexpected messages: %s", data)
	}
	wantRoles := []string{"user", "assistant", "tool", "tool", "tool", "user", "assistant", "tool", "user"}
	for i, role := range wantRoles {
		if body.Messages[i].Role != role {
			t.Fatalf("message %d: role=%q, want %q: %s", i, body.Messages[i].Role, role, data)
		}
	}
	assertToolImageContent(t, Chat, body.Messages[0].Content, history[0])
	if len(body.Messages[1].Calls) != len(history[1].Calls) {
		t.Fatalf("assistant tool calls lost: %s", data)
	}
	for index, call := range history[1].Calls {
		if body.Messages[1].Calls[index].ID != call.ID {
			t.Fatalf("assistant call identity lost: %s", data)
		}
		result := body.Messages[index+2]
		var text string
		if err := json.Unmarshal(result.Content, &text); err != nil {
			t.Fatalf("tool result must remain a string: %s", result.Content)
		}
		if result.ToolCallID != call.ID || text != history[index+2].Text {
			t.Fatalf("tool result identity/text lost: %s", data)
		}
	}
	var images []toolImageWirePart
	if err := json.Unmarshal(body.Messages[5].Content, &images); err != nil {
		t.Fatal(err)
	}
	if len(images) != 5 {
		t.Fatalf("tool batch images/captions lost: %s", body.Messages[5].Content)
	}
	if images[0].Type != "text" || !strings.Contains(images[0].Text, "call-a") ||
		images[3].Type != "text" || !strings.Contains(images[3].Text, "call-b") {
		t.Fatalf("tool image call captions lost: %+v", images)
	}
	assertToolImagePart(t, Chat, images[1], history[2].Images[0])
	assertToolImagePart(t, Chat, images[2], history[2].Images[1])
	assertToolImagePart(t, Chat, images[4], history[3].Images[0])
	if len(body.Messages[6].Calls) != 1 || body.Messages[6].Calls[0].ID != "call-c" ||
		body.Messages[7].ToolCallID != "call-c" {
		t.Fatalf("second batch call identity lost: %s", data)
	}
	if err := json.Unmarshal(body.Messages[8].Content, &images); err != nil {
		t.Fatal(err)
	}
	if len(images) != 2 || !strings.Contains(images[0].Text, "call-c") {
		t.Fatalf("trailing tool batch images missing or contaminated: %s", body.Messages[8].Content)
	}
	assertToolImagePart(t, Chat, images[1], history[6].Images[0])
}

func TestResponsesToolImagesRemainInsideCallOutputs(t *testing.T) {
	t.Parallel()
	history := persistedToolImageHistory(t)
	data := toolImageRequestJSON(t, Responses, history)
	var body struct {
		Input []struct {
			Type    string          `json:"type"`
			Role    string          `json:"role"`
			CallID  string          `json:"call_id"`
			Content json.RawMessage `json:"content"`
			Output  json.RawMessage `json:"output"`
		} `json:"input"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatal(err)
	}
	pending := map[string]bool{}
	outputIDs := []string{}
	userMessages := 0
	for _, item := range body.Input {
		switch item.Type {
		case "function_call":
			pending[item.CallID] = true
		case "function_call_output":
			if !pending[item.CallID] {
				t.Fatalf("unpaired tool output %q: %s", item.CallID, data)
			}
			delete(pending, item.CallID)
			outputIDs = append(outputIDs, item.CallID)
			result := toolImageHistoryResult(t, history, item.CallID)
			if len(result.Images) == 0 {
				var text string
				if err := json.Unmarshal(item.Output, &text); err != nil || text != result.Text {
					t.Fatalf("text/error tool output changed: %s", item.Output)
				}
				continue
			}
			assertToolImageContent(t, Responses, item.Output, result)
		default:
			if item.Role == "user" {
				userMessages++
				assertToolImageContent(t, Responses, item.Content, history[0])
			}
		}
	}
	if userMessages != 1 || len(pending) != 0 ||
		!slices.Equal(outputIDs, []string{"call-a", "call-b", "call-error", "call-c"}) {
		t.Fatalf("tool calls/results lost: %s", data)
	}
}

func TestMessagesToolImagesRemainInsideToolResults(t *testing.T) {
	t.Parallel()
	history := persistedToolImageHistory(t)
	data := toolImageRequestJSON(t, Messages, history)
	var body struct {
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatal(err)
	}
	// Adjacent tool results belong to one user message, so results are matched
	// by call ID instead of by message position.
	results := map[string]Message{}
	for _, message := range history {
		if message.Role == "tool" {
			results[message.CallID] = message
		}
	}
	assertToolImageContent(t, Messages, body.Messages[0].Content, history[0])
	pending := map[string]bool{}
	outputIDs := []string{}
	for _, item := range body.Messages {
		var blocks []struct {
			Type      string          `json:"type"`
			ID        string          `json:"id"`
			ToolUseID string          `json:"tool_use_id"`
			IsError   bool            `json:"is_error"`
			Content   json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal(item.Content, &blocks); err != nil {
			t.Fatal(err)
		}
		for _, block := range blocks {
			switch block.Type {
			case "tool_use":
				pending[block.ID] = true
			case "tool_result":
				if !pending[block.ToolUseID] || item.Role != "user" {
					t.Fatalf("unpaired tool result %q: %s", block.ToolUseID, data)
				}
				delete(pending, block.ToolUseID)
				outputIDs = append(outputIDs, block.ToolUseID)
				result, ok := results[block.ToolUseID]
				if !ok {
					t.Fatalf("unknown tool result %q: %s", block.ToolUseID, data)
				}
				if block.IsError != result.IsError {
					t.Fatalf("tool result identity/error flag changed: %s", item.Content)
				}
				if len(result.Images) == 0 {
					var text string
					if err := json.Unmarshal(block.Content, &text); err != nil || text != result.Text {
						t.Fatalf("text/error tool result changed: %s", block.Content)
					}
					continue
				}
				assertToolImageContent(t, Messages, block.Content, result)
			}
		}
	}
	if len(pending) != 0 || !slices.Equal(outputIDs, []string{"call-a", "call-b", "call-error", "call-c"}) {
		t.Fatalf("tool calls/results lost: %s", data)
	}
}

func TestToolImagesSurvivePersistenceAndClone(t *testing.T) {
	t.Parallel()
	original := toolImageHistory(t)
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var restored []Message
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	cloned := CloneMessages(restored)
	for index := range original {
		if !slices.Equal(cloned[index].Images, original[index].Images) ||
			cloned[index].CallID != original[index].CallID ||
			cloned[index].IsError != original[index].IsError {
			t.Fatalf("persisted clone lost image dimensions/data or tool identity: %+v", cloned[index])
		}
	}
	cloned[2].Images[0].Data = "changed"
	cloned[2].Images[0].Width = 999
	if restored[2].Images[0] != original[2].Images[0] {
		t.Fatal("clone mutation changed persisted image history")
	}
}

func toolImageRequestJSON(t *testing.T, protocol Protocol, history []Message) []byte {
	t.Helper()
	body := requestBody(t, protocol, Model{ID: "image-model"}, Request{History: history})
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func persistedToolImageHistory(t *testing.T) []Message {
	t.Helper()
	data, err := json.Marshal(toolImageHistory(t))
	if err != nil {
		t.Fatal(err)
	}
	var history []Message
	if err := json.Unmarshal(data, &history); err != nil {
		t.Fatal(err)
	}
	return CloneMessages(history)
}

func toolImageHistory(t *testing.T) []Message {
	t.Helper()
	picture := image.NewRGBA(image.Rect(0, 0, 2, 3))
	picture.Set(0, 0, color.RGBA{R: 255, A: 255})
	picture.Set(1, 2, color.RGBA{B: 255, A: 255})
	var pngData, jpegData bytes.Buffer
	if err := png.Encode(&pngData, picture); err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(&jpegData, picture, nil); err != nil {
		t.Fatal(err)
	}
	first := Image{
		MIME: "image/png", Data: base64.StdEncoding.EncodeToString(pngData.Bytes()), Width: 2, Height: 3,
	}
	second := Image{
		MIME: "image/jpeg", Data: base64.StdEncoding.EncodeToString(jpegData.Bytes()), Width: 2, Height: 3,
	}
	return []Message{
		{Role: "user", Text: "Compare these files", Images: []Image{second}},
		{Role: "assistant", Calls: []Call{
			{ID: "call-a", Name: "read", Arguments: `{"path":"first.png"}`},
			{ID: "call-b", Name: "read", Arguments: `{"path":"second.jpg"}`},
			{ID: "call-error", Name: "read", Arguments: `{"path":"missing.png"}`},
		}},
		{Role: "tool", CallID: "call-a", Text: "First images", Images: []Image{first, second}},
		{Role: "tool", CallID: "call-b", Text: "Second image", Images: []Image{second}},
		{Role: "tool", CallID: "call-error", Text: "read failed: file does not exist", IsError: true},
		{Role: "assistant", Text: "Read another", Calls: []Call{
			{ID: "call-c", Name: "read", Arguments: `{"path":"third.png"}`},
		}},
		{Role: "tool", CallID: "call-c", Images: []Image{first}},
	}
}

func toolImageHistoryResult(t *testing.T, history []Message, callID string) Message {
	t.Helper()
	for _, message := range history {
		if message.Role == "tool" && message.CallID == callID {
			return message
		}
	}
	t.Fatalf("unexpected call ID %q", callID)
	return Message{}
}

func assertToolImageContent(t *testing.T, protocol Protocol, raw json.RawMessage, want Message) {
	t.Helper()
	var parts []toolImageWirePart
	if err := json.Unmarshal(raw, &parts); err != nil {
		t.Fatalf("multimodal content is not an array: %s", raw)
	}
	textParts := 0
	if want.Text != "" {
		textParts = 1
	}
	if len(parts) != len(want.Images)+textParts {
		t.Fatalf("image/text content lost: %s", raw)
	}
	textType := "text"
	if protocol == Responses {
		textType = "input_text"
	}
	if textParts > 0 && (parts[0].Type != textType || parts[0].Text != want.Text) {
		t.Fatalf("multimodal text changed: %s", raw)
	}
	for index, picture := range want.Images {
		assertToolImagePart(t, protocol, parts[index+textParts], picture)
	}
}

func assertToolImagePart(t *testing.T, protocol Protocol, part toolImageWirePart, want Image) {
	t.Helper()
	var encoded string
	switch protocol {
	case Chat, Responses:
		var url string
		if protocol == Chat {
			var source struct {
				URL string `json:"url"`
			}
			if err := json.Unmarshal(part.ImageURL, &source); err != nil || part.Type != "image_url" {
				t.Fatalf("invalid Chat image: %+v", part)
			}
			url = source.URL
		} else {
			if err := json.Unmarshal(part.ImageURL, &url); err != nil || part.Type != "input_image" {
				t.Fatalf("invalid Responses image: %+v", part)
			}
		}
		prefix := "data:" + want.MIME + ";base64,"
		if !strings.HasPrefix(url, prefix) {
			t.Fatalf("wrong image MIME/data URL: %q", url)
		}
		encoded = strings.TrimPrefix(url, prefix)
	case Messages:
		if part.Type != "image" || part.Source.Type != "base64" || part.Source.MediaType != want.MIME {
			t.Fatalf("invalid Messages image source: %+v", part)
		}
		encoded = part.Source.Data
	default:
		t.Fatalf("unexpected protocol %q", protocol)
	}
	got, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := base64.StdEncoding.DecodeString(want.Data)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, expected) {
		t.Fatal("request image bytes differ from the tool result")
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(got))
	if err != nil || config.Width != want.Width || config.Height != want.Height {
		t.Fatalf("request image is invalid or dimensions changed: %+v, %v", config, err)
	}
}
