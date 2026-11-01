package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"gcode/internal/contracts"
	"gcode/internal/plugins"
)

const maxPluginConfigValue = 64 << 10

var pluginOptionIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// The native input contract uses strings. String/path values stay opaque;
// booleans/numbers and multiple lists have explicit JSON representations. These
// routines never expand templates, execute commands, or inspect a host path.
func validatePluginOption(key string, option plugins.UserConfigOption) error {
	if !pluginOptionIdentifier.MatchString(key) {
		return errors.New("userConfig keys must be identifiers without a leading digit")
	}
	switch option.Type {
	case "string", "number", "boolean", "directory", "file":
	default:
		return fmt.Errorf("userConfig %q has an unsupported type", key)
	}
	if option.Multiple && option.Type != "string" {
		return fmt.Errorf("userConfig %q: multiple is only supported for string fields", key)
	}
	if option.Type != "number" && (option.Min != nil || option.Max != nil) {
		return fmt.Errorf("userConfig %q: min/max only apply to number fields", key)
	}
	for _, bound := range []*float64{option.Min, option.Max} {
		if bound != nil && (math.IsNaN(*bound) || math.IsInf(*bound, 0)) {
			return fmt.Errorf("userConfig %q has a non-finite number bound", key)
		}
	}
	if option.Min != nil && option.Max != nil && *option.Min > *option.Max {
		return fmt.Errorf("userConfig %q: min exceeds max", key)
	}
	if option.Options != nil {
		if option.Type != "string" || option.Multiple || option.Sensitive {
			return fmt.Errorf("userConfig %q: options require a non-sensitive, single string field", key)
		}
		if len(option.Options) == 0 {
			return fmt.Errorf("userConfig %q: options must not be empty", key)
		}
		seen := map[string]bool{}
		for _, label := range option.Options {
			if !utf8.ValidString(label) || utf8.RuneCountInString(label) < 1 || utf8.RuneCountInString(label) > 64 || strings.TrimSpace(label) != label {
				return fmt.Errorf("userConfig %q: option labels must contain 1 to 64 plain characters", key)
			}
			for _, r := range label {
				if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
					return fmt.Errorf("userConfig %q: option labels must not contain control or formatting characters", key)
				}
			}
			if seen[label] {
				return fmt.Errorf("userConfig %q: option labels must be unique", key)
			}
			seen[label] = true
		}
		if option.Default == nil && !option.Required {
			return fmt.Errorf("userConfig %q: options require a valid default or required: true", key)
		}
	}
	if option.Default != nil {
		if _, err := pluginDefaultValue(key, option); err != nil {
			return err
		}
	}
	return nil
}

