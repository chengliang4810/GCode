package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gcode/internal/model"
)

func attachmentImage(t *testing.T, width, height int) model.Image {
	t.Helper()
	var data bytes.Buffer
	pixels := image.NewNRGBA(image.Rect(0, 0, width, height))
	pixels.SetNRGBA(width/2, height/2, color.NRGBA{R: 180, A: 150})
	if err := png.Encode(&data, pixels); err != nil {
		t.Fatal(err)
	}
	return model.Image{MIME: "image/png", Data: base64.StdEncoding.EncodeToString(data.Bytes()), Width: width, Height: height}
}

func TestInputImagesShareReadPreparationAndPreserveSource(t *testing.T) {
	original := attachmentImage(t, 4096, 64)
	input := model.Message{Role: "user", UserInput: true, Text: "inspect both", Images: []model.Image{original, original}}
	prepared, err := prepareInputImages(t.Context(), nil, input)
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Images) != 1 || prepared.Images[0].Width != 2048 || prepared.Images[0].Height != 32 ||
		len(prepared.Images[0].Data) > maxReadImageBase64 || !strings.Contains(prepared.Text, "this message") ||
		!strings.Contains(prepared.Text, "Original: 4096x64; displayed: 2048x32") {
		t.Fatal("attachment was not bounded, identified and deduplicated")
	}
	if len(input.Images) != 2 || input.Images[0] != original || input.Text != "inspect both" {
		t.Fatal("preparation changed the source")
	}
	path := filepath.Join(t.TempDir(), "original.png")
	writePNG(t, path, decodeReadImage(t, original))
	read, err := executeRead(t, filepath.Dir(path), map[string]any{"file_path": path})
	if err != nil || read.Images[0] != prepared.Images[0] {
		t.Fatal("Read and attachment generated different images", err)
	}
	tool, err := prepareToolResult(t.Context(), nil, []model.Message{prepared}, model.Call{Name: "Read"},
		model.Message{Role: "tool", CallID: "read-original", Images: read.Images})
	if err != nil || len(tool.Images) != 0 || !strings.Contains(tool.Text, "earlier user message") {
		t.Fatal("Read failed to reuse a user attachment", err)
	}
}

func TestInputImagesUseOnlyLiveContextAcrossRestoreAndCompact(t *testing.T) {
	picture := attachmentImage(t, 3, 2)
	first, err := prepareInputImages(t.Context(), nil, model.Message{Role: "user", Images: []model.Image{picture}})
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"user", "tool"} {
		t.Run(role, func(t *testing.T) {
			prior := first
			prior.Role, prior.CallID = role, "original-read"
			encoded, _ := json.Marshal([]model.Message{prior})
			var restored []model.Message
			if err := json.Unmarshal(encoded, &restored); err != nil {
				t.Fatal(err)
			}
			input := model.Message{Role: "user", Text: "look again", Images: []model.Image{picture}}
			duplicate, err := prepareInputImages(t.Context(), restored, input)
			if err != nil || len(duplicate.Images) != 0 || !strings.Contains(duplicate.Text, "look again") {
				t.Fatal("restored context failed to reuse actual image", err)
			}
			// A summary and reference-only messages cannot substitute for pixels.
			compacted := []model.Message{{Role: "user", Summary: true, Text: "An image was inspected."}, duplicate}
			again, err := prepareInputImages(t.Context(), compacted, input)
			if err != nil || len(again.Images) != 1 {
				t.Fatal("image remained suppressed after compaction", err)
			}
			changed := input
			changed.Images = []model.Image{attachmentImage(t, 4, 2)}
			got, err := prepareInputImages(t.Context(), restored, changed)
			if err != nil || len(got.Images) != 1 {
				t.Fatal("different image was suppressed", err)
			}
		})
	}
}

