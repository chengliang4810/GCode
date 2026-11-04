package model

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

const maxProviderErrorBytes = 64 << 10

// ProviderError contains provider diagnostics safe to publish to the UI and
// persist in a trajectory. Construct it only at the credential-owning boundary.
type ProviderError struct {
	HTTPStatus int
	operation  string
	detail     string
	truncated  bool
	cause      error
	retryAfter *time.Duration
}

func (e *ProviderError) Unwrap() error { return e.cause }

func (e *ProviderError) Error() string {
	message := e.operation
	if e.HTTPStatus != 0 {
		message += fmt.Sprintf(" (HTTP %d)", e.HTTPStatus)
	}
	if e.detail != "" {
		message += "\n" + e.detail
	}
	if e.truncated {
		message += "\n[provider error truncated]"
	}
	return message
}

// streamProviderError remains private until streamAttempt redacts it using the
// frozen provider credential. Decoders never publish or persist errors directly.
type streamProviderError struct{ payload string }

func (e *streamProviderError) Error() string { return "model reported a streaming error" }

func providerHTTPError(response *http.Response, operation, secret string) error {
	// Read beyond the visible limit so a credential crossing the truncation
	// boundary can still be fully redacted before slicing the displayed text.
	lookahead := 1
	for _, value := range credentialRepresentations(secret) {
		lookahead = max(lookahead, len(value)+1)
	}
	readLimit := maxProviderErrorBytes + lookahead
	body, readErr := io.ReadAll(io.LimitReader(response.Body, int64(readLimit)))
	detail, truncated := providerErrorDetail(string(body), secret)
	truncated = truncated || len(body) == readLimit
	if readErr != nil {
		detail += "\n[provider error body could not be fully read]"
	}
	return &ProviderError{HTTPStatus: response.StatusCode, operation: operation, detail: detail, truncated: truncated,
		retryAfter: parseRetryAfter(response.Header.Get("Retry-After"), time.Now())}
}

func providerConnectionError(operation string, err error, secret string) error {
	detail, truncated := providerErrorDetail(err.Error(), secret)
	return &ProviderError{operation: operation, detail: detail, truncated: truncated, cause: err}
}

func credentialRepresentations(secret string) []string {
	if secret == "" {
		return nil
	}
	encoded, _ := json.Marshal(secret)
	return []string{secret, string(encoded[1 : len(encoded)-1]), url.QueryEscape(secret)}
}

func providerErrorDetail(detail, secret string) (string, bool) {
	// Preserve body formatting, including JSON, while handling the common
	// escaped representations of a reflected request credential.
	for _, value := range credentialRepresentations(secret) {
		detail = strings.ReplaceAll(detail, value, "[redacted]")
	}
	truncated := len(detail) > maxProviderErrorBytes
	if truncated {
		detail = detail[:maxProviderErrorBytes]
		for !utf8.ValidString(detail) {
			detail = detail[:len(detail)-1]
		}
	}
	return strings.TrimSpace(detail), truncated
}
