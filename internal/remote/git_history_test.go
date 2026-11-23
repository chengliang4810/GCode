package remote

import (
	"context"
	"gcode/internal/contracts"
	"gcode/internal/mock"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type historyHostService struct {
	contracts.Service
	received contracts.GitHistoryRequest
}

func (s *historyHostService) ListGitHistory(_ context.Context, r contracts.GitHistoryRequest) (contracts.GitHistoryResult, error) {
	s.received = r
	return contracts.GitHistoryResult{Repository: true, Root: "/remote/checkout", Commits: []contracts.Commit{{SHA: "abc", Subject: "remote commit"}}}, nil
}

func TestRemoteGitHistoryUsesHostAndAuthentication(t *testing.T) {
	base, err := mock.New(mock.Options{Instant: true})
	if err != nil {
		t.Fatal(err)
	}
	defer base.Close()
	host := &historyHostService{Service: base}
	token := strings.Repeat("h", 32)
	handler, err := (Server{Core: host, Token: token}).Handler()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	client, err := NewClient(server.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	result, err := client.ListGitHistory(t.Context(), contracts.GitHistoryRequest{SessionID: "host-session"})
	if err != nil || host.received.SessionID != "host-session" || result.Root != "/remote/checkout" || len(result.Commits) != 1 || result.Commits[0].Subject != "remote commit" {
		t.Fatalf("remote history: %+v %v", result, err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/git-history", strings.NewReader(`{"sessionId":"host-session"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous request accepted: %d", response.Code)
	}
}
