package remote

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gcode/internal/contracts"
)

func BenchmarkAttachmentBinaryUploadBoundary(b *testing.B) {
	core := &attachmentUploadCore{}
	token := strings.Repeat("upload-test", 3)
	handler, err := (Server{Core: core, Token: token}).Handler()
	if err != nil {
		b.Fatal(err)
	}
	payload := bytes.Repeat([]byte("x"), contracts.MaxAttachmentBytes)
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	for b.Loop() {
		request := httptest.NewRequest(http.MethodPost, "/v1/attachments?name=limit.bin", bytes.NewReader(payload))
		request.Header.Set("Content-Type", "application/octet-stream")
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK || core.size != len(payload) {
			b.Fatal("binary upload failed")
		}
	}
}
