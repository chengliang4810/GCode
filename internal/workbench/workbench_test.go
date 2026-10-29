package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"image"
	"io"
	"strings"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/mock"

	"github.com/egoist/mygo/ui"
)

func testWorkspace(t *testing.T) (*State, *mock.Server, contracts.Snapshot) {
	t.Helper()
	backend, err := mock.New(mock.Options{Instant: true})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := backend.Bootstrap(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Settings.General.Language = "en"
	if err := backend.UpdateSettings(context.Background(), snapshot.Settings); err != nil {
		t.Fatal(err)
	}
	state := New(nil, backend)
	state.Update(snapshot)
	t.Cleanup(func() { _ = state.Close(); _ = backend.Close() })
	return state, backend, snapshot
}

func TestFileSaveAndUnsavedClose(t *testing.T) {
	s, backend, snapshot := testWorkspace(t)
	f := snapshot.Files[0]
	for _, candidate := range snapshot.Files {
		if candidate.Kind != "directory" && candidate.Kind != "image" && !candidate.ReadOnly {
			f = candidate
			break
		}
	}
	s.OpenFile(f.ID)
	s.OpenFile(f.ID)
	if s.TabCount() != 1 {
		t.Fatalf("duplicate document tabs: %d", s.TabCount())
	}
	d := s.documents[f.ID]
	if d == nil {
		t.Fatal("file did not open")
	}
	d.editor.Buffer.Set("package main\n\n// saved edit\n")
	s.CloseActive()
	if s.pendingClose != f.ID || s.TabCount() != 1 {
		t.Fatal("unsaved document closed without confirmation")
	}
	s.pendingClose = ""
	d.closeAfterSave = true
	s.save(f.ID)
	if s.TabCount() != 0 {
		t.Fatal("saved document was not closed after acknowledgement")
	}
	current, err := backend.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range current.Files {
		if candidate.ID == f.ID && candidate.Content != "package main\n\n// saved edit\n" {
			t.Fatalf("save lost edit: %q", candidate.Content)
		}
	}
}

func TestConflictPreservesLocalBuffer(t *testing.T) {
	s, backend, snapshot := testWorkspace(t)
	var file contracts.File
	for _, f := range snapshot.Files {
		if f.Kind != "directory" && f.Kind != "image" && !f.ReadOnly {
			file = f
			break
		}
	}
	s.OpenFile(file.ID)
	d := s.documents[file.ID]
	d.editor.Buffer.Set("local edit")
	if err := backend.Execute(context.Background(), contracts.Command{Kind: "file.save", ID: file.ID, Text: "remote edit", Version: file.Version}); err != nil {
		t.Fatal(err)
	}
	current, err := backend.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	s.Update(current)
	if !d.conflict || d.editor.Buffer.String() != "local edit" {
		t.Fatal("server update destroyed unsaved local content")
	}
	s.save(file.ID)
	if s.Error == "" {
		t.Fatal("stale version was accepted")
	}
}

func TestNativeSurfacesRenderHeadlessly(t *testing.T) {
	s, _, snapshot := testWorkspace(t)
	for _, kind := range []string{"files", "changes", "history", "pullrequests"} {
		t.Run(kind, func(t *testing.T) {
			s.OpenSurface(kind)
			tt := ui.NewTester(s.View, 720, 560)
			tt.Frame()
			if len(tt.Texts()) == 0 {
				t.Fatal("empty native surface")
			}
			s.CloseActive()
			tt.Frame()
		})
	}
	if len(snapshot.Changes) > 0 {
		change := snapshot.Changes[0]
		s.OpenChange(change.ID)
		tt := ui.NewTester(s.View, 720, 560)
		tt.Frame()
		s.CloseActive()
	}
	if len(snapshot.PullRequests) > 0 {
		pr := snapshot.PullRequests[0]
		s.OpenPR(pr.ID)
		tt := ui.NewTester(s.View, 720, 560)
		tt.Frame()
		if !tt.HasText(pr.Title) {
			t.Fatal("PR detail title missing")
		}
		s.CloseActive()
	}
}

func TestNormalizeURL(t *testing.T) {
	for _, tt := range []struct {
		input, want string
		valid       bool
	}{
		{"go.dev", "https://go.dev", true},
		{" https://example.com/a?q=b ", "https://example.com/a?q=b", true},
		{"http://localhost:8080/test", "http://localhost:8080/test", true},
		{"file:///C:/secret", "", false},
		{"javascript:alert(1)", "", false},
		{"https://user:password@example.com", "", false},
		{"", "", false},
	} {
		t.Run(tt.input, func(t *testing.T) {
			got, err := NormalizeURL(tt.input)
			if (err == nil) != tt.valid || got != tt.want {
				t.Fatalf("NormalizeURL(%q)=%q,%v", tt.input, got, err)
			}
		})
	}
}

func TestTerminalTransportIsInMemoryAndCloses(t *testing.T) {
	var lines []string
	conn := newStreamConn(func(line string) { lines = append(lines, line) }, nil)
	if _, err := conn.Write([]byte("echo hello\r")); err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || lines[0] != "echo hello" {
		t.Fatalf("input callbacks: %#v", lines)
	}
	buffer := make([]byte, 64)
	n, err := conn.Read(buffer)
	if err != nil || string(buffer[:n]) != "echo hello\r\n" {
		t.Fatalf("echo: %q, %v", buffer[:n], err)
	}
	if got := conn.stripEcho("echo hello\r\nhello\r\n"); got != "hello\r\n" {
		t.Fatalf("duplicate echo: %q", got)
	}
	finished := make(chan error, 1)
	go func() { _, err := conn.Read(buffer); finished <- err }()
	_ = conn.Close()
	select {
	case err := <-finished:
		if !errors.Is(err, io.EOF) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("closed connection leaked blocked reader")
	}
	if _, err := conn.Write([]byte("x")); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal("write after close succeeded")
	}
}

