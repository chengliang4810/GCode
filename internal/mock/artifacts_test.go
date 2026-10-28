package mock

import (
	"context"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReferenceArtifactsPresentAndRethemed(t *testing.T) {
	catalog := artifactCatalog()
	if len(catalog) != 210 {
		t.Fatalf("expected 30 diagram and 180 formula exports, got %d", len(catalog))
	}
	for id, a := range catalog {
		if a.Width <= 0 || a.Height <= 0 || a.Source == "" {
			t.Fatalf("invalid layout metadata %s", id)
		}
		path := filepath.Join("..", "assets", "data", strings.TrimPrefix(a.AssetRef, "asset://"))
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		decoder := xml.NewDecoder(strings.NewReader(string(data)))
		for {
			_, err = decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("invalid SVG %s: %v", id, err)
			}
		}
	}
	s := newTestServer(t, Options{Scenario: "showcase"})
	v, _ := s.Snapshot(context.Background())
	kinds := make(map[string]bool)
	for _, m := range v.Messages["chat-0"] {
		for _, b := range m.Blocks {
			kinds[b.Kind] = true
			if (b.Kind == "math" || b.Kind == "diagram") && b.Artifact == nil {
				t.Fatalf("missing showcase artifact %s", b.Kind)
			}
		}
	}
	for _, kind := range []string{"text", "reasoning", "code", "tool", "todo", "subagent", "question", "permission", "attachment", "image", "math", "diagram"} {
		if !kinds[kind] {
			t.Fatalf("showcase misses %s", kind)
		}
	}
	v.Settings.Appearance.Mode = "light"
	v.Settings.Appearance.ThemeID = ""
	v.Settings.Appearance.MathFont = "Euler"
	if err := s.UpdateSettings(context.Background(), v.Settings); err != nil {
		t.Fatal(err)
	}
	updated, _ := s.Snapshot(context.Background())
	for _, m := range updated.Messages["chat-0"] {
		for _, b := range m.Blocks {
			if b.Kind == "math" && !strings.Contains(b.Artifact.AssetRef, "math-euler-zeron-light.svg") {
				t.Fatalf("formula did not follow theme/font: %s", b.Artifact.AssetRef)
			}
			if b.Kind == "diagram" && !strings.Contains(b.Artifact.AssetRef, "diagram-zeron-light.svg") {
				t.Fatal("diagram did not follow selected light palette")
			}
		}
	}
}
