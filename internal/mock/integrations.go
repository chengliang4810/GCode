package mock

import (
	"fmt"
	"slices"
	"time"

	"gcode/internal/contracts"
)

func cloneIntegrations(v contracts.IntegrationState) contracts.IntegrationState {
	v.Providers = slices.Clone(v.Providers)
	for i := range v.Providers {
		v.Providers[i].LoginOptions = slices.Clone(v.Providers[i].LoginOptions)
	}
	v.Accounts = slices.Clone(v.Accounts)
	for i := range v.Accounts {
		v.Accounts[i].Usage = slices.Clone(v.Accounts[i].Usage)
	}
	if v.Login != nil {
		x := *v.Login
		v.Login = &x
	}
	v.Cloud.Repositories = slices.Clone(v.Cloud.Repositories)
	v.Cloud.Connections = slices.Clone(v.Cloud.Connections)
	v.Cloud.Usage = slices.Clone(v.Cloud.Usage)
	v.Billing.Plans = slices.Clone(v.Billing.Plans)
	if v.Billing.Subscription != nil {
		x := *v.Billing.Subscription
		v.Billing.Subscription = &x
	}
	if v.Billing.Checkout != nil {
		x := *v.Billing.Checkout
		v.Billing.Checkout = &x
	}
	return v
}

// Missing metadata is additive: existing provider switches and user histories survive.
func (s *Server) ensureIntegrationsLocked() {
	if s.state.Integrations.ProvidersState != "" {
		return
	}
	now := s.opts.Now().UTC()
	v := contracts.IntegrationState{ObservedAt: now, WorkspaceScope: "synced", ProvidersState: contracts.LoadReady}
	for _, device := range s.state.Devices {
		for _, p := range s.state.Providers {
			installed := (p.ID == "claude" || p.ID == "openai") && device.Status == "connected"
			d := contracts.ProviderDetails{TargetID: device.ID, ProviderID: p.ID, Installed: installed, CanInstall: device.Current, Enabled: p.Enabled, InstallState: "idle", Version: "1.0.0", LatestVersion: "1.0.0", UpdateState: "idle", UpdatePolicy: "notify", DollarSkills: true, SeparateSlash: true, NativeSubagents: true, CanSwitchSubagents: p.ID == "claude" || p.ID == "openai", AccountsState: contracts.LoadReady, LoginKind: "browser"}
			if p.ID == "claude" {
				d.LoginKind = "code"
				d.Version = "2.1.31"
				d.LatestVersion = "2.1.32"
				d.UpdateState = "available"
			}
			if p.ID == "openai" {
				d.Version = "0.112.0"
				d.LatestVersion = d.Version
			}
			d.LoginOptions = providerLoginOptions(p.ID)
			if p.ID == "antigravity" {
				d.SingleLogin = true
				d.CanInstall = false
			}
			if p.ID == "hermes" {
				d.AccountNote = "Hermes manages its own credential pool and rotates through it. Accounts added here go through `hermes auth add`; remove one with `hermes auth remove`."
			}
			if device.Status != "connected" {
				d.AccountsState = contracts.LoadError
				d.AccountsError = "Connection closed. Reconnecting to device…"
			}
			v.Providers = append(v.Providers, d)
			if p.Account != "" && installed {
				v.Accounts = append(v.Accounts, contracts.ProviderAccount{ID: "account-" + device.ID + "-" + p.ID, ProviderID: p.ID, TargetID: device.ID, Email: p.Account, Plan: "Pro", InUse: true, AuthStatus: "ready", UsageState: contracts.LoadReady, CanRemove: true, Usage: []contracts.UsageWindow{{Label: "Session", UsedPercent: 24, ResetsAt: now.Add(3 * time.Hour)}, {Label: "Week", UsedPercent: 38, ResetsAt: now.Add(4 * 24 * time.Hour)}}})
			}
		}
	}
	v.Cloud = contracts.CloudSettingsState{LoadState: contracts.LoadReady, Available: s.state.User.SignedIn, State: "off", UsageRange: "7d", RepositoriesState: contracts.LoadReady, Github: contracts.CloudConnection{ProviderID: "github", State: "disconnected"}}
	if s.state.Account.CloudStatus == "connected" {
		v.Cloud.State = "ready"
		v.Cloud.DeviceID = "cloud-1"
	}
	v.Billing = contracts.BillingState{LoadState: contracts.LoadReady, Available: s.state.User.SignedIn, HasCustomer: true, Plans: []contracts.BillingPlan{{ID: "base", Name: "Base", PriceCents: 2000, MonthlyCredits: 45000}, {ID: "pro", Name: "Pro", PriceCents: 5000, MonthlyCredits: 112500}, {ID: "max", Name: "Max", PriceCents: 20000, MonthlyCredits: 450000}}, Pack: contracts.BillingPack{PriceCents: 1000, Credits: 12000}}
	end := now.AddDate(0, 1, 0)
	v.Billing.Subscription = &contracts.BillingSubscription{PlanID: "pro", Status: "active", PeriodEnd: end}
	v.Billing.Credits = contracts.BillingCredits{Monthly: 82500, MonthlyTotal: 112500, MonthlyEndsAt: end, Extra: 24000, Total: 106500}
	s.state.Integrations = v
	for i := range s.state.Integrations.Accounts {
		a := &s.state.Integrations.Accounts[i]
		a.Switchable = a.ProviderID != "hermes" && a.ProviderID != "antigravity"
		a.ReportsUsage = a.ProviderID != "antigravity"
	}
	s.regenerateCloudUsageLocked("7d")
	for _, p := range s.state.Providers {
		if p.ID == "claude" || p.ID == "openai" {
			s.state.Integrations.Providers = append(s.state.Integrations.Providers, contracts.ProviderDetails{ProviderID: p.ID, TargetID: "cloud-1", Installed: true, Enabled: true, AccountsState: contracts.LoadReady, LoginKind: "browser", DollarSkills: true, SeparateSlash: true, UpdatePolicy: "off"})
		}
	}
	if v.Cloud.State == "ready" {
		found := false
		for _, d := range s.state.Devices {
			if d.ID == "cloud-1" {
				found = true
			}
		}
		if !found {
			s.state.Devices = append(s.state.Devices, contracts.Device{ID: "cloud-1", Name: "Cloud", Platform: "cloud", Architecture: "amd64", Status: "asleep"})
		}
	}
}

