package remote

import (
	"bytes"
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/storage"
	"gcode/internal/testenv"
)

func TestRealRemoteTerminalSurvivesDisconnect(t *testing.T) {
	store, err := storage.Open(storage.Options{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	core, err := engine.Open(engine.Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	directory := t.TempDir()
	if err = core.Execute(ctx, contracts.Command{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: directory, Name: "PTY"}}); err != nil {
		t.Fatal(err)
	}
	state, _ := core.Snapshot(ctx)
	project := state.Projects[0].ID
	if err = core.Execute(ctx, contracts.Command{Kind: "session.create", ProjectID: project}); err != nil {
		t.Fatal(err)
	}
	state, _ = core.Snapshot(ctx)
	session := state.Sessions[0].ID
	handler, err := (Server{Core: core, Token: strings.Repeat("terminal", 4)}).Handler()
	if err != nil {
		t.Fatal(err)
	}
	host := httptest.NewServer(handler)
	defer host.Close()
	connect := func() *Client {
		client, connectErr := NewClient(host.URL, strings.Repeat("terminal", 4))
		if connectErr != nil {
			t.Fatal(connectErr)
		}
		t.Cleanup(func() { _ = client.Close() })
		return client
	}
	first := connect()
	if err = first.Execute(ctx, contracts.Command{Kind: "terminal.create", ProjectID: project, SessionID: session}); err != nil {
		t.Fatal(err)
	}
	state, err = first.Snapshot(ctx, session)
	if err != nil || len(state.Terminals) != 1 {
		t.Fatalf("terminal creation failed: %v", err)
	}
	meta := state.Terminals[0]
	if meta.CWD != directory || meta.SessionID != session || meta.Status != "running" {
		t.Fatalf("incorrect terminal metadata: %+v", meta)
	}
	attach := func(client *Client) contracts.TerminalStream {
		stream, attachErr := client.OpenTerminal(ctx, meta.ID)
		if attachErr != nil {
			t.Fatal(attachErr)
		}
		t.Cleanup(func() { _ = stream.Close() })
		ready := make(chan struct{})
		go func() {
			announced := false
			buf := make([]byte, 4096)
			var tail []byte
			for {
				n, readErr := stream.Read(buf)
				tail = append(tail, buf[:n]...)
				if !announced && bytes.Contains(tail, []byte(testenv.ShellPrompt())) {
					close(ready)
					announced = true
				}
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
		select {
		case <-ready:
		case <-ctx.Done():
			t.Fatalf("remote shell did not become ready: %v", ctx.Err())
		}
		return stream
	}
	stream := attach(first)
	if err = stream.Resize(111, 39); err != nil {
		t.Fatal(err)
	}
	state, _ = first.Snapshot(ctx, session)
	if state.Terminals[0].Columns != 111 || state.Terminals[0].Rows != 39 {
		t.Fatal("resize did not reach the host")
	}
	writeProof := func(stream contracts.TerminalStream, name, content string, delayed bool) {
		t.Helper()
		command := "printf '%s' '" + content + "' > '" + name + "'"
		delay := "sleep 1; "
		if runtime.GOOS == "windows" {
			command = "[IO.File]::WriteAllText((Join-Path (Get-Location) '" + name + "'), '" + content + "')"
			delay = "Start-Sleep -Milliseconds 500; "
		}
		if delayed {
			command = delay + command
		}
		if _, writeErr := stream.Write([]byte(command + "\r")); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	waitProof := func(name, expected string) {
		t.Helper()
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			data, readErr := os.ReadFile(filepath.Join(directory, name))
			if readErr == nil && string(data) == expected {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("real remote shell did not write %s", name)
	}
	writeProof(stream, "remote.txt", "远程终端", false)
	waitProof("remote.txt", "远程终端")
	writeProof(stream, "disconnected.txt", "host-owned", true)
	_ = stream.Close()
	_ = first.Close()
	waitProof("disconnected.txt", "host-owned")
	second := connect()
	resumed := attach(second)
	writeProof(resumed, "reconnected.txt", "same-process", false)
	waitProof("reconnected.txt", "same-process")
	if err = second.Execute(ctx, contracts.Command{Kind: "terminal.clear", ID: meta.ID}); err != nil {
		t.Fatal(err)
	}
	if err = second.Execute(ctx, contracts.Command{Kind: "terminal.close", ID: meta.ID}); err != nil {
		t.Fatal(err)
	}
	state, _ = second.Snapshot(ctx, session)
	if len(state.Terminals) != 0 {
		t.Fatal("closed terminal remained in snapshot")
	}
	if _, err = second.OpenTerminal(ctx, meta.ID); err == nil {
		t.Fatal("closed process accepted a new connection")
	}
}
