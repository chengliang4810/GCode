package remote

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gcode/internal/engine"
	"gcode/internal/modelcatalog"
	"gcode/internal/storage"
)

func TestRemoteModelMetadataReadsHostCacheAndRequiresAuthentication(t *testing.T) {
	store, err := storage.Open(storage.Options{Directory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	core, err := engine.Open(engine.Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	token := strings.Repeat("metadata", 4)
	handler, err := (Server{Core: core, Token: token}).Handler()
	if err != nil {
		t.Fatal(err)
	}
	host := httptest.NewServer(handler)
	defer host.Close()
	response, err := http.Get(host.URL + "/v1/model-metadata?id=deepseek-v4-pro")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatal("metadata endpoint bypassed authentication")
	}
	client, err := NewClient(host.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	result, err := client.LookupModelMetadata(context.Background(), modelcatalog.Query{ID: "deepseek-v4-pro", BaseURL: "https://api.deepseek.com/v1"})
	if err != nil || !result.Found || result.ContextWindow != 1048576 {
		t.Fatalf("remote host cache lookup=%+v error=%v", result, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.LookupModelMetadata(ctx, modelcatalog.Query{ID: "anything"}); err == nil {
		t.Fatal("canceled query succeeded")
	}
}
