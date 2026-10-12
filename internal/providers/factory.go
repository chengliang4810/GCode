package providers

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"gcode/internal/config"
	"gcode/internal/modelregistry"
	"gcode/internal/providercatalog"
	"gcode/internal/providermodelcatalog"
	"gcode/internal/providers/anthropic"
	"gcode/internal/providers/gemini"
	"gcode/internal/providers/openai"
	"gcode/internal/providers/providerio"
	"gcode/internal/zeroruntime"
)

// Options configures provider construction.
type Options struct {
	UserAgent     string
	HTTPClient    *http.Client
	ModelRegistry *modelregistry.Registry
}

// New creates a runtime provider for a resolved provider profile.
func New(profile config.ProviderProfile, options Options) (zeroruntime.Provider, error) {
	resolved, err := resolveProfile(profile, options)
	if err != nil {
		return nil, err
	}

	sessionHeader := sessionHeaderForProfile(profile, resolved)
	if sessionHeader != "" && strings.TrimSpace(options.UserAgent) == "" {
		options.UserAgent = "gcode"
	}

	switch resolved.apiFormat {
	case "responses":
		return openai.NewResponsesProvider(openai.Options{
			APIKey:          profile.APIKey,
			BaseURL:         resolved.baseURL,
			Model:           resolved.apiModel,
			AuthHeader:      profile.AuthHeader,
			AuthScheme:      profile.AuthScheme,
			AuthHeaderValue: profile.AuthHeaderValue,
			CustomHeaders:   providerio.CopyHeaders(profile.CustomHeaders),
			MaxTokens:       resolved.maxOutputTokens,
			HTTPClient:      options.HTTPClient,
			UserAgent:       options.UserAgent,
		})
	case "chat-completions":
		// prompt_cache_key is an OpenAI-only chat-completions field. Strict
		// openai-compatible gateways (NVIDIA NIM, etc.) reject it with a 400
		// instead of ignoring unknown parameters — so omit it for every
		// openai-compatible profile. Official OpenAI keeps the field so
		// multi-turn sessions still route to a cached-prefix replica.
		return openai.New(openai.Options{
			APIKey:                profile.APIKey,
			BaseURL:               resolved.baseURL,
			Model:                 resolved.apiModel,
			AuthHeader:            profile.AuthHeader,
			AuthScheme:            profile.AuthScheme,
			AuthHeaderValue:       profile.AuthHeaderValue,
			CustomHeaders:         providerio.CopyHeaders(profile.CustomHeaders),
			MaxTokens:             resolved.maxOutputTokens,
			HTTPClient:            options.HTTPClient,
			UserAgent:             options.UserAgent,
			ParseThinkTags:        parseThinkTagsForProfile(profile, resolved),
			DisablePromptCacheKey: resolved.providerKind == config.ProviderKindOpenAICompatible,
			SessionHeader:         sessionHeader,
		})
	case "anthropic":
		return anthropic.New(anthropic.Options{
			APIKey:          profile.APIKey,
			BaseURL:         resolved.baseURL,
			Model:           resolved.apiModel,
			AuthHeader:      profile.AuthHeader,
			AuthScheme:      profile.AuthScheme,
			AuthHeaderValue: profile.AuthHeaderValue,
			CustomHeaders:   providerio.CopyHeaders(profile.CustomHeaders),
			MaxTokens:       resolved.maxOutputTokens,
			HTTPClient:      options.HTTPClient,
			UserAgent:       options.UserAgent,
			SessionHeader:   sessionHeader,
		})
	case "gemini":
		return gemini.New(gemini.Options{
			APIKey:          profile.APIKey,
			BaseURL:         resolved.baseURL,
			Model:           resolved.apiModel,
			AuthHeader:      profile.AuthHeader,
			AuthScheme:      profile.AuthScheme,
			AuthHeaderValue: profile.AuthHeaderValue,
			CustomHeaders:   providerio.CopyHeaders(profile.CustomHeaders),
			MaxTokens:       resolved.maxOutputTokens,
			HTTPClient:      options.HTTPClient,
			UserAgent:       options.UserAgent,
		})
	default:
		return nil, fmt.Errorf("unsupported API format %q", resolved.apiFormat)
	}
}

