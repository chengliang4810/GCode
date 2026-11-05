package mcp

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
)

const stderrCaptureLimit = 16 << 10

// stderrCapture never blocks a child's stderr on a consumer or retains
// unbounded logs. Keep the first diagnostics, where startup failures appear.
type stderrCapture struct {
	mu        sync.Mutex
	data      []byte
	truncated bool
}

func (b *stderrCapture) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	count := min(len(data), stderrCaptureLimit-len(b.data))
	b.data = append(b.data, data[:count]...)
	b.truncated = b.truncated || count < len(data)
	return len(data), nil
}

type stderrDiagnosticError struct {
	cause      error
	diagnostic string
}

func (e *stderrDiagnosticError) Error() string {
	return fmt.Sprintf("%v\nMCP stderr: %s", e.cause, e.diagnostic)
}
func (e *stderrDiagnosticError) Unwrap() error { return e.cause }

func (c *Client) withStderr(err error) error {
	if err == nil || c.stderr == nil {
		return err
	}
	var existing *stderrDiagnosticError
	if errors.As(err, &existing) {
		return err
	}
	c.stderr.mu.Lock()
	text, truncated := string(c.stderr.data), c.stderr.truncated
	c.stderr.mu.Unlock()
	text = c.scrub(text)
	// A capture boundary or a concurrent child write may split a credential.
	// Remove any remaining credential prefix at the end as well as full values.
	secrets := configuredSecrets(c.config)
	c.mu.Lock()
	secrets = append(secrets, c.secrets...)
	c.mu.Unlock()
	for _, secret := range secrets {
		for _, encoded := range []string{secret, url.QueryEscape(secret), url.PathEscape(secret)} {
			for size := min(len(encoded)-1, len(text)); size > 0; size-- {
				if strings.HasSuffix(text, encoded[:size]) {
					text = text[:len(text)-size] + "[redacted]"
					break
				}
			}
		}
	}
	text = strings.TrimSpace(text)
	if truncated {
		text += "\n[stderr capture truncated at 16 KiB]"
	}
	if text == "" {
		return err
	}
	return &stderrDiagnosticError{cause: err, diagnostic: text}
}
