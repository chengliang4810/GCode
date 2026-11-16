package remote

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gcode/internal/engine"
	"gcode/internal/storage"
)

func TestGlobalInstructionsRemoteHostAndAuthentication(t *testing.T) {
	store, err := storage.Open(storage.Options{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	core, err := engine.Open(engine.Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	token := strings.Repeat("x", 24)
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
	if err := client.WriteGlobalInstructions(t.Context(), "远程宿主指令"); err != nil {
		t.Fatal(err)
	}
	text, err := client.ReadGlobalInstructions(t.Context())
	if err != nil || text != "远程宿主指令" {
		t.Fatalf("remote instructions: %q %v", text, err)
	}
	data, err := os.ReadFile(filepath.Join(store.ConfigDirectory(), "AGENTS.md"))
	if err != nil || string(data) != text {
		t.Fatalf("host file mismatch: %q %v", data, err)
	}
	response, err := http.Get(host.URL + "/v1/instructions")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatal("unauthenticated instructions exposed")
	}
}
