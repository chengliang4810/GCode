package contracts

import (
	"context"
	"gcode/internal/model"
	"gcode/internal/modelcatalog"
)

type RuntimeInfo struct {
	ConfigDirectory  string `json:"configDirectory"`
	DataDirectory    string `json:"dataDirectory"`
	PendingDirectory string `json:"pendingDirectory,omitempty"`
	StorageError     string `json:"storageError,omitempty"`
	Remote           bool   `json:"remote"`
}

type ProviderConfiguration struct {
	Provider    model.Provider `json:"provider"`
	HasAPIKey   bool           `json:"hasApiKey"`
	ModelChecks []ModelCheck   `json:"modelChecks"`
	ModelFetch  *ModelFetch    `json:"modelFetch,omitempty"`
}

type ProviderInput struct {
	Provider    model.Provider `json:"provider"`
	PreserveKey bool           `json:"preserveKey"`
}

// ProviderKeyService reveals a saved credential to one explicit user action.
// Keys must not enter snapshots or broadcast events, and the business host may
// record every reveal: nothing reads this interface implicitly.
type ProviderKeyService interface {
	RevealProviderKey(context.Context, string) (string, error)
}

// ProviderLogoService fetches public website artwork through the business host.
type ProviderLogoService interface {
	FetchProviderLogo(context.Context, string) (string, error)
}

// ModelMetadataService looks up metadata in the business host's local catalog.
type ModelMetadataService interface {
	LookupModelMetadata(context.Context, modelcatalog.Query) (modelcatalog.Metadata, error)
}

type ConnectionInfo struct {
	URL       string
	ActiveURL string
	Enabled   bool
	HasToken  bool
	Pending   bool
}
type ConnectionInput struct {
	URL           string
	Token         string
	Enabled       bool
	PreserveToken bool
}
type ConnectionSettingsService interface {
	ConnectionInfo(context.Context) (ConnectionInfo, error)
	ConfigureConnection(context.Context, ConnectionInput) error
}
