//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/storage"
)

// The transcript is produced and persisted by the real local core talking to a
// loopback model fixture. Native link actions then read real host files and load
// a loopback web page; no transcript or snapshot is fabricated.
func TestDesktopLocalTranscriptLinks(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			binary, artifacts := testPaths(t)
			data, workspace := filepath.Join(artifacts, "data"), filepath.Join(artifacts, "workspace")
			if err := os.MkdirAll(filepath.Join(workspace, "示例"), 0700); err != nil {
				t.Fatal(err)
			}
			const source = "package main\n\n// 中文🙂 TARGET\nfunc main() {}\n"
			sourcePath := filepath.Join(workspace, "示例", "source.go")
			for name, contents := range map[string]string{
				"README.MARKDOWN": "# NATIVE README PREVIEW\n\n[Preview source](示例/source.go:3:8)\n",
				"示例/source.go":    source,
				"icon.svg":        `<svg xmlns="http://www.w3.org/2000/svg" width="40" height="40"><rect width="40" height="40" fill="#c23668"/></svg>`,
			} {
				if err := os.WriteFile(filepath.Join(workspace, filepath.FromSlash(name)), []byte(contents), 0600); err != nil {
					t.Fatal(err)
				}
			}
			var pngData bytes.Buffer
			bitmap := image.NewNRGBA(image.Rect(0, 0, 40, 40))
			for y := range 40 {
				for x := range 40 {
					bitmap.SetNRGBA(x, y, color.NRGBA{R: 42, G: 132, B: 208, A: 255})
				}
			}
			if err := png.Encode(&pngData, bitmap); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(workspace, "image.png"), pngData.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			var pageRequests atomic.Int32
			page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/links-proof" {
					http.NotFound(w, r)
					return
				}
				pageRequests.Add(1)
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				fmt.Fprint(w, `<!doctype html><html><head><title>NATIVE LOCAL LINK PAGE</title><link rel="icon" href="data:,"></head><body><h1>Loopback link opened inside GCode</h1></body></html>`)
			}))
			defer page.Close()
			pageURL := page.URL + "/links-proof"
			reply := fmt.Sprintf("[Open README](README.MARKDOWN)\n\n[Source line](<%s:3:8>)\n\n[PNG image](image.png)\n\n[SVG image](icon.svg)\n\n[Local web page](%s)",
				filepath.ToSlash(sourcePath), pageURL)
			var modelRequests atomic.Int32
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				number := modelRequests.Add(1)
				if err := os.WriteFile(filepath.Join(artifacts, fmt.Sprintf("provider-request-%02d.json", number)), body, 0600); err != nil {
					t.Error(err)
				}
				payload, err := json.Marshal(map[string]any{"choices": []any{map[string]any{
					"delta": map[string]any{"content": reply}, "finish_reason": "stop",
				}}})
				if err != nil {
					t.Error(err)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", payload)
			}))
			defer provider.Close()
			store, err := storage.Open(storage.Options{Directory: data})
			if err != nil {
				t.Fatal(err)
			}
			core, err := openExecutionFixture(t, engine.Options{Storage: store})
			if err != nil {
				_ = store.Close()
				t.Fatal(err)
			}
			defer core.Close()
			snapshot, err := core.Snapshot(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			settings := snapshot.Settings
			settings.Appearance.ReduceMotion = true
			settings.General.CompactMode = false
			settings.General.OpenLinksExternally = false
			settings.Appearance.FontSize, settings.Appearance.CodeFontSize = 14, 12
			config := model.Provider{ID: "links", Name: "Local link fixture", Protocol: model.Chat,
				BaseURL: provider.URL, Models: []model.Model{{ID: "test"}}}
			for _, command := range []contracts.Command{
				{Kind: "settings.update", Settings: &settings},
				{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: config}},
				{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: workspace, Name: "Native link fixture"}},
			} {
				if err := core.Execute(t.Context(), command); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, err = core.Snapshot(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if err := core.Execute(t.Context(), contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
				RequestID: "links-seed", ProjectID: snapshot.Projects[0].ID, ModelID: "links/test", Text: "Show the local fixture links.",
			}}); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(stateTimeout)
			for {
				snapshot, err = core.Snapshot(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				if len(snapshot.Sessions) == 1 && snapshot.Sessions[0].Status == contracts.Idle && snapshot.Sessions[0].LastOutcome == "completed" {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("real core link fixture did not complete")
				}
				time.Sleep(20 * time.Millisecond)
			}
			sessionID := snapshot.Sessions[0].ID
			snapshot, err = core.Snapshot(t.Context(), sessionID)
			if err != nil {
				t.Fatal(err)
			}
			messageCount := len(snapshot.Messages[sessionID])
			if modelRequests.Load() != 1 {
				t.Fatalf("model fixture received %d requests, want 1", modelRequests.Load())
			}
			var persistedReply bool
			for _, message := range snapshot.Messages[sessionID] {
				for _, block := range message.Blocks {
					persistedReply = persistedReply || block.Kind == "text" && block.Text == reply
				}
			}
			if !persistedReply {
				t.Fatal("real provider reply is absent from the core transcript")
			}
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}

			app := startApp(t, binary, artifacts, "links", data, "--local", "--appearance", appearance,
				"--language", "en", "--width", "900", "--height", "880")
			app.act("back-to-conversations", "press", "")
			app.act("sidebar-session:"+sessionID, "press", "")
			// The new-conversation canvas already has an "input" key. Wait
			// for the requested conversation, rather than typing into that
			// previous canvas while the sidebar action is still queued.
			app.waitKey("titlebar-session-info")
			app.await("requested conversation and its real transcript are active before typing", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				for _, node := range tree.Nodes {
					if node.Role == "link" && node.Label == "Open README" {
						return nil
					}
				}
				return fmt.Errorf("requested source transcript has not appeared")
			})
			const draft = "KEEP CENTRAL LINK DRAFT"
			app.act("input", "setValue", draft)
			app.assertLinkDraft(draft)
			app.await("central draft has committed to the application before changing pane layout", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				send, err := tree.findKey("send")
				if err != nil {
					return err
				}
				if !slices.Contains(send.Actions, "press") {
					return fmt.Errorf("central draft is visible but its business-bound send action is not enabled")
				}
				return nil
			})
			app.capture("committed-central-draft")

			// A newly created browser tab must stay blank and must not create a
			// WebView or load go.dev before the user supplies an address.
			app.act("titlebar-right-pane", "press", "")
			app.act("browser", "press", "")
			app.await("new native browser tab is blank", func() error {
				tree, err := app.linkTree()
				if err != nil {
					return err
				}
				var addresses int
				for _, node := range tree.Nodes {
					if strings.Contains(node.Label, "go.dev") || strings.Contains(node.Value, "go.dev") {
						return fmt.Errorf("new browser exposes the removed go.dev default")
					}
					if node.WebView {
						return fmt.Errorf("blank browser created a WebView")
					}
					if node.Label == "Website address" {
						addresses++
						if node.Value != "" {
							return fmt.Errorf("new browser address=%q, want empty", node.Value)
						}
					}
				}
				if addresses != 1 || pageRequests.Load() != 0 {
					return fmt.Errorf("blank browser addresses=%d page requests=%d", addresses, pageRequests.Load())
				}
				return nil
			})
			app.capture("blank-browser")
			app.assertLinkDraft(draft)

			app.pressLinkLabel("link", "Open README")
			app.await("uppercase MARKDOWN file opens native preview", func() error {
				tree, err := app.linkTree()
				if err != nil {
					return err
				}
				var heading, edit bool
				for _, node := range tree.Nodes {
					heading = heading || node.Role == "heading" && node.Label == "NATIVE README PREVIEW"
					edit = edit || node.Role == "button" && node.Label == "Edit"
					if node.Role == "textField" && node.Label == "README.MARKDOWN" {
						return fmt.Errorf("Markdown opened an editor instead of its preview")
					}
				}
				if !heading || !edit {
					return fmt.Errorf("preview heading=%t edit action=%t", heading, edit)
				}
				return nil
			})
			app.capture("markdown-default-preview")
			app.assertLinkDraft(draft)

			// The relative preview link and the absolute transcript link must
			// both navigate to line 3, character column 8 after Chinese/emoji.
			selection := utf8.RuneCountInString("package main\n\n") + 7
			app.pressLinkLabel("link", "Preview source")
			app.assertLinkSource(source, selection)
			app.capture("preview-relative-source-location")
			app.pressLinkLabel("link", "Source line")
			app.assertLinkSource(source, selection)
			app.capture("transcript-absolute-source-location")
			app.assertLinkDraft(draft)

			for _, name := range []string{"PNG image", "SVG image"} {
				app.pressLinkLabel("link", name)
				app.await("native "+name+" preview decodes", func() error {
					tree, err := app.linkTree()
					if err != nil {
						return err
					}
					var preview bool
					for _, node := range tree.Nodes {
						if node.Label == "Unable to open image." {
							return fmt.Errorf("native image has not decoded")
						}
						preview = preview || node.Label == "Image preview"
					}
					if !preview {
						return fmt.Errorf("native image preview is missing")
					}
					return nil
				})
				app.capture(strings.ToLower(strings.Fields(name)[0]) + "-native-image")
				app.assertLinkDraft(draft)
			}
			app.pressLinkLabel("link", "Local web page")
			app.await("URL opens the loopback page in the native browser pane", func() error {
				tree, err := app.linkTree()
				if err != nil {
					return err
				}
				var address, webview, title bool
				for _, node := range tree.Nodes {
					address = address || node.Label == "Website address" && node.Value == pageURL
					webview = webview || node.WebView
					title = title || node.Role == "tab" && node.Label == "NATIVE LOCAL LINK PAGE"
				}
				if !address || !webview || !title || pageRequests.Load() < 1 {
					return fmt.Errorf("browser address=%t WebView=%t title=%t loopback requests=%d", address, webview, title, pageRequests.Load())
				}
				return nil
			})
			app.capture("loopback-browser")
			app.assertLinkDraft(draft)
			app.await("link actions preserve the central session and its history", func() error {
				current, err := activityDiskSnapshot(data)
				if err != nil {
					return err
				}
				if len(current.Sessions) != 1 || current.Sessions[0].ID != sessionID ||
					len(current.Messages[sessionID]) != messageCount || modelRequests.Load() != 1 {
					return fmt.Errorf("link actions changed session/history or requested the model")
				}
				return nil
			})
			if err := storage.WriteJSON(filepath.Join(artifacts, "link-result.json"), map[string]any{
				"sessionId": sessionID, "appearance": appearance, "providerRequests": modelRequests.Load(),
				"loopbackPageRequests": pageRequests.Load(), "sourceSelectionRunes": selection,
				"realLocalCore": true, "externalProvider": false, "blankBrowserURL": "",
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

type transcriptLinkNode struct {
	accessNode
	Selection *[2]int `json:"selection,omitempty"`
	WebView   bool    `json:"webView,omitempty"`
}

type transcriptLinkTree struct {
	Nodes []transcriptLinkNode `json:"nodes"`
}

func (app *nativeApp) linkTree() (transcriptLinkTree, error) {
	var tree transcriptLinkTree
	err := app.client.call("tree", nil, &tree)
	return tree, err
}

func (app *nativeApp) pressLinkLabel(role, label string) {
	app.t.Helper()
	// Resolve the current node immediately before acting. Never keep numeric
	// IDs between frames; these inline links expose their authored labels.
	app.await("activate "+role+" "+label, func() error {
		tree, err := app.client.tree()
		if err != nil {
			return err
		}
		var matched []accessNode
		for _, node := range tree.Nodes {
			if node.Role == role && node.Label == label && slices.Contains(node.Actions, "press") {
				matched = append(matched, node)
			}
		}
		if len(matched) != 1 {
			return fmt.Errorf("%s %q has %d actionable nodes", role, label, len(matched))
		}
		return app.client.call("act", map[string]any{"id": matched[0].ID, "action": "press"}, nil)
	})
}

func (app *nativeApp) assertLinkDraft(want string) {
	app.t.Helper()
	app.await("central composer preserves its draft while opening links", func() error {
		tree, err := app.client.tree()
		if err != nil {
			return err
		}
		input, err := tree.findKey("input")
		if err != nil {
			return err
		}
		if input.Value != want {
			return fmt.Errorf("central draft=%q, want %q", input.Value, want)
		}
		return nil
	})
}

func (app *nativeApp) assertLinkSource(content string, selection int) {
	app.t.Helper()
	app.await("Unicode source opens in code mode at the linked character column", func() error {
		tree, err := app.linkTree()
		if err != nil {
			return err
		}
		for _, node := range tree.Nodes {
			if node.Role == "textField" && node.Label == "source.go" {
				if node.Value != content || node.Selection == nil ||
					node.Selection[0] != selection || node.Selection[1] != selection {
					return fmt.Errorf("source editor content=%t selection=%v, want [%d,%d]",
						node.Value == content, node.Selection, selection, selection)
				}
				return nil
			}
		}
		return fmt.Errorf("native source editor is missing")
	})
}
