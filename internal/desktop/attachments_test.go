package desktop

import (
	"os"
	"path/filepath"
	"testing"

	"gcode/internal/assets"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

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
