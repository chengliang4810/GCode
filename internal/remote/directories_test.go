package remote

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

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
	root := t.TempDir()
	for i := contracts.DirectoryResultLimit; i >= 0; i-- {
		if err := os.Mkdir(filepath.Join(root, fmt.Sprintf("folder-%04d", i)), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	listing, err := client.ListDirectories(context.Background(), contracts.DirectoryRequest{DeviceID: "local", Path: root})
	if err != nil || len(listing.Entries) != contracts.DirectoryResultLimit || !listing.Truncated {
		t.Fatalf("remote host result cap: entries=%d, truncated=%v, err=%v", len(listing.Entries), listing.Truncated, err)
	}
	for i := 1; i < len(listing.Entries); i++ {
		if listing.Entries[i-1].Path >= listing.Entries[i].Path {
			t.Fatal("remote directory paths are not sorted")
		}
	}
	if _, err := client.ListDirectories(context.Background(), contracts.DirectoryRequest{DeviceID: "unknown", Path: root}); err == nil {
		t.Fatal("host browsed a directory for an unknown device")
	}
}

// This fixture proves protocol field and cancellation transport only; the test
// above exercises the actual host filesystem and its bounded directory scan.
type remoteDirectoryFixture struct {
	contracts.Service
	listing   contracts.DirectoryListing
	locations contracts.DirectoryLocations
	entered   chan struct{}
}

func (s *remoteDirectoryFixture) wait(ctx context.Context) error {
	if s.entered == nil {
		return nil
	}
	close(s.entered)
	<-ctx.Done()
	return ctx.Err()
}

func (s *remoteDirectoryFixture) ListDirectories(ctx context.Context, _ contracts.DirectoryRequest) (contracts.DirectoryListing, error) {
	return s.listing, s.wait(ctx)
}

func (s *remoteDirectoryFixture) ListDirectoryLocations(ctx context.Context, _ contracts.DirectoryRequest) (contracts.DirectoryLocations, error) {
	return s.locations, s.wait(ctx)
}

func remoteDirectoryTestClient(t *testing.T, core contracts.Service) *Client {
	t.Helper()
	token := strings.Repeat("directory-test-", 3)
	handler, err := (Server{Core: core, Token: token}).Handler()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := NewClient(server.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestRemoteDirectoryTruncatedStateRoundTrip(t *testing.T) {
	core := &remoteDirectoryFixture{
		listing: contracts.DirectoryListing{DeviceID: "local", Path: "/host/work", Home: "/host", Truncated: true},
		locations: contracts.DirectoryLocations{DeviceID: "local", Truncated: true, Locations: []contracts.DirectoryLocation{
			{Name: "Home", Path: "/host", Kind: contracts.LocationHome},
		}},
	}
	client := remoteDirectoryTestClient(t, core)
	got, err := client.ListDirectories(context.Background(), contracts.DirectoryRequest{DeviceID: "local"})
	if err != nil || !reflect.DeepEqual(got, core.listing) {
		t.Fatalf("directory state round trip: %+v, %v", got, err)
	}
	locations, err := client.ListDirectoryLocations(context.Background(), contracts.DirectoryRequest{DeviceID: "local"})
	if err != nil || !reflect.DeepEqual(locations, core.locations) {
		t.Fatalf("location state round trip: %+v, %v", locations, err)
	}
}

func TestRemoteDirectoryCallerDeadline(t *testing.T) {
	for _, locations := range []bool{false, true} {
		t.Run(fmt.Sprint(locations), func(t *testing.T) {
			core := &remoteDirectoryFixture{entered: make(chan struct{})}
			client := remoteDirectoryTestClient(t, core)
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
			defer cancel()
			result := make(chan error, 1)
			go func() {
				var err error
				if locations {
					_, err = client.ListDirectoryLocations(ctx, contracts.DirectoryRequest{DeviceID: "local"})
				} else {
					_, err = client.ListDirectories(ctx, contracts.DirectoryRequest{DeviceID: "local"})
				}
				result <- err
			}()
			select {
			case <-core.entered:
			case <-time.After(2 * time.Second):
				t.Fatal("remote host query was not reached")
			}
			select {
			case err := <-result:
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("remote caller deadline: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("remote directory caller did not honor its deadline")
			}
		})
	}
}
