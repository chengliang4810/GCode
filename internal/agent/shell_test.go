package agent

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunShellSpillsCompleteStreamBeyondMemoryLimit(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "complete-output.txt")
	opened := 0
	output, err := RunShell(t.Context(), directory, "bash",
		`for ((i=0;i<12000;i++)); do printf '0123456789\n'; done; printf 'TAIL-END'`,
		nil, 30*time.Second, func() (io.WriteCloser, string, error) {
			opened++
			file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			return file, path, err
		})
	if err != nil || opened != 1 || output.OutputFile != path || len(output.Text) > 64100 {
		t.Fatalf("stream output was not safely bounded: %v opened=%d path=%s bytes=%d", err, opened, output.OutputFile, len(output.Text))
	}
	full, err := os.ReadFile(path)
	if err != nil || string(full) != strings.Repeat("0123456789\n", 12000)+"TAIL-END" {
		t.Fatalf("stream output was truncated or duplicated: %v bytes=%d", err, len(full))
	}
}
