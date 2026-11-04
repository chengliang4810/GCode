package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/storage"
)

func TestLiveUnifiedShellUsesHostRuntime(t *testing.T) {
	baseURL := os.Getenv("GCODE_LIVE_BASE_URL")
	if baseURL == "" {
		t.Skip("set GCODE_LIVE_BASE_URL, GCODE_LIVE_API_KEY and GCODE_LIVE_MODEL to run")
	}
	key, modelID := os.Getenv("GCODE_LIVE_API_KEY"), os.Getenv("GCODE_LIVE_MODEL")
	if key == "" || modelID == "" {
		t.Fatal("live API key and model ID are required")
	}
	store, err := storage.Open(storage.Options{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	core, err := openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	workspace := filepath.Join(t.TempDir(), "工作 目录")
	if err = os.Mkdir(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	config := model.Provider{ID: "live", Name: "Live shell verification", Protocol: model.Chat, BaseURL: baseURL, APIKey: key, Models: []model.Model{{ID: modelID, MaxOutput: 2048}}}
	for _, command := range []contracts.Command{
		{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: config}},
		{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: workspace, Name: "Host shell verification"}},
	} {
		if err = core.Execute(ctx, command); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, _ := core.Snapshot(ctx)
	script := `printf 'Bash|%s|%s' "$BASH_VERSION" "$PWD" > shell-proof.txt`
	wantShell := "Bash|"
	if runtime.GOOS == "windows" {
		script = `printf 'Bash|%s|%s' "$BASH_VERSION" "$(pwd -W)" > shell-proof.txt`
	}
	prompt := "这是实际宿主 shell 验证。请仅调用 Bash 工具，在当前工作目录实际执行下列脚本，不能使用 Write 工具替代，不要安装软件或访问其他目录。执行成功后只回复 GCODE_HOST_SHELL_OK。\n" + script
	if err = core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{ProjectID: snapshot.Projects[0].ID, Text: prompt, ModelID: "live/" + modelID}}); err != nil {
		t.Fatal(err)
	}
	for {
		snapshot, err = core.Snapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(snapshot.Runs) > 0 && snapshot.Runs[len(snapshot.Runs)-1].Status != "running" {
			break
		}
		select {
		case <-ctx.Done():
			last := snapshot.Runs[len(snapshot.Runs)-1]
			toolCalls, outputChars := 0, 0
			for _, message := range snapshot.Messages[last.SessionID] {
				for _, block := range message.Blocks {
					outputChars += len(block.Text)
					if block.Tool != nil {
						toolCalls++
					}
				}
			}
			t.Fatalf("live shell verification timed out: status=%s tools=%d emittedChars=%d", last.Status, toolCalls, outputChars)
		case <-time.After(100 * time.Millisecond):
		}
	}
	run := snapshot.Runs[len(snapshot.Runs)-1]
	if run.Status != "completed" {
		t.Fatalf("live shell run failed: %s", run.Error)
	}
	if run.SessionID != strings.ToLower(run.SessionID) {
		t.Fatalf("new conversation ID is not lowercase: %s", run.SessionID)
	}
	proof, err := os.ReadFile(filepath.Join(workspace, "shell-proof.txt"))
	parts := strings.SplitN(string(proof), "|", 3)
	if err != nil || !strings.HasPrefix(string(proof), wantShell) || len(parts) != 3 || filepath.Clean(filepath.FromSlash(parts[2])) != filepath.Clean(workspace) {
		t.Fatalf("host shell proof is invalid: %q, %v", proof, err)
	}
	shellCalls := 0
	reply := ""
	for _, message := range core.history[run.SessionID] {
		for _, call := range message.Calls {
			if call.Name != "Bash" {
				t.Fatalf("unexpected tool used instead of the unified shell: %s", call.Name)
			}
			shellCalls++
		}
		if message.Role == "assistant" {
			reply += message.Text
		}
	}
	if shellCalls == 0 || !strings.Contains(reply, "GCODE_HOST_SHELL_OK") {
		t.Fatalf("model did not finish the real shell round trip: calls=%d, reply=%q", shellCalls, reply)
	}
	t.Logf("session=%s platform=%s tool=Bash proof=%s reply=%s", run.SessionID, runtime.GOOS, proof, reply)
}

