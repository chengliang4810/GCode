package terminalhost

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRealShellInputResizeInterruptDetachAndExit(t *testing.T) {
	directory := t.TempDir()
	host, err := Start(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = host.Close() })
	attach := func() *Stream {
		stream, attachErr := host.Attach(context.Background())
		if attachErr != nil {
			t.Fatal(attachErr)
		}
		// ConPTY asks the terminal for its cursor before initializing PowerShell.
		// An actual emulator supplies this reply; the test still runs a real shell.
		go func() {
			buf := make([]byte, 4096)
			var tail []byte
			for {
				n, readErr := stream.Read(buf)
				tail = append(tail, buf[:n]...)
				if bytes.Contains(tail, []byte("\x1b[6n")) {
					_, _ = stream.Write([]byte("\x1b[1;1R"))
					tail = nil
				}
				if len(tail) > 32 {
					tail = tail[len(tail)-32:]
				}
				if readErr != nil {
					return
				}
			}
		}()
		return stream
	}
	first := attach()
	t.Cleanup(func() { _ = first.Close() })
	write := func(command string) {
		if _, writeErr := host.Write([]byte(command + "\r")); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	proof := func(name, text string) string {
		if runtime.GOOS == "windows" {
			return "[IO.File]::WriteAllText((Join-Path (Get-Location) '" + name + "'), '" + text + "')"
		}
		return "printf '%s' '" + text + "' > '" + name + "'"
	}
	waitProof := func(name, text string) {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			data, readErr := os.ReadFile(filepath.Join(directory, name))
			if readErr == nil && string(data) == text {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("real shell did not write %s; output: %q", name, host.History())
	}
	write(proof("first.txt", "中文真实终端"))
	waitProof("first.txt", "中文真实终端")
	if err = first.Resize(103, 37); err != nil {
		t.Fatal(err)
	}
	if err = first.Resize(0, 24); err == nil {
		t.Fatal("invalid size accepted")
	}
	if runtime.GOOS == "windows" {
		write("Start-Sleep -Seconds 30")
	} else {
		write("sleep 30")
	}
	time.Sleep(300 * time.Millisecond)
	beforeInterrupt := len(host.History())
	if _, err = first.Write([]byte{3}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		output := string(host.History()[beforeInterrupt:])
		if strings.Contains(output, "PS ") || strings.Contains(output, "$ ") || strings.Contains(output, "# ") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	write(proof("interrupt.txt", "interrupted"))
	waitProof("interrupt.txt", "interrupted")

	if runtime.GOOS == "windows" {
		write("Start-Sleep -Milliseconds 500; " + proof("detached.txt", "still-running"))
	} else {
		write("sleep 1; " + proof("detached.txt", "still-running"))
	}
	_ = first.Close()
	waitProof("detached.txt", "still-running")
	second := attach()
	defer second.Close()
	write(proof("reconnected.txt", "reconnected"))
	waitProof("reconnected.txt", "reconnected")
	if !strings.Contains(string(host.History()), "first.txt") {
		t.Fatal("reconnect lost terminal history")
	}
	write("exit 7")
	select {
	case <-host.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("shell did not exit")
	}
	if code, _ := host.Result(); code != 7 {
		t.Fatalf("exit code = %d, want 7", code)
	}
}

func TestCloseUnblocksReaderAndCanceledAttachment(t *testing.T) {
	host, err := Start(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := host.Attach(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		buf := make([]byte, 4096)
		for {
			if _, readErr := stream.Read(buf); readErr != nil {
				return
			}
		}
	}()
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatal("canceled view blocked")
	}
	if err = host.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-host.Done():
	default:
		t.Fatal("close left the shell running")
	}
}

func TestHistoryRetainsRecentBytesInOrder(t *testing.T) {
	host := &Session{streams: map[*Stream]struct{}{}, done: make(chan struct{})}
	var expected []byte
	for _, chunk := range [][]byte{bytes.Repeat([]byte("start"), HistoryLimit/5), bytes.Repeat([]byte("middle"), 913), []byte("中文-last")} {
		expected = append(expected, chunk...)
		host.broadcastLocked(chunk)
	}
	expected = expected[max(0, len(expected)-HistoryLimit):]
	if actual := host.History(); !bytes.Equal(actual, expected) {
		t.Fatal("bounded history lost byte order")
	}
	stream, err := host.Attach(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	data := make([]byte, len(expected))
	if _, err = io.ReadFull(stream, data); err != nil || !bytes.Equal(data, expected) {
		t.Fatal("reconnect did not receive the recent history")
	}
}
