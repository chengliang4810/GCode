package desktop

import (
	"fmt"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func linkTestApp(t *testing.T) *App {
	t.Helper()
	a := testApp(t, "default")
	a.state.Projects = []contracts.Project{{ID: "links", Path: "/workspace", Name: "Links"}}
	a.state.Sessions = []contracts.Session{
		{ID: "root-links", ProjectID: "links", CWD: "/workspace"},
		{ID: "child-links", ParentSessionID: "root-links", ProjectID: "links", CWD: "/workspace/child"},
		{ID: "other-links", ProjectID: "links", CWD: "/other"},
	}
	a.state.Files = []contracts.File{
		{ID: "guide", ProjectID: "links", Path: "guide.MARKDOWN", Name: "guide.MARKDOWN", Kind: "file", Loaded: true, Content: "# Guide\n\nNative preview."},
		{ID: "root-note", ProjectID: "links", Path: "note.md", Name: "note.md", Kind: "file", Loaded: true, Content: "# Root"},
		{ID: "child-note", ProjectID: "links", Path: "child/note.md", Name: "note.md", Kind: "file", Loaded: true, Content: "# Child"},
	}
	a.sessionID, a.projectID = "root-links", "links"
	a.workspace.ProjectID = a.projectID
	a.workspace.Update(a.state)
	a.workspace.SwitchSession(a.sessionID)
	return a
}

func linkMessageTester(t *testing.T, a *App, message contracts.Message, dark bool) *ui.Tester {
	t.Helper()
	a.palette = design.Palette("rcode", dark)
	return ui.NewTester(func(c *ui.Context) {
		c.DisableAnimations()
		c.SetTheme(a.palette.UI())
		a.services = c.Services()
		a.prepareTranscriptMarkdown(c)
		ui.Column(c).Fill().Padding(24).Selectable().Children(func() { a.transcriptMessageForTest(c, message) })
	}, 680, 500)
}

func TestTranscriptLinksShareFileRoutingAcrossRenderers(t *testing.T) {
	for _, scene := range []struct{ name, role, text string }{
		{"ordinary", "assistant", "Read [guide](guide.MARKDOWN)."},
		{"math", "assistant", `Use \(x^2\), then [guide](guide.MARKDOWN).`},
		{"reference", "user", "Use [review](gcode://command/review), then [guide](guide.MARKDOWN)."},
		{"table", "assistant", "| File |\n|---|\n| Read [guide](guide.MARKDOWN) |"},
	} {
		for _, dark := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/dark=%t", scene.name, dark), func(t *testing.T) {
				a := linkTestApp(t)
				a.draft = "Keep central draft"
				m := contracts.Message{ID: "links", SessionID: a.sessionID, Role: scene.role, Blocks: []contracts.Block{{ID: "text", Kind: "text", Text: scene.text}}}
				tt := linkMessageTester(t, a, m, dark)
				if err := tt.Click("guide"); err != nil {
					t.Fatal(err)
				}
				if a.workspace.ActiveKind() != "file" || a.workspace.TabCount() != 1 || a.errorText != "" {
					t.Fatalf("link did not open native file: kind=%s error=%s", a.workspace.ActiveKind(), a.errorText)
				}
				if a.sessionID != "root-links" || a.draft != "Keep central draft" || !a.right {
					t.Fatal("file link changed central conversation or did not reveal the side pane")
				}
				if urls := tt.OpenedURLs(); len(urls) != 0 {
					t.Fatalf("file link leaked to the system opener: %v", urls)
				}
			})
		}
	}
}

func TestTranscriptURLDefaultsToSideBrowserAndRejectsInvalidDestinations(t *testing.T) {
	for _, raw := range []string{"https://example.com/docs", "https://user:pass@example.com", "javascript:alert(1)", "https://example.com/%zz"} {
		t.Run(raw, func(t *testing.T) {
			a := linkTestApp(t)
			m := contracts.Message{ID: "web", SessionID: a.sessionID, Role: "assistant",
				Blocks: []contracts.Block{{ID: "text", Kind: "text", Text: "Visit [website](<" + raw + ">)"}}}
			tt := linkMessageTester(t, a, m, false)
			_ = tt.Click("website")
			if len(tt.OpenedURLs()) != 0 {
				t.Fatal("model URL escaped the internal link policy")
			}
			if raw == "https://example.com/docs" {
				if a.workspace.ActiveKind() != "browser" || !a.right {
					t.Fatal("valid URL did not open the browser pane")
				}
			} else if a.workspace.TabCount() != 0 {
				t.Fatal("invalid URL created a browser tab")
			}
		})
	}
}

