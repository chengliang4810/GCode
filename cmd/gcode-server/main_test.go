package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gcode/internal/storage"
)

func TestServerStartupFailurePersistsAndReleasesRuntimeLog(t *testing.T) {
	// This owns an ephemeral listener and profile, never an existing service.
	directory := t.TempDir()
	console, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	defer console.Close()
	previousArgs, previousFlags := os.Args, flag.CommandLine
	previousStderr, previousLog := os.Stderr, log.Writer()
	os.Args = []string{"gcode-server", "--data-dir", directory, "--listen", "127.0.0.1:0", "--tls-cert", "unused.pem"}
	flag.CommandLine = flag.NewFlagSet("gcode-server-test", flag.ContinueOnError)
	os.Stderr = console
	defer func() {
		os.Args, flag.CommandLine = previousArgs, previousFlags
		os.Stderr = previousStderr
		log.SetOutput(previousLog)
	}()
	err = run()
	if err == nil || !strings.Contains(err.Error(), "provide both --tls-cert and --tls-key") {
		t.Fatalf("run = %v", err)
	}
	if log.Writer() != previousLog {
		t.Fatal("server did not restore standard log output")
	}
	logData, err := os.ReadFile(filepath.Join(directory, "logs", "gcode.log"))
	if err != nil {
		t.Fatal(err)
	}
	consoleData, err := os.ReadFile(console.Name())
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"GCode server listening", "provide both --tls-cert and --tls-key"} {
		if !strings.Contains(string(logData), text) || !strings.Contains(string(consoleData), text) {
			t.Fatalf("missing %q in log %q or stderr %q", text, logData, consoleData)
		}
	}
	if strings.Contains(string(consoleData), "runtime log write failed") {
		t.Fatalf("shutdown wrote after storage was closed: %s", consoleData)
	}
	var config struct{ Token string }
	data, err := os.ReadFile(filepath.Join(directory, "server.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if config.Token == "" || strings.Contains(string(logData), config.Token) || strings.Contains(string(consoleData), config.Token) {
		t.Fatal("server token is missing or was exposed in diagnostics")
	}
	replacement, err := storage.Open(storage.Options{Directory: directory})
	if err != nil {
		t.Fatalf("replacement could not open isolated storage: %v", err)
	}
	defer replacement.Close()
	writer, err := storage.OpenRuntimeLog(replacement)
	if err != nil {
		t.Fatalf("replacement could not acquire runtime log lock: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
}