// TestLiveModelToolRoundTrip is explicitly enabled to exercise an actual model
// endpoint through the production engine. It never starts a model fixture.
func TestLiveModelToolRoundTrip(t *testing.T) {
	baseURL := os.Getenv("GCODE_LIVE_BASE_URL")
	if baseURL == "" {
		t.Skip("set GCODE_LIVE_BASE_URL, GCODE_LIVE_API_KEY and GCODE_LIVE_MODEL to run")
	}
	key, modelID := os.Getenv("GCODE_LIVE_API_KEY"), os.Getenv("GCODE_LIVE_MODEL")
	if key == "" || modelID == "" {
		t.Fatal("live API key and model ID are required")
	}
	store, err := storage.Open(storage.Options{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	core, err := openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	defer core.Close()
	workspace := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	config := model.Provider{ID: "live", Name: "Live endpoint", Protocol: model.Chat, BaseURL: baseURL, APIKey: key, Models: []model.Model{{ID: modelID, MaxOutput: 4096}}}
	if err = core.Execute(ctx, contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: config}}); err != nil {
		t.Fatal(err)
	}
	if err = core.Execute(ctx, contracts.Command{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: workspace, Name: "Live model verification"}}); err != nil {
		t.Fatal(err)
	}
	initial, err := core.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	prompt := "这是一次真实工具调用验证。请调用 write 工具，在当前工作目录创建 gcode-live-proof.txt，内容严格为 GCODE_REAL_MODEL_OK 加一个换行；再调用 read 工具读取这个文件，确认内容正确后，最后只回复 GCODE_LIVE_OK。必须实际调用这两个工具，不要只描述操作。不要使用 shell，不要访问或修改其他文件。"
	if err = core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{RequestID: "live-model-check", ProjectID: initial.Projects[0].ID, ModelID: "live/" + modelID, Text: prompt}}); err != nil {
		t.Fatal(err)
	}
	var state contracts.Snapshot
	for {
		state, err = core.Snapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(state.Runs) > 0 && state.Runs[len(state.Runs)-1].Status != "running" {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("live model round trip timed out")
		case <-time.After(100 * time.Millisecond):
		}
	}
	run := state.Runs[len(state.Runs)-1]
	if run.Status != "completed" {
		t.Fatalf("live run did not complete: %s: %s", run.Status, run.Error)
	}
	proof, err := os.ReadFile(filepath.Join(workspace, "gcode-live-proof.txt"))
	if err != nil || string(proof) != "GCODE_REAL_MODEL_OK\n" {
		t.Fatalf("real file verification failed: %q, %v", proof, err)
	}
	var calls []string
	for _, message := range core.history[run.SessionID] {
		for _, call := range message.Calls {
			calls = append(calls, call.Name)
		}
	}
	called := func(name string) bool {
		for _, call := range calls {
			if call == name {
				return true
			}
		}
		return false
	}
	if !called("Write") || !called("Read") {
		t.Fatalf("model did not complete both real tools: %v", calls)
	}
	var reply strings.Builder
	inputTokens, outputTokens := 0, 0
	for _, message := range state.Messages[run.SessionID] {
		if message.Role != "assistant" {
			continue
		}
		inputTokens += message.Usage.InputTokens
		outputTokens += message.Usage.OutputTokens
		for _, block := range message.Blocks {
			if block.Kind == "text" {
				reply.WriteString(block.Text)
			}
		}
	}
	if !strings.Contains(reply.String(), "GCODE_LIVE_OK") {
		t.Fatalf("model did not confirm the tool result: %q", reply.String())
	}
	report := struct {
		BaseURL        string   `json:"baseUrl"`
		Model          string   `json:"model"`
		Status         string   `json:"status"`
		Tools          []string `json:"tools"`
		Proof          string   `json:"proof"`
		Reply          string   `json:"reply"`
		InputTokens    int      `json:"inputTokens"`
		OutputTokens   int      `json:"outputTokens"`
		ElapsedSeconds float64  `json:"elapsedSeconds"`
	}{baseURL, modelID, run.Status, calls, string(proof), reply.String(), inputTokens, outputTokens, time.Since(started).Seconds()}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(data))
	if path := os.Getenv("GCODE_LIVE_REPORT"); path != "" {
		if err = os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
