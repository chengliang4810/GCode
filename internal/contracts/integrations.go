package contracts

import (
	"errors"
	"strings"
	"time"
)

// LoadStatus describes a backend-owned settings resource, independently of its data.
type LoadStatus string

const (
	LoadReady   LoadStatus = "ready"
	LoadLoading LoadStatus = "loading"
	LoadError   LoadStatus = "error"
)

// IntegrationState contains account/device settings data; credentials never enter snapshots.
type IntegrationState struct {
	ObservedAt     time.Time          `json:"observedAt"`
	WorkspaceScope string             `json:"workspaceScope"`
	ProvidersState LoadStatus         `json:"providersState"`
	ProvidersError string             `json:"providersError,omitempty"`
	Providers      []ProviderDetails  `json:"providers"`
	Accounts       []ProviderAccount  `json:"accounts"`
	Login          *ProviderLogin     `json:"login,omitempty"`
	Cloud          CloudSettingsState `json:"cloud"`
	Billing        BillingState       `json:"billing"`
}

type ProviderDetails struct {
	LoginOptions       []ProviderLoginOption `json:"loginOptions"`
	AccountNote        string                `json:"accountNote,omitempty"`
	ProviderID         string                `json:"providerId"`
	TargetID           string                `json:"targetId"`
	Installed          bool                  `json:"installed"`
	CanInstall         bool                  `json:"canInstall"`
	Enabled            bool                  `json:"enabled"`
	InstallState       string                `json:"installState"`
	Error              string                `json:"error,omitempty"`
	Version            string                `json:"version"`
	LatestVersion      string                `json:"latestVersion"`
	UpdateState        string                `json:"updateState"`
	UpdatePolicy       string                `json:"updatePolicy"`
	DollarSkills       bool                  `json:"dollarSkills"`
	SeparateSlash      bool                  `json:"separateSlash"`
	NativeSubagents    bool                  `json:"nativeSubagents"`
	CanSwitchSubagents bool                  `json:"canSwitchSubagents"`
	AccountsState      LoadStatus            `json:"accountsState"`
	AccountsError      string                `json:"accountsError,omitempty"`
	Warning            string                `json:"warning,omitempty"`
	LoginKind          string                `json:"loginKind"`
	SingleLogin        bool                  `json:"singleLogin"`
}

type ProviderLoginOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
}

type ProviderAccount struct {
	Switchable    bool          `json:"switchable"`
	ReportsUsage  bool          `json:"reportsUsage"`
	Organization  string        `json:"organization,omitempty"`
	LoginProvider string        `json:"loginProvider,omitempty"`
	ID            string        `json:"id"`
	ProviderID    string        `json:"providerId"`
	TargetID      string        `json:"targetId"`
	Email         string        `json:"email"`
	DisplayName   string        `json:"displayName,omitempty"`
	Plan          string        `json:"plan"`
	InUse         bool          `json:"inUse"`
	AuthStatus    string        `json:"authStatus"`
	UsageState    LoadStatus    `json:"usageState"`
	UsageError    string        `json:"usageError,omitempty"`
	Usage         []UsageWindow `json:"usage"`
	CanRemove     bool          `json:"canRemove"`
}

type ProviderLogin struct {
	LoginProvider string    `json:"loginProvider,omitempty"`
	ID            string    `json:"id"`
	ProviderID    string    `json:"providerId"`
	TargetID      string    `json:"targetId"`
	Kind          string    `json:"kind"`
	State         string    `json:"state"`
	URL           string    `json:"url"`
	UserCode      string    `json:"userCode,omitempty"`
	Error         string    `json:"error,omitempty"`
	ExpiresAt     time.Time `json:"expiresAt"`
}

type CloudSettingsState struct {
	LoadState         LoadStatus        `json:"loadState"`
	Available         bool              `json:"available"`
	State             string            `json:"state"`
	DeviceID          string            `json:"deviceId,omitempty"`
	Error             string            `json:"error,omitempty"`
	Github            CloudConnection   `json:"github"`
	Repositories      []CloudRepository `json:"repositories"`
	RepositoriesState LoadStatus        `json:"repositoriesState"`
	UsageRange        string            `json:"usageRange"`
	Usage             []CloudUsageDay   `json:"usage"`
	Connections       []CloudConnection `json:"connections"`
}

