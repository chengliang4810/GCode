package remote

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/storage"
)

func TestBinaryFileUploadStoresBytesOnHost(t *testing.T) {
	store, err := storage.Open(storage.Options{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	core, err := engine.Open(engine.Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	token := strings.Repeat("fixture-token", 3)
	handler, err := (Server{Core: core, Token: token}).Handler()
	if err != nil {
		t.Fatal(err)
	}
	host := httptest.NewServer(handler)
	defer host.Close()
	client, err := NewClient(host.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	source := filepath.Join(t.TempDir(), "client report.pdf")
	payload := []byte("%PDF-1.7\n\x00\xffbinary data")
	if err := os.WriteFile(source, payload, 0600); err != nil {
		t.Fatal(err)
	}
	items, err := client.ImportAttachments(t.Context(), contracts.AttachmentImportRequest{Paths: []string{source}})
	if err != nil || len(items) != 1 {
		t.Fatalf("binary file upload failed: %+v, %v", items, err)
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	data, err := client.ReadResource(t.Context(), items[0].AssetRef)
	if err != nil || !bytes.Equal(data, payload) {
		t.Fatalf("host resource lost uploaded bytes: %q, %v", data, err)
	}
	path := filepath.Join(store.Directory(), "uploads", items[0].ID)
	if data, err := os.ReadFile(path); err != nil || !bytes.Equal(data, payload) || filepath.Ext(path) != ".pdf" {
		t.Fatalf("host file is unavailable to agent tools: %q, %v", data, err)
	}
}

func TestAttachmentBatchKeepsSourceSessionForEveryUpload(t *testing.T) {
	var sessions []string
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/attachments" {
			t.Errorf("unexpected upload path %q", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		var request contracts.AttachmentImportRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			http.Error(w, "invalid request", 400)
			return
		}
		sessions = append(sessions, request.SessionID)
		if request.Clipboard == nil || len(request.Paths) != 0 {
			t.Error("remote attachment did not contain encoded bytes")
		}
		_ = json.NewEncoder(w).Encode([]contracts.Attachment{{ID: "uploaded"}})
	}))
	defer host.Close()
	client, err := NewClient(host.URL, strings.Repeat("fixture-token", 3))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	path := filepath.Join(t.TempDir(), "local.txt")
	if err := os.WriteFile(path, []byte("local file"), 0600); err != nil {
		t.Fatal(err)
	}
	items, err := client.ImportAttachments(t.Context(), contracts.AttachmentImportRequest{
		SessionID: "old-session", Paths: []string{path},
		Clipboard: &contracts.ClipboardAttachment{Name: "clipboard.txt", Data: []byte("clipboard")},
	})
	if err != nil || len(items) != 2 || len(sessions) != 2 {
		t.Fatalf("uploaded batch = %+v, sessions %v, error %v", items, sessions, err)
	}
	for _, session := range sessions {
		if session != "old-session" {
			t.Fatalf("upload lost source session: %q", session)
		}
	}
}
