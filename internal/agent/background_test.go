package agent

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func ownedTool(t *testing.T, tools []Tool, name string) Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Definition().Name == name {
			return tool
		}
	}
	t.Fatalf("missing tool %s", name)
	return nil
}
func executeOwned(t *testing.T, tool Tool, args map[string]any) modelOutput {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	out, err := tool.Execute(context.Background(), "", raw)
	if err != nil {
		t.Fatal(err)
	}
	return modelOutput{out.Text, out.Data}
}

type modelOutput struct {
	Text string
	Data any
}

func TestRunBashDefinitionLeavesBaseSchemaImmutable(t *testing.T) {
	var base Tool
	for _, tool := range CoreTools() {
		if tool.Definition().Name == "Bash" {
			base = tool
			break
		}
	}
	before := base.Definition().Parameters["properties"].(map[string]any)["run_in_background"].(map[string]any)["description"]
	tool := runBashTool{base: base}
	if tool.Definition().Parameters["properties"].(map[string]any)["run_in_background"].(map[string]any)["description"] == before {
		t.Fatal("owned Bash did not advertise background execution")
	}
	if base.Definition().Parameters["properties"].(map[string]any)["run_in_background"].(map[string]any)["description"] != before {
		t.Fatal("owned Bash mutated shared base schema")
	}
}

func TestBackgroundTaskRunsAndCanBeRetrieved(t *testing.T) {
	root := t.TempDir()
	tools, cleanup := RunToolSet(context.Background(), root)
	defer cleanup()
	out := executeOwned(t, ownedTool(t, tools, "Bash"), map[string]any{"command": "sleep 0.1; printf 'background result'; printf 'warning' >&2", "run_in_background": true})
	id := out.Data.(map[string]any)["backgroundTaskId"].(string)
	result := executeOwned(t, ownedTool(t, tools, "TaskOutput"), map[string]any{"task_id": id, "block": true, "timeout": 10000})
	data := result.Data.(map[string]any)
	task := data["task"].(map[string]any)
	if data["retrieval_status"] != "success" || task["status"] != "completed" || task["exit_code"] != 0 || !strings.Contains(task["output"].(string), "background result") {
		t.Fatalf("bad task result: %s", result.Text)
	}
}

func TestBackgroundTaskPreservesCompleteOutputBeyondMemoryLimit(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "background-output.txt")
	tools, cleanup := RunToolSet(t.Context(), directory, RunToolOptions{
		OpenOutput: func() (io.WriteCloser, string, error) {
			file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			return file, path, err
		},
	})
	defer cleanup()
	started := executeOwned(t, ownedTool(t, tools, "Bash"), map[string]any{
		"command": `for ((i=0;i<12000;i++)); do printf '0123456789\n'; done; printf 'TAIL-END'`, "run_in_background": true,
	})
	id := started.Data.(map[string]any)["backgroundTaskId"].(string)
	raw, _ := json.Marshal(map[string]any{"task_id": id, "block": true, "timeout": 30000})
	result, err := ownedTool(t, tools, "TaskOutput").Execute(t.Context(), directory, raw)
	if err != nil || result.OutputFile != path {
		t.Fatalf("background output did not retain its durable file: %v %+v", err, result)
	}
	full, err := os.ReadFile(path)
	if err != nil || string(full) != strings.Repeat("0123456789\n", 12000)+"TAIL-END" {
		t.Fatalf("background stream lost output: %v bytes=%d", err, len(full))
	}
}

