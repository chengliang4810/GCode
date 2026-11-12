package appconfig

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/storage"
)

func TestConnectionPersistsAndKeepsCredentialsPrivate(t *testing.T) {
	store, err := storage.Open(storage.Options{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	manager, err := OpenConnections(store)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	secret := strings.Repeat("test-only", 4)
	err = manager.ConfigureConnection(ctx, contracts.ConnectionInput{URL: "https://example.test", Token: secret, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	info, _ := manager.ConnectionInfo(ctx)
	if !info.HasToken || !info.Pending || info.ActiveURL != "" {
		t.Fatalf("%+v", info)
	}
	manager, err = OpenConnections(store)
	if err != nil || manager.Credentials().Token != secret {
		t.Fatal("credentials did not persist", err)
	}
	manager.Activate("https://example.test")
	info, _ = manager.ConnectionInfo(ctx)
	if info.Pending {
		t.Fatal("active connection still pending")
	}
	err = manager.ConfigureConnection(ctx, contracts.ConnectionInput{URL: "https://other.test", Enabled: true, PreserveToken: true})
	if err == nil {
		t.Fatal("saved token reused for another host")
	}
	err = manager.ConfigureConnection(ctx, contracts.ConnectionInput{URL: info.URL, Enabled: false, PreserveToken: true})
	if err != nil {
		t.Fatal(err)
	}
	info, _ = manager.ConnectionInfo(ctx)
	if info.Enabled || !info.Pending || manager.Credentials().Token != secret {
		t.Fatal("disconnect lost saved credentials or pending state")
	}
}

func TestConfigureConnectionRequiresTLSOutsideLoopback(t *testing.T) {
	store, err := storage.Open(storage.Options{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	manager, err := OpenConnections(store)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	secret := strings.Repeat("test-only", 4)
	for _, address := range []string{"http://host.example:27655", "http://192.168.1.2:27655"} {
		if err := manager.ConfigureConnection(ctx, contracts.ConnectionInput{URL: address, Token: secret, Enabled: true}); err == nil {
			t.Fatalf("plaintext remote host %q accepted", address)
		}
	}
	if err := manager.ConfigureConnection(ctx, contracts.ConnectionInput{URL: "http://127.0.0.1:27655", Token: secret, Enabled: true}); err != nil {
		t.Fatalf("loopback tunnel rejected: %v", err)
	}
	if err := manager.ConfigureConnection(ctx, contracts.ConnectionInput{URL: "https://host.example:27655/", Token: secret, Enabled: true}); err != nil {
		t.Fatalf("TLS host rejected: %v", err)
	}
	if got := manager.Credentials().URL; got != "https://host.example:27655" {
		t.Fatalf("stored URL = %q", got)
	}
}

func TestConnectionStaysInConfigurationRootAfterDataRootSwitch(t *testing.T) {
	options := storage.Options{Home: t.TempDir()}
	store, err := storage.Open(options)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := OpenConnections(store)
	if err != nil {
		t.Fatal(err)
	}
	secret := strings.Repeat("test-only", 4)
	if err := manager.ConfigureConnection(t.Context(), contracts.ConnectionInput{
		URL: "https://example.test", Token: secret, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(options.Home, "runtime-data")
	if err := store.RequestDirectory(target); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = storage.Open(options)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	manager, err = OpenConnections(store)
	if err != nil || manager.Credentials().Token != secret {
		t.Fatalf("data root switch lost fixed connection configuration: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "connection.json")); !os.IsNotExist(err) {
		t.Fatalf("connection credentials were copied to the runtime root: %v", err)
	}
}