// NewTurnSessionProvider builds the default TurnSessionProvider for a resolved
// profile: it constructs the provider exactly as New does, then wraps it with a
// ProviderCapabilities projection computed from the same model-registry entry
// New already resolves. The default session's Stream is the provider's
// StreamCompletion, so runtime behavior is identical to using New directly.
func NewTurnSessionProvider(profile config.ProviderProfile, options Options) (zeroruntime.TurnSessionProvider, error) {
	provider, err := New(profile, options)
	if err != nil {
		return nil, err
	}
	caps, err := resolveCapabilities(profile, options)
	if err != nil {
		return nil, err
	}
	return zeroruntime.NewProviderTurnSessionProvider(provider, caps), nil
}

// resolveCapabilities projects the resolved profile's model-registry entry into
// the flat zeroruntime.ProviderCapabilities (zeroruntime cannot reference
// modelregistry types — modelregistry imports zeroruntime, so a typed field
// would form an import cycle). A model absent from the registry yields only the
// resolved API model id and max-output tokens; the rest stays zero (unknown).
func resolveCapabilities(profile config.ProviderProfile, options Options) (zeroruntime.ProviderCapabilities, error) {
	resolved, err := resolveProfile(profile, options)
	if err != nil {
		return zeroruntime.ProviderCapabilities{}, err
	}
	caps := zeroruntime.ProviderCapabilities{
		Model:           resolved.apiModel,
		MaxOutputTokens: resolved.maxOutputTokens,
	}
	registry, err := defaultRegistry(options.ModelRegistry)
	if err != nil {
		return zeroruntime.ProviderCapabilities{}, err
	}
	if entry, ok := registry.Get(strings.TrimSpace(profile.Model)); ok {
		caps.ContextWindow = entry.ContextLimits.ContextWindow
		caps.SupportsVision = entry.Supports(modelregistry.ModelCapabilityVision)
		caps.SupportsReasoning = entry.Supports(modelregistry.ModelCapabilityReasoning)
		caps.SupportsPromptCache = entry.Supports(modelregistry.ModelCapabilityPromptCache)
		// Read efforts through Registry.ReasoningEfforts (not the raw entry) so
		// the projection matches what the /effort picker and the run-time
		// resolver advertise — including the name-based fallback for catalog
		// entries that enumerate no efforts of their own.
		for _, effort := range registry.ReasoningEfforts(entry.ID) {
			caps.ReasoningEfforts = append(caps.ReasoningEfforts, string(effort))
		}
	}
	return caps, nil
}

func parseThinkTagsForProfile(profile config.ProviderProfile, resolved resolvedProfile) bool {
	if profile.ParseThinkTags != nil {
		return *profile.ParseThinkTags
	}
	if resolved.providerKind != config.ProviderKindOpenAICompatible {
		return false
	}
	return modelMayEmitThinkTags(resolved.apiModel)
}

func modelMayEmitThinkTags(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	for _, marker := range []string{
		"deepseek-r1",
		"deepseek-reasoner",
		"gpt-oss",
		"glm-z1",
		"kimi-k2-thinking",
		"magistral",
		"minimax-m2",
		"minimax-m3",
		"nemotron",
		"qwen3",
		"qwq",
		"reasoner",
		"reasoning",
		"thinking",
	} {
		if strings.Contains(model, marker) {
			return true
		}
	}
	return false
}

type resolvedProfile struct {
	providerKind    config.ProviderKind
	apiFormat       string
	apiModel        string
	baseURL         string
	maxOutputTokens int
}

// RuntimeMetadata describes the provider identity and concrete API model used
// after GCode model aliases and provider-kind defaults are resolved.
type RuntimeMetadata struct {
	ProviderKind config.ProviderKind
	APIModel     string
}

// ResolveRuntimeMetadata returns the provider kind and API model that New would
// use for a profile, without constructing a network client.
func ResolveRuntimeMetadata(profile config.ProviderProfile, options Options) (RuntimeMetadata, error) {
	resolved, err := resolveProfile(profile, options)
	if err != nil {
		return RuntimeMetadata{}, err
	}
	return RuntimeMetadata{
		ProviderKind: resolved.providerKind,
		APIModel:     resolved.apiModel,
	}, nil
}