func TestBackgroundOwnerAdmitsNewWorkAfterCompletedHistoryFills(t *testing.T) {
	tools, cleanup := RunToolSet(context.Background(), t.TempDir())
	defer cleanup()
	bash, output := ownedTool(t, tools, "Bash"), ownedTool(t, tools, "TaskOutput")
	running := executeOwned(t, bash, map[string]any{"command": "sleep 120", "run_in_background": true})
	runningID := running.Data.(map[string]any)["backgroundTaskId"].(string)
	first, last := "", ""
	for i := 0; i < maxCompletedBackgroundTasks+3; i++ {
		out := executeOwned(t, bash, map[string]any{"command": "printf done", "run_in_background": true})
		last = out.Data.(map[string]any)["backgroundTaskId"].(string)
		if first == "" {
			first = last
		}
		result := executeOwned(t, output, map[string]any{"task_id": last, "timeout": 10000})
		if result.Data.(map[string]any)["task"].(map[string]any)["status"] != "completed" {
			t.Fatalf("task did not complete: %s", result.Text)
		}
	}
	latest := executeOwned(t, output, map[string]any{"task_id": last, "block": false})
	if latest.Data.(map[string]any)["task"].(map[string]any)["output"] != "done" {
		t.Fatalf("latest completed result was lost: %s", latest.Text)
	}
	active := executeOwned(t, output, map[string]any{"task_id": runningID, "block": false})
	if active.Data.(map[string]any)["task"].(map[string]any)["status"] != "running" {
		t.Fatalf("history eviction removed active work: %s", active.Text)
	}
	raw, _ := json.Marshal(map[string]any{"task_id": first, "block": false})
	if _, err := output.Execute(context.Background(), "", raw); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("old result did not expire explicitly: %v", err)
	}
	owner := bash.(runBashTool).owner
	owner.mu.Lock()
	defer owner.mu.Unlock()
	if len(owner.tasks) != maxCompletedBackgroundTasks+1 || owner.active != 1 {
		t.Fatalf("unbounded task retention: total=%d active=%d", len(owner.tasks), owner.active)
	}
}

func TestBackgroundTasksAreScopedAndCleanupStopsOnlyOwnedProcess(t *testing.T) {
	tools, cleanup := RunToolSet(context.Background(), t.TempDir())
	defer cleanup()
	other, otherCleanup := RunToolSet(context.Background(), t.TempDir())
	defer otherCleanup()
	started := executeOwned(t, ownedTool(t, tools, "Bash"), map[string]any{"command": "printf started; sleep 30", "run_in_background": true})
	id := started.Data.(map[string]any)["backgroundTaskId"].(string)
	raw, _ := json.Marshal(map[string]any{"task_id": id})
	if _, err := ownedTool(t, other, "TaskStop").Execute(context.Background(), "", raw); err == nil {
		t.Fatal("another run could stop this task")
	}
	begin := time.Now()
	cleanup()
	cleanup()
	if time.Since(begin) > 5*time.Second {
		t.Fatal("cleanup failed to join owned process")
	}
	result := executeOwned(t, ownedTool(t, tools, "TaskOutput"), map[string]any{"task_id": id, "block": false})
	if result.Data.(map[string]any)["task"].(map[string]any)["status"] != "killed" {
		t.Fatalf("cleanup task status: %s", result.Text)
	}
	raw, _ = json.Marshal(map[string]any{"command": "printf invalid", "run_in_background": true})
	if _, err := ownedTool(t, tools, "Bash").Execute(context.Background(), "", raw); err == nil {
		t.Fatal("closed owner accepted task")
	}
}

func TestBackgroundTaskStopAndNonblockingOutput(t *testing.T) {
	tools, cleanup := RunToolSet(context.Background(), t.TempDir())
	defer cleanup()
	started := executeOwned(t, ownedTool(t, tools, "Bash"), map[string]any{"command": "sleep 30", "run_in_background": true})
	id := started.Data.(map[string]any)["backgroundTaskId"].(string)
	out := executeOwned(t, ownedTool(t, tools, "TaskOutput"), map[string]any{"task_id": id, "block": false})
	if out.Data.(map[string]any)["retrieval_status"] != "not_ready" {
		t.Fatalf("nonblocking output: %s", out.Text)
	}
	waited := executeOwned(t, ownedTool(t, tools, "TaskOutput"), map[string]any{"task_id": id, "block": true, "timeout": 10})
	if waited.Data.(map[string]any)["retrieval_status"] != "timeout" {
		t.Fatalf("bounded wait: %s", waited.Text)
	}
	executeOwned(t, ownedTool(t, tools, "TaskStop"), map[string]any{"task_id": id})
	out = executeOwned(t, ownedTool(t, tools, "TaskOutput"), map[string]any{"task_id": id})
	if out.Data.(map[string]any)["task"].(map[string]any)["status"] != "killed" {
		t.Fatalf("stop did not finish: %s", out.Text)
	}
}

