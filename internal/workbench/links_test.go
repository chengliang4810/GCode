package workbench

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/linktarget"
	"gcode/internal/linkui"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

type linkedTestService struct {
	contracts.Service
	request   contracts.LinkedFileRequest
	result    contracts.LinkedFileResult
	err       error
	resources map[string][]byte
}

func (service *linkedTestService) ReadLinkedFile(_ context.Context, request contracts.LinkedFileRequest) (contracts.LinkedFileResult, error) {
	service.request = request
	return service.result, service.err
}

func (service *linkedTestService) ReadResource(_ context.Context, ref string) ([]byte, error) {
	if data, ok := service.resources[ref]; ok {
		return data, nil
	}
	return nil, fmt.Errorf("resource unavailable")
}

func linkWorkspace(t *testing.T) *State {
	t.Helper()
	s := New(nil, nil)
	s.Update(contracts.Snapshot{
		Projects: []contracts.Project{{ID: "project", Path: "/workspace"}},
		Sessions: []contracts.Session{
			{ID: "root", ProjectID: "project", CWD: "/workspace"},
			{ID: "child", ParentSessionID: "root", ProjectID: "project", CWD: "/workspace/child"},
			{ID: "unrelated", ProjectID: "project", CWD: "/elsewhere"},
		},
	})
	s.SwitchSession("root")
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestBrowserLauncherStartsNativeBlankTab(t *testing.T) {
	s := linkWorkspace(t)
	s.OpenSurface("browser")
	if s.TabCount() != 1 || s.ActiveKind() != "browser" {
		t.Fatal("browser did not open")
	}
	b := s.browsers[s.tabs[0].id]
	if b == nil || b.data.URL != "" || b.address != "" || b.view != nil || b.data.Loading {
		t.Fatalf("blank browser loaded a site: %#v", b)
	}
	tt := ui.NewTester(s.View, 700, 500)
	if !tt.HasText("Open a website") {
		t.Fatal("native blank state missing")
	}
	if urls := tt.OpenedURLs(); len(urls) != 0 {
		t.Fatalf("blank browser opened URLs: %v", urls)
	}
	b.address = "https://example.com/docs"
	s.navigateBrowser(b)
	if b.data.URL != b.address || b.data.Error != "" {
		t.Fatalf("explicit website navigation failed: %#v", b.data)
	}
}

func TestLinkFromChildRetainsSourceWithoutChangingRoot(t *testing.T) {
	s := linkWorkspace(t)
	service := &linkedTestService{result: contracts.LinkedFileResult{
		File:         contracts.File{ID: "linked", Path: "/workspace/child/说明.MARKDOWN", Name: "说明.MARKDOWN", Content: "第一行\n你好 world", Loaded: true, ReadOnly: true},
		AbsolutePath: "/workspace/child/说明.MARKDOWN",
	}}
	s.backend = service
	if err := s.OpenLink("child", "说明.MARKDOWN:2:3"); err != nil {
		t.Fatal(err)
	}
	if service.request.SessionID != "child" || service.request.Path != "说明.MARKDOWN" || s.scopeID != "root" {
		t.Fatalf("source ownership changed: %#v scope=%q", service.request, s.scopeID)
	}
	d := s.documents["linked"]
	if d == nil || d.preview || d.editor.Cursor() != len("第一行\n你好") || s.ActiveKind() != "file" {
		t.Fatalf("Unicode line/column did not reveal code: %#v", d)
	}
	if err := s.OpenLink("child", "说明.MARKDOWN"); err != nil || !d.preview {
		t.Fatalf("Markdown did not default to preview: %v", err)
	}
	s.Update(s.snapshot)
	if d.deleted {
		t.Fatal("non-snapshot linked document marked deleted")
	}
	if err := s.OpenLink("child", "https://example.com/path"); err != nil || s.ActiveKind() != "browser" || s.scopeID != "root" {
		t.Fatalf("URL did not route to root workspace: %v", err)
	}
}

func TestLinksRejectUnrelatedOrMissingSources(t *testing.T) {
	s := linkWorkspace(t)
	for _, source := range []string{"unrelated", "missing"} {
		if err := s.OpenLink(source, "https://example.com"); err == nil {
			t.Errorf("accepted unrelated source %q", source)
		}
	}
	for _, raw := range []string{"javascript:alert(1)", "data:text/html,test", "mailto:user@example.com"} {
		if err := s.OpenLink("root", raw); err == nil {
			t.Errorf("accepted transcript destination %q", raw)
		}
	}
	if s.TabCount() != 0 {
		t.Fatal("invalid links opened a tab")
	}
}

func TestSnapshotLinksResolveAgainstProjectAndPreviewDirectory(t *testing.T) {
	s := linkWorkspace(t)
	s.snapshot.Files = []contracts.File{
		{ID: "first", ProjectID: "project", Path: "child/docs/Readme.MD", Name: "Readme.MD", Content: "# Heading\n", Loaded: true, ReadOnly: true},
		{ID: "second", ProjectID: "project", Path: "child/next.markdown", Name: "next.markdown", Content: "# Next\n", Loaded: true, ReadOnly: true},
	}
	if err := s.OpenLink("child", "docs/Readme.MD"); err != nil {
		t.Fatal(err)
	}
	d := s.documents["first"]
	if d == nil || !d.preview || d.absolutePath != "/workspace/child/docs/Readme.MD" {
		t.Fatalf("relative child file resolved incorrectly: %#v", d)
	}
	if err := s.openPreviewLink(d, "../next.markdown", func(string) { t.Fatal("file opened externally") }); err != nil {
		t.Fatal(err)
	}
	if s.activeFile() != "second" || !s.documents["second"].preview {
		t.Fatal("preview relative link did not open native file")
	}
	opened := ""
	if err := s.openPreviewLink(d, "mailto:user@example.com", func(raw string) { opened = raw }); err != nil || !strings.HasPrefix(opened, "mailto:") {
		t.Fatalf("mail link failed: %q %v", opened, err)
	}
	if err := s.OpenLink("child", "missing.md"); err == nil || s.documents["missing.md"] != nil {
		t.Fatal("mock fabricated a linked resource")
	}
}

func TestLinkedImageReadsManagedResource(t *testing.T) {
	s := linkWorkspace(t)
	ref := "resource://managed/linked-image.svg"
	service := &linkedTestService{
		result:    contracts.LinkedFileResult{File: contracts.File{ID: "image", Path: "/tmp/image.svg", Name: "image.svg", Kind: "image", Loaded: true, ReadOnly: true, AssetRef: ref}, AbsolutePath: "/tmp/image.svg"},
		resources: map[string][]byte{ref: []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20"><rect width="20" height="20" fill="red"/></svg>`)},
	}
	s.backend = service
	if err := s.OpenLink("root", "/tmp/image.svg"); err != nil {
		t.Fatal(err)
	}
	d := s.documents["image"]
	if d == nil || d.image == nil || d.imageError != "" {
		t.Fatalf("managed linked image did not decode: %#v", d)
	}
}

func TestReopeningLinkedFilePreservesDraftAndRefreshesReadOnlyContent(t *testing.T) {
	s := linkWorkspace(t)
	service := &linkedTestService{result: contracts.LinkedFileResult{
		File:         contracts.File{ID: "document", Path: "notes.md", Name: "notes.md", Content: "original", Loaded: true},
		AbsolutePath: "/workspace/notes.md",
	}}
	s.backend = service
	if err := s.OpenLink("root", "notes.md"); err != nil {
		t.Fatal(err)
	}
	d := s.documents["document"]
	d.editor.Buffer.Set("draft")
	if err := s.OpenLink("root", "notes.md"); err != nil || d.editor.Buffer.String() != "draft" || d.conflict {
		t.Fatal("reopening unchanged file replaced an unsaved draft")
	}
	service.result.File.Content = "external change"
	if err := s.OpenLink("root", "notes.md"); err != nil || d.editor.Buffer.String() != "draft" || !d.conflict {
		t.Fatal("reopening changed file did not preserve draft and conflict")
	}
	d.editor.Buffer.Reset("original")
	service.result.File.ReadOnly = true
	if err := s.OpenLink("root", "notes.md"); err != nil || d.editor.Buffer.String() != "external change" {
		t.Fatal("read-only file did not refresh")
	}
}

func TestPreviewUsesSharedNativeLinksAndCrossDocumentAnchor(t *testing.T) {
	s := linkWorkspace(t)
	s.snapshot.Files = []contracts.File{
		{ID: "guide", ProjectID: "project", Path: "docs/guide.md", Name: "guide.md", Content: "[Website](https://example.com)\n\n[Next](next.md#destination)", Loaded: true, ReadOnly: true},
		{ID: "next", ProjectID: "project", Path: "docs/next.md", Name: "next.md", Content: "# Destination\n\nText", Loaded: true, ReadOnly: true},
	}
	var openedExternal string
	s.OnLinkConfig = func(source, documentPath string) linkui.Config {
		return linkui.Config{
			Internal: false,
			Resolve: func(raw string) (linkui.Destination, bool) {
				target, err := linktarget.ParsePreview(raw, documentPath)
				return linkui.Destination{Raw: raw, Kind: target.Kind, Tooltip: target.Path + target.URL}, err == nil && target.Kind != linktarget.Anchor
			},
			Open: func(raw string) {
				if err := s.OpenPreviewLink(source, documentPath, raw); err != nil {
					t.Error(err)
				}
			},
			OpenExternal: func(raw string) { openedExternal = raw },
		}
	}
	if err := s.OpenLink("root", "docs/guide.md"); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(s.View, 700, 500)
	// The paragraph label covers its full width; activate visible glyphs.
	website, ok := tt.Find("Website")
	if !ok {
		t.Fatal("website link is missing")
	}
	tt.ClickAt(website.X+10, website.Y+website.H/2)
	if openedExternal != "https://example.com" || s.ActiveKind() != "file" || len(tt.OpenedURLs()) != 0 {
		t.Fatalf("preview bypassed shared external policy: external=%q kind=%q urls=%v", openedExternal, s.ActiveKind(), tt.OpenedURLs())
	}
	nextLink, ok := tt.Find("Next")
	if !ok {
		t.Fatal("document link is missing")
	}
	tt.ClickAt(nextLink.X+10, nextLink.Y+nextLink.H/2)
	next := s.documents["next"]
	if next == nil || !next.preview || next.markdown.Anchor != "destination" || next.markdown.AnchorVersion == 0 {
		t.Fatalf("cross-document anchor not retained: %#v", next)
	}
}

func TestPreviewSelectionCopiesOriginalFilePathAcrossParagraphs(t *testing.T) {
	s := linkWorkspace(t)
	const source = "Read `next.md:9` then [website](https://example.com).\n\nSecond paragraph."
	s.snapshot.Files = []contracts.File{
		{ID: "guide", ProjectID: "project", Path: "docs/guide.md", Name: "guide.md", Content: source, Loaded: true, ReadOnly: true},
	}
	s.OnLinkConfig = func(_, documentPath string) linkui.Config {
		return linkui.Config{Internal: true, Resolve: func(raw string) (linkui.Destination, bool) {
			target, err := linktarget.ParsePreview(raw, documentPath)
			return linkui.Destination{Raw: raw, Kind: target.Kind, KnownFile: target.Kind == linktarget.File, Label: "next.md"}, err == nil
		}}
	}
	if err := s.OpenLink("root", "docs/guide.md"); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(s.View, 700, 500)
	file, ok := tt.Find("next.md")
	if !ok {
		t.Fatal("automatic basename link is missing")
	}
	tt.Press(file.X+1, file.Y+file.H/2)
	tt.Move(file.X+file.W-1, file.Y+file.H/2)
	tt.Release(file.X+file.W-1, file.Y+file.H/2)
	tt.Key(ui.Cmd, ui.KeyC)
	if tt.Clipboard() != "next.md:9" {
		t.Fatalf("file label copied %q", tt.Clipboard())
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Key(ui.Cmd, ui.KeyC)
	if tt.Clipboard() != "Read next.md:9 then website.\nSecond paragraph." {
		t.Fatalf("cross-paragraph selection copied %q", tt.Clipboard())
	}
	md, err := tt.ClipboardData().Read(transfer.Markdown)
	if err != nil || string(md) != source {
		t.Fatalf("Markdown clipboard=%q error=%v", md, err)
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyC)
	if tt.Clipboard() != source {
		t.Fatalf("Copy as Markdown copied %q", tt.Clipboard())
	}
}

type controlledImageRead struct {
	ref      string
	response chan []byte
}

type controlledImageService struct {
	contracts.Service
	requests chan controlledImageRead
}

func (service *controlledImageService) ReadResource(ctx context.Context, ref string) ([]byte, error) {
	request := controlledImageRead{ref: ref, response: make(chan []byte, 1)}
	select {
	case service.requests <- request:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case data := <-request.response:
		return data, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestDocumentImageIgnoresOlderReadCompletingLast(t *testing.T) {
	s := linkWorkspace(t)
	service := &controlledImageService{requests: make(chan controlledImageRead, 2)}
	s.backend = service
	s.openDocument(contracts.File{ID: "image", Path: "image.svg", Name: "image.svg"}, "root", "/workspace/image.svg", true)
	d := s.documents["image"]
	d.file.Kind, d.file.AssetRef, d.file.Version = "image", "resource://managed/old.svg", 1
	readOld, finishOld := s.documentImageRequest(d)
	type result struct {
		data []byte
		err  error
	}
	start := func(read func(context.Context) ([]byte, error)) <-chan result {
		done := make(chan result, 1)
		go func() { data, err := read(s.ctx); done <- result{data, err} }()
		return done
	}
	receiveRequest := func() controlledImageRead {
		select {
		case request := <-service.requests:
			return request
		case <-time.After(time.Second):
			t.Fatal("controlled image read did not start")
			return controlledImageRead{}
		}
	}
	oldDone := start(readOld)
	oldRequest := receiveRequest()
	d.file.AssetRef, d.file.Version = "resource://managed/new.svg", 2
	readNew, finishNew := s.documentImageRequest(d)
	newDone := start(readNew)
	newRequest := receiveRequest()
	if oldRequest.ref != "resource://managed/old.svg" || newRequest.ref != d.file.AssetRef {
		t.Fatal("reader did not snapshot its resource")
	}
	receive := func(done <-chan result) result {
		select {
		case value := <-done:
			return value
		case <-time.After(time.Second):
			t.Fatal("controlled image read did not complete")
			return result{}
		}
	}
	newRequest.response <- []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="40" height="40"/>`)
	newResult := receive(newDone)
	finishNew(newResult.data, newResult.err)
	newImage := d.image
	if newImage == nil {
		t.Fatal("new image did not decode")
	}
	oldRequest.response <- []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"/>`)
	oldResult := receive(oldDone)
	finishOld(oldResult.data, oldResult.err)
	finishOld(nil, fmt.Errorf("obsolete read failed"))
	if d.image != newImage || d.imageError != "" {
		t.Fatal("old image read replaced newer image or error")
	}
}

func TestImageRefreshWithSnapshotAndLinkedReadInEitherOrder(t *testing.T) {
	for _, snapshotFirst := range []bool{true, false} {
		for _, versionChanged := range []bool{true, false} {
			t.Run(fmt.Sprintf("snapshot-first=%t/version-changed=%t", snapshotFirst, versionChanged), func(t *testing.T) {
				s := linkWorkspace(t)
				oldFile := contracts.File{ID: "image", ProjectID: "project", Path: "image.svg", Name: "image.svg",
					Kind: "image", AssetRef: "resource://managed/old.svg", Version: 1, Loaded: true}
				newFile := oldFile
				newFile.AssetRef = "resource://managed/new.svg"
				if versionChanged {
					newFile.Version++
				}
				service := &linkedTestService{
					result: contracts.LinkedFileResult{File: newFile, AbsolutePath: "/workspace/image.svg"},
					resources: map[string][]byte{
						oldFile.AssetRef: []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"/>`),
						newFile.AssetRef: []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="40" height="40"/>`),
					},
				}
				s.backend = service
				s.snapshot.Files = []contracts.File{oldFile}
				s.OpenFile(oldFile.ID)
				d := s.documents[oldFile.ID]
				assertWidth := func(want float32) {
					t.Helper()
					svg, ok := d.image.(*ui.SVG)
					if !ok {
						t.Fatalf("image did not decode: error=%q", d.imageError)
					}
					width, _ := svg.Size()
					if width != want {
						t.Fatalf("image width=%v, want=%v", width, want)
					}
				}
				assertWidth(10)
				d.imageError = "obsolete image error"
				snapshot := s.snapshot
				snapshot.Files = []contracts.File{newFile}
				reopen := func() {
					if err := s.OpenLink("root", "image.svg"); err != nil {
						t.Fatal(err)
					}
				}
				if snapshotFirst {
					s.Update(snapshot)
					assertWidth(40)
					reopen()
				} else {
					reopen()
					assertWidth(40)
					s.Update(snapshot)
				}
				assertWidth(40)
				if d.imageError != "" || d.file.AssetRef != newFile.AssetRef {
					t.Fatalf("image resource did not refresh: file=%#v error=%q", d.file, d.imageError)
				}
			})
		}
	}
}

func TestSameVersionFileRestorationRefreshesMetadataAndPreservesDraft(t *testing.T) {
	for _, dirty := range []bool{false, true} {
		t.Run(fmt.Sprintf("dirty=%t", dirty), func(t *testing.T) {
			s := linkWorkspace(t)
			file := contracts.File{ID: "notes", ProjectID: "project", Path: "notes.md", Name: "notes.md",
				Kind: "file", Content: "accepted baseline", Version: 7, Loaded: true}
			s.snapshot.Files = []contracts.File{file}
			s.OpenFile(file.ID)
			d := s.documents[file.ID]
			if dirty {
				d.editor.Buffer.Set("unsaved draft")
				d.conflict = true
			}
			deleted := s.snapshot
			deleted.Files = nil
			s.Update(deleted)
			if !d.deleted || !d.file.ReadOnly {
				t.Fatal("deleted document did not become read-only")
			}
			restored := s.snapshot
			restored.Files = []contracts.File{file}
			s.Update(restored)
			if d.deleted || d.file.ReadOnly || d.editor.ReadOnly || !d.file.Loaded {
				t.Fatalf("same-version restored metadata is stale: %#v", d.file)
			}
			want := file.Content
			if dirty {
				want = "unsaved draft"
			}
			if d.editor.Buffer.String() != want || d.editor.Buffer.Modified() != dirty || d.conflict != dirty {
				t.Fatal("metadata update changed the buffer, saved status, or conflict")
			}
			if d.file.Content != file.Content || d.file.Version != file.Version {
				t.Fatal("metadata update changed the accepted edit baseline")
			}
			// Loading and access metadata can also change without a new version.
			metadata := file
			metadata.Loaded, metadata.ReadOnly, metadata.Name = false, true, "renamed.md"
			restored.Files = []contracts.File{metadata}
			s.Update(restored)
			if d.file.Loaded || !d.file.ReadOnly || !d.editor.ReadOnly || d.file.Name != metadata.Name || d.editor.Label != metadata.Name {
				t.Fatal("same-version metadata was ignored")
			}
			if d.editor.Buffer.String() != want || d.file.Content != file.Content || d.file.Version != file.Version || d.conflict != dirty {
				t.Fatal("access metadata discarded the edit baseline or draft")
			}
		})
	}
}
