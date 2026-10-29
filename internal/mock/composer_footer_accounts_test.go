package mock

import (
	"context"
	"errors"
	"testing"

	"gcode/internal/contracts"
)

func TestComposerAccountRefreshListsWholeTargetWithoutBorrowingAnotherDevice(t *testing.T) {
	s := newTestServer(t, Options{Instant: true})
	s.mu.Lock()
	s.state.Integrations.Accounts = []contracts.ProviderAccount{
		{ID: "claude", TargetID: "local", ProviderID: "claude", ReportsUsage: true, UsageState: contracts.LoadError, UsageError: "old"},
		{ID: "openai", TargetID: "local", ProviderID: "openai", ReportsUsage: true, UsageState: contracts.LoadError, UsageError: "old"},
		{ID: "remote", TargetID: "remote-only", ProviderID: "claude", ReportsUsage: true, UsageState: contracts.LoadError, UsageError: "remote retained"},
	}
	s.mu.Unlock()
	command := contracts.Command{Kind: "provider.accounts.refresh", Integration: &contracts.IntegrationRequest{TargetID: "local", Value: "force"}}
	if err := s.Execute(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, account := range snapshot.Integrations.Accounts {
		if account.TargetID == "local" {
			if account.UsageState != contracts.LoadReady || account.UsageError != "" || len(account.Usage) == 0 {
				t.Fatalf("target refresh skipped provider: %+v", account)
			}
		} else if account.UsageError != "remote retained" || len(account.Usage) != 0 {
			t.Fatal("local refresh changed another target")
		}
	}
	command.Integration.TargetID = "unknown-target"
	if err := s.Execute(context.Background(), command); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown target fell back to local: %v", err)
	}
}
