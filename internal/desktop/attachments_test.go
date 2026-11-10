package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gcode/internal/assets"
	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/storage"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

func TestClipboardFilesReachModelThroughRealCore(t *testing.T) {
	for _, surface := range []string{"canvas", "primary", "side"} {
		t.Run(surface, func(t *testing.T) {
			requests := make(chan string, 4)
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Messages []struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					} `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				for _, message := range request.Messages {
					if message.Role == "user" {
						requests <- message.Content
					}
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Files received.\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			}))
			defer provider.Close()
			store, err := storage.Open(storage.Options{Home: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			core, err := engine.Open(engine.Options{Storage: store})
			if err != nil {
				t.Fatal(err)
			}
			defer core.Close()
			snapshot, _ := core.Snapshot(t.Context())
			settings := snapshot.Settings
			settings.General.Language = "en"
			settings.General.ThreadNamingModel = "first-message"
			config := model.Provider{ID: "files", Name: "Files fixture", Protocol: model.Chat, BaseURL: provider.URL, Models: []model.Model{{ID: "test"}}}
			for _, command := range []contracts.Command{
				{Kind: "settings.update", Settings: &settings},
				{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: config}},
				{Kind: "session.create", Name: "Attachment conversation"},
			} {
				if err := core.Execute(t.Context(), command); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, _ = core.Snapshot(t.Context())
			mainID := snapshot.Sessions[0].ID
			if surface == "side" {
				if err := core.Execute(t.Context(), contracts.Command{Kind: "session.sidechat", SessionID: mainID}); err != nil {
					t.Fatal(err)
				}
			}
			a, err := New(core)
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			a.modelID = "files/test"
			label := "Message"
			if surface != "canvas" {
				a.selectSession(mainID)
			}
			var side *sideComposerState
			if surface == "side" {
				child := a.state.Sessions[0]
				a.workspace.OpenSession(child.ID)
				side = a.sideComposer(child.ID)
				side.modelID = "files/test"
				label = "Message to " + child.Title
				a.draft = "Keep the central draft"
			}
			tt := ui.NewTester(a.View, 1320, 880)
			if err := tt.Click(label); err != nil {
				t.Fatal(err)
			}
			tt.Type("Inspect the files")
			directory := t.TempDir()
			var sources []string
			for _, name := range []string{"报告 空格.pdf", "sheet.xlsx", "archive.zip"} {
				path := filepath.Join(directory, name)
				if err := os.WriteFile(path, []byte("BINARY_PAYLOAD\x00\xff"), 0600); err != nil {
					t.Fatal(err)
				}
				sources = append(sources, path)
			}
			data, err := transfer.FileData(sources...)
			if err != nil {
				t.Fatal(err)
			}
			tt.SetClipboardData(data)
			tt.Command("paste")
			items, draft := a.attachments, a.draft
			if side != nil {
				items, draft = side.Attachments, side.Draft
				if len(a.attachments) != 0 || a.draft != "Keep the central draft" {
					t.Fatal("side paste changed the central composer")
				}
			}
			if len(items) != 3 || draft != "Inspect the files" {
				t.Fatalf("file paste lost attachments or draft: %+v, %q, %s", items, draft, a.errorText)
			}
			if err := os.RemoveAll(directory); err != nil {
				t.Fatal(err)
			}
			tt.Key(0, ui.KeyEnter)
			select {
			case prompt := <-requests:
				if !strings.HasPrefix(prompt, "Inspect the files") || strings.Contains(prompt, "BINARY_PAYLOAD") {
					t.Fatalf("unexpected model prompt: %q", prompt)
				}
				for _, item := range items {
					path := filepath.Join(store.Directory(), "uploads", item.ID)
					if !strings.Contains(prompt, fmt.Sprintf("name=%q path=%q", item.Name, path)) {
						t.Fatalf("model did not receive the host path for %q: %q", item.Name, prompt)
					}
					if payload, err := os.ReadFile(path); err != nil || string(payload) != "BINARY_PAYLOAD\x00\xff" {
						t.Fatalf("model attachment path is unreadable: %q, %v", payload, err)
					}
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("paste/send did not reach model: %s", a.errorText)
			}
		})
	}
}

type completingAttachmentService struct {
	contracts.Service
	importer    contracts.AttachmentImporter
	afterImport func()
}

type sessionAttachmentService struct {
	contracts.Service
	sessions *[]string
}

func (s sessionAttachmentService) ImportAttachments(_ context.Context, request contracts.AttachmentImportRequest) ([]contracts.Attachment, error) {
	*s.sessions = append(*s.sessions, request.SessionID)
	return nil, nil
}

func TestAttachmentImportUsesItsComposerSession(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	var sessions []string
	a.service = sessionAttachmentService{Service: a.service, sessions: &sessions}
	request := contracts.AttachmentImportRequest{Clipboard: &contracts.ClipboardAttachment{Name: "notes.txt", Data: []byte("notes")}}
	a.importAttachments(request)
	a.sideAttachmentWork("chat-1", a.sideComposer("chat-1"), request, false)
	if len(sessions) != 2 || sessions[0] != "chat-0" || sessions[1] != "chat-1" {
		t.Fatalf("composer imports lost their source session: %v", sessions)
	}
}

func (s completingAttachmentService) ImportAttachments(ctx context.Context, request contracts.AttachmentImportRequest) ([]contracts.Attachment, error) {
	items, err := s.importer.ImportAttachments(ctx, request)
	s.afterImport()
	return items, err
}

func TestAttachmentCompletionCannotRecreateDeletedSession(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	path := filepath.Join(t.TempDir(), "r11-owned.txt")
	if err := os.WriteFile(path, []byte("R11 attachment"), 0600); err != nil {
		t.Fatal(err)
	}
	service := a.service
	a.service = completingAttachmentService{Service: service, importer: service.(contracts.AttachmentImporter), afterImport: func() {
		a.dispatch(contracts.Command{Kind: "session.delete", ID: "chat-0"})
	}}
	a.importAttachments(contracts.AttachmentImportRequest{Paths: []string{path}})
	if a.sessionID != "" || len(a.attachments) != 0 || len(a.attachmentImports) != 0 {
		t.Fatal("late attachment changed the restored canvas or retained busy state")
	}
	if _, exists := a.sessionStates["chat-0"]; exists {
		t.Fatal("late attachment recreated deleted view")
	}
}

func TestNativeClipboardImageStagesAnAttachment(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 1320, 880)
	data, err := assets.Read("app-icon.png")
	if err != nil {
		t.Fatal(err)
	}
	tt.SetClipboardData(transfer.New(transfer.NewItem(transfer.Bytes(transfer.PNG, data))))
	if err := tt.Click("Message"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Keep this text")
	tt.Command("paste")
	if a.draft != "Keep this text" || len(a.attachments) != 1 || a.attachments[0].Width == 0 {
		t.Fatalf("image paste lost text or image metadata: %q %#v / %s", a.draft, a.attachments, a.errorText)
	}
	if loadMedia(a.attachments[0].AssetRef).bitmap == nil {
		t.Fatal("managed image was not decoded on its state transition")
	}
	tt.SetClipboard(" and paste text")
	tt.Command("paste")
	if a.draft != "Keep this text and paste text" {
		t.Fatal("plain-text paste was intercepted")
	}
}

func TestFilePickerAdapterUsesSameImportPath(t *testing.T) {
	a := testApp(t, "default")
	path := filepath.Join(t.TempDir(), "design.md")
	if err := os.WriteFile(path, []byte("# Design\n"), 0600); err != nil {
		t.Fatal(err)
	}
	a.pickAttachmentFiles = func() ([]string, error) { return []string{path}, nil }
	tt := ui.NewTester(a.View, 1320, 880)
	if err := tt.Click("Attach files"); err != nil {
		t.Fatal(err)
	}
	if a.attachmentOpen || len(a.attachments) != 1 || a.attachments[0].Name != "design.md" {
		t.Fatalf("file adapter failed: %#v / %s", a.attachments, a.errorText)
	}
	if err := tt.Click("Remove design.md"); err != nil {
		t.Fatal(err)
	}
	if len(a.attachments) != 0 {
		t.Fatal("removed attachment remained in draft")
	}
}

func TestNativeFileDropTargetsTheComposer(t *testing.T) {
	a := testApp(t, "default")
	path := filepath.Join(t.TempDir(), "drop.txt")
	if err := os.WriteFile(path, []byte("Dropped content"), 0600); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(a.View, 1320, 880)
	message, ok := tt.Find("Message")
	if !ok {
		t.Fatal("composer not found")
	}
	if !tt.DropFiles(message.X+10, message.Y+10, path) {
		t.Fatal("composer rejected native file drop")
	}
	if len(a.attachments) != 1 || a.attachments[0].Name != "drop.txt" {
		t.Fatalf("drop did not reach importer: %#v / %s", a.attachments, a.errorText)
	}
	if tt.DropFiles(8, 8, path) {
		t.Fatal("titlebar accepted composer file drop")
	}
}
