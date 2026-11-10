package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"gcode/internal/model"
)

type fileTool struct {
	name, description string
	properties        map[string]any
	required          []string
}

func (t fileTool) Definition() model.Tool {
	return model.Tool{Name: t.name, Description: t.description, Parameters: map[string]any{"type": "object", "properties": t.properties, "required": t.required, "additionalProperties": false}}
}
func (t fileTool) Concurrent() bool {
	return t.name == "ListDirectory"
}

func CoreTools() []Tool {
	text := func(description string) any { return map[string]any{"type": "string", "description": description} }
	tools := []Tool{
		readTool{},
		fileTool{"Write", "Create or replace a file with the supplied complete content.", map[string]any{"file_path": text("File path"), "content": text("Complete content")}, []string{"file_path", "content"}},
		fileTool{"Edit", "Replace an exact old_string with new_string. Read the file first. The match must be unique unless replace_all is true. CRLF and LF match equivalently.", map[string]any{"file_path": text("File path"), "old_string": text("Exact old text"), "new_string": text("Replacement"), "replace_all": map[string]any{"type": "boolean", "default": false}}, []string{"file_path", "old_string", "new_string"}},
		fileTool{"ListDirectory", "List directory entries. GCode extension.", map[string]any{"path": text("Directory path")}, []string{"path"}},
		globTool{},
		grepTool{},
		fileTool{"Bash", shellDescription(), map[string]any{
			"command": text("Bash script"), "description": text("Brief description of the command"),
			"timeout":                   map[string]any{"type": "integer", "minimum": 1, "maximum": 600000, "description": "Timeout in milliseconds, default 120000"},
			"run_in_background":         map[string]any{"type": "boolean", "description": "Background execution is currently unsupported"},
			"dangerouslyDisableSandbox": map[string]any{"type": "boolean", "description": "Sandbox override is currently unsupported"},
		}, []string{"command"}},
		todoWriteTool{},
		webFetchTool{},
	}
	return tools
}

func shellDescription() string {
	return shellExecutionDescription() + " Background execution and sandbox overrides return explicit unsupported errors."
}

func shellExecutionDescription() string {
	shell := "Run a non-interactive Bash script using Unix syntax and paths."
	if runtime.GOOS == "windows" {
		shell += " On Windows, Git Bash is required; Windows paths may be passed to file tools, while scripts use Git Bash paths such as /c/work. RCODE_GIT_BASH_PATH selects its executable."
	}
	return shell + " Commands run in the configured working directory by default. Use cd only when the command needs to run in a different directory. Each invocation starts a fresh shell in the configured working directory; profiles, directory changes and variables do not persist between calls. Prefer the dedicated file tools for file operations. Timeout is in milliseconds (default 120000, maximum 600000). Cancellation terminates the command process tree. Nonzero exit codes and command errors are returned to the model."
}

// EnvironmentInstructions describes the actual host shell independently of its tool name.
func EnvironmentInstructions(cwd string) string {
	return fmt.Sprintf("Host operating system: %s.\nWorking directory: %q.\nShell execution: %s", runtime.GOOS, cwd, shellExecutionDescription())
}