func TestTerminalUnicodeBackspace(t *testing.T) {
	line := ""
	conn := newStreamConn(func(s string) { line = s }, nil)
	defer conn.Close()
	_, _ = conn.Write([]byte("echo 你好\x7f呀\r"))
	if line != "echo 你呀" {
		t.Fatalf("UTF-8 input damaged: %q", line)
	}
	if err := conn.push([]byte(strings.Repeat("x", 8*1024*1024))); err == nil {
		t.Fatal("unbounded output queue accepted")
	}
}

func TestFileActionsCreateRenameDelete(t *testing.T) {
	s, backend, snapshot := testWorkspace(t)
	s.ProjectID = snapshot.Projects[0].ID
	s.requestFileAction("create", "")
	s.fileActionName = "notes.md"
	s.performFileAction()
	if s.Error != "" {
		t.Fatal(s.Error)
	}
	var created contracts.File
	current, _ := backend.Snapshot(context.Background())
	for _, f := range current.Files {
		if f.Name == "notes.md" {
			created = f
			break
		}
	}
	if created.ID == "" {
		t.Fatal("create did not publish a file")
	}
	s.OpenFile(created.ID)
	s.documents[created.ID].editor.Buffer.Set("local draft")
	s.requestFileAction("rename", created.ID)
	s.fileActionName = "journal.md"
	s.performFileAction()
	d := s.documents[created.ID]
	if d.file.Name != "journal.md" || d.conflict || d.editor.Buffer.String() != "local draft" {
		t.Fatal("rename lost local edit or introduced conflict")
	}
	s.requestFileAction("delete", created.ID)
	s.performFileAction()
	current, _ = backend.Snapshot(context.Background())
	for _, f := range current.Files {
		if f.ID == created.ID {
			t.Fatal("file was not deleted")
		}
	}
	if !d.deleted || d.editor.Buffer.String() != "local draft" {
		t.Fatal("deleted document lost unsaved content")
	}
}

func TestHistoryTopologyMerge(t *testing.T) {
	commits := []contracts.Commit{{SHA: "merge", Parents: []string{"left", "right"}}, {SHA: "left", Parents: []string{"base"}}, {SHA: "right", Parents: []string{"base"}}, {SHA: "base"}}
	rows, columns := layoutHistory(commits)
	if columns != 2 || len(rows) != 4 {
		t.Fatalf("merge lanes=%d,rows=%d", columns, len(rows))
	}
	var mergeOut, baseIn int
	for _, segment := range rows[0].segments {
		if segment.shape == "outgoing" {
			mergeOut++
		}
	}
	for _, segment := range rows[3].segments {
		if segment.shape == "incoming" {
			baseIn++
		}
	}
	if mergeOut != 2 || baseIn != 2 {
		t.Fatalf("split/rejoin lost parents: outgoing=%d,incoming=%d", mergeOut, baseIn)
	}
}

