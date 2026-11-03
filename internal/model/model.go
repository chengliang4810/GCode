// Package model implements the three user-configured model protocols.
package model

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"slices"
	"strings"
	"time"

	"gcode/internal/providerlogo"
)

type Protocol string

const (
	Chat      Protocol = "openai-chat"
	Responses Protocol = "openai-responses"
	Messages  Protocol = "anthropic-messages"
)

type Model struct {
	ID              string   `json:"id"`
	Disabled        bool     `json:"disabled"`
	InputTypes      []string `json:"inputTypes"`
	ReasoningLevels []string `json:"reasoningLevels"`
	ContextWindow   int      `json:"contextWindow"`
	MaxOutput       int      `json:"maxOutput"`
	InputPrice      float64  `json:"inputPrice"`
	OutputPrice     float64  `json:"outputPrice"`
}

// ReasoningLabel returns the display label for a configured reasoning level.
func ReasoningLabel(id string) string {
	switch id {
	case "none":
		return "None"
	case "minimal":
		return "Minimal"
	case "low":
		return "Low"
	case "medium":
		return "Medium"
	case "high":
		return "High"
	case "xhigh":
		return "Extra high"
	case "max":
		return "Max"
	default:
		return id
	}
}

// EffortID converts a display label to its protocol ID.
// Unknown values remain visible to validation instead of silently becoming unset.
func EffortID(label string) string {
	switch value := strings.ToLower(strings.TrimSpace(label)); value {
	case "extra high":
		return "xhigh"
	default:
		return value
	}
}

// ResolveEffort validates an explicit level or selects the last configured level.
// Only a configured none level disables thinking.
func (m Model) ResolveEffort(value string) (string, error) {
	if err := m.validateReasoningLevels(); err != nil {
		return "", err
	}
	effort := EffortID(value)
	if effort == "" {
		if len(m.ReasoningLevels) == 0 {
			return "", nil
		}
		return m.ReasoningLevels[len(m.ReasoningLevels)-1], nil
	}
	if !slices.Contains(m.ReasoningLevels, effort) {
		return "", fmt.Errorf("model %q does not support reasoning level %q", m.ID, effort)
	}
	return effort, nil
}

func (m Model) validateReasoningLevels() error {
	seen := make(map[string]bool, len(m.ReasoningLevels))
	for _, level := range m.ReasoningLevels {
		switch level {
		case "none", "minimal", "low", "medium", "high", "xhigh", "max":
		default:
			return fmt.Errorf("model %q has invalid reasoning level %q", m.ID, level)
		}
		if seen[level] {
			return fmt.Errorf("model %q repeats reasoning level %q", m.ID, level)
		}
		seen[level] = true
	}
	return nil
}

type Provider struct {
	Logo     string   `json:"logo"`
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Disabled bool     `json:"disabled"`
	Protocol Protocol `json:"protocol"`
	BaseURL  string   `json:"baseUrl"`
	APIKey   string   `json:"apiKey,omitempty"`
	Models   []Model  `json:"models"`
}

// CloneProvider freezes the caller-owned model and reasoning-level slices.
func CloneProvider(provider Provider) Provider {
	provider.Models = slices.Clone(provider.Models)
	for index := range provider.Models {
		provider.Models[index].ReasoningLevels = slices.Clone(provider.Models[index].ReasoningLevels)
		provider.Models[index].InputTypes = slices.Clone(provider.Models[index].InputTypes)
	}
	return provider
}

// Validate checks saved configuration, including an unfinished provider draft.
// Execution additionally requires a base URL and an enabled configured model.
func (p Provider) Validate() error {
	if err := providerlogo.Validate(p.Logo); err != nil {
		return err
	}
	return p.validateConfiguration()
}

