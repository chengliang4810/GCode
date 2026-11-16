package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		sessions = append(sessions, r.URL.Query().Get("sessionId"))
		if r.Header.Get("Content-Type") != "application/octet-stream" ||
			r.URL.Query().Get("name") == "" || len(data) == 0 {
			t.Error("remote attachment did not stream binary bytes and metadata")
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

type attachmentUploadCore struct {
	contracts.Service
	calls int
	size  int
}

func (c *attachmentUploadCore) ImportAttachments(_ context.Context, request contracts.AttachmentImportRequest) ([]contracts.Attachment, error) {
	c.calls++
	c.size = len(request.Clipboard.Data)
	return []contracts.Attachment{{ID: "uploaded", Name: request.Clipboard.Name}}, nil
}

type unreadUpload struct{ t *testing.T }

func (r unreadUpload) Read([]byte) (int, error) {
	r.t.Error("oversized upload body was read")
	return 0, io.EOF
}

type generatedUpload struct{}

func (generatedUpload) Read(data []byte) (int, error) {
	for i := range data {
		data[i] = 'x'
	}
	return len(data), nil
}

func TestAttachmentChunkedUploadRejectsActualOversize(t *testing.T) {
	core := &attachmentUploadCore{}
	token := strings.Repeat("upload-test", 3)
	handler, err := (Server{Core: core, Token: token}).Handler()
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/attachments?name=large.bin",
		io.LimitReader(generatedUpload{}, contracts.MaxAttachmentBytes+1))
	request.ContentLength = -1
	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge || core.calls != 0 {
		t.Fatalf("chunked oversized upload reached import: status=%d calls=%d", response.Code, core.calls)
	}
}

func TestAttachmentUploadEnforcesBinaryBoundaryBeforeImport(t *testing.T) {
	for _, tc := range []struct {
		name, contentType, body string
		length                  int64
		want                    int
	}{
		{"binary", "application/octet-stream", "binary\x00payload", 14, http.StatusOK},
		{"unknown length", "application/octet-stream", "chunked", -1, http.StatusOK},
		{"truncated", "application/octet-stream", "short", 20, http.StatusBadRequest},
		{"empty", "application/octet-stream", "", 0, http.StatusBadRequest},
		{"old JSON removed", "application/json", `{"clipboard":{"data":"eA=="}}`, -1, http.StatusUnsupportedMediaType},
		{"oversized declaration", "application/octet-stream", "", contracts.MaxAttachmentBytes + 1, http.StatusRequestEntityTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			core := &attachmentUploadCore{}
			token := strings.Repeat("upload-test", 3)
			handler, err := (Server{Core: core, Token: token}).Handler()
			if err != nil {
				t.Fatal(err)
			}
			var reader io.Reader = strings.NewReader(tc.body)
			if tc.length > contracts.MaxAttachmentBytes {
				reader = unreadUpload{t}
			}
			request := httptest.NewRequest(http.MethodPost, "/v1/attachments?"+url.Values{"name": {"测试报告.bin"}}.Encode(), reader)
			request.ContentLength = tc.length
			request.Header.Set("Content-Type", tc.contentType)
			request.Header.Set("Authorization", "Bearer "+token)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != tc.want || (tc.want == http.StatusOK) != (core.calls == 1) {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, core.calls, response.Body.String())
			}
		})
	}
}

func TestAttachmentBatchRejectsOversizeBeforeAnyUpload(t *testing.T) {
	var calls int
	host := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer host.Close()
	client, err := NewClient(host.URL, strings.Repeat("upload-test", 3))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	var paths []string
	for i := 0; i < 5; i++ {
		path := filepath.Join(t.TempDir(), "large.bin")
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Truncate(contracts.MaxAttachmentBytes); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
		paths = append(paths, path)
	}
	_, err = client.ImportAttachments(t.Context(), contracts.AttachmentImportRequest{Paths: paths})
	if err == nil || calls != 0 {
		t.Fatalf("oversized batch uploaded data: %v calls=%d", err, calls)
	}
}