func TestTerminalCRLFAndEscapeFiltering(t *testing.T) {
	var lines []string
	c := newStreamConn(func(line string) { lines = append(lines, line) }, nil)
	defer c.Close()
	_, err := c.Write([]byte("echo \x1b[Ahello\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || lines[0] != "echo hello" {
		t.Fatalf("terminal control sequences leaked into command: %#v", lines)
	}
}

func TestSurfacePickerMatchesReferenceChoices(t *testing.T) {
	s, _, _ := testWorkspace(t)
	s.ProjectID = s.snapshot.Projects[0].ID
	tt := ui.NewTester(s.View, 520, 842)
	for _, label := range []string{"Browser", "Terminal", "Diffs", "History"} {
		if !tt.HasText(label) {
			t.Fatalf("missing reference picker choice %q", label)
		}
	}
	for _, label := range []string{"Open in workspace", "Files", "Pull requests"} {
		if tt.HasText(label) {
			t.Fatalf("non-reference picker entry %q", label)
		}
	}
	for i := range s.snapshot.Projects {
		// A stale branch label is not evidence of a currently detected repo.
		s.snapshot.Projects[i].GitDetected = false
	}
	tt.Frame()
	if tt.HasText("Diffs") || tt.HasText("History") {
		t.Fatal("Git-only surfaces shown for a non-Git project")
	}
	for i := range s.snapshot.Projects {
		s.snapshot.Projects[i].GitDetected = true
		s.snapshot.Projects[i].Branch = ""
		s.snapshot.Projects[i].Branches = nil
	}
	tt.Frame()
	if !tt.HasText("Diffs") || !tt.HasText("History") {
		t.Fatal("detected Git without a loaded branch list lost its launcher entries")
	}
}

func TestImageGeometryFitAndCursorAnchor(t *testing.T) {
	g := imageGeometry{}
	g.resize(1000, 500, 500, 300)
	x, y := g.origin()
	if g.scale != .5 || x != 0 || y != 25 {
		t.Fatalf("fit scale/origin = %v %v %v", g.scale, x, y)
	}
	ix, iy := (100-x)/g.scale, (150-y)/g.scale
	g.zoom(2, 100, 150)
	x, y = g.origin()
	if dx, dy := (100-x)/g.scale-ix, (150-y)/g.scale-iy; dx > .001 || dx < -.001 || dy > .001 || dy < -.001 {
		t.Fatal("zoom moved the cursor's image point")
	}
	g.zoom(1e9, 250, 150)
	if g.scale != 32 {
		t.Fatalf("unbounded zoom %v", g.scale)
	}
	g.pan(1e9, -1e9)
	if g.panX != 15750 || g.panY != -7850 {
		t.Fatalf("unbounded pan %v %v", g.panX, g.panY)
	}
	g.zoom(1e-9, 250, 150)
	if g.scale != .01 || g.panX != 0 || g.panY != 0 {
		t.Fatal("minimum scale/pan differs from source")
	}
}

func TestAnnotationPollCapsAndPreservesData(t *testing.T) {
	picks := make([]annotationPick, 30)
	for i := range picks {
		picks[i] = annotationPick{Annotation: contracts.Annotation{Element: "button", Text: "Save", HTML: strings.Repeat("界", 1000), Selector: "#save", Path: []string{"body", "main", "button"}}, Marker: uint32(i + 1)}
	}
	data, err := json.Marshal(annotationPoll{State: "active", Picks: picks})
	if err != nil {
		t.Fatal(err)
	}
	poll, err := decodeAnnotationPoll(string(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(poll.Picks) != 24 {
		t.Fatalf("unbounded pick count: %d", len(poll.Picks))
	}
	if poll.Picks[0].Selector != "#save" || poll.Picks[0].Marker != 1 || len(poll.Picks[0].HTML) > 2403 {
		t.Fatal("annotation data lost or exceeded source caps")
	}
	for _, value := range []any{nil, 12, `{}`, strings.Repeat("x", 1024*1024+1)} {
		if _, err := decodeAnnotationPoll(value); err == nil {
			t.Fatalf("malformed poll accepted: %T", value)
		}
	}
}

func TestImageViewportSettlesHeadlessly(t *testing.T) {
	var viewport ImageViewport
	bitmap := ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 1000, 500)))
	tt := ui.NewTester(func(c *ui.Context) { viewport.View(c, bitmap) }, 500, 300)
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	if viewport.geometry.scale != .5 {
		t.Fatalf("native image viewport did not fit: %g", viewport.geometry.scale)
	}
	if tt.FramePending() {
		t.Fatal("image viewport never stopped rebuilding")
	}
}

func TestPRActivityCommentUpdatesBackend(t *testing.T) {
	s, backend, snapshot := testWorkspace(t)
	pr := snapshot.PullRequests[0]
	s.OpenPR(pr.ID)
	s.prTab = 2
	tt := ui.NewTester(s.View, 720, 700)
	if err := tt.Click("Write a comment"); err != nil {
		t.Fatal(err)
	}
	tt.Type("The updated spacing looks good.")
	if err := tt.Click("Comment"); err != nil {
		t.Fatal(err)
	}
	current, err := backend.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range current.PullRequests {
		if candidate.ID == pr.ID {
			if len(candidate.Comments) != len(pr.Comments)+1 {
				t.Fatal("comment was not persisted")
			}
			return
		}
	}
	t.Fatal("pull request disappeared")
}
