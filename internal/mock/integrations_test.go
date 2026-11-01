package mock

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gcode/internal/contracts"
)

func integrationRequest(kind, provider string) contracts.Command {
	return contracts.Command{Kind: kind, Integration: &contracts.IntegrationRequest{TargetID: "local", ProviderID: provider}}
}
func executeIntegration(t *testing.T, s *Server, c contracts.Command) {
	t.Helper()
	if err := s.Execute(context.Background(), c); err != nil {
		t.Fatal(c.Kind, err)
	}
}

func TestIntegrationsProviderPreferencesAndLoginPreserveCurrentAccount(t *testing.T) {
	s := newTestServer(t, Options{Instant: true})
	ctx := context.Background()
	cmd := integrationRequest("provider.preference", "claude")
	cmd.Name = "dollar"
	cmd.Enabled = false
	executeIntegration(t, s, cmd)
	executeIntegration(t, s, integrationRequest("provider.login.start", "claude"))
	v, _ := s.Snapshot(ctx)
	id := v.Integrations.Login.ID
	if v.Integrations.Login.Kind != "code" {
		t.Fatal("Claude paste-code state missing")
	}
	cmd = integrationRequest("provider.login.submit", "claude")
	cmd.ID = id
	cmd.Text = "invalid"
	executeIntegration(t, s, cmd)
	v, _ = s.Snapshot(ctx)
	if v.Integrations.Login.State != "failed" {
		t.Fatal("failed code has no retry state")
	}
	executeIntegration(t, s, integrationRequest("provider.login.retry", "claude"))
	v, _ = s.Snapshot(ctx)
	cmd.ID = v.Integrations.Login.ID
	cmd.Text = "authorization-code"
	executeIntegration(t, s, cmd)
	v, _ = s.Snapshot(ctx)
	current, count, added := "", 0, ""
	for _, a := range v.Integrations.Accounts {
		if a.TargetID == "local" && a.ProviderID == "claude" {
			count++
			if a.InUse {
				current = a.ID
			} else {
				added = a.ID
			}
		}
	}
	if count != 2 || current != "account-local-claude" || added == "" {
		t.Fatal("adding account changed the current login")
	}
	cmd = integrationRequest("provider.account.switch", "claude")
	cmd.Integration.AccountID = added
	executeIntegration(t, s, cmd)
	v, _ = s.Snapshot(ctx)
	for _, a := range v.Integrations.Accounts {
		if a.TargetID == "local" && a.ProviderID == "claude" && a.InUse && a.ID != added {
			t.Fatal("switch did not settle active account")
		}
	}
	cmd.Kind = "provider.account.remove"
	executeIntegration(t, s, cmd)
	v, _ = s.Snapshot(ctx)
	for _, d := range v.Integrations.Providers {
		if d.TargetID == "local" && d.ProviderID == "claude" && d.DollarSkills {
			t.Fatal("account updates lost per-agent preference")
		}
	}
}

func TestIntegrationsTargetScopeLastAgentAndOffline(t *testing.T) {
	s := newTestServer(t, Options{Instant: true})
	cmd := integrationRequest("provider.enable", "claude")
	cmd.Enabled = false
	executeIntegration(t, s, cmd)
	cmd.Integration.ProviderID = "openai"
	if err := s.Execute(context.Background(), cmd); err == nil {
		t.Fatal("last installed agent could be disabled")
	}
	cmd = integrationRequest("provider.preference", "claude")
	cmd.Name = "slash"
	cmd.Integration.TargetID = "remote"
	if err := s.Execute(context.Background(), cmd); err == nil {
		t.Fatal("offline device accepted a preference write")
	}
	cmd.Integration.TargetID = "macbook"
	executeIntegration(t, s, cmd)
	v, _ := s.Snapshot(context.Background())
	for _, d := range v.Integrations.Providers {
		if d.ProviderID == "claude" && d.TargetID == "local" && !d.SeparateSlash {
			t.Fatal("remote preference affected local device")
		}
	}
	cmd = integrationRequest("provider.install", "cursor")
	executeIntegration(t, s, cmd)
	v, _ = s.Snapshot(context.Background())
	for _, d := range v.Integrations.Providers {
		if d.ProviderID == "cursor" && d.TargetID == "local" && (!d.Installed || !d.Enabled) {
			t.Fatal("install did not update descriptor")
		}
	}
}

func TestIntegrationsCancelledFlowCannotCompleteAfterReplacement(t *testing.T) {
	s := newTestServer(t, Options{TokenInterval: time.Millisecond})
	ctx := context.Background()
	executeIntegration(t, s, integrationRequest("provider.login.start", "openai"))
	v, _ := s.Snapshot(ctx)
	old := v.Integrations.Login.ID
	executeIntegration(t, s, integrationRequest("provider.login.start", "claude"))
	v, _ = s.Snapshot(ctx)
	newID := v.Integrations.Login.ID
	cmd := integrationRequest("provider.login.cancel", "openai")
	cmd.ID = old
	if err := s.Execute(ctx, cmd); !errors.Is(err, ErrNotFound) {
		t.Fatal("stale cancellation touched a replacement login")
	}
	time.Sleep(650 * time.Millisecond)
	v, _ = s.Snapshot(ctx)
	if v.Integrations.Login == nil || v.Integrations.Login.ID != newID || v.Integrations.Login.State != "waiting" {
		t.Fatal("old browser completion changed current code flow")
	}
}

