package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gcode/internal/contracts"
)

func TestBashUsesOwnedTemporaryDirectory(t *testing.T) {
	if _, err := bashExecutable(); err != nil {
		t.Skip(err)
	}
	temporary := filepath.ToSlash(t.TempDir())
	before := os.Getenv("TEMP")
	tools, cleanup := RunToolSet(context.Background(), t.TempDir(), RunToolOptions{Env: map[string]string{
		"TEMP": temporary, "TMP": temporary, "TMPDIR": temporary,
	}})
	defer cleanup()
	raw := json.RawMessage(`{"command":"test \"$TEMP\" = \"$TMP\" && test \"$TMP\" = \"$TMPDIR\" && mktemp \"$TMPDIR/owned.XXXXXXXX\""}`)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := ownedTool(t, tools, "Bash").Execute(ctx, "", raw)
	if err != nil {
		t.Fatalf("owned Bash failed: %s %v", out.Text, err)
	}
	path := strings.TrimSpace(out.Data.(map[string]any)["stdout"].(string))
	// Git Bash may print /c/...; let the same shell verify the created file's owner.
	raw, _ = json.Marshal(map[string]string{"command": "test -f '" + strings.ReplaceAll(path, "'", "'\\''") + "' && case '" + strings.ReplaceAll(path, "'", "'\\''") + "' in \"$TMPDIR\"/*) printf OWNED;; *) exit 9;; esac"})
	out, err = ownedTool(t, tools, "Bash").Execute(ctx, "", raw)
	if err != nil || !strings.Contains(out.Text, "OWNED") {
		t.Fatalf("mktemp escaped owned directory: %q %s %v", path, out.Text, err)
	}
	if os.Getenv("TEMP") != before {
		t.Fatal("child environment changed host TEMP")
	}
}

func TestHookProcessReceivesTypedEnvironmentWithoutJSONLeak(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	temporary := filepath.ToSlash(t.TempDir())
	input := HookInput{Event: contracts.HookSessionStart, CWD: t.TempDir(), Environment: map[string]string{
		"TEMP": temporary, "TMP": temporary, "TMPDIR": temporary, "GCODE_TEMP_HOOK_HELPER": "1",
	}}
	data, err := json.Marshal(input)
	if err != nil || strings.Contains(string(data), temporary) || strings.Contains(string(data), "GCODE_TEMP_HOOK_HELPER") {
		t.Fatalf("host process environment leaked into hook JSON: %s %v", data, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := executeHookCommand(ctx, contracts.HookRule{Command: executable, Args: []string{"-test.run=^TestTempHookHelper$"}}, input)
	want := temporary + "\n" + temporary + "\n" + temporary
	if err != nil || result.ExitCode != 0 || result.Output != want {
		t.Fatalf("hook child did not receive defaults: %q %v", result.Output, err)
	}
}

func TestTempHookHelper(t *testing.T) {
	if os.Getenv("GCODE_TEMP_HOOK_HELPER") != "1" {
		return
	}
	_, _ = os.Stdout.WriteString(os.Getenv("TEMP") + "\n" + os.Getenv("TMP") + "\n" + os.Getenv("TMPDIR"))
	os.Exit(0)
}
