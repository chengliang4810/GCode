package engine

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/storage"
)

func TestAttachmentsAndLinkedImagesKeepHistoricalRootAfterSwitch(t *testing.T) {
	options := storage.Options{Home: t.TempDir()}
	store, err := storage.Open(options)
	if err != nil {
		t.Fatal(err)
	}
	core, err := openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = core.Close() })
	if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create"}); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := core.Snapshot(t.Context())
	session := snapshot.Sessions[0]
	oldRoot, oldID := store.Directory(), store.ActiveRootID()
	importText := func(sessionID, text string) contracts.Attachment {
		t.Helper()
		items, err := core.ImportAttachments(t.Context(), contracts.AttachmentImportRequest{
			SessionID: sessionID,
			Clipboard: &contracts.ClipboardAttachment{Name: "notes.txt", Data: []byte(text)},
		})
		if err != nil {
			t.Fatal(err)
		}
		return items[0]
	}
	original := importText(session.ID, "original root content")
	target := filepath.Join(options.Home, "next-data-root")
	if err := store.RequestDirectory(target); err != nil {
		t.Fatal(err)
	}
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = storage.Open(options)
	if err != nil {
		t.Fatal(err)
	}
	core, err = openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	continued := importText(session.ID, "continued historical session")
	draft := importText("", "new draft")
	if continued.StorageRootID != oldID || draft.StorageRootID != store.ActiveRootID() || oldID == draft.StorageRootID {
		t.Fatalf("incorrect root binding: continued %+v, draft %+v", continued, draft)
	}
	if _, err := os.Stat(filepath.Join(oldRoot, "uploads", continued.ID)); err != nil {
		t.Fatal("historical attachment was not written to its bound root:", err)
	}
	if _, err := os.Stat(filepath.Join(target, "uploads", continued.ID)); !os.IsNotExist(err) {
		t.Fatalf("historical attachment was copied into the new root: %v", err)
	}
	// Matching names in another registered root never redirect a resource.
	if err := os.WriteFile(filepath.Join(target, "uploads", original.ID), []byte("wrong root"), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := core.ReadResource(t.Context(), original.AssetRef)
	if err != nil || string(data) != "original root content" {
		t.Fatalf("resource followed the current root: %q, %v", data, err)
	}
	imagePath := filepath.Join(t.TempDir(), "linked.svg")
	if err := os.WriteFile(imagePath, []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), 0600); err != nil {
		t.Fatal(err)
	}
	linked, err := core.ReadLinkedFile(t.Context(), contracts.LinkedFileRequest{SessionID: session.ID, Path: imagePath})
	if err != nil || !strings.HasPrefix(linked.File.AssetRef, resourcePrefix+oldID+"/") {
		t.Fatalf("historical linked image cache changed roots: %+v, %v", linked, err)
	}
	// Unavailability must not cause fallback writes, and retrying after the
	// registered root returns must make the existing session usable again.
	marker := filepath.Join(oldRoot, ".gcode-root.json")
	if err := os.Rename(marker, marker+".offline"); err != nil {
		t.Fatal(err)
	}
	if _, err := core.ReadResource(t.Context(), original.AssetRef); err == nil {
		t.Error("read an unavailable registered root")
	}
	_, importErr := core.ImportAttachments(t.Context(), contracts.AttachmentImportRequest{
		SessionID: session.ID,
		Clipboard: &contracts.ClipboardAttachment{Name: "offline.txt", Data: []byte("blocked")},
	})
	if err := os.Rename(marker+".offline", marker); err != nil {
		t.Fatal(err)
	}
	if importErr == nil {
		t.Fatal("import fell back from an unavailable session root")
	}
	restored := importText(session.ID, "restored root")
	if restored.StorageRootID != oldID {
		t.Fatal("restored session did not retain its root")
	}
}

func TestImportAttachmentsBindsSourceSessionAndRejectsUnavailableSession(t *testing.T) {
	core, _, session := linkedFileTestCore(t)
	payload := []byte("source session attachment")
	items, err := core.ImportAttachments(t.Context(), contracts.AttachmentImportRequest{
		SessionID: session.ID,
		Clipboard: &contracts.ClipboardAttachment{Name: "notes.txt", Data: payload},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].StorageRootID != session.StorageRootID ||
		items[0].AssetRef != managedResourceRef(session.StorageRootID, items[0].ID) {
		t.Fatalf("attachment lost its source storage: %+v", items)
	}
	read, err := core.ReadResource(t.Context(), items[0].AssetRef)
	if err != nil || !bytes.Equal(read, payload) {
		t.Fatalf("read imported resource = %q, %v", read, err)
	}
	for _, unavailable := range []string{"missing", session.ID} {
		if unavailable == session.ID {
			core.mu.Lock()
			core.state.Sessions[core.sessionIndex(session.ID)].StorageRootID = "missing-root"
			core.state.Sessions[core.sessionIndex(session.ID)].StorageError = "session storage is unavailable"
			core.mu.Unlock()
		}
		if _, err := core.ImportAttachments(t.Context(), contracts.AttachmentImportRequest{
			SessionID: unavailable,
			Clipboard: &contracts.ClipboardAttachment{Name: "blocked.txt", Data: payload},
		}); err == nil {
			t.Fatalf("accepted unavailable session %q", unavailable)
		}
	}
}

func TestReadResourceRejectsInvalidRootAndPathReferences(t *testing.T) {
	core, _, _ := linkedFileTestCore(t)
	rootID := core.storage.ActiveRootID()
	for _, ref := range []string{
		"resource://managed/attachment-old-format",
		"resource://managed/missing-root/secret",
		managedResourceRef(rootID, "../secret"),
		managedResourceRef(rootID, `..\secret`),
		managedResourceRef(rootID, "file:stream"),
		managedResourceRef(rootID, "file\x00name"),
		managedResourceRef(rootID, "."),
		managedResourceRef("..", "secret"),
		managedResourceRef("", "secret"),
	} {
		t.Run(strings.ReplaceAll(ref, "/", "_"), func(t *testing.T) {
			if _, err := core.ReadResource(t.Context(), ref); err == nil {
				t.Fatalf("accepted invalid resource reference %q", ref)
			}
		})
	}
}

func TestReadResourceRejectsSymlinkOutsideDataRoot(t *testing.T) {
	core, _, _ := linkedFileTestCore(t)
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("must not be disclosed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(core.storage.Directory(), "uploads", "escape")); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if _, err := core.ReadResource(t.Context(), managedResourceRef(core.storage.ActiveRootID(), "escape")); err == nil {
		t.Fatal("managed resource escaped its data root through a symlink")
	}
}

func TestImportAttachmentsRejectsUploadsSymlinkOutsideDataRoot(t *testing.T) {
	core, _, session := linkedFileTestCore(t)
	outside := t.TempDir()
	uploads := filepath.Join(core.storage.Directory(), "uploads")
	if err := os.Remove(uploads); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, uploads); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if _, err := core.ImportAttachments(t.Context(), contracts.AttachmentImportRequest{
		SessionID: session.ID,
		Clipboard: &contracts.ClipboardAttachment{Name: "escape.txt", Data: []byte("must stay inside")},
	}); err == nil {
		t.Fatal("managed upload escaped its data root through a symlink")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("upload wrote outside its data root: %v, %v", entries, err)
	}
}
