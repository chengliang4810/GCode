package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestExactEditPreservesUnicodeAndCRLF(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "one.txt")
	if err := os.WriteFile(path, []byte("前言\r\n旧内容\r\n结尾\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	tool := fileTool{name: "Edit"}
	_, err := tool.Execute(context.Background(), root, json.RawMessage(`{"file_path":"one.txt","old_string":"旧内容\n","new_string":"新内容\n新增\n"}`))
	data, _ := os.ReadFile(path)
	if err != nil || string(data) != "前言\r\n新内容\r\n新增\r\n结尾\r\n" {
		t.Fatalf("%q %v", data, err)
	}
	_, err = tool.Execute(context.Background(), root, json.RawMessage(`{"file_path":"one.txt","old_string":"过期内容","new_string":"错误"}`))
	if err == nil {
		t.Fatal("stale edit succeeded")
	}
}

func TestFileWriteToolsStayInsideWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "secret.txt")
	tools := []fileTool{{name: "Write"}, {name: "Edit"}}
	cases := []struct {
		name string
		args string
	}{
		{"absolute", `{"file_path":"` + filepath.ToSlash(target) + `","content":"escaped","old_string":"","new_string":"escaped"}`},
		{"parent traversal", `{"file_path":"../` + filepath.Base(outside) + `/secret.txt","content":"escaped","old_string":"","new_string":"escaped"}`},
		{"symlink escape", `{"file_path":"link/secret.txt","content":"escaped","old_string":"","new_string":"escaped"}`},
	}
	if runtime.GOOS == "windows" {
		cases = cases[:2]
	} else if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools {
		for _, test := range cases {
			if test.name == "symlink escape" && runtime.GOOS == "windows" {
				continue
			}
			if _, err := tool.Execute(context.Background(), root, json.RawMessage(test.args)); err == nil {
				t.Fatalf("%s %s escaped the working directory", tool.name, test.name)
			}
		}
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("out-of-workspace file was written: %v", err)
	}
	if _, err := (fileTool{name: "Write"}).Execute(context.Background(), "", json.RawMessage(`{"file_path":"outside.txt","content":"x"}`)); err == nil {
		t.Fatal("write without a working directory succeeded")
	}
	if _, err := (fileTool{name: "Write"}).Execute(context.Background(), root, json.RawMessage(`{"file_path":"nested/inside.txt","content":"ok"}`)); err != nil {
		t.Fatalf("workspace write rejected: %v", err)
	}
}

func TestEditReplacesAllMatchesAndCreatesOnlyEmptyFiles(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "all.txt")
	if err := os.WriteFile(path, []byte("旧\r\n旧\r\n末尾"), 0600); err != nil {
		t.Fatal(err)
	}
	tool := fileTool{name: "Edit"}
	if _, err := tool.Execute(context.Background(), root, json.RawMessage(`{"file_path":"all.txt","old_string":"旧\n","new_string":"新\n"}`)); err == nil {
		t.Fatal("ambiguous edit succeeded")
	}
	if _, err := tool.Execute(context.Background(), root, json.RawMessage(`{"file_path":"all.txt","old_string":"旧\n","new_string":"新\n","replace_all":true}`)); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "新\r\n新\r\n末尾" {
		t.Fatalf("replace_all damaged CRLF: %q", data)
	}
	if _, err := tool.Execute(context.Background(), root, json.RawMessage(`{"file_path":"all.txt","old_string":"","new_string":"lost"}`)); err == nil {
		t.Fatal("empty old_string overwrote nonempty file")
	}
	if _, err := tool.Execute(context.Background(), root, json.RawMessage(`{"file_path":"created/new.txt","old_string":"","new_string":"new"}`)); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "created", "new.txt"))
	if err != nil || string(data) != "new" {
		t.Fatalf("empty old_string creation: %q %v", data, err)
	}
}