func resolveProfile(profile config.ProviderProfile, options Options) (resolvedProfile, error) {
	model := strings.TrimSpace(profile.Model)
	if model == "" {
		return resolvedProfile{}, fmt.Errorf("provider %s requires model", profile.Name)
	}
	providerKind, explicitProvider := explicitProviderKind(profile)
	registry, err := defaultRegistry(options.ModelRegistry)
	if err != nil {
		return resolvedProfile{}, err
	}

	// baseURL resolves in this order: profile override, then catalog default.
	baseURL := strings.TrimSpace(profile.BaseURL)
	if baseURL == "" {
		if descriptor, ok := providercatalog.Get(profile.CatalogID); ok {
			baseURL = strings.TrimSpace(descriptor.DefaultBaseURL)
		}
	}
	if !explicitProvider {
		if inferred, ok := providerKindForAPIFormat(profile.APIFormat, baseURL); ok {
			// An explicit wire format is the user's transport choice. Keep it
			// ahead of model-registry inference so a custom gateway does not
			// accidentally inherit the native provider kind of a known model.
			providerKind = inferred
			explicitProvider = true
		}
	}

	if entry, ok := registry.Get(model); ok {
		modelProvider := configKind(entry.Provider)
		// Adopt the registry entry's provider only when the caller did not pin one.
		// (The old `|| isImplicitOpenAI(...)` clause was dead: explicitProvider==true
		// means ProviderKind or Provider is set, but isImplicitOpenAI required both
		// empty, so it could never add a case.)
		if !explicitProvider {
			providerKind = modelProvider
		}
		if providerKind == config.ProviderKindOpenAICompatible {
			if !entry.AllowsProvider(modelregistry.ProviderOpenAICompatible) {
				return resolvedProfile{}, fmt.Errorf("zero model %s belongs to %s, not %s", entry.ID, entry.Provider, modelregistry.ProviderOpenAICompatible)
			}
		} else if providerKind == config.ProviderKindAnthropicCompat {
			if !entry.AllowsProvider(modelregistry.ProviderAnthropic) {
				return resolvedProfile{}, fmt.Errorf("zero model %s belongs to %s, not %s", entry.ID, entry.Provider, providerKind)
			}
		} else if providerKind != modelProvider {
			return resolvedProfile{}, fmt.Errorf("zero model %s belongs to %s, not %s", entry.ID, entry.Provider, providerKind)
		}
		if err := validateModelAllowedForProvider(profile, entry.ID); err != nil {
			return resolvedProfile{}, err
		}
		apiFormat, err := resolveAPIFormat(profile, providerKind)
		if err != nil {
			return resolvedProfile{}, err
		}
		return resolvedProfile{
			providerKind:    providerKind,
			apiFormat:       apiFormat,
			apiModel:        entry.APIModel,
			baseURL:         baseURL,
			maxOutputTokens: entry.ContextLimits.MaxOutputTokens,
		}, nil
	}

	if providerKind == "" {
		providerKind = config.ProviderKindOpenAI
	}
	if err := validateModelAllowedForProvider(profile, model); err != nil {
		return resolvedProfile{}, err
	}
	apiFormat, err := resolveAPIFormat(profile, providerKind)
	if err != nil {
		return resolvedProfile{}, err
	}
	return resolvedProfile{
		providerKind: providerKind,
		apiFormat:    apiFormat,
		apiModel:     model,
		baseURL:      baseURL,
	}, nil
}

// validateModelAllowedForProvider enforces provider-scoped model allowlists at
// runtime resolution time. It delegates to
// providermodelcatalog.ModelIDAllowedForProvider, which encodes the per-provider
// rules in one place; restricted providers (such as
// opencode-go-anthropic-compatible) only accept a curated subset of model
// families, while the default branch makes unrestricted providers a no-op. This
// guards both New() (TUI /model, headless --model overrides) and
// ResolveRuntimeMetadata (the read-only metadata path) so a catalog-backed
// profile cannot persist or send a model outside its scoped allowlist.
func validateModelAllowedForProvider(profile config.ProviderProfile, model string) error {
	providerID := strings.TrimSpace(profile.CatalogID)
	if providermodelcatalog.ModelIDAllowedForProvider(providerID, model) {
		return nil
	}
	if providerID == "" {
		providerID = "provider"
	}
	return fmt.Errorf("provider %s does not allow model %s", providerID, strings.TrimSpace(model))
}

func explicitProviderKind(profile config.ProviderProfile) (config.ProviderKind, bool) {
	providerKind := config.ProviderKind(strings.TrimSpace(strings.ToLower(string(profile.ProviderKind))))
	if providerKind != "" {
		return providerKind, true
	}
	provider := strings.TrimSpace(strings.ToLower(profile.Provider))
	if provider != "" {
		return config.ProviderKind(provider), true
	}
	return "", false
}

