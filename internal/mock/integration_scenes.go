package mock

import "gcode/internal/contracts"

// IntegrationScenario is an optional backend fixture selection, never a UI panel.
// It enables account/platform conditions without asking the view to invent data.
func (s *Server) seedIntegrationScenarioLocked(scene string) {
	v := &s.state.Integrations
	d := s.providerDetailsLocked("local", "claude")
	switch scene {
	case "providers.loading":
		v.ProvidersState = contracts.LoadLoading
	case "providers.error":
		v.ProvidersState = contracts.LoadError
		v.ProvidersError = "Engine not connected"
	case "provider.disabled":
		if d != nil {
			d.Enabled = false
			for i := range s.state.Providers {
				if s.state.Providers[i].ID == "claude" {
					s.state.Providers[i].Enabled = false
				}
			}
		}
	case "provider.notConnected":
		v.Accounts = filterIntegrationAccounts(v.Accounts, "local", "claude")
		s.syncProviderAccountLocked("local", "claude")
	case "provider.updateError":
		if d != nil {
			d.UpdateState = "failed"
			d.Error = "Update failed"
		}
	case "accounts.loading":
		if d != nil {
			d.AccountsState = contracts.LoadLoading
		}
	case "accounts.error":
		if d != nil {
			d.AccountsState = contracts.LoadError
			d.AccountsError = "Engine not connected"
		}
	case "accounts.expired", "accounts.usageError":
		for i := range v.Accounts {
			a := &v.Accounts[i]
			if a.TargetID == "local" && a.ProviderID == "claude" {
				a.Usage = nil
				a.UsageState = contracts.LoadError
				a.UsageError = "Rate limited by Anthropic — retrying in 2m"
				if scene == "accounts.expired" {
					a.AuthStatus = "expired"
					a.UsageError = "Signed out — sign in again"
				}
			}
		}
	case "cloud.loading":
		v.Cloud.LoadState = contracts.LoadLoading
	case "cloud.error":
		v.Cloud.LoadState = contracts.LoadError
		v.Cloud.Error = "Engine not connected"
	case "cloud.unavailable":
		v.Cloud.Available = false
	case "github.waiting":
		v.Cloud.Github = contracts.CloudConnection{ProviderID: "github", State: "waiting", UserCode: "WDJB-MJHT", URL: "https://github.com/login/device"}
	case "github.needsReconnect":
		v.Cloud.Github = contracts.CloudConnection{ProviderID: "github", State: "needsReconnect", Account: "alex"}
	case "github.notAuthorized":
		v.Cloud.Github = contracts.CloudConnection{ProviderID: "github", State: "notAuthorized", Account: "alex"}
	case "github.failed":
		v.Cloud.Github = contracts.CloudConnection{ProviderID: "github", State: "failed", Error: "The code expired before it was approved."}
	case "billing.loading":
		v.Billing.LoadState = contracts.LoadLoading
	case "billing.error":
		v.Billing.LoadState = contracts.LoadError
		v.Billing.Error = "Engine not connected"
	case "billing.unavailable":
		v.Billing.Available = false
	case "billing.noPlan":
		v.Billing.Subscription = nil
		v.Billing.HasCustomer = false
		v.Billing.Credits.Monthly = 0
		v.Billing.Credits.MonthlyTotal = 0
		v.Billing.Credits.Total = v.Billing.Credits.Extra
	case "billing.pastDue":
		if v.Billing.Subscription != nil {
			v.Billing.Subscription.Status = "past_due"
		}
	case "billing.cancelled":
		if v.Billing.Subscription != nil {
			v.Billing.Subscription.CancelAtPeriodEnd = true
		}
	case "billing.pending":
		v.Billing.Checkout = &contracts.BillingCheckout{ID: "checkout-review", Kind: "credits", Packs: 1, State: "waiting"}
	}
}

func filterIntegrationAccounts(accounts []contracts.ProviderAccount, target, provider string) []contracts.ProviderAccount {
	out := make([]contracts.ProviderAccount, 0, len(accounts))
	for _, a := range accounts {
		if a.TargetID != target || a.ProviderID != provider {
			out = append(out, a)
		}
	}
	return out
}