func TestIntegrationsCloudGithubRepositoryAndUsage(t *testing.T) {
	s := newTestServer(t, Options{Instant: true})
	ctx := context.Background()
	executeIntegration(t, s, contracts.Command{Kind: "cloud.github.start"})
	v, _ := s.Snapshot(ctx)
	if v.Integrations.Cloud.Github.State != "connected" || len(v.Integrations.Cloud.Repositories) != 1 {
		t.Fatal("GitHub connection did not produce accessible repositories")
	}
	executeIntegration(t, s, contracts.Command{Kind: "cloud.repositories.choose"})
	v, _ = s.Snapshot(ctx)
	if len(v.Integrations.Cloud.Repositories) != 2 {
		t.Fatal("repository choice did not update snapshot")
	}
	for _, rangeID := range []string{"7d", "30d", "thisMonth", "lastMonth", "90d"} {
		t.Run(rangeID, func(t *testing.T) {
			executeIntegration(t, s, contracts.Command{Kind: "cloud.usage", Integration: &contracts.IntegrationRequest{Value: rangeID}})
			v, _ := s.Snapshot(ctx)
			if v.Integrations.Cloud.UsageRange != rangeID || len(v.Integrations.Cloud.Usage) == 0 {
				t.Fatal("range did not update data")
			}
		})
	}
	executeIntegration(t, s, contracts.Command{Kind: "cloud.github.disconnect"})
	v, _ = s.Snapshot(ctx)
	if len(v.Integrations.Cloud.Repositories) != 0 || v.Integrations.Cloud.Github.State != "disconnected" {
		t.Fatal("disconnect left authorized repositories")
	}
}

func TestIntegrationsBillingExactTotalsAndCheckoutIdempotence(t *testing.T) {
	s := newTestServer(t, Options{TokenInterval: time.Hour})
	ctx := context.Background()
	before, _ := s.Snapshot(ctx)
	executeIntegration(t, s, contracts.Command{Kind: "billing.buy", Integration: &contracts.IntegrationRequest{Packs: 2}})
	v, _ := s.Snapshot(ctx)
	id := v.Integrations.Billing.Checkout.ID
	for range 2 {
		executeIntegration(t, s, contracts.Command{Kind: "billing.checkout.poll", ID: id})
	}
	v, _ = s.Snapshot(ctx)
	b := v.Integrations.Billing
	if b.Credits.Extra != before.Integrations.Billing.Credits.Extra+24000 || b.Credits.Total != b.Credits.Monthly+b.Credits.Extra {
		t.Fatal("checkout charged twice or produced contradictory totals")
	}
	executeIntegration(t, s, contracts.Command{Kind: "billing.subscribe", Integration: &contracts.IntegrationRequest{Value: "base"}})
	v, _ = s.Snapshot(ctx)
	executeIntegration(t, s, contracts.Command{Kind: "billing.checkout.poll", ID: v.Integrations.Billing.Checkout.ID})
	v, _ = s.Snapshot(ctx)
	if v.Integrations.Billing.Subscription.PlanID != "base" || v.Integrations.Billing.Credits.MonthlyTotal != 45000 {
		t.Fatal("plan change did not update monthly allowance")
	}
}

func TestIntegrationsPersistenceCopiesAndNoCredentialStorage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := New(Options{StatePath: path, Instant: true})
	if err != nil {
		t.Fatal(err)
	}
	executeIntegration(t, s, contracts.Command{Kind: "device.rename", ID: "local", Name: "Studio"})
	cmd := integrationRequest("provider.apiKey.save", "openai")
	cmd.Integration.TargetID = "cloud-1"
	cmd.Text = "secret-test-key-not-real"
	executeIntegration(t, s, cmd)
	v, _ := s.Snapshot(context.Background())
	v.Integrations.Billing.Plans[0].Name = "mutated"
	v.Integrations.Accounts[0].Usage[0].Label = "mutated"
	v.Integrations.Providers[0].LoginOptions[0].Kind = "mutated"
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(bytes), cmd.Text) {
		t.Fatal("credential leaked into persistence")
	}
	reopened, err := New(Options{StatePath: path, Instant: true})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	actual, _ := reopened.Snapshot(context.Background())
	if actual.Devices[0].Name != "Studio" || actual.Integrations.Billing.Plans[0].Name == "mutated" || actual.Integrations.Accounts[0].Usage[0].Label == "mutated" || actual.Integrations.Providers[0].LoginOptions[0].Kind == "mutated" {
		t.Fatal("snapshot ownership or persistence failed")
	}
	data, _ := json.Marshal(actual)
	if strings.Contains(string(data), cmd.Text) {
		t.Fatal("credential leaked into snapshot")
	}
}

func TestIntegrationCommandValidationOwnsPayload(t *testing.T) {
	cmd := integrationRequest("provider.preference", "claude")
	cmd.Name = "dollar"
	copy := cmd.Clone()
	cmd.Integration.TargetID = "changed"
	if copy.Integration.TargetID != "local" {
		t.Fatal("integration payload aliases command")
	}
	for _, n := range []int{0, 21} {
		if err := (contracts.Command{Kind: "billing.buy", Integration: &contracts.IntegrationRequest{Packs: n}}).Validate(); err == nil {
			t.Fatal("invalid pack count accepted")
		}
	}
}
