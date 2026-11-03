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

// ProviderKeyService reads a saved credential only on an explicit reveal action.
// Keys must not enter snapshots or broadcast events.
type ProviderKeyService interface {
	ReadProviderKey(context.Context, string) (string, error)
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