func normalizePluginConfigValue(key string, option plugins.UserConfigOption, raw string) (string, error) {
	fail := func(reason string) (string, error) { return "", fmt.Errorf("plugin configuration %q: %s", key, reason) }
	if len(raw) > maxPluginConfigValue {
		return fail("value exceeds 64 KiB")
	}
	switch option.Type {
	case "string":
		if !option.Multiple {
			if option.Options != nil && !slices.Contains(option.Options, raw) {
				return fail("value must be one of the declared options")
			}
			return raw, nil
		}
		value, err := decodePluginJSON(raw)
		if err != nil {
			return fail("multiple values must be a JSON array of strings")
		}
		list, ok := value.([]any)
		if !ok {
			return fail("multiple values must be a JSON array of strings")
		}
		for _, item := range list {
			if _, ok := item.(string); !ok {
				return fail("multiple values must contain only strings")
			}
		}
		encoded, err := json.Marshal(list)
		if len(encoded) > maxPluginConfigValue {
			return fail("encoded multiple values exceed 64 KiB")
		}
		if err != nil {
			return fail("multiple values could not be encoded")
		}
		return string(encoded), nil
	case "directory", "file":
		return raw, nil
	case "boolean":
		value, err := decodePluginJSON(raw)
		if err != nil {
			return fail("value must be JSON true or false")
		}
		boolean, ok := value.(bool)
		if !ok {
			return fail("value must be JSON true or false")
		}
		return strconv.FormatBool(boolean), nil
	case "number":
		value, err := decodePluginJSON(raw)
		if err != nil {
			return fail("value must be a finite JSON number")
		}
		number, ok := value.(json.Number)
		if !ok {
			return fail("value must be a finite JSON number")
		}
		n, err := strconv.ParseFloat(number.String(), 64)
		if err != nil || math.IsInf(n, 0) || math.IsNaN(n) {
			return fail("value must be a finite JSON number")
		}
		if option.Min != nil && n < *option.Min {
			return fail("number is below its declared minimum")
		}
		if option.Max != nil && n > *option.Max {
			return fail("number exceeds its declared maximum")
		}
		return number.String(), nil
	default:
		return fail("field type is unsupported")
	}
}
func decodePluginJSON(raw string) (any, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, errors.New("invalid JSON value")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, errors.New("multiple JSON values are not allowed")
	}
	return value, nil
}
func pluginDefaultValue(key string, option plugins.UserConfigOption) (string, error) {
	if option.Default == nil {
		return "", nil
	}
	var raw string
	switch option.Type {
	case "string", "directory", "file":
		if value, ok := option.Default.(string); ok {
			if option.Multiple && value != "" {
				encoded, _ := json.Marshal([]string{value})
				raw = string(encoded)
			} else {
				raw = value
			}
		} else if option.Type == "string" && option.Multiple {
			encoded, err := json.Marshal(option.Default)
			if err != nil {
				return "", fmt.Errorf("userConfig %q has an invalid default", key)
			}
			raw = string(encoded)
		} else {
			return "", fmt.Errorf("userConfig %q default must be a string", key)
		}
	case "number", "boolean":
		encoded, err := json.Marshal(option.Default)
		if err != nil {
			return "", fmt.Errorf("userConfig %q has an invalid default", key)
		}
		raw = string(encoded)
	default:
		return "", fmt.Errorf("userConfig %q default has an unsupported type", key)
	}
	// An empty string means no usable value, even when the schema declares it.
	// It can still be supplied later for a required field.
	if raw == "" {
		if option.Options != nil {
			return "", fmt.Errorf("userConfig %q default must be a declared option", key)
		}
		return "", nil
	}
	normalized, err := normalizePluginConfigValue(key, option, raw)
	if err != nil {
		return "", fmt.Errorf("userConfig %q has an invalid default: %w", key, err)
	}
	return normalized, nil
}
func effectivePluginConfig(schema map[string]plugins.UserConfigOption, saved map[string]string) (map[string]string, error) {
	values := map[string]string{}
	for _, key := range slices.Sorted(maps.Keys(schema)) {
		option := schema[key]
		if err := validatePluginOption(key, option); err != nil {
			return nil, err
		}
		value, err := pluginDefaultValue(key, option)
		if err != nil {
			return nil, err
		}
		if stored, exists := saved[key]; exists && stored != "" {
			value, err = normalizePluginConfigValue(key, option, stored)
			if err != nil {
				return nil, err
			}
		}
		if value != "" {
			values[key] = value
		}
	}
	for key := range saved {
		if _, exists := schema[key]; !exists {
			return nil, fmt.Errorf("plugin configuration key %q is not declared", key)
		}
	}
	return values, nil
}
func updatePluginConfig(schema map[string]plugins.UserConfigOption, saved, updates map[string]string, removals []string) (map[string]string, error) {
	next := maps.Clone(saved)
	if next == nil {
		next = map[string]string{}
	}
	removed := map[string]bool{}
	for _, key := range removals {
		if _, exists := schema[key]; !exists {
			return nil, fmt.Errorf("plugin configuration key %q is not declared", key)
		}
		if _, exists := updates[key]; exists {
			return nil, fmt.Errorf("plugin configuration %q cannot be removed and set together", key)
		}
		removed[key] = true
		delete(next, key)
	}
	for _, key := range slices.Sorted(maps.Keys(updates)) {
		option, exists := schema[key]
		if !exists {
			return nil, fmt.Errorf("plugin configuration key %q is not declared", key)
		}
		if err := validatePluginOption(key, option); err != nil {
			return nil, err
		}
		raw := updates[key]
		if raw == "" {
			continue
		} // Blank native fields preserve saved values.
		value, err := normalizePluginConfigValue(key, option, raw)
		if err != nil {
			return nil, err
		}
		next[key] = value
	}
	effective, err := effectivePluginConfig(schema, next)
	if err != nil {
		return nil, err
	}
	for _, key := range slices.Sorted(maps.Keys(schema)) {
		if schema[key].Required && effective[key] == "" && !removed[key] {
			return nil, fmt.Errorf("plugin configuration %q is required", key)
		}
	}
	return next, nil
}

