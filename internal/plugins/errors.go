package plugins

import (
	"net/url"
	"regexp"
	"strings"
)

var sourceURLPattern = regexp.MustCompile(`(?:https?|ssh|file)://[^\s"'<>]+`)

type sourceError struct {
	text  string
	cause error
}

func (e sourceError) Error() string { return e.text }
func (e sourceError) Unwrap() error { return e.cause }

// Network command failures are user-visible. Preserve errors.Is while removing
// source credentials before an operation error can enter application state.
func redactSourceError(err error, addresses []string, headers map[string]string) error {
	if err == nil {
		return nil
	}
	text := err.Error()
	secrets := []string{}
	cleanURL := func(address string) string {
		parsed, parseErr := url.Parse(address)
		if parseErr != nil {
			return "[redacted-source]"
		}
		if parsed.User != nil {
			secrets = append(secrets, parsed.User.Username())
			if password, ok := parsed.User.Password(); ok {
				secrets = append(secrets, password)
			}
		}
		for _, values := range parsed.Query() {
			secrets = append(secrets, values...)
		}
		secrets = append(secrets, parsed.Fragment)
		parsed.User, parsed.RawQuery, parsed.Fragment, parsed.RawFragment = nil, "", "", ""
		parsed.ForceQuery = false
		return parsed.String()
	}
	for _, address := range addresses {
		if address != "" && strings.Contains(address, "://") {
			text = strings.ReplaceAll(text, address, cleanURL(address))
		}
	}
	text = sourceURLPattern.ReplaceAllStringFunc(text, cleanURL)
	for _, value := range headers {
		secrets = append(secrets, value)
		if _, credential, ok := strings.Cut(value, " "); ok {
			secrets = append(secrets, credential)
		}
	}
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		for _, form := range []string{secret, url.QueryEscape(secret), url.PathEscape(secret)} {
			text = strings.ReplaceAll(text, form, "[redacted]")
		}
	}
	return sourceError{text: text, cause: err}
}
