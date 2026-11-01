package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStdioMCPChildTemporaryEnvironment(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	temporary, proof := t.TempDir(), filepath.Join(t.TempDir(), "environment.json")
	before := os.Getenv("TEMP")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := Connect(ctx, Config{Name: "temp-proof", Command: executable,
		Args: []string{"-test.run=^TestMCPHelper$"}, Env: map[string]string{
			"GCODE_TEST_MCP_HELPER": "1", "GCODE_TEST_MCP_ENV_PROOF": proof,
			"TEMP": temporary, "TMP": temporary, "TMPDIR": temporary,
		}}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	data, err := os.ReadFile(proof)
	if err != nil {
		t.Fatal(err)
	}
	var values map[string]string
	if err := json.Unmarshal(data, &values); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"TEMP", "TMP", "TMPDIR"} {
		if values[key] != temporary {
			t.Fatalf("%s = %q", key, values[key])
		}
	}
	if os.Getenv("TEMP") != before {
		t.Fatal("MCP child changed host TEMP")
	}
}
