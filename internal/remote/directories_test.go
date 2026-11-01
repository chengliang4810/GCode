package remote

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/storage"
)

// The location list is a host query: a client must see the volumes of the
// machine that owns the files, not its own.
func TestRemoteDirectoryLocationsComeFromTheHost(t *testing.T) {
	store, err := storage.Open(storage.Options{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	core, err := engine.Open(engine.Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	host, err := (Server{Core: core, Token: strings.Repeat("host", 8)}).Handler()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(host)
	defer server.Close()
	client, err := NewClient(server.URL, strings.Repeat("host", 8))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	want, err := core.ListDirectoryLocations(context.Background(), contracts.DirectoryRequest{DeviceID: "local"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.ListDirectoryLocations(context.Background(), contracts.DirectoryRequest{DeviceID: "local"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Locations) != len(want.Locations) || got.Locations[0].Path != want.Locations[0].Path || got.Locations[0].Kind != contracts.LocationHome {
		t.Fatalf("client locations %+v, host locations %+v", got.Locations, want.Locations)
	}
	if _, err := client.ListDirectoryLocations(context.Background(), contracts.DirectoryRequest{DeviceID: "unknown"}); err == nil {
		t.Fatal("host accepted an unknown device")
	}
}
