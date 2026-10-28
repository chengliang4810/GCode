package assets

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestImportedAssetsMatchFrozenManifest(t *testing.T) {
	data, err := os.ReadFile("manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		SourceCommit string `json:"sourceCommit"`
		Files        []struct {
			Path, Source, SHA256 string
			Bytes                int
		} `json:"files"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.SourceCommit != "ab189159f10cf8e63c0502be75a59bbc487e0eaf" {
		t.Fatal("source drift")
	}
	var imported int
	for _, name := range Names() {
		if !strings.HasPrefix(name, "artifacts/") {
			imported++
		}
	}
	if len(manifest.Files) != imported {
		t.Fatalf("manifest %d files, embedded imports %d", len(manifest.Files), imported)
	}
	for _, file := range manifest.Files {
		t.Run(file.Path, func(t *testing.T) {
			data, err := Read(file.Path)
			if err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256(data)
			if len(data) != file.Bytes || hex.EncodeToString(hash[:]) != file.SHA256 {
				t.Fatal("imported bytes differ from source manifest")
			}
			if file.Source == "" {
				t.Fatal("missing original path")
			}
		})
	}
}

func TestEveryControlIconCanBeParsed(t *testing.T) {
	for _, name := range Names() {
		if !strings.HasPrefix(name, "icons/") || !strings.HasSuffix(name, ".svg") {
			continue
		}
		t.Run(name, func(t *testing.T) {
			data, err := Read(name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ui.ParseSVG(data); err != nil {
				t.Fatal(err)
			}
			if Icon(strings.TrimPrefix(name, "icons/")) == nil {
				t.Fatal("icon resolution failed")
			}
		})
	}
	if Icon("nonexistent-icon") != nil {
		t.Fatal("unknown icon should not be approximated")
	}
}

func TestReadRejectsPathTraversal(t *testing.T) {
	for _, name := range []string{"../go.mod", "/app-icon.png", "icons/../../go.mod"} {
		if _, err := Read(name); err == nil {
			t.Fatalf("accepted %q", name)
		}
	}
}