type CloudConnection struct {
	ProviderID string `json:"providerId"`
	State      string `json:"state"`
	Account    string `json:"account,omitempty"`
	UserCode   string `json:"userCode,omitempty"`
	URL        string `json:"url,omitempty"`
	Error      string `json:"error,omitempty"`
	Authorized bool   `json:"authorized"`
}

type CloudRepository struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Private    bool   `json:"private"`
	Authorized bool   `json:"authorized"`
}

type CloudUsageDay struct {
	Day     time.Time `json:"day"`
	Credits float64   `json:"credits"`
}

type BillingState struct {
	LoadState    LoadStatus           `json:"loadState"`
	Available    bool                 `json:"available"`
	Error        string               `json:"error,omitempty"`
	Plans        []BillingPlan        `json:"plans"`
	Subscription *BillingSubscription `json:"subscription,omitempty"`
	Credits      BillingCredits       `json:"credits"`
	Pack         BillingPack          `json:"pack"`
	HasCustomer  bool                 `json:"hasCustomer"`
	Checkout     *BillingCheckout     `json:"checkout,omitempty"`
	Notice       string               `json:"notice,omitempty"`
}

type BillingPlan struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	PriceCents     int     `json:"priceCents"`
	MonthlyCredits float64 `json:"monthlyCredits"`
}

type BillingSubscription struct {
	PlanID            string    `json:"planId"`
	Status            string    `json:"status"`
	PeriodEnd         time.Time `json:"periodEnd"`
	CancelAtPeriodEnd bool      `json:"cancelAtPeriodEnd"`
}

type BillingCredits struct {
	Monthly       float64   `json:"monthly"`
	MonthlyTotal  float64   `json:"monthlyTotal"`
	MonthlyEndsAt time.Time `json:"monthlyEndsAt"`
	Extra         float64   `json:"extra"`
	Total         float64   `json:"total"`
}

type BillingPack struct {
	PriceCents int     `json:"priceCents"`
	Credits    float64 `json:"credits"`
}

type BillingCheckout struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	PlanID string `json:"planId,omitempty"`
	Packs  int    `json:"packs,omitempty"`
	State  string `json:"state"`
	Error  string `json:"error,omitempty"`
}

// IntegrationRequest identifies a target device and the selected settings operation.
// Value holds an enum value (preference, plan, range), never an arbitrary object.
type IntegrationRequest struct {
	TargetID   string `json:"targetId,omitempty"`
	ProviderID string `json:"providerId,omitempty"`
	AccountID  string `json:"accountId,omitempty"`
	Value      string `json:"value,omitempty"`
	Packs      int    `json:"packs,omitempty"`
}

func IsIntegrationCommand(kind string) bool {
	switch kind {
	case "provider.enable":
		return true
	case "providers.refresh", "provider.install", "provider.install.cancel", "provider.update", "provider.update.cancel", "provider.preference", "provider.accounts.refresh", "provider.account.switch", "provider.account.remove", "provider.login.start", "provider.login.submit", "provider.login.cancel", "provider.login.retry", "provider.login.poll", "provider.apiKey.save", "device.rename", "cloud.refresh", "cloud.enable", "cloud.delete", "cloud.retry", "cloud.github.start", "cloud.github.poll", "cloud.github.cancel", "cloud.github.disconnect", "cloud.authorize", "cloud.repositories.refresh", "cloud.repositories.choose", "cloud.usage", "billing.refresh", "billing.subscribe", "billing.buy", "billing.manage", "billing.checkout.poll", "billing.checkout.dismiss":
		return true
	}
	return false
}

func (c Command) validateIntegration() error {
	if c.Adjustment != nil || c.Send != nil || c.Settings != nil || c.Session != nil || c.Attachment != nil || c.TerminalSize != nil || c.ProjectCreate != nil {
		return errors.New("unexpected payload for integration command")
	}
	if strings.HasPrefix(c.Kind, "provider.") && (c.Integration == nil || c.Kind != "provider.accounts.refresh" && c.Integration.ProviderID == "") {
		return errors.New("provider is required")
	}
	if c.Kind == "device.rename" && (c.ID == "" || strings.TrimSpace(c.Name) == "") {
		return errors.New("device and name are required")
	}
	if c.Kind == "billing.buy" && (c.Integration == nil || c.Integration.Packs < 1 || c.Integration.Packs > 20) {
		return errors.New("credit packs must be between 1 and 20")
	}
	return nil
}
