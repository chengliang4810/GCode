// Package acceptance records plugin acceptance without treating static evidence as runtime success.
package acceptance

import (
	"context"
	"encoding/json"
	"time"

	"gcode/internal/model"
	"gcode/internal/plugins"
)

const (
	OfficialCommit         = "f8c5e7a1dcb71b10cb74586224b5401f3de5db28"
	OfficialRepository     = "anthropics/claude-plugins-official"
	OfficialMarketplace    = "claude-plugins-official"
	OfficialManifestSHA256 = "1150acdc77c88c087e09fbec563eb6b052e4ee2044082ab9dc60698d5fb023e5"
	DefaultMarketplaceFile = "docs/investigations/claude-plugins-2026-10-30/snapshots/anthropics--claude-plugins-official--" + OfficialCommit + "--marketplace.json"
)

type Config struct {
	MarketplaceFile         string
	MarketplaceCommit       string
	MarketplaceRepository   string
	SourceEvidenceDir       string
	SourceRoot              string
	Cache                   string
	OutputDir               string
	Platform                string
	Model                   string
	Workers                 int
	Timeout                 time.Duration
	InstallAndDiscover      bool
	Run                     bool
	ProviderConfig          string
	ProbeDir                string
	RuntimeEvidence         []string
	RequireOfficialBaseline bool
}

type Stage struct {
	Status   string   `json:"status"`
	Method   string   `json:"method,omitempty"`
	Evidence []string `json:"evidence,omitempty"`
	Error    string   `json:"error,omitempty"`
}

type FixedSource struct {
	Repository         string `json:"repository,omitempty"`
	Commit             string `json:"commit,omitempty"`
	Path               string `json:"path,omitempty"`
	ArchiveSHA256      string `json:"archiveSha256,omitempty"`
	ManifestTextSHA256 string `json:"manifestTextSha256,omitempty"`
}

type Component struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Digest string `json:"digest"`
	Status string `json:"status"`
}

type Row struct {
	ID                string      `json:"id"`
	Name              string      `json:"name"`
	Marketplace       string      `json:"marketplace"`
	MarketplaceCommit string      `json:"marketplaceCommit"`
	EntrySHA256       string      `json:"entrySha256"`
	FixedSource       FixedSource `json:"fixedSource"`
	Version           string      `json:"version,omitempty"`
	Platform          string      `json:"platform"`
	Model             string      `json:"model"`
	Source            Stage       `json:"source"`
	Install           Stage       `json:"install"`
	Discover          Stage       `json:"discover"`
	Runtime           Stage       `json:"runtime"`
	Components        []Component `json:"components"`
	Probes            []Probe     `json:"probes"`
	Unavailable       bool        `json:"unavailable"`
	Blocked           []string    `json:"blocked"`
	Evidence          []string    `json:"evidence"`
	Errors            []string    `json:"errors"`
	ArtifactDigest    string      `json:"artifactDigest,omitempty"`
	ArtifactRoot      string      `json:"artifactRoot,omitempty"`
	Passed            bool        `json:"passed"`
	entry             plugins.MarketplaceEntry
}

type Report struct {
	SchemaVersion         int       `json:"schemaVersion"`
	RunID                 string    `json:"runId"`
	Directory             string    `json:"directory"`
	CreatedAt             time.Time `json:"createdAt"`
	Marketplace           string    `json:"marketplace"`
	MarketplaceRepository string    `json:"marketplaceRepository"`
	MarketplaceCommit     string    `json:"marketplaceCommit"`
	MarketplaceSHA256     string    `json:"marketplaceSha256"`
	OfficialBaseline      bool      `json:"officialBaseline"`
	Platform              string    `json:"platform"`
	Model                 string    `json:"model"`
	Denominator           int       `json:"denominator"`
	RequiredPasses        int       `json:"requiredPasses"`
	Passed                int       `json:"passed"`
	Percent               float64   `json:"percent"`
	TargetMet             bool      `json:"targetMet"`
	Rows                  []Row     `json:"rows"`
}

type CachedSource struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
	Root       string `json:"root"`
}

type CacheIndex struct {
	Sources []CachedSource `json:"sources"`
}

type ProviderConfig struct {
	Provider      model.Provider               `json:"provider"`
	ModelID       string                       `json:"modelId"`
	Reasoning     string                       `json:"reasoning,omitempty"`
	PluginConfigs map[string]map[string]string `json:"pluginConfigs,omitempty"`
}

// RuntimeRunner must invoke the real core in the supplied isolated directory.
// It returns per-component trace references, never a plugin-wide success boolean.
type RuntimeRunner interface {
	Run(context.Context, RunRequest) (RuntimeEvidence, error)
}

type RunRequest struct {
	RunID             string
	PluginID          string
	MarketplaceCommit string
	SourceCommit      string
	ArtifactDigest    string
	Platform          string
	Model             string
	Bundle            plugins.Bundle
	Components        []Component
	Workspace         string
	EvidenceDir       string
	Provider          ProviderConfig
	ProbeDir          string
	EntryManifest     plugins.Manifest
	EntrySource       plugins.Source
}

type EvidenceFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type Probe struct {
	ID              string       `json:"id"`
	ComponentID     string       `json:"componentId"`
	ComponentDigest string       `json:"componentDigest"`
	Operation       string       `json:"operation"`
	Outcome         string       `json:"outcome"`
	StartedAt       time.Time    `json:"startedAt"`
	FinishedAt      time.Time    `json:"finishedAt"`
	Log             EvidenceFile `json:"log"`
	Error           string       `json:"error,omitempty"`
}

type RuntimeEvidence struct {
	SchemaVersion     int     `json:"schemaVersion"`
	RunID             string  `json:"runId"`
	PluginID          string  `json:"pluginId"`
	MarketplaceCommit string  `json:"marketplaceCommit"`
	SourceCommit      string  `json:"sourceCommit"`
	ArtifactDigest    string  `json:"artifactDigest"`
	Platform          string  `json:"platform"`
	Model             string  `json:"model"`
	Probes            []Probe `json:"probes"`
}

// TraceRecord is a JSONL record emitted at the actual component invocation boundary.
// An accepted success needs matching invoke/result records and nontrivial structured
// input/output; a status boolean or a manifest discovery event is insufficient.
type TraceRecord struct {
	RunID             string          `json:"runId"`
	PluginID          string          `json:"pluginId"`
	MarketplaceCommit string          `json:"marketplaceCommit"`
	SourceCommit      string          `json:"sourceCommit"`
	ArtifactDigest    string          `json:"artifactDigest"`
	Platform          string          `json:"platform"`
	Model             string          `json:"model"`
	ProbeID           string          `json:"probeId"`
	ComponentID       string          `json:"componentId"`
	ComponentDigest   string          `json:"componentDigest"`
	Operation         string          `json:"operation"`
	Phase             string          `json:"phase"`
	At                time.Time       `json:"at"`
	Payload           json.RawMessage `json:"payload"`
	Error             string          `json:"error,omitempty"`
}