func TestTranscriptSourceRemainsBoundAfterAnotherPaneBuilds(t *testing.T) {
	a := linkTestApp(t)
	messages := []contracts.Message{
		{ID: "root", SessionID: "root-links", Role: "assistant", Blocks: []contracts.Block{{ID: "root-text", Kind: "text", Text: "Read [root note](note.md)"}}},
		{ID: "child", SessionID: "child-links", Role: "assistant", Blocks: []contracts.Block{{ID: "child-text", Kind: "text", Text: "Read [child note](note.md)"}}},
	}
	tt := ui.NewTester(func(c *ui.Context) {
		a.services = c.Services()
		a.prepareTranscriptMarkdown(c)
		ui.Column(c).Fill().Children(func() {
			for _, message := range messages {
				c.Key(message.ID)
				a.transcriptMessageForTest(c, message)
			}
		})
	}, 760, 500)
	if err := tt.Click("root note"); err != nil {
		t.Fatal(err)
	}
	if a.workspace.TabCount() != 1 || len(a.workspace.RecentFiles()) != 1 || a.workspace.RecentFiles()[0].FileID != "root-note" {
		t.Fatal("root link inherited the child working directory")
	}
	if err := tt.Click("child note"); err != nil {
		t.Fatal(err)
	}
	if a.workspace.TabCount() != 2 || a.sessionID != "root-links" {
		t.Fatal("child link did not preserve source/root ownership")
	}
}

func TestLinkResolutionDisambiguatesKnownFilesAndProtectsRemoteSystemActions(t *testing.T) {
	a := linkTestApp(t)
	config := a.linkConfig("root-links", "", ui.Services{})
	destination, ok := config.Resolve("note.md")
	if !ok || !destination.KnownFile || destination.Label != "workspace/note.md" {
		t.Fatalf("ambiguous file label: %+v", destination)
	}
	destination, ok = config.Resolve("guide.MARKDOWN:2:3")
	if !ok || destination.Tooltip != "/workspace/guide.MARKDOWN:2:3" || !destination.Local {
		t.Fatalf("missing full target/position: %+v", destination)
	}
	a.state.Runtime = &contracts.RuntimeInfo{Remote: true}
	destination, ok = config.Resolve("guide.MARKDOWN")
	if !ok || destination.Local {
		t.Fatal("remote file advertised local shell actions")
	}
	a.workspace.SwitchSession("other-links")
	if _, ok := config.Resolve("https://example.com/docs"); ok {
		t.Fatal("stale link remained active after changing root conversation")
	}
}

func TestKnownInlineCodeBecomesFileLinkWhileUnknownCodeRemainsCode(t *testing.T) {
	a := linkTestApp(t)
	m := contracts.Message{ID: "inline", SessionID: a.sessionID, Role: "assistant", Blocks: []contracts.Block{
		{ID: "code", Kind: "text", Text: "Read `guide.MARKDOWN` then run `unknown()`."},
	}}
	tt := linkMessageTester(t, a, m, false)
	if !tt.HasText("unknown()") {
		t.Fatal("ordinary code was lost")
	}
	if err := tt.Click("guide.MARKDOWN"); err != nil {
		t.Fatal(err)
	}
	if a.workspace.ActiveKind() != "file" {
		t.Fatal("known inline path did not become a file link")
	}
	if strings.Contains(a.errorText, "unknown") {
		t.Fatal("ordinary code was treated as a link")
	}
}

func TestTranscriptFileLabelSelectionCopiesOriginalPath(t *testing.T) {
	const source = "Read `/workspace/guide.MARKDOWN` then [reference](https://example.com)."
	for _, role := range []string{"assistant", "user"} {
		t.Run(role, func(t *testing.T) {
			a := linkTestApp(t)
			m := contracts.Message{ID: "copy", SessionID: a.sessionID, Role: role, Blocks: []contracts.Block{
				{ID: "copy-body", Kind: "text", Text: source},
			}}
			tt := linkMessageTester(t, a, m, false)
			file, ok := tt.Find("guide.MARKDOWN")
			if !ok {
				t.Fatal("known file label is missing")
			}
			tt.Press(file.X+1, file.Y+file.H/2)
			tt.Move(file.X+file.W-1, file.Y+file.H/2)
			tt.Release(file.X+file.W-1, file.Y+file.H/2)
			tt.Key(ui.Cmd, ui.KeyC)
			if tt.Clipboard() != "/workspace/guide.MARKDOWN" || a.workspace.TabCount() != 0 {
				t.Fatalf("drag selection opened a file or lost its original path: tabs=%d copied=%q", a.workspace.TabCount(), tt.Clipboard())
			}
			tt.Key(ui.Cmd, ui.KeyA)
			tt.Key(ui.Cmd|ui.Shift, ui.KeyC)
			// The assistant's selectable scope also contains its timestamp.
			if !strings.HasPrefix(tt.Clipboard(), source) {
				t.Fatalf("whole paragraph Markdown copy=%q", tt.Clipboard())
			}
		})
	}
}
