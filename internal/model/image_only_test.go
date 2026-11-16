package model

import (
	"encoding/json"
	"testing"
)

func TestImageOnlyRequestsOmitEmptyTextParts(t *testing.T) {
	for _, tc := range []struct {
		name    string
		adapter adapter
	}{{"chat", chatAdapter{}}, {"responses", responsesAdapter{}}, {"messages", messagesAdapter{}}} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := tc.adapter.encode(Request{History: []Message{{Role: "user", Images: []Image{{MIME: "image/png", Data: "fixture"}}}}}, adapterTarget{})
			if err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			var value any
			if err := json.Unmarshal(data, &value); err != nil {
				t.Fatal(err)
			}
			images := 0
			var inspect func(any)
			inspect = func(value any) {
				switch value := value.(type) {
				case []any:
					for _, item := range value {
						inspect(item)
					}
				case map[string]any:
					switch value["type"] {
					case "text", "input_text", "output_text":
						if value["text"] == "" {
							t.Fatal("image-only request contains an empty text part")
						}
					case "image", "image_url", "input_image":
						images++
					}
					for _, item := range value {
						inspect(item)
					}
				}
			}
			inspect(value)
			if images != 1 {
				t.Fatalf("image payload changed: %s", data)
			}
		})
	}
}
