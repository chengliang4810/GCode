package mcp

import (
	"net/url"
	"slices"
	"strings"
)

type sanitizedError struct {
	cause error
	text  string
}

func (e sanitizedError) Error() string { return e.text }
func (e sanitizedError) Unwrap() error { return e.cause }

func sanitizeError(err error, config Config) error {
	if err == nil {
		return nil
	}
	return sanitizedError{cause: err, text: SanitizeText(err.Error(), config)}
}

// SanitizeText removes configured credentials before diagnostics leave the MCP
// boundary. It also covers URL query values repeated in HTTP/client errors.
func SanitizeText(text string, config Config) string {
	secrets := append(argumentSecrets(config.Args), config.SensitiveValues...)
	for _, values := range []map[string]string{config.Env, config.Headers} {
		for _, value := range values {
			if value != "" {
				secrets = append(secrets, value)
				if _, token, found := strings.Cut(value, " "); found && token != "" {
					secrets = append(secrets, token)
				}
			}
		}
	}
	if parsed, err := url.Parse(config.URL); err == nil {
		for _, values := range parsed.Query() {
			secrets = append(secrets, values...)
		}
		if parsed.User != nil {
			secrets = append(secrets, parsed.User.Username())
			if password, present := parsed.User.Password(); present {
				secrets = append(secrets, password)
			}
		}
	}
	// Replace long values first so a short value cannot leave a suffix exposed.
	slices.SortFunc(secrets, func(a, b string) int { return len(b) - len(a) })
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		for _, encoded := range []string{secret, url.QueryEscape(secret), url.PathEscape(secret)} {
			text = strings.ReplaceAll(text, encoded, "[redacted]")
		}
	}
	return text
}

func schemaContainsCredentials(value any, config Config) bool {
	switch value := value.(type) {
	case string:
		return SanitizeText(value, config) != value
	case map[string]any:
		for key, child := range value {
			if SanitizeText(key, config) != key || schemaContainsCredentials(child, config) {
				return true
			}
		}
	case []any:
		for _, child := range value {
			if schemaContainsCredentials(child, config) {
				return true
			}
		}
	}
	return false
}

// Credential-bearing flags are explicit configuration too. Ordinary arguments
// (paths, package names, transport flags) are not treated as passwords.
func argumentSecrets(args []string) []string {
	var secrets []string
	for i, arg := range args {
		flag, value, inline := strings.Cut(arg, "=")
		if !strings.HasPrefix(flag, "-") {
			continue
		}
		flag = strings.ToLower(flag)
		sensitive := false
		for _, part := range []string{"token", "secret", "password", "api-key", "api_key", "apikey", "access-key", "authorization"} {
			if strings.Contains(flag, part) {
				sensitive = true
				break
			}
		}
		if !sensitive {
			continue
		}
		if inline {
			secrets = append(secrets, value)
		} else if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			secrets = append(secrets, args[i+1])
		}
	}
	return secrets
}