func (s *Server) providerDetailsLocked(target, id string) *contracts.ProviderDetails {
	if target == "" {
		target = "local"
	}
	for i := range s.state.Integrations.Providers {
		v := &s.state.Integrations.Providers[i]
		if v.TargetID == target && v.ProviderID == id {
			return v
		}
	}
	return nil
}

func (s *Server) regenerateCloudUsageLocked(value string) {
	days := 7
	switch value {
	case "30d":
		days = 30
	case "90d":
		days = 90
	case "thisMonth":
		days = s.opts.Now().UTC().Day()
	case "lastMonth":
		now := s.opts.Now().UTC()
		days = time.Date(now.Year(), now.Month(), 0, 0, 0, 0, 0, time.UTC).Day()
	}
	v := &s.state.Integrations.Cloud
	v.UsageRange = value
	v.Usage = make([]contracts.CloudUsageDay, days)
	now := s.opts.Now().UTC()
	if value == "lastMonth" {
		now = time.Date(now.Year(), now.Month(), 0, 0, 0, 0, 0, time.UTC)
	}
	for i := range v.Usage {
		v.Usage[i] = contracts.CloudUsageDay{Day: now.AddDate(0, 0, i-days+1), Credits: float64((i*173 + 41) % 900)}
	}
}

// Generations reject stale completion after cancellation/retry; Close owns all jobs.
func (s *Server) integrationJobLocked(key string, apply func()) {
	if s.integrationJobs == nil {
		s.integrationJobs = make(map[string]uint64)
	}
	s.integrationJobs[key]++
	if s.opts.Instant {
		apply()
		return
	}
	generation := s.integrationJobs[key]
	delay := max(600*time.Millisecond, s.opts.TokenInterval*20)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-s.done:
			return
		case <-timer.C:
		}
		s.mu.Lock()
		if s.closed || s.integrationJobs[key] != generation {
			s.mu.Unlock()
			return
		}
		apply()
		s.publishLocked("integrations.updated", "", "", "", "")
		s.mu.Unlock()
		_ = s.persist()
	}()
}

func (s *Server) invalidateIntegrationJobLocked(key string) {
	if s.integrationJobs != nil {
		s.integrationJobs[key]++
	}
}

func (s *Server) finishProviderLoginLocked(id string) {
	v := &s.state.Integrations
	login := v.Login
	if login == nil || login.ID != id || login.State != "waiting" {
		return
	}
	account := contracts.ProviderAccount{ID: s.idLocked("account"), ProviderID: login.ProviderID, TargetID: login.TargetID, Email: s.state.User.Email, Plan: "Pro", InUse: true, AuthStatus: "ready", CanRemove: true, UsageState: contracts.LoadReady, Usage: []contracts.UsageWindow{{Label: "Session", ResetsAt: s.opts.Now().Add(5 * time.Hour)}, {Label: "Week", ResetsAt: s.opts.Now().AddDate(0, 0, 7)}}}
	account.LoginProvider = login.LoginProvider
	account.Switchable = login.ProviderID != "hermes" && login.ProviderID != "antigravity"
	account.CanRemove = login.ProviderID != "hermes"
	account.ReportsUsage = login.ProviderID != "antigravity"
	if !account.ReportsUsage {
		account.Usage = nil
	}
	if login.TargetID == "cloud-1" {
		v.Accounts = slices.DeleteFunc(v.Accounts, func(a contracts.ProviderAccount) bool {
			return a.TargetID == login.TargetID && a.ProviderID == login.ProviderID
		})
	}
	for i := range v.Accounts {
		if v.Accounts[i].ProviderID == login.ProviderID && v.Accounts[i].TargetID == login.TargetID && v.Accounts[i].LoginProvider == login.LoginProvider && v.Accounts[i].InUse {
			account.InUse = false
		}
	}
	v.Accounts = append(v.Accounts, account)
	login.State = "completed"
	if login.TargetID == "local" && account.InUse {
		for i := range s.state.Providers {
			if s.state.Providers[i].ID == login.ProviderID {
				s.state.Providers[i].Status = "connected"
				s.state.Providers[i].Account = account.Email
			}
		}
	}
}

