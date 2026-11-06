package desktop

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func TestTranscriptFileLinksDoNotRescanFilesOnScroll(t *testing.T) {
	a := linkTestApp(t)
	for i := range 5000 {
		a.state.Files = append(a.state.Files, contracts.File{
			ID: fmt.Sprint(i), ProjectID: "links", Path: fmt.Sprintf("src/package%d/file.go", i), Kind: "file",
		})
	}
	targets := []string{"src/package0/file.go", "src/package1/file.go", "src/package2/file.go", "unknown.go"}
	resolve := func() {
		for _, target := range targets {
			// Markdown creates a config per block on each rebuild.
			a.linkConfig("root-links", "", ui.Services{}).Resolve(target)
		}
	}
	resolve()
	if allocations := testing.AllocsPerRun(10, resolve); allocations > 256 {
		t.Fatalf("scrolling four file links allocated %.0f objects; project paths must be indexed once per snapshot", allocations)
	}
}

func TestTranscriptFileLinksRefreshOnAcceptedSnapshot(t *testing.T) {
	a := linkTestApp(t)
	config := a.linkConfig("root-links", "", ui.Services{})
	initial, ok := config.Resolve("guide.MARKDOWN")
	if !ok || !initial.KnownFile || initial.Label != "guide.MARKDOWN" {
		t.Fatalf("initial file link: %+v, %v", initial, ok)
	}
	next := a.state
	next.Revision++
	next.Files = append(slices.Clone(next.Files), contracts.File{
		ID: "duplicate", ProjectID: "links", Path: "docs/guide.MARKDOWN", Kind: "file",
	})
	a.apply(next)
	duplicate, ok := config.Resolve("guide.MARKDOWN")
	if !ok || !duplicate.KnownFile || duplicate.Label != "workspace/guide.MARKDOWN" {
		t.Fatalf("new duplicate did not update the label: %+v, %v", duplicate, ok)
	}
	next = a.state
	next.Revision++
	next.Files = slices.DeleteFunc(slices.Clone(next.Files), func(file contracts.File) bool { return file.ID == "guide" })
	a.apply(next)
	deleted, ok := config.Resolve("guide.MARKDOWN")
	if !ok || deleted.KnownFile {
		t.Fatalf("deleted file remained indexed: %+v, %v", deleted, ok)
	}
	next = a.state
	next.Revision++
	next.Projects = slices.Clone(next.Projects)
	next.Projects[0].Path = "/relocated"
	next.Sessions = slices.Clone(next.Sessions)
	next.Sessions[0].CWD = "/relocated"
	a.apply(next)
	moved, ok := config.Resolve("docs/guide.MARKDOWN")
	if !ok || !moved.KnownFile || moved.Tooltip != "/relocated/docs/guide.MARKDOWN" {
		t.Fatalf("project relocation retained old paths: %+v, %v", moved, ok)
	}
}

func TestTranscriptFileIndexPreservesPathSemantics(t *testing.T) {
	for _, tc := range []struct {
		root, file, target string
		known              bool
	}{
		{"/workspace", "Guide.md", "/workspace/Guide.md", true},
		{"/workspace", "Guide.md", "/workspace/guide.md", false},
		{`C:\workspace`, `docs\Guide.md`, "c:/WORKSPACE/docs/guide.md", true},
		{"/workspace", "Σ.md", "/workspace/ς.md", false},
	} {
		t.Run(tc.root+"/"+tc.target, func(t *testing.T) {
			a := &App{}
			project := contracts.Project{ID: "project", Path: tc.root}
			a.state.Files = []contracts.File{{ID: "file", ProjectID: project.ID, Path: tc.file, Kind: "file"}}
			known, label := a.transcriptFiles(project).lookup(tc.target)
			if known != tc.known || label == "" {
				t.Fatalf("known=%v label=%q, want known=%v", known, label, tc.known)
			}
		})
	}
	for _, pair := range [][2]string{{"Σ", "ς"}, {"K", "K"}, {"S", "ſ"}, {"i", "ı"}} {
		if got := foldLinkName(pair[0]) == foldLinkName(pair[1]); got != strings.EqualFold(pair[0], pair[1]) {
			t.Fatalf("changed Unicode case semantics for %q", pair)
		}
	}
}