// providerKindForAPIFormat derives a transport kind when a profile supplies the
// user-facing wire format but omits the internal kind. Name is intentionally
// ignored: it is a display/lookup label, not a transport selector.
func providerKindForAPIFormat(apiFormat string, baseURL string) (config.ProviderKind, bool) {
	switch normalizeAPIFormat(apiFormat) {
	case "responses", "chat-completions":
		if isOfficialEndpoint(baseURL, config.OpenAIBaseURL) {
			return config.ProviderKindOpenAI, true
		}
		return config.ProviderKindOpenAICompatible, true
	case "anthropic":
		if isOfficialEndpoint(baseURL, config.AnthropicBaseURL) {
			return config.ProviderKindAnthropic, true
		}
		return config.ProviderKindAnthropicCompat, true
	case "gemini":
		return config.ProviderKindGoogle, true
	default:
		return "", false
	}
}

func isOfficialEndpoint(rawURL string, officialURL string) bool {
	normalized := strings.TrimRight(strings.TrimSpace(rawURL), "/")
	official := strings.TrimRight(strings.TrimSpace(officialURL), "/")
	if normalized == "" {
		return true
	}
	if normalized == "openai" {
		return strings.EqualFold(official, strings.TrimRight(config.OpenAIBaseURL, "/"))
	}
	if strings.EqualFold(normalized, official) {
		return true
	}
	parsed, err := url.Parse(normalized)
	if err != nil || parsed.Hostname() == "" {
		return false
	}
	officialParsed, err := url.Parse(official)
	if err != nil || officialParsed.Hostname() == "" {
		return false
	}
	return strings.EqualFold(parsed.Hostname(), officialParsed.Hostname())
}

func configKind(provider modelregistry.ProviderKind) config.ProviderKind {
	return config.ProviderKind(provider)
}

func defaultRegistry(registry *modelregistry.Registry) (modelregistry.Registry, error) {
	if registry != nil {
		return *registry, nil
	}
	return modelregistry.DefaultRegistry()
}

func resolveAPIFormat(profile config.ProviderProfile, providerKind config.ProviderKind) (string, error) {
	format := normalizeAPIFormat(profile.APIFormat)
	if strings.TrimSpace(profile.APIFormat) != "" && format == "" {
		return "", fmt.Errorf("unsupported API format %q", profile.APIFormat)
	}
	if format == "" {
		switch providerKind {
		case config.ProviderKindOpenAI, config.ProviderKindOpenAICompatible:
			format = "chat-completions"
		case config.ProviderKindAnthropic, config.ProviderKindAnthropicCompat:
			format = "anthropic"
		case config.ProviderKindGoogle:
			format = "gemini"
		default:
			return "", fmt.Errorf("unsupported provider kind %q", providerKind)
		}
	}
	switch format {
	case "responses", "chat-completions":
		if providerKind != config.ProviderKindOpenAI && providerKind != config.ProviderKindOpenAICompatible {
			return "", fmt.Errorf("API format %q is incompatible with provider kind %q", format, providerKind)
		}
	case "anthropic":
		if providerKind != config.ProviderKindAnthropic && providerKind != config.ProviderKindAnthropicCompat {
			return "", fmt.Errorf("API format %q is incompatible with provider kind %q", format, providerKind)
		}
	case "gemini":
		if providerKind != config.ProviderKindGoogle {
			return "", fmt.Errorf("API format %q is incompatible with provider kind %q", format, providerKind)
		}
	}
	return format, nil
}

func normalizeAPIFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "responses":
		return "responses"
	case "chat-completions", "chat_completions", "chat-completion":
		return "chat-completions"
	case "anthropic", "messages", "anthropic-messages":
		return "anthropic"
	case "gemini", "generate-content", "generate_content", "google-generate-content":
		return "gemini"
	default:
		return ""
	}
}

func sessionHeaderForProfile(profile config.ProviderProfile, resolved resolvedProfile) string {
	switch providercatalog.NormalizeID(profile.CatalogID) {
	case "opencode", "opencode-go", "opencode-go-anthropic-compatible":
		return "x-opencode-session"
	}
	endpoint, err := url.Parse(resolved.baseURL)
	if err == nil && strings.EqualFold(endpoint.Hostname(), "opencode.ai") && (endpoint.Path == "/zen" || strings.HasPrefix(endpoint.Path, "/zen/")) {
		return "x-opencode-session"
	}
	return ""
}