func TestBackgroundTaskOwnerCancellationDoesNotAffectAnotherOwner(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tools, cleanup := RunToolSet(ctx, t.TempDir())
	defer cleanup()
	other, otherCleanup := RunToolSet(context.Background(), t.TempDir())
	defer otherCleanup()
	first := executeOwned(t, ownedTool(t, tools, "Bash"), map[string]any{"command": "sleep 30", "run_in_background": true})
	second := executeOwned(t, ownedTool(t, other, "Bash"), map[string]any{"command": "sleep 0.2; printf alive", "run_in_background": true})
	cancel()
	id := first.Data.(map[string]any)["backgroundTaskId"].(string)
	out := executeOwned(t, ownedTool(t, tools, "TaskOutput"), map[string]any{"task_id": id, "timeout": 10000})
	if out.Data.(map[string]any)["task"].(map[string]any)["status"] != "killed" {
		t.Fatalf("owner cancellation: %s", out.Text)
	}
	id = second.Data.(map[string]any)["backgroundTaskId"].(string)
	out = executeOwned(t, ownedTool(t, other, "TaskOutput"), map[string]any{"task_id": id, "timeout": 10000})
	if out.Data.(map[string]any)["task"].(map[string]any)["status"] != "completed" {
		t.Fatalf("other owner stopped: %s", out.Text)
	}
}

func TestRunShellKeepsEnvironmentLocalAndUsesExplicitShell(t *testing.T) {
	root := t.TempDir()
	before := os.Getenv("RCODE_TEST_CHILD_ONLY")
	out, err := RunShell(context.Background(), root, "bash", `printf '%s' "$RCODE_TEST_CHILD_ONLY"`, map[string]string{"RCODE_TEST_CHILD_ONLY": "child value"}, 10*time.Second, nil)
	if err != nil || out.Text != "child value" || os.Getenv("RCODE_TEST_CHILD_ONLY") != before {
		t.Fatalf("child env: %q %v", out.Text, err)
	}
	out, err = RunShell(context.Background(), root, "powershell", `[Console]::Write($env:RCODE_TEST_CHILD_ONLY)`, map[string]string{"RCODE_TEST_CHILD_ONLY": "powershell value"}, 60*time.Second, nil)
	_, pwshErr := exec.LookPath("pwsh")
	_, powershellErr := exec.LookPath("powershell")
	if pwshErr != nil && powershellErr != nil {
		if err == nil || !strings.Contains(err.Error(), "PowerShell shell requires") {
			t.Fatalf("missing PowerShell dependency: %v", err)
		}
	} else if err != nil || out.Text != "powershell value" {
		t.Fatalf("explicit PowerShell: %q %v", out.Text, err)
	}
	if _, err := RunShell(context.Background(), root, "invalid", "echo invalid", nil, time.Second, nil); err == nil {
		t.Fatal("accepted unknown shell")
	}
}

func TestRunToolsSourceOnlyManagedSessionEnvFile(t *testing.T) {
	root := t.TempDir()
	envFile := filepath.Join(root, "session env.sh")
	if err := os.WriteFile(envFile, []byte("export RCODE_PLUGIN_SESSION_VALUE='session value'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	tools, cleanup := RunToolSet(context.Background(), root, RunToolOptions{EnvFile: envFile, Env: map[string]string{"RCODE_PLUGIN_ARGUMENT": "argument"}})
	defer cleanup()
	out := executeOwned(t, ownedTool(t, tools, "Bash"), map[string]any{"command": `printf '%s|%s' "$RCODE_PLUGIN_SESSION_VALUE" "$RCODE_PLUGIN_ARGUMENT"`})
	if out.Text != "session value|argument" {
		t.Fatalf("session environment: %s", out.Text)
	}
	if _, err := RunShell(context.Background(), root, "powershell", "echo x", map[string]string{"RCODE_SESSION_ENV_FILE": envFile}, time.Second, nil); err == nil {
		t.Fatal("PowerShell accepted Bash environment file")
	}
}
