package mock

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"gcode/internal/contracts"
)

func (s *Server) integrationCommandLocked(c contracts.Command) error {
	s.ensureIntegrationsLocked()
	r := contracts.IntegrationRequest{TargetID: "local"}
	if c.Integration != nil {
		r = *c.Integration
		if r.TargetID == "" {
			r.TargetID = "local"
		}
	}
	v := &s.state.Integrations
	if strings.HasPrefix(c.Kind, "provider.") {
		d := s.providerDetailsLocked(r.TargetID, r.ProviderID)
		if d == nil {
			return ErrNotFound
		}
		if r.TargetID != "cloud-1" {
			for _, device := range s.state.Devices {
				if device.ID == r.TargetID && device.Status != "connected" {
					return errors.New("connection closed; reconnect to the selected device")
				}
			}
		}
		key := "provider/" + r.TargetID + "/" + r.ProviderID
		switch c.Kind {
		case "provider.enable":
			if r.TargetID == "cloud-1" {
				return errors.New("cloud providers are always enabled")
			}
			if c.Enabled && !d.Installed {
				return errors.New("install the agent before enabling it")
			}
			if !c.Enabled && d.Enabled && d.Installed {
				count := 0
				for _, other := range v.Providers {
					if other.TargetID == r.TargetID && other.Enabled && other.Installed {
						count++
					}
				}
				if count <= 1 {
					return errors.New("keep at least one installed agent enabled")
				}
			}
			d.Enabled = c.Enabled
			if r.TargetID == "local" {
				for i := range s.state.Providers {
					if s.state.Providers[i].ID == r.ProviderID {
						s.state.Providers[i].Enabled = c.Enabled
					}
				}
				s.state.ModelCatalog.State = catalogStatus(s.state)
			}
			return nil
		case "provider.preference":
			switch c.Name {
			case "dollar":
				d.DollarSkills = c.Enabled
			case "slash":
				d.SeparateSlash = c.Enabled
			case "subagents":
				if !d.CanSwitchSubagents || r.TargetID == "cloud-1" {
					return errors.New("native subagents cannot be changed on the selected device")
				}
				d.NativeSubagents = c.Enabled
			case "updatePolicy":
				if r.Value != "notify" && r.Value != "autoWhenIdle" && r.Value != "off" {
					return errors.New("invalid update policy")
				}
				d.UpdatePolicy = r.Value
			default:
				return errors.New("unknown provider preference")
			}
			return nil
		case "provider.install":
			if d.Installed {
				return nil
			}
			if !d.CanInstall {
				return errors.New("this device cannot install the agent")
			}
			d.InstallState = "installing"
			d.Error = ""
			s.integrationJobLocked(key, func() {
				if x := s.providerDetailsLocked(r.TargetID, r.ProviderID); x != nil {
					x.Installed = true
					x.InstallState = "idle"
					x.Enabled = true
					x.Version = x.LatestVersion
					for i := range s.state.Providers {
						if r.TargetID == "local" && s.state.Providers[i].ID == r.ProviderID {
							s.state.Providers[i].Enabled = true
							if len(s.state.Providers[i].Models) == 0 {
								modelID := r.ProviderID + "-default"
								s.state.Providers[i].Models = []string{modelID}
								s.state.Models = append(s.state.Models, contracts.Model{ID: modelID, ProviderID: r.ProviderID, Label: "Default", ContextLimit: 200000})
							}
						}
					}
				}
			})
			return nil
		case "provider.install.cancel":
			s.invalidateIntegrationJobLocked(key)
			d.InstallState = "idle"
			return nil
		case "provider.update":
			if !d.Installed {
				return errors.New("install the agent first")
			}
			if r.TargetID == "cloud-1" {
				return errors.New("cloud does not offer agent updates")
			}
			d.UpdateState = "downloading"
			d.Error = ""
			s.integrationJobLocked(key, func() {
				if x := s.providerDetailsLocked(r.TargetID, r.ProviderID); x != nil {
					x.Version = x.LatestVersion
					x.UpdateState = "updated"
				}
			})
			return nil
		case "provider.update.cancel":
			s.invalidateIntegrationJobLocked(key)
			d.UpdateState = "available"
			return nil
		case "provider.accounts.refresh":
			d.AccountsState = contracts.LoadLoading
			d.AccountsError = ""
			d.Warning = ""
			for i := range v.Accounts {
				a := &v.Accounts[i]
				if a.TargetID == r.TargetID && a.ProviderID == r.ProviderID {
					a.UsageState = contracts.LoadLoading
				}
			}
			s.integrationJobLocked(key+"/accounts", func() {
				if x := s.providerDetailsLocked(r.TargetID, r.ProviderID); x != nil {
					x.AccountsState = contracts.LoadReady
					x.AccountsError = ""
				}
				for i := range s.state.Integrations.Accounts {
					a := &s.state.Integrations.Accounts[i]
					if a.TargetID == r.TargetID && a.ProviderID == r.ProviderID {
						a.UsageState = contracts.LoadReady
						a.UsageError = ""
						if a.AuthStatus == "expired" {
							a.UsageState = contracts.LoadError
							a.UsageError = "Signed out — sign in again"
						} else if a.ReportsUsage && len(a.Usage) == 0 {
							a.Usage = []contracts.UsageWindow{{Label: "Session", ResetsAt: s.opts.Now().Add(3 * time.Hour)}, {Label: "Week", ResetsAt: s.opts.Now().AddDate(0, 0, 7)}}
						}
					}
				}
			})
			return nil
		case "provider.account.switch", "provider.account.remove":
			index := -1
			for i, a := range v.Accounts {
				if a.ID == r.AccountID && a.ProviderID == r.ProviderID && a.TargetID == r.TargetID {
					index = i
					break
				}
			}
			if index < 0 {
				return ErrNotFound
			}
			if c.Kind == "provider.account.remove" {
				if !v.Accounts[index].CanRemove {
					return errors.New("this account cannot be removed")
				}
				wasCurrent := v.Accounts[index].InUse
				v.Accounts = slices.Delete(v.Accounts, index, index+1)
				if wasCurrent {
					for i := range v.Accounts {
						if v.Accounts[i].ProviderID == r.ProviderID && v.Accounts[i].TargetID == r.TargetID {
							v.Accounts[i].InUse = true
							break
						}
					}
				}
			} else {
				if !v.Accounts[index].Switchable {
					return errors.New("this agent manages its own accounts")
				}
				for i := range v.Accounts {
					if v.Accounts[i].ProviderID == r.ProviderID && v.Accounts[i].TargetID == r.TargetID && v.Accounts[i].LoginProvider == v.Accounts[index].LoginProvider {
						v.Accounts[i].InUse = i == index
					}
				}
			}
			s.syncProviderAccountLocked(r.TargetID, r.ProviderID)
			return nil
		case "provider.login.start", "provider.login.retry":
			if !d.Installed {
				return errors.New("install the agent before signing in")
			}
			if d.SingleLogin {
				for _, a := range v.Accounts {
					if a.ProviderID == r.ProviderID && a.TargetID == r.TargetID {
						return errors.New("this agent keeps one login")
					}
				}
			}
			id := s.idLocked("login")
			if r.Value != "" {
				valid := false
				for _, o := range d.LoginOptions {
					if o.ID == r.Value {
						valid = true
					}
				}
				if !valid {
					return errors.New("unknown sign-in provider")
				}
			}
			v.Login = &contracts.ProviderLogin{ID: id, ProviderID: r.ProviderID, TargetID: r.TargetID, Kind: d.LoginKind, State: "waiting", URL: "https://example.test/sign-in/" + r.ProviderID, ExpiresAt: s.opts.Now().UTC().Add(10 * time.Minute)}
			if len(d.LoginOptions) > 0 {
				option := d.LoginOptions[0]
				for _, o := range d.LoginOptions {
					if o.ID == r.Value {
						option = o
						break
					}
				}
				v.Login.LoginProvider = option.ID
				v.Login.Kind = option.Kind
			}
			if v.Login.Kind == "deviceCode" {
				v.Login.UserCode = "WDJB-MJHT"
			}
			if v.Login.Kind != "code" {
				s.integrationJobLocked("login", func() { s.finishProviderLoginLocked(id) })
			}
			return nil
		case "provider.login.cancel":
			if v.Login != nil && (v.Login.ProviderID != r.ProviderID || v.Login.TargetID != r.TargetID || c.ID != "" && v.Login.ID != c.ID) {
				return ErrNotFound
			}
			s.invalidateIntegrationJobLocked("login")
			v.Login = nil
			return nil
		case "provider.login.submit", "provider.login.poll":
			if v.Login == nil || v.Login.ProviderID != r.ProviderID || v.Login.TargetID != r.TargetID || c.ID != "" && c.ID != v.Login.ID {
				return ErrNotFound
			}
			if !s.opts.Now().Before(v.Login.ExpiresAt) {
				v.Login.State = "failed"
				v.Login.Error = "The sign-in expired. Try again."
				return nil
			}
			if c.Kind == "provider.login.submit" {
				if strings.TrimSpace(c.Text) == "" {
					return errors.New("paste the authorization code first")
				}
				if strings.TrimSpace(c.Text) == "invalid" {
					v.Login.State = "failed"
					v.Login.Error = "The authorization code could not be verified."
					return nil
				}
			}
			s.finishProviderLoginLocked(v.Login.ID)
			return nil
		case "provider.apiKey.save":
			if r.TargetID != "cloud-1" || v.Cloud.State != "ready" {
				return errors.New("turn Cloud on in Settings → Cloud first")
			}
			if len(strings.TrimSpace(c.Text)) < 8 {
				return errors.New("paste a key first")
			}
			id := s.idLocked("login")
			v.Login = &contracts.ProviderLogin{ID: id, ProviderID: r.ProviderID, TargetID: r.TargetID, Kind: "apiKey", State: "waiting"}
			s.finishProviderLoginLocked(id)
			v.Login = nil
			return nil
		}
	}
	switch c.Kind {
	case "providers.refresh":
		v.ProvidersState = contracts.LoadLoading
		v.ProvidersError = ""
		s.integrationJobLocked("providers", func() {
			s.state.Integrations.ProvidersState = contracts.LoadReady
			s.state.Integrations.ProvidersError = ""
		})
		return nil
	case "device.rename":
		name := strings.TrimSpace(c.Name)
		if len([]rune(name)) > 80 {
			return errors.New("device name must be 80 characters or fewer")
		}
		for i := range s.state.Devices {
			if s.state.Devices[i].ID == c.ID {
				s.state.Devices[i].Name = name
				return nil
			}
		}
		return ErrNotFound
	case "cloud.refresh", "cloud.retry":
		v.Cloud.LoadState = contracts.LoadReady
		v.Cloud.Error = ""
		v.Cloud.Available = s.state.User.SignedIn
		if v.Cloud.State == "error" {
			v.Cloud.State = "off"
		}
		return nil
	case "cloud.enable":
		if !s.state.User.SignedIn {
			return errors.New("sign in with sync turned on first")
		}
		v.Cloud.State = "provisioning"
		s.integrationJobLocked("cloud", func() {
			v.Cloud.State = "ready"
			v.Cloud.DeviceID = "cloud-1"
			s.state.Account.CloudStatus = "connected"
			found := false
			for _, d := range s.state.Devices {
				if d.ID == "cloud-1" {
					found = true
				}
			}
			if !found {
				s.state.Devices = append(s.state.Devices, contracts.Device{ID: "cloud-1", Name: "Cloud", Platform: "cloud", Architecture: "amd64", Status: "asleep"})
			}
		})
		return nil
	case "cloud.delete":
		s.invalidateIntegrationJobLocked("cloud")
		s.invalidateIntegrationJobLocked("github")
		v.Cloud.State = "off"
		v.Cloud.DeviceID = ""
		v.Cloud.Github = contracts.CloudConnection{ProviderID: "github", State: "disconnected"}
		v.Cloud.Repositories = nil
		s.state.Account.CloudStatus = "disconnected"
		s.state.Devices = slices.DeleteFunc(s.state.Devices, func(d contracts.Device) bool { return d.ID == "cloud-1" })
		return nil
	case "cloud.github.start":
		if v.Cloud.State != "ready" {
			return errors.New("turn on Cloud to connect GitHub")
		}
		v.Cloud.Github = contracts.CloudConnection{ProviderID: "github", State: "waiting", UserCode: "WDJB-MJHT", URL: "https://github.com/login/device"}
		s.integrationJobLocked("github", func() { s.finishGithubLocked() })
		return nil
	case "cloud.github.poll":
		s.finishGithubLocked()
		return nil
	case "cloud.github.cancel", "cloud.github.disconnect":
		s.invalidateIntegrationJobLocked("github")
		v.Cloud.Github = contracts.CloudConnection{ProviderID: "github", State: "disconnected"}
		v.Cloud.Repositories = nil
		return nil
	case "cloud.authorize":
		if v.Cloud.State != "ready" {
			return errors.New("turn on Cloud first")
		}
		if r.ProviderID == "github" {
			v.Cloud.Github.Authorized = c.Enabled
			v.Cloud.Github.State = "connected"
			if !c.Enabled {
				v.Cloud.Github.State = "notAuthorized"
			}
			return nil
		}
		found := false
		for i := range v.Cloud.Connections {
			if v.Cloud.Connections[i].ProviderID == r.ProviderID {
				v.Cloud.Connections[i].Authorized = c.Enabled
				found = true
			}
		}
		if !found {
			v.Cloud.Connections = append(v.Cloud.Connections, contracts.CloudConnection{ProviderID: r.ProviderID, State: "connected", Account: s.state.User.Email, Authorized: c.Enabled})
		}
		return nil
	case "cloud.repositories.refresh":
		if v.Cloud.Github.State != "connected" {
			return errors.New("connect GitHub first")
		}
		v.Cloud.RepositoriesState = contracts.LoadReady
		return nil
	case "cloud.repositories.choose":
		if v.Cloud.Github.State != "connected" {
			return errors.New("connect GitHub first")
		}
		v.Cloud.Repositories = []contracts.CloudRepository{{ID: "fieldnotes", Name: "alex/fieldnotes", Private: true, Authorized: true}, {ID: "zeron", Name: "alex/zeron", Authorized: true}}
		return nil
	case "cloud.usage":
		if r.Value != "7d" && r.Value != "30d" && r.Value != "90d" && r.Value != "thisMonth" && r.Value != "lastMonth" {
			return errors.New("invalid usage range")
		}
		s.regenerateCloudUsageLocked(r.Value)
		return nil
	case "billing.refresh":
		v.Billing.LoadState = contracts.LoadReady
		v.Billing.Error = ""
		v.Billing.Available = s.state.User.SignedIn
		return nil
	case "billing.subscribe", "billing.buy", "billing.manage":
		b := &v.Billing
		if !b.Available || !s.state.User.SignedIn {
			return errors.New("billing is unavailable for this account")
		}
		if b.Checkout != nil && b.Checkout.State == "waiting" {
			return errors.New("a checkout is already pending")
		}
		x := &contracts.BillingCheckout{ID: s.idLocked("checkout"), State: "waiting"}
		switch c.Kind {
		case "billing.subscribe":
			valid := false
			for _, p := range b.Plans {
				if p.ID == r.Value {
					valid = true
				}
			}
			if !valid {
				return errors.New("unknown billing plan")
			}
			x.Kind = "plan"
			x.PlanID = r.Value
		case "billing.buy":
			x.Kind = "credits"
			x.Packs = r.Packs
		case "billing.manage":
			if !b.HasCustomer {
				return errors.New("there is no subscription to manage")
			}
			x.Kind = "portal"
		}
		b.Checkout = x
		s.integrationJobLocked("checkout", func() { s.finishCheckoutLocked(x.ID) })
		return nil
	case "billing.checkout.poll":
		if v.Billing.Checkout == nil {
			return ErrNotFound
		}
		if c.ID != "" && v.Billing.Checkout.ID != c.ID {
			return ErrNotFound
		}
		s.finishCheckoutLocked(v.Billing.Checkout.ID)
		return nil
	case "billing.checkout.dismiss":
		if c.ID != "" && (v.Billing.Checkout == nil || v.Billing.Checkout.ID != c.ID) {
			return ErrNotFound
		}
		s.invalidateIntegrationJobLocked("checkout")
		v.Billing.Checkout = nil
		return nil
	}
	return fmt.Errorf("unsupported integration command %q", c.Kind)
}

func (s *Server) syncProviderAccountLocked(target, id string) {
	if target != "local" {
		return
	}
	email := ""
	for _, a := range s.state.Integrations.Accounts {
		if a.TargetID == target && a.ProviderID == id && a.InUse {
			email = a.Email
			break
		}
	}
	for i := range s.state.Providers {
		p := &s.state.Providers[i]
		if p.ID == id {
			p.Account = email
			p.Status = "notConnected"
			if email != "" {
				p.Status = "connected"
			}
		}
	}
}
func (s *Server) finishGithubLocked() {
	v := &s.state.Integrations.Cloud
	if v.Github.State != "waiting" {
		return
	}
	v.Github.State = "connected"
	v.Github.Account = "alex"
	v.Github.UserCode = ""
	v.Github.URL = ""
	v.Github.Authorized = true
	v.Repositories = []contracts.CloudRepository{{ID: "fieldnotes", Name: "alex/fieldnotes", Private: true, Authorized: true}}
}