func TestInputImagesRejectInvalidAndCancelledImages(t *testing.T) {
	for _, test := range []struct {
		name    string
		picture model.Image
	}{
		{"base64", model.Image{Data: "invalid!"}},
		{"image", model.Image{Data: base64.StdEncoding.EncodeToString([]byte("not an image"))}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := prepareInputImages(t.Context(), nil, model.Message{Images: []model.Image{test.picture}}); err == nil {
				t.Fatal("invalid image accepted")
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := prepareInputImages(ctx, nil, model.Message{Images: []model.Image{attachmentImage(t, 2, 3)}})
	if !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled preparation continued", err)
	}
}

func TestRunCommitsBoundedInputBeforeFirstRequest(t *testing.T) {
	original := attachmentImage(t, 4096, 64)
	old := model.Message{Role: "user", Text: "stable prefix"}
	input := model.Message{Role: "user", UserInput: true, Text: "inspect", Images: []model.Image{original}}
	var committed model.Message
	calls := 0
	err := Run(t.Context(), Options{
		Request: model.Request{History: []model.Message{old, input}},
		Model: testModel{stream: func(request model.Request) (model.Result, error) {
			calls++
			if !reflect.DeepEqual(request.History[0], old) || len(committed.Images) != 1 ||
				committed.Images[0].Width != 2048 || !reflect.DeepEqual(request.History[1], committed) {
				t.Fatal("request escaped before bounded input was committed")
			}
			return model.Result{Message: model.Message{Role: "assistant", Text: "done"}}, nil
		}},
		CommitPreparedInput: func(message model.Message) error { committed = message; return nil },
		Record:              func(model.Message) error { return nil }, Emit: func(Event) error { return nil },
		Pending: func(bool) []model.Message { return nil },
	})
	if err != nil || calls != 1 || input.Images[0] != original {
		t.Fatal("input preparation round trip failed", err, calls)
	}
}

func TestInputImagesReappearAfterActualCompaction(t *testing.T) {
	picture := attachmentImage(t, 3, 2)
	history := []model.Message{
		{Role: "user", Text: "inspect"},
		{Role: "assistant", Calls: []model.Call{{ID: "read", Name: "Read"}}},
		{Role: "tool", CallID: "read", Images: []model.Image{picture}},
	}
	for range 8 {
		history = append(history, model.Message{Role: "user", Text: "continue"})
	}
	compacted, err := Compact(t.Context(), testModel{stream: func(model.Request) (model.Result, error) {
		return model.Result{Message: model.Message{Role: "assistant", Text: "Image inspected; original remains available by path."}}, nil
	}}, model.Request{History: history})
	if err != nil {
		t.Fatal(err)
	}
	again, err := prepareInputImages(t.Context(), compacted, model.Message{Role: "user", Images: []model.Image{picture}})
	if err != nil || len(again.Images) != 1 {
		t.Fatal("actual compaction left an invalid image reference", err)
	}
}

func TestCompactionRetainsPixelsNeededBySurvivingReferences(t *testing.T) {
	for _, hierarchical := range []bool{false, true} {
		name := "full summary"
		if hierarchical {
			name = "hierarchical fold"
		}
		t.Run(name, func(t *testing.T) {
			picture := attachmentImage(t, 3, 2)
			history := contextMemoryHistory()
			history[2].Images = []model.Image{picture}
			last := len(history) - 1
			latest := history[last]
			latest.Images = []model.Image{picture}
			duplicate, err := prepareInputImages(t.Context(), history[:last], latest)
			if err != nil || len(duplicate.Images) != 0 || len(duplicate.ImageRefs) != 1 {
				t.Fatal("test input was not deduplicated", err)
			}
			history[last] = duplicate
			original := model.CloneMessages(history)
			var compacted []model.Message
			if hierarchical {
				compacted, _, err = foldContext(history, ContextMemory{}, 1, 3, "Image inspected.")
			} else {
				compacted, err = Compact(t.Context(), testModel{stream: func(model.Request) (model.Result, error) {
					return model.Result{Message: model.Message{Role: "assistant", Text: "Image inspected."}}, nil
				}}, model.Request{History: history})
			}
			if err != nil {
				t.Fatal(err)
			}
			retained := compacted[len(compacted)-1]
			if len(retained.Images) != 1 || retained.Images[0] != picture || !strings.Contains(retained.Text, "restored here") {
				t.Fatal("compaction removed the image needed by the latest question")
			}
			if !reflect.DeepEqual(history, original) {
				t.Fatal("compaction changed the source history or archive")
			}
		})
	}
}

func TestRunDoesNotSendImagesWhenInputCommitFails(t *testing.T) {
	failure := errors.New("disk full")
	err := Run(t.Context(), Options{
		Request: model.Request{History: []model.Message{{Role: "user", UserInput: true, Images: []model.Image{attachmentImage(t, 3, 2)}}}},
		Model: testModel{stream: func(model.Request) (model.Result, error) {
			t.Fatal("model received an uncommitted image")
			return model.Result{}, nil
		}},
		CommitPreparedInput: func(model.Message) error { return failure },
		Record:              func(model.Message) error { return nil }, Emit: func(Event) error { return nil },
		Pending: func(bool) []model.Message { return nil },
	})
	if !errors.Is(err, failure) {
		t.Fatal("commit failure was lost", err)
	}
}