func TestCoreToolsRejectObsoleteFieldsAndUnsupportedShellModes(t *testing.T) {
	for _, test := range []struct{ name, tool, raw string }{
		{"write old path", "Write", `{"path":"lost.txt","content":"x"}`},
		{"write unrelated field", "Write", `{"file_path":"lost.txt","content":"x","command":"ignored"}`},
		{"edit old fields", "Edit", `{"file_path":"lost.txt","oldText":"x","newText":"y"}`},
		{"shell background", "Bash", `{"command":"exit 0","run_in_background":true}`},
		{"shell sandbox override", "Bash", `{"command":"exit 0","dangerouslyDisableSandbox":true}`},
		{"shell zero timeout", "Bash", `{"command":"exit 0","timeout":0}`},
		{"shell oversized timeout", "Bash", `{"command":"exit 0","timeout":600001}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := (fileTool{name: test.tool}).Execute(context.Background(), t.TempDir(), json.RawMessage(test.raw)); err == nil {
				t.Fatal("invalid request succeeded")
			}
		})
	}
}

func TestBashTimeoutIsMillisecondsAndNeverLoadsBashEnv(t *testing.T) {
	root := t.TempDir()
	profile := filepath.Join(root, "profile.sh")
	if err := os.WriteFile(profile, []byte("printf 'profile-was-loaded'"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASH_ENV", profile)
	tool := fileTool{name: "Bash"}
	out, err := tool.Execute(context.Background(), root, json.RawMessage(`{"command":"printf 'clean'","description":"Check clean shell","timeout":10000}`))
	if err != nil || out.Text != "clean" {
		t.Fatalf("shell profile leaked: %q %v", out.Text, err)
	}
	started := time.Now()
	_, err = tool.Execute(context.Background(), root, json.RawMessage(`{"command":"sleep 20","timeout":100}`))
	if err == nil || time.Since(started) > 5*time.Second {
		t.Fatalf("timeout was not milliseconds: %v", err)
	}
}

func TestFileAndBashOutputsExposeStructuredHookResponses(t *testing.T) {
	root := t.TempDir()
	write, err := (fileTool{name: "Write"}).Execute(context.Background(), root, json.RawMessage(`{"file_path":"one.txt","content":"first\nsecond"}`))
	if err != nil {
		t.Fatal(err)
	}
	w := write.Data.(map[string]any)
	if w["type"] != "create" || w["content"] != "first\nsecond" || w["originalFile"] != nil {
		t.Fatalf("Write data: %+v", w)
	}
	read, err := (readTool{}).Execute(context.Background(), root, json.RawMessage(`{"file_path":"one.txt","offset":2,"limit":1}`))
	if err != nil {
		t.Fatal(err)
	}
	r := read.Data.(map[string]any)["file"].(map[string]any)
	if r["filePath"] != filepath.Join(root, "one.txt") || r["startLine"] != 2 || r["numLines"] != 1 || r["totalLines"] != 2 {
		t.Fatalf("Read data: %+v", r)
	}
	edit, err := (fileTool{name: "Edit"}).Execute(context.Background(), root, json.RawMessage(`{"file_path":"one.txt","old_string":"second","new_string":"next"}`))
	if err != nil {
		t.Fatal(err)
	}
	e := edit.Data.(map[string]any)
	if e["oldString"] != "second" || e["newString"] != "next" || e["originalFile"] != "first\nsecond" {
		t.Fatalf("Edit data: %+v", e)
	}
	bash, err := (fileTool{name: "Bash"}).Execute(context.Background(), root, json.RawMessage(`{"command":"printf out; printf err >&2"}`))
	if err != nil {
		t.Fatal(err)
	}
	b := bash.Data.(map[string]any)
	if b["stdout"] != "out" || b["stderr"] != "err" || b["interrupted"] != false {
		t.Fatalf("Bash data: %+v", b)
	}
}
func TestShellCancellationAndUnicode(t *testing.T) {
	root := t.TempDir()
	tool := fileTool{name: "Bash"}
	command := "printf '你好'"

	raw, _ := json.Marshal(map[string]any{"command": command})
	output, err := tool.Execute(context.Background(), root, raw)
	if err != nil || !strings.Contains(output.Text, "你好") {
		t.Fatalf("%s %v", output.Text, err)
	}
	command = "sleep 30"

	raw, _ = json.Marshal(map[string]any{"command": command})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err = tool.Execute(ctx, root, raw)
	if err == nil || time.Since(start) > 5*time.Second {
		t.Fatalf("cancellation failed: %v", err)
	}
}

func TestShellToolMatchesHostPlatform(t *testing.T) {
	want, unavailable := "Bash", "powershell"
	found := false
	for _, tool := range CoreTools() {
		name := tool.Definition().Name
		if name == unavailable {
			t.Fatalf("%s must not be advertised on %s", name, runtime.GOOS)
		}
		found = found || name == want
		if name == want && runtime.GOOS == "windows" && !strings.Contains(tool.Definition().Description, "Git Bash") {
			t.Fatal("Windows shell syntax is not declared to the model")
		}
	}
	if !found {
		t.Fatalf("native shell tool %s is missing", want)
	}
}

func TestUnifiedShellUsesWorkingDirectoryAndReportsExit(t *testing.T) {
	root := filepath.Join(t.TempDir(), "工作 目录")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	command := `printf 'Bash|中文' > shell.txt; printf 'shell-completed'`

	raw, _ := json.Marshal(map[string]any{"command": command})
	tool := fileTool{name: "Bash"}
	output, err := tool.Execute(context.Background(), root, raw)
	if err != nil || !strings.Contains(output.Text, "shell-completed") {
		t.Fatalf("native shell failed: %s %v", output.Text, err)
	}
	proof, err := os.ReadFile(filepath.Join(root, "shell.txt"))
	if err != nil || !strings.Contains(string(proof), "|中文") {
		t.Fatalf("native shell used the wrong working directory: %q %v", proof, err)
	}

	command = "printf 'diagnostic'; exit 7"

	raw, _ = json.Marshal(map[string]any{"command": command})
	output, err = tool.Execute(context.Background(), root, raw)
	if err == nil || !strings.Contains(output.Text, "diagnostic") {
		t.Fatalf("shell failure lost its exit status or output: %s %v", output.Text, err)
	}
	t.Logf("tool=bash platform=%s proof=%s", runtime.GOOS, proof)
}

func TestCoreSearchToolDefinitions(t *testing.T) {
	found := map[string]bool{}
	for _, tool := range CoreTools() {
		definition := tool.Definition()
		if definition.Name == "find" {
			t.Fatal("removed find tool is still registered")
		}
		if definition.Name != "Glob" && definition.Name != "Grep" {
			continue
		}
		found[definition.Name] = true
		if !tool.Concurrent() {
			t.Fatalf("read-only %s is not concurrent", definition.Name)
		}
		required, ok := definition.Parameters["required"].([]string)
		if !ok || len(required) != 1 || required[0] != "pattern" {
			t.Fatalf("%s requires more than pattern: %+v", definition.Name, definition.Parameters)
		}
		properties := definition.Parameters["properties"].(map[string]any)
		for _, key := range []string{"pattern", "path", "head_limit", "offset"} {
			if _, exists := properties[key]; !exists {
				t.Fatalf("%s does not describe %s", definition.Name, key)
			}
		}
		if definition.Name == "Grep" {
			for _, key := range []string{
				"glob", "type", "output_mode", "-i", "multiline",
				"context", "-B", "-A", "-n",
			} {
				if _, exists := properties[key]; !exists {
					t.Fatalf("grep does not describe %s", key)
				}
			}
		}
	}
	if !found["Glob"] || !found["Grep"] {
		t.Fatalf("search registry incomplete: %+v", found)
	}
}
