package terminalhost

import (
	"bytes"
	"testing"
	"time"

	"gcode/internal/testenv"
)

func TestMain(m *testing.M) { testenv.Main(m) }

func waitShellReady(t *testing.T, host *Session) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if bytes.Contains(host.History(), []byte(testenv.ShellPrompt())) {
			return
		}
		select {
		case <-host.Done():
			t.Fatalf("shell exited before its prompt: %q", host.History())
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("shell did not become ready: %q", host.History())
}
