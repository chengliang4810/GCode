package desktop

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gcode/internal/assets"
	"gcode/internal/contracts"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

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
