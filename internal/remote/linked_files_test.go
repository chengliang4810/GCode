package remote

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/storage"
)

func TestRemoteLinkedFilesReadOwningHostAndAuthenticate(t *testing.T) {
	store, err := storage.Open(storage.Options{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	core, err := engine.Open(engine.Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	workspace := t.TempDir()
	path := filepath.Join(workspace, "远程宿主 文件.md")
	if err := os.WriteFile(path, []byte("# host-only contents"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := core.Execute(t.Context(), contracts.Command{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: workspace}}); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := core.Snapshot(t.Context())
	if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create", ProjectID: snapshot.Projects[0].ID}); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = core.Snapshot(t.Context())
	token := strings.Repeat("linked-file-token", 2)
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
	events, err := client.Subscribe(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-events:
		if event.Snapshot == nil {
			t.Fatal("remote stream did not begin with its authoritative snapshot")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("remote initial snapshot timed out")
	}
	result, err := client.ReadLinkedFile(t.Context(), contracts.LinkedFileRequest{SessionID: snapshot.Sessions[0].ID, Path: filepath.Base(path)})
	canonicalPath, canonicalErr := filepath.EvalSymlinks(path)
	if canonicalErr != nil {
		t.Fatal(canonicalErr)
	}
	if err != nil || result.File.Content != "# host-only contents" || result.File.ReadOnly || result.AbsolutePath != filepath.ToSlash(canonicalPath) {
		t.Fatalf("host linked file = %+v, %v", result, err)
	}
	deadline := time.After(3 * time.Second)
	for {
		select {
		case event := <-events:
			if event.Kind != "file.read" {
				continue
			}
			if event.Snapshot == nil || len(event.Snapshot.Files) != 1 || event.Snapshot.Files[0] != result.File {
				t.Fatalf("remote file read did not publish the returned loaded buffer: %+v", event)
			}
		case <-deadline:
			t.Fatal("remote linked file projection timed out")
		}
		break
	}
	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	imagePath := filepath.Join(workspace, "host.png")
	if err := os.WriteFile(imagePath, imageBytes.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	result, err = client.ReadLinkedFile(t.Context(), contracts.LinkedFileRequest{SessionID: snapshot.Sessions[0].ID, Path: imagePath})
	if err != nil || result.File.Kind != "image" {
		t.Fatalf("host image = %+v, %v", result, err)
	}
	data, err := client.ReadResource(t.Context(), result.File.AssetRef)
	if err != nil || !bytes.Equal(data, imageBytes.Bytes()) {
		t.Fatalf("remote managed image = %d bytes, %v", len(data), err)
	}
	response, err := http.Post(host.URL+"/v1/linked-files/read", "application/json", strings.NewReader(`{"path":"`+filepath.ToSlash(path)+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated host read returned %d", response.StatusCode)
	}
	for _, body := range []string{`{"path":"/tmp/x","unknown":true}`, `{} {}`, `{"path":`} {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, host.URL+"/v1/linked-files/read", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+token)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("invalid body %q returned %d", body, response.StatusCode)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := client.ReadLinkedFile(ctx, contracts.LinkedFileRequest{Path: path}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled remote read = %v", err)
	}
}