func (s *Server) finishCheckoutLocked(id string) {
	v := &s.state.Integrations.Billing
	x := v.Checkout
	if x == nil || x.ID != id || x.State != "waiting" {
		return
	}
	if x.Kind == "credits" {
		added := v.Pack.Credits * float64(x.Packs)
		v.Credits.Extra += added
		v.Notice = fmt.Sprintf("Added %s credits", groupCredits(added))
	} else if x.Kind == "plan" {
		for _, plan := range v.Plans {
			if plan.ID == x.PlanID {
				end := s.opts.Now().UTC().AddDate(0, 1, 0)
				v.Subscription = &contracts.BillingSubscription{PlanID: plan.ID, Status: "active", PeriodEnd: end}
				v.Credits.Monthly = plan.MonthlyCredits
				v.Credits.MonthlyTotal = plan.MonthlyCredits
				v.Credits.MonthlyEndsAt = end
				v.HasCustomer = true
				v.Notice = fmt.Sprintf("You're on %s — %s credits a month", plan.Name, groupCredits(plan.MonthlyCredits))
				s.state.Account.Plan = plan.Name
				break
			}
		}
	} else if v.Subscription != nil {
		v.Subscription.CancelAtPeriodEnd = !v.Subscription.CancelAtPeriodEnd
		v.Notice = "Your plan won't renew"
		if !v.Subscription.CancelAtPeriodEnd {
			v.Notice = "Your plan will renew"
		}
	}
	v.Credits.Total = v.Credits.Monthly + v.Credits.Extra
	s.state.Account.Credits = v.Credits.Total
	x.State = "completed"
}

func groupCredits(value float64) string {
	raw := fmt.Sprintf("%.0f", value)
	for i := len(raw) - 3; i > 0; i -= 3 {
		raw = raw[:i] + "," + raw[i:]
	}
	return raw
}

func seedAdditionalProviders(s *contracts.Snapshot) {
	for _, p := range []contracts.Provider{{ID: "antigravity", Name: "Antigravity", Icon: "antigravity-mark"}, {ID: "grok", Name: "Grok", Icon: "grok-mark"}, {ID: "devin", Name: "Devin", Icon: "devin-mark"}, {ID: "opencode", Name: "OpenCode", Icon: "opencode-mark"}, {ID: "pi", Name: "Pi", Icon: "pi-mark"}, {ID: "hermes", Name: "Hermes", Icon: "hermes-mark"}, {ID: "muse", Name: "Muse Code", Icon: "muse-mark"}} {
		p.Status = "notConnected"
		s.Providers = append(s.Providers, p)
	}
}

func providerLoginOptions(id string) []contracts.ProviderLoginOption {
	switch id {
	case "opencode":
		return []contracts.ProviderLoginOption{{ID: "openai", Label: "ChatGPT", Kind: "browser"}, {ID: "github-copilot", Label: "GitHub Copilot", Kind: "deviceCode"}}
	case "pi":
		return []contracts.ProviderLoginOption{{ID: "openai-codex", Label: "ChatGPT", Kind: "browser"}}
	case "hermes":
		return []contracts.ProviderLoginOption{{ID: "openai-codex", Label: "ChatGPT", Kind: "deviceCode"}, {ID: "nous", Label: "Nous Portal", Kind: "deviceCode"}}
	}
	kind := "browser"
	if id == "claude" {
		kind = "code"
	}
	if id == "grok" {
		kind = "deviceCode"
	}
	return []contracts.ProviderLoginOption{{Kind: kind}}
}

func (s *Server) restoreIntegrationsLocked() {
	v := &s.state.Integrations
	v.ObservedAt = s.opts.Now().UTC()
	v.Login = nil
	if v.Billing.Checkout != nil && v.Billing.Checkout.State == "waiting" {
		v.Billing.Checkout.State = "cancelled"
	}
	for i := range v.Providers {
		if v.Providers[i].InstallState == "installing" {
			v.Providers[i].InstallState = "idle"
		}
		if v.Providers[i].UpdateState == "downloading" {
			v.Providers[i].UpdateState = "available"
		}
	}
	if v.Cloud.Github.State == "waiting" {
		v.Cloud.Github = contracts.CloudConnection{ProviderID: "github", State: "disconnected"}
	}
	if v.Cloud.State == "provisioning" {
		v.Cloud.State = "off"
	}
	v.Cloud.Available = s.state.User.SignedIn
	v.Billing.Available = s.state.User.SignedIn
}