func (t fileTool) Execute(ctx context.Context, cwd string, raw json.RawMessage) (model.ToolOutput, error) {
	allowed := map[string]bool{}
	for _, tool := range CoreTools() {
		if definition := tool.Definition(); definition.Name == t.name {
			for key := range definition.Parameters["properties"].(map[string]any) {
				allowed[key] = true
			}
			break
		}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return model.ToolOutput{}, err
	}
	for key := range fields {
		if !allowed[key] {
			return model.ToolOutput{}, fmt.Errorf("unknown %s parameter %q", t.name, key)
		}
	}
	var args struct {
		FilePath       string  `json:"file_path"`
		Path           string  `json:"path"`
		Content        *string `json:"content"`
		Old            *string `json:"old_string"`
		New            *string `json:"new_string"`
		ReplaceAll     bool    `json:"replace_all"`
		Command        string  `json:"command"`
		Timeout        *int    `json:"timeout"`
		Description    string  `json:"description"`
		Background     bool    `json:"run_in_background"`
		DisableSandbox bool    `json:"dangerouslyDisableSandbox"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&args); err != nil {
		return model.ToolOutput{}, err
	}
	if err := ctx.Err(); err != nil {
		return model.ToolOutput{}, err
	}
	path := args.FilePath
	if t.name == "ListDirectory" {
		path = args.Path
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}
	switch t.name {
	case "Write":
		if args.FilePath == "" || args.Content == nil {
			return model.ToolOutput{}, errors.New("file_path and content are required")
		}
		previous, readErr := os.ReadFile(path)
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return model.ToolOutput{}, readErr
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return model.ToolOutput{}, err
		}
		err := os.WriteFile(path, []byte(*args.Content), 0644)
		kind := "update"
		var original any = string(previous)
		if errors.Is(readErr, os.ErrNotExist) {
			kind = "create"
			original = nil
		}
		return model.ToolOutput{Text: "Wrote " + path, Data: map[string]any{"type": kind, "filePath": path, "content": *args.Content, "originalFile": original, "structuredPatch": filePatch(string(previous), *args.Content)}}, err
	case "Edit":
		if args.FilePath == "" || args.Old == nil || args.New == nil {
			return model.ToolOutput{}, errors.New("file_path, old_string and new_string are required")
		}
		if *args.Old == *args.New {
			return model.ToolOutput{}, errors.New("old_string and new_string must differ")
		}
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) && *args.Old == "" {
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return model.ToolOutput{}, err
			}
			err = os.WriteFile(path, []byte(*args.New), 0644)
			return model.ToolOutput{Text: "Created " + path, Data: map[string]any{"filePath": path, "oldString": "", "newString": *args.New, "originalFile": nil, "replaceAll": args.ReplaceAll, "structuredPatch": filePatch("", *args.New)}}, err
		}
		if err != nil {
			return model.ToolOutput{}, err
		}
		content := string(data)
		old := *args.Old
		normalized, indices := normalizeLines(content)
		old = strings.ReplaceAll(old, "\r\n", "\n")
		count := strings.Count(normalized, old)
		if old == "" && content != "" {
			return model.ToolOutput{}, errors.New("empty old_string requires an empty or missing file")
		}
		if count == 0 || (!args.ReplaceAll && count != 1) {
			return model.ToolOutput{}, errors.New("old_string must match; use replace_all only when replacing all occurrences")
		}
		replacement := strings.ReplaceAll(*args.New, "\r\n", "\n")
		result := content
		// Apply matches from right to left so byte offsets stay valid. Preserve
		// the newline style of each matched region independently.
		if old == "" {
			result = replacement
		} else {
			positions := []int{}
			for next := 0; next <= len(normalized)-len(old); {
				at := strings.Index(normalized[next:], old)
				if at < 0 {
					break
				}
				at += next
				positions = append(positions, at)
				next = at + len(old)
				if !args.ReplaceAll {
					break
				}
			}
			for i := len(positions) - 1; i >= 0; i-- {
				start, end := indices[positions[i]], indices[positions[i]+len(old)]
				insert := replacement
				if strings.Contains(content[start:end], "\r\n") {
					insert = strings.ReplaceAll(insert, "\n", "\r\n")
				}
				result = result[:start] + insert + result[end:]
			}
		}
		info, err := os.Stat(path)
		if err != nil {
			return model.ToolOutput{}, err
		}
		err = os.WriteFile(path, []byte(result), info.Mode().Perm())
		return model.ToolOutput{Text: "Edited " + path, Data: map[string]any{"filePath": path, "oldString": *args.Old, "newString": *args.New, "originalFile": content, "replaceAll": args.ReplaceAll, "structuredPatch": filePatch(content, result)}}, err
	case "ListDirectory":
		entries, err := os.ReadDir(path)
		if err != nil {
			return model.ToolOutput{}, err
		}
		var out strings.Builder
		for i, entry := range entries {
			if i >= 1000 {
				out.WriteString("[directory truncated]")
				break
			}
			out.WriteString(entry.Name())
			if entry.IsDir() {
				out.WriteByte('/')
			}
			out.WriteByte('\n')
		}
		return model.ToolOutput{Text: out.String()}, nil
	case "Bash":
		if strings.TrimSpace(args.Command) == "" {
			return model.ToolOutput{}, errors.New("command is required")
		}
		if args.Background {
			return model.ToolOutput{}, errors.New("run_in_background is not supported by this host")
		}
		if args.DisableSandbox {
			return model.ToolOutput{}, errors.New("dangerouslyDisableSandbox is not supported by this host")
		}
		milliseconds := 120000
		if args.Timeout != nil {
			milliseconds = *args.Timeout
		}
		if milliseconds < 1 || milliseconds > 600000 {
			return model.ToolOutput{}, errors.New("timeout must be 1..600000 milliseconds")
		}
		return RunShell(ctx, cwd, "bash", args.Command, nil, time.Duration(milliseconds)*time.Millisecond, nil)
	}
	return model.ToolOutput{}, errors.New("unknown tool")
}

func normalizeLines(s string) (string, []int) {
	var out strings.Builder
	indices := make([]int, 0, len(s)+1)
	for i := 0; i < len(s); i++ {
		if s[i] == '\r' && i+1 < len(s) && s[i+1] == '\n' {
			continue
		}
		indices = append(indices, i)
		out.WriteByte(s[i])
	}
	return out.String(), append(indices, len(s))
}

func filePatch(before, after string) []map[string]any {
	if before == after {
		return []map[string]any{}
	}
	lines := func(text string) []string {
		if text == "" {
			return nil
		}
		return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	}
	old, next := lines(before), lines(after)
	patch := make([]string, 0, len(old)+len(next))
	for _, line := range old {
		patch = append(patch, "-"+line)
	}
	for _, line := range next {
		patch = append(patch, "+"+line)
	}
	return []map[string]any{{"oldStart": 1, "oldLines": len(old), "newStart": 1, "newLines": len(next), "lines": patch}}
}

type boundedOutput struct {
	mu         sync.Mutex
	data       []byte
	truncated  bool
	openOutput func() (io.WriteCloser, string, error)
	stream     io.WriteCloser
	outputPath string
	streamErr  error
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	if b.openOutput != nil && b.stream == nil && b.streamErr == nil && len(b.data)+len(p) > 64000 {
		b.stream, b.outputPath, b.streamErr = b.openOutput()
		if b.streamErr == nil && (b.stream == nil || b.outputPath == "") {
			b.streamErr = errors.New("output storage returned no stream or path")
		}
		if b.streamErr == nil {
			_, b.streamErr = b.stream.Write(b.data)
		}
	}
	if b.streamErr != nil {
		return 0, b.streamErr
	}
	if b.stream != nil {
		if _, err := b.stream.Write(p); err != nil {
			b.streamErr = err
			return 0, err
		}
	}
	space := 64000 - len(b.data)
	if len(p) > space {
		p = p[:space]
		b.truncated = true
	}
	b.data = append(b.data, p...)
	return n, nil
}

func (b *boundedOutput) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stream != nil {
		b.streamErr = errors.Join(b.streamErr, b.stream.Close())
		b.stream = nil
	}
	return b.streamErr
}

func (b *boundedOutput) OutputPath() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.streamErr != nil {
		return ""
	}
	return b.outputPath
}
func (b *boundedOutput) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := string(b.data)
	if b.truncated {
		s += "\n[output truncated]"
	}
	return s
}
