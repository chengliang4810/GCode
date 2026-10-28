package desktop

import (
	"context"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func integrationTestView(t *testing.T, section string) (*App, *ui.Tester) {
	t.Helper()
	a, _ := settingsTestApp(t)
	a.OpenRoute("settings/" + section)
	return a, ui.NewTester(a.View, 1320, 1000)
}
func integrationClick(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	for range 12 {
		if r, ok := tt.Find(label); ok && r.H > 0 && r.Y >= 0 && r.Y+r.H < 995 {
			if err := tt.Click(label); err != nil {
				t.Fatal(err)
			}
			return
		}
		tt.Scroll(1150, 700, 0, 160)
	}
	t.Fatalf("unable to reveal %q", label)
}

func TestProviderPreferencesAndAccountAuthorizationReachBackend(t *testing.T) {
	a, tt := integrationTestView(t, "Providers")
	if len(a.state.Providers) != 10 {
		t.Fatal("source provider catalog is incomplete")
	}
	integrationClick(t, tt, "Claude Code preferences")
	for _, label := range []string{"Completion", "Use $ for skills", "Separate / commands", "Delegation", "Native subagents", "Updates", "Update policy", "Accounts", "In use"} {
		if !tt.HasText(label) {
			t.Fatalf("missing provider detail %s", label)
		}
	}
	integrationClick(t, tt, "Claude Code: Use $ for skills")
	if d, _ := a.providerDetails("claude"); d.DollarSkills {
		t.Fatal("completion switch did not mutate backend")
	}
	integrationClick(t, tt, "Add account")
	if !tt.HasText("Sign in to Claude Code") || !tt.HasText("Authorization code") {
		t.Fatal("code flow dialog missing")
	}
	if err := tt.Click("Authorization code"); err != nil {
		t.Fatal(err)
	}
	tt.Type("authorization-code")
	tt.Key(0, ui.KeyEnter)
	if a.state.Integrations.Login == nil || a.state.Integrations.Login.State != "completed" {
		t.Fatal("code entry did not finish login")
	}
	accounts := 0
	active := ""
	for _, account := range a.state.Integrations.Accounts {
		if account.ProviderID == "claude" && account.TargetID == "local" {
			accounts++
			if account.InUse {
				active = account.ID
			}
		}
	}
	if accounts != 2 || active != "account-local-claude" {
		t.Fatal("adding login changed the active account")
	}
}

func TestProviderUpdateAndInstallActionsDoNotToggleDetails(t *testing.T) {
	a, tt := integrationTestView(t, "Providers")
	integrationClick(t, tt, "Update Claude Code")
	if a.integrationUI.ExpandedProvider != "" {
		t.Fatal("update action opened preferences")
	}
	d, _ := a.providerDetails("claude")
	if d.UpdateState != "updated" || d.Version != d.LatestVersion {
		t.Fatal("update action had no result")
	}
	integrationClick(t, tt, "Install Cursor")
	d, _ = a.providerDetails("cursor")
	if !d.Installed || !d.Enabled {
		t.Fatal("install action had no result")
	}
}

func TestProviderDeviceScopeCloudAPIKeyAndOfflineFailure(t *testing.T) {
	a, tt := integrationTestView(t, "Providers")
	if err := tt.Click("Provider device"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Cloud"); err != nil {
		t.Fatal(err)
	}
	if a.integrationTarget() != "cloud-1" {
		t.Fatal("device selector retained local target")
	}
	integrationClick(t, tt, "Claude Code preferences")
	integrationClick(t, tt, "Add Anthropic API key")
	if err := tt.Click("API key"); err != nil {
		t.Fatal(err)
	}
	tt.Type("secret-test-key-not-real")
	integrationClick(t, tt, "Save key")
	if a.integrationUI.APIProvider != "" || a.integrationUI.APIKey != "" {
		t.Fatal("successful key save retained credential input")
	}
	cloud, local := 0, 0
	for _, account := range a.state.Integrations.Accounts {
		if account.ProviderID == "claude" {
			switch account.TargetID {
			case "cloud-1":
				cloud++
			case "local":
				local++
			}
		}
	}
	if cloud != 1 || local != 1 {
		t.Fatal("Cloud login changed local credentials")
	}
}

func TestDevicesCopyRenameAndScope(t *testing.T) {
	a, tt := integrationTestView(t, "Devices")
	if !tt.HasText("Devices synced to this workspace.") || !tt.HasText("Other devices") {
		t.Fatal("scope subtitle/group missing")
	}
	if err := tt.Click("Copy device ID local"); err != nil {
		t.Fatal(err)
	}
	if tt.Clipboard() != "local" {
		t.Fatal("device id did not reach clipboard")
	}
	if err := tt.Click("Rename This device"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Device name"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("Studio")
	tt.Key(0, ui.KeyEnter)
	if a.integrationUI.RenameOpen || a.state.Devices[0].Name != "Studio" {
		t.Fatal("rename did not save/close dialog")
	}
	a.state.Integrations.WorkspaceScope = "local"
	tt.Frame()
	if tt.HasText("Other devices") || !tt.HasText("Devices in this local workspace.") {
		t.Fatal("local workspace claimed synced device metadata")
	}
}

func TestCloudGithubAndUsageRangeHaveVisibleResults(t *testing.T) {
	a, tt := integrationTestView(t, "Cloud")
	if tt.HasText("Cloud sync") || tt.HasText("Storage") {
		t.Fatal("old cloud demo rows remain")
	}
	integrationClick(t, tt, "Connect")
	if a.state.Integrations.Cloud.Github.State != "connected" || !tt.HasText("Repositories") {
		t.Fatal("GitHub connection has no repository result")
	}
	integrationClick(t, tt, "Choose on GitHub")
	if len(a.state.Integrations.Cloud.Repositories) != 2 {
		t.Fatal("repository choice not reflected")
	}
	integrationClick(t, tt, "Usage range")
	if err := tt.ChooseMenuItem("Last month"); err != nil {
		t.Fatal(err)
	}
	if a.state.Integrations.Cloud.UsageRange != "lastMonth" {
		t.Fatal("usage selection not applied")
	}
	integrationClick(t, tt, "Disconnect")
	if len(a.state.Integrations.Cloud.Repositories) != 0 {
		t.Fatal("disconnected GitHub kept repositories")
	}
}

func TestBillingPlanPacksAndManagementMutateLedger(t *testing.T) {
	a, tt := integrationTestView(t, "Billing")
	before := a.state.Integrations.Billing.Credits.Extra
	integrationClick(t, tt, "More packs")
	if a.integrationUI.Packs != 2 {
		t.Fatal("pack increment failed")
	}
	integrationClick(t, tt, "Buy for $20")
	if a.state.Integrations.Billing.Credits.Extra != before+24000 {
		t.Fatal("purchase did not settle credits")
	}
	integrationClick(t, tt, "Switch Base")
	if a.state.Integrations.Billing.Subscription.PlanID != "base" {
		t.Fatal("plan change did not settle")
	}
	integrationClick(t, tt, "Manage subscription")
	if !a.state.Integrations.Billing.Subscription.CancelAtPeriodEnd {
		t.Fatal("subscription management had no result")
	}
}

func TestIntegrationPageErrorsAreInlineAndPreserveFailedDraft(t *testing.T) {
	a, tt := integrationTestView(t, "Devices")
	integrationClick(t, tt, "Rename This device")
	a.integrationUI.RenameText = strings.Repeat("x", 81)
	a.renameDevice()
	tt.Frame()
	if !a.integrationUI.RenameOpen || a.integrationUI.Error == "" || a.errorText != "" {
		t.Fatal("rename failure escaped page or lost editable input")
	}
	if got := a.state.Devices[0].Name; got != "This device" {
		t.Fatal("invalid rename mutated device")
	}
}

func TestCaptureIntegrationSettingsAudit(t *testing.T) {
	dir := os.Getenv("ZERON_INTEGRATIONS_AUDIT_DIR")
	if dir == "" {
		t.Skip("explicit audit directory required")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, section string
		setup         func(*App)
	}{
		{"providers", "Providers", nil},
		{"provider-details", "Providers", func(a *App) { a.integrationUI.ExpandedProvider = "claude" }},
		{"provider-login", "Providers", func(a *App) {
			a.integrationUI.ExpandedProvider = "claude"
			a.dispatch(a.integrationCommand("provider.login.start", "claude"))
		}},
		{"provider-login-error", "Providers", func(a *App) {
			a.integrationUI.ExpandedProvider = "claude"
			a.dispatch(a.integrationCommand("provider.login.start", "claude"))
			cmd := a.integrationCommand("provider.login.submit", "claude")
			cmd.Text = "invalid"
			a.dispatch(cmd)
		}},
		{"provider-cloud", "Providers", func(a *App) { a.integrationUI.ProviderTarget = "cloud-1"; a.integrationUI.ExpandedProvider = "openai" }},
		{"devices", "Devices", nil},
		{"device-rename", "Devices", func(a *App) {
			a.integrationUI.RenameOpen = true
			a.integrationUI.RenameID = "local"
			a.integrationUI.RenameText = "This device"
		}},
		{"cloud", "Cloud", nil},
		{"cloud-repositories", "Cloud", func(a *App) { a.dispatch(contracts.Command{Kind: "cloud.github.start"}) }},
		{"billing", "Billing", nil},
		{"billing-no-plan", "Billing", func(a *App) {
			a.state.Integrations.Billing.Subscription = nil
			a.state.Integrations.Billing.Credits.Monthly = 0
			a.state.Integrations.Billing.Credits.MonthlyTotal = 0
			a.state.Integrations.Billing.Credits.Total = a.state.Integrations.Billing.Credits.Extra
		}},
		{"billing-error", "Billing", func(a *App) {
			a.state.Integrations.Billing.LoadState = contracts.LoadError
			a.state.Integrations.Billing.Error = "Engine not connected"
		}},
	}
	for _, mode := range []string{"light", "dark"} {
		for _, tc := range cases {
			t.Run(tc.name+"-"+mode, func(t *testing.T) {
				a, _ := settingsTestApp(t)
				a.OpenRoute("settings/" + tc.section)
				if tc.setup != nil {
					tc.setup(a)
				}
				a.state.Settings.Appearance.Mode = mode
				a.state.Settings.Appearance.ThemeID = "zeron-" + mode
				a.palette = design.Palette("zeron-"+mode, mode == "dark")
				tt := ui.NewTester(a.View, 1320, 880)
				file, err := os.Create(filepath.Join(dir, tc.name+"-"+mode+".png"))
				if err != nil {
					t.Fatal(err)
				}
				if err := png.Encode(file, tt.Image()); err != nil {
					t.Fatal(err)
				}
				if err := file.Close(); err != nil {
					t.Fatal(err)
				}
				if _, err := a.service.Snapshot(context.Background()); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
