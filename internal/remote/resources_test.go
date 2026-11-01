package remote

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gcode/internal/contracts"
)

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
