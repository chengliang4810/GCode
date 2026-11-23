package remote

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gcode/internal/contracts"
)

type remoteDirectoryFailureFixture struct {
	contracts.Service
	err error
}

func (s *remoteDirectoryFailureFixture) ListDirectories(context.Context, contracts.DirectoryRequest) (contracts.DirectoryListing, error) {
	return contracts.DirectoryListing{}, s.err
}

func (s *remoteDirectoryFailureFixture) ListDirectoryLocations(context.Context, contracts.DirectoryRequest) (contracts.DirectoryLocations, error) {
	return contracts.DirectoryLocations{}, s.err
}

func TestRemoteDirectoryHostErrorRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name   string
		cause  error
		status int
	}{
		{"deadline", context.DeadlineExceeded, http.StatusGatewayTimeout},
		{"busy", contracts.ErrDirectoryBrowseBusy, http.StatusServiceUnavailable},
		{"invalid", errors.New("device not found"), http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			core := &remoteDirectoryFailureFixture{err: fmt.Errorf("host directory query: %w", tc.cause)}
			client := remoteDirectoryTestClient(t, core)
			for _, query := range []func(context.Context, contracts.DirectoryRequest) error{
				func(ctx context.Context, request contracts.DirectoryRequest) error {
					_, err := client.ListDirectories(ctx, request)
					return err
				},
				func(ctx context.Context, request contracts.DirectoryRequest) error {
					_, err := client.ListDirectoryLocations(ctx, request)
					return err
				},
			} {
				err := query(context.Background(), contracts.DirectoryRequest{DeviceID: "local"})
				if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("HTTP %d:", tc.status)) || !strings.Contains(err.Error(), "host directory query") {
					t.Fatalf("host error lost status or context: %v", err)
				}
				if errors.Is(err, context.DeadlineExceeded) != (tc.status == http.StatusGatewayTimeout) ||
					errors.Is(err, contracts.ErrDirectoryBrowseBusy) != (tc.status == http.StatusServiceUnavailable) {
					t.Fatalf("incorrect host error classification: %v", err)
				}
			}
		})
	}
}

func TestRemoteDirectoryErrorMappingDoesNotAffectOtherRoutes(t *testing.T) {
	for _, status := range []int{http.StatusGatewayTimeout, http.StatusServiceUnavailable} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "unrelated endpoint failure", status)
			}))
			defer server.Close()
			client, err := NewClient(server.URL, strings.Repeat("fixture-", 4))
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			_, err = client.Snapshot(context.Background())
			if err == nil || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, contracts.ErrDirectoryBrowseBusy) {
				t.Fatalf("unrelated HTTP error was classified as a directory failure: %v", err)
			}
		})
	}
}
