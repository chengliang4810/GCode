package mcp

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestConnectStdioFailureIncludesSanitizedStderr(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	const secret = "startup-sensitive/token?value"
	_, err = Connect(t.Context(), Config{
		Name: "failed-stdio", Command: executable, Args: []string{"-test.run=^TestMCPStderrHelper$"},
		Env: map[string]string{"GCODE_TEST_MCP_STDERR": "enabled", "MCP_API_KEY": secret},
	}, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "missing dependency") || !strings.Contains(err.Error(), "MCP stderr:") {
		t.Fatalf("missing child diagnostics: %v", err)
	}
	for _, value := range []string{secret, url.QueryEscape(secret), url.PathEscape(secret)} {
		if strings.Contains(err.Error(), value) {
			t.Fatalf("child diagnostics exposed a credential: %v", err)
		}
	}
}

func TestMCPStderrHelper(t *testing.T) {
	if os.Getenv("GCODE_TEST_MCP_STDERR") != "enabled" {
		return
	}
	secret := os.Getenv("MCP_API_KEY")
	// Split writes must not defeat sanitization.
	fmt.Fprint(os.Stderr, "missing dependency; credential="+secret[:8])
	fmt.Fprint(os.Stderr, secret[8:]+" escaped="+url.QueryEscape(secret)+"\n")
	os.Exit(3)
}

func TestStderrCaptureBoundsAndRedactsSplitCredential(t *testing.T) {
	const secret = "/sensitive startup?credential"
	for _, value := range []string{secret, url.QueryEscape(secret), url.PathEscape(secret)} {
		t.Run(value, func(t *testing.T) {
			config := Config{SensitiveValues: []string{secret}}
			capture := &stderrCapture{}
			prefix := "missing dependency\n"
			filler := strings.Repeat(".", stderrCaptureLimit-len(prefix)-8)
			_, _ = capture.Write([]byte(prefix + filler + value))
			_, _ = capture.Write([]byte(strings.Repeat("ignored", stderrCaptureLimit)))
			if len(capture.data) > stderrCaptureLimit {
				t.Fatal("child stderr exceeded its memory bound")
			}
			c := &Client{config: config, stderr: capture}
			cause := errors.New("connection closed")
			err := c.sanitizeError(c.withStderr(cause))
			if !errors.Is(err, cause) || !strings.Contains(err.Error(), "missing dependency") ||
				!strings.Contains(err.Error(), "truncated") || strings.Contains(err.Error(), value[:8]) {
				t.Fatalf("unsafe or missing bounded diagnostics: %v", err)
			}
			if second := c.withStderr(err); second != err {
				t.Fatal("nested connection errors duplicated child diagnostics")
			}
		})
	}
}