func (p Provider) validateConfiguration() error {
	if strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.Name) == "" {
		return errors.New("provider ID and name are required")
	}
	if p.Protocol != Chat && p.Protocol != Responses && p.Protocol != Messages {
		return errors.New("choose OpenAI Chat, OpenAI Responses or Anthropic Messages")
	}
	if p.BaseURL != "" {
		u, err := url.Parse(p.BaseURL)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("provider base URL must be an HTTP(S) URL without credentials or query")
		}
	}
	seen := map[string]bool{}
	for _, m := range p.Models {
		if strings.TrimSpace(m.ID) == "" || seen[m.ID] || m.MaxOutput < 0 || m.ContextWindow < 0 {
			return errors.New("model IDs must be nonempty and unique; token limits cannot be negative")
		}
		if m.InputPrice < 0 || m.OutputPrice < 0 || math.IsNaN(m.InputPrice) || math.IsNaN(m.OutputPrice) ||
			math.IsInf(m.InputPrice, 0) || math.IsInf(m.OutputPrice, 0) {
			return errors.New("model token prices must be finite and nonnegative")
		}
		seen[m.ID] = true
		inputs := map[string]bool{}
		for _, input := range m.InputTypes {
			if !slices.Contains([]string{"text", "image", "audio", "video", "pdf"}, input) || inputs[input] {
				return errors.New("model input types must be unique text, image, audio, video or pdf values")
			}
			inputs[input] = true
		}
		if err := m.validateReasoningLevels(); err != nil {
			return err
		}
		if p.Protocol == Messages && slices.Contains(m.ReasoningLevels, "minimal") {
			return errors.New("anthropic messages does not support minimal effort")
		}
	}
	return nil
}

// Ready reports whether the provider has at least one model available to run.
// Credentials may be empty for services that do not require authentication.
func (p Provider) Ready() bool {
	if p.Disabled || p.BaseURL == "" || p.validateConfiguration() != nil {
		return false
	}
	for _, m := range p.Models {
		if !m.Disabled {
			return true
		}
	}
	return false
}

type Call struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Image struct {
	MIME   string `json:"mime"`
	Data   string `json:"data"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}

// ToolOutput preserves multimodal results without encoding them as display text.
type ToolOutput struct {
	Text   string
	Images []Image
	// Data is the typed tool response delivered to hooks. Model adapters use
	// Text and Images; no in-process serialization is required.
	Data any
}

// Message is execution history, not a reconstruction from display summaries.
type Message struct {
	// SourceText retains the accepted user invocation for hook input and skill
	// expansion. Provider adapters only send the prepared Text.
	SourceText string `json:"sourceText,omitempty"`
	// UserInput distinguishes human submissions from host-generated context,
	// goal continuations and subagent notifications. Providers never receive it.
	UserInput bool `json:"userInput,omitempty"`
	// InputIDs track accepted queued inputs through compaction. Adapters do
	// not send these host-owned identifiers to providers.
	InputIDs     []string      `json:"inputIds,omitempty"`
	Role         string        `json:"role"`
	Text         string        `json:"text,omitempty"`
	Reasoning    string        `json:"reasoning,omitempty"`
	Calls        []Call        `json:"calls,omitempty"`
	CallID       string        `json:"callId,omitempty"`
	IsError      bool          `json:"isError,omitempty"`
	Images       []Image       `json:"images,omitempty"`
	Continuation *Continuation `json:"continuation,omitempty"`
}

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type Usage struct {
	// Known requires provider-reported input and output counters; explicit zero
	// counts. Otherwise Input and Output retain only the reported lower bounds.
	Known bool `json:"known"`
	// OutputKnown permits decode-speed accounting even if input was omitted.
	OutputKnown bool `json:"outputKnown"`
	// CacheKnown requires explicitly reported cached input and total input.
	// Missing cache fields are never an observed zero-hit request.
	CacheKnown bool `json:"cacheKnown"`
	// Input includes cached input. Cached is a subset, never added to Input again.
	Input  int `json:"input"`
	Output int `json:"output"`
	Cached int `json:"cached"`
}

type Request struct {
	System  string
	History []Message
	Tools   []Tool
	Effort  string
}

type Delta struct {
	Text      string
	Reasoning string
	// Output also marks tool argument fragments that have no visible text.
	Output bool
}

type Result struct {
	Message Message
	Usage   Usage
	Stop    StopReason
	// DecodeDuration spans the first output delta through protocol completion.
	// Zero means no observed streaming output, never request wall time.
	DecodeDuration time.Duration
}

// StopReason describes execution semantics, independent of provider finish strings.
type StopReason string

const (
	StopComplete StopReason = "complete"
	StopTools    StopReason = "tools"
)

// SystemPromptDynamicBoundary separates the stable part of a system prompt
// from the part that changes per request. Model adapters place a prompt-cache
// breakpoint at this seam; it is removed from the text they send.
const SystemPromptDynamicBoundary = "SYSTEM_PROMPT_DYNAMIC_BOUNDARY"
