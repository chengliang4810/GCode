package terminalhost

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRealTerminalChildTemporaryEnvironment(t *testing.T) {
	directory := t.TempDir()
	temporary := filepath.ToSlash(t.TempDir())
	before := os.Getenv("TEMP")
	host, err := StartWithEnvironment(directory, map[string]string{"TEMP": temporary, "TMP": temporary, "TMPDIR": temporary})
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	stream, err := host.Attach(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		buf := make([]byte, 4096)
		var tail []byte
		for {
			n, err := stream.Read(buf)
			tail = append(tail, buf[:n]...)
			if bytes.Contains(tail, []byte("\x1b[6n")) {
				_, _ = stream.Write([]byte("\x1b[1;1R"))
				tail = nil
			}
			if len(tail) > 32 {
				tail = tail[len(tail)-32:]
			}
			if err != nil {
				return
			}
		}
	}()
	defer func() {
		_ = stream.Close()
		select {
		case <-readerDone:
		case <-time.After(time.Second):
			t.Error("terminal reader did not stop")
		}
	}()
	waitShellReady(t, host)
	command := `printf '%s\n%s\n%s' "$TEMP" "$TMP" "$TMPDIR" > environment.txt`
	if runtime.GOOS == "windows" {
		command = "[IO.File]::WriteAllText((Join-Path (Get-Location) 'environment.txt'), ($env:TEMP+\"`n\"+$env:TMP+\"`n\"+$env:TMPDIR))"
	}
	if _, err := host.Write([]byte(command + "\r")); err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{temporary, temporary, temporary}, "\n")
	deadline := time.Now().Add(20 * time.Second)
	for {
		data, err := os.ReadFile(filepath.Join(directory, "environment.txt"))
		if err == nil && string(data) == want {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("terminal environment = %q, want %q; output %q", data, want, host.History())
		}
		time.Sleep(20 * time.Millisecond)
	}
	if os.Getenv("TEMP") != before {
		t.Fatal("terminal child changed host TEMP")
	}
}
