package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gcode/internal/storage"
)

func TestListenerSecurityRequiresTLSOutsideLoopback(t *testing.T) {
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(previous)
	for _, test := range []struct {
		name, address string
		tls, allowed  bool
		refused       bool
		warning       bool
	}{
		{"IPv4 loopback", "127.0.0.1", false, false, false, false},
		{"IPv6 loopback", "::1", false, false, false, false},
		{"IPv4 all interfaces", "0.0.0.0", false, false, true, false},
		{"IPv6 all interfaces", "::", false, false, true, false},
		{"LAN plaintext", "192.168.1.2", false, false, true, false},
		{"LAN plaintext acknowledged", "192.168.1.2", false, true, false, true},
		{"LAN TLS", "192.168.1.2", true, false, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			output.Reset()
			err := checkListenerSecurity(&net.TCPAddr{IP: net.ParseIP(test.address), Port: 27655}, test.tls, test.allowed)
			if (err != nil) != test.refused {
				t.Fatalf("error = %v, wanted refusal %v", err, test.refused)
			}
			if got := strings.Contains(output.String(), "without TLS"); got != test.warning {
				t.Fatalf("warning = %q, wanted warning %v", output.String(), test.warning)
			}
		})
	}
}

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