func publicPluginConfigField(key string, option plugins.UserConfigOption) (contracts.PluginConfigField, error) {
	field := contracts.PluginConfigField{Key: key, Type: option.Type, Title: option.Title, Description: option.Description, Required: option.Required, Sensitive: option.Sensitive, Multiple: option.Multiple, Options: slices.Clone(option.Options), Min: copyPluginBound(option.Min), Max: copyPluginBound(option.Max)}
	if err := validatePluginOption(key, option); err != nil {
		return field, err
	}
	value, err := pluginDefaultValue(key, option)
	if err != nil {
		return field, err
	}
	field.DefaultConfigured = value != ""
	field.Configured = field.DefaultConfigured
	return field, nil
}
func copyPluginBound(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
func pluginConfigOptionFromField(field contracts.PluginConfigField) plugins.UserConfigOption {
	return plugins.UserConfigOption{Type: field.Type, Required: field.Required, Sensitive: field.Sensitive, Multiple: field.Multiple, Min: field.Min, Max: field.Max, Options: field.Options}
}

// pluginConfigSecrets includes list members, not just the enclosing JSON text.
// Callers use it at output boundaries; it never expands a saved value.
func pluginConfigSecrets(schema map[string]plugins.UserConfigOption, values map[string]string) []string {
	var secrets []string
	for key, option := range schema {
		if !option.Sensitive {
			continue
		}

		candidates := []string{values[key]}
		if fallback, err := pluginDefaultValue(key, option); err == nil && fallback != values[key] {
			candidates = append(candidates, fallback)
		}
		for _, value := range candidates {
			if value == "" {
				continue
			}
			secrets = append(secrets, value)
			if option.Multiple {
				decoded, err := decodePluginJSON(value)
				if err == nil {
					if list, ok := decoded.([]any); ok {
						for _, item := range list {
							if text, ok := item.(string); ok && text != "" {
								secrets = append(secrets, text)
							}
						}
					}
				}
			}
		}
	}
	var variants []string
	for _, secret := range secrets {
		variants = append(variants, secret, url.QueryEscape(secret), url.PathEscape(secret))
		if encoded, err := json.Marshal(secret); err == nil && len(encoded) > 2 {
			variants = append(variants, string(encoded[1:len(encoded)-1]))
		}
	}
	slices.SortFunc(variants, func(a, b string) int {
		if len(a) != len(b) {
			return len(b) - len(a)
		}
		return strings.Compare(a, b)
	})
	return slices.Compact(variants)
}
func redactPluginConfigText(text string, secrets []string) string {
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		parts := strings.Split(text, "[REDACTED]")
		for i := range parts {
			parts[i] = strings.ReplaceAll(parts[i], secret, "[REDACTED]")
		}
		text = strings.Join(parts, "[REDACTED]")
	}
	return text
}
func redactPluginConfigProjection(item *contracts.InstalledPlugin, secrets []string) {
	for i := range item.Config {
		field := &item.Config[i]
		field.Title = redactPluginConfigText(field.Title, secrets)
		field.Description = redactPluginConfigText(field.Description, secrets)
		for j := range field.Options {
			field.Options[j] = redactPluginConfigText(field.Options[j], secrets)
		}
	}
	for i := range item.Issues {
		item.Issues[i] = redactPluginConfigText(item.Issues[i], secrets)
	}
}
