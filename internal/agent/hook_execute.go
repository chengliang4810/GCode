package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/processutil"
)

// execute runs one hook. The parent context carries host cancellation and is
// reported as a failure; the timeout bounds the hook itself and stays a
// diagnostic.
func (h *HookRunner) execute(ctx context.Context, timeout time.Duration, rule contracts.HookRule, input HookInput) (execution HookExecution, fatal error) {
	execution.Rule, execution.Input = rule, input
	defer func() {
		if h.report != nil {
			fatal = errors.Join(fatal, h.report(execution))
		}
	}()
	if rule.Type == "prompt" || rule.Type == "agent" {
		select {
		case h.llm <- struct{}{}:
			defer func() { <-h.llm }()
		case <-ctx.Done():
			execution.Err = ctx.Err()
			return execution, ctx.Err()
		}
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var result HookEvaluation
	switch rule.Type {
	case "command":
		result, execution.Err = executeHookCommand(runCtx, rule, input)
	case "http":
		result, execution.Err = h.executeHookHTTP(runCtx, rule, input)
	case "prompt", "agent", "mcp_tool":
		if h.evaluate == nil {
			execution.Err = fmt.Errorf("hook %q requires a connected %s evaluator", rule.ID, rule.Type)
		} else {
			prepared, err := prepareHookEvaluation(rule, input)
			if err != nil {
				execution.Err = err
			} else {
				result, execution.Err = h.evaluate(runCtx, prepared, input)
			}
		}
	default:
		execution.Err = fmt.Errorf("unsupported hook type %q", rule.Type)
	}
	execution.Output, execution.Stderr, execution.ExitCode = result.Output, result.Stderr, result.ExitCode
	execution.ExitCodeKnown = result.ExitCodeKnown
	if runCtx.Err() != nil {
		execution.Err = runCtx.Err()
	}
	if ctx.Err() != nil {
		return execution, ctx.Err()
	}
	if execution.Err == nil && (result.ExitCode == 0 || result.ExitCode == 2) {
		switch {
		case result.ExitCode == 2:
			if strings.HasPrefix(strings.TrimSpace(result.Output), "{") && input.Event != contracts.HookElicitation && input.Event != contracts.HookElicitationResult {
				execution.Result, execution.Err = parseHookOutput(input.Event, result.Output, false)
			}
			execution.Result = mergeHookResults(execution.Result, exitTwoHookResult(input.Event, firstHookText(result.Stderr, "Hook blocked execution.")))
		case rule.Type == "prompt" || rule.Type == "agent":
			execution.Result, execution.Err = parseHookEvaluation(rule, result.Output)
		case input.Event == contracts.HookWorktreeCreate && rule.Type == "command":
			if result.ExitCode == 0 {
				execution.Result.WorktreePath = strings.TrimSpace(result.Output)
			}
		default:
			text := result.Output
			if rule.Async || rule.AsyncRewake {
				text = asyncHookJSON(text)
			}
			execution.Result, execution.Err = parseHookOutput(input.Event, text, rule.Type == "http")
		}
	} else if execution.Err == nil {
		execution.Err = fmt.Errorf("hook exited with non-blocking status %d: %s", result.ExitCode, result.Stderr)
		if strings.HasPrefix(strings.TrimSpace(result.Output), "{") && rule.OnFailure != "block" {
			if parsed, err := parseHookOutput(input.Event, result.Output, false); err == nil {
				execution.Result = parsed
			}
		}
	}
	if input.Event == contracts.HookWorktreeCreate && execution.Err == nil &&
		(execution.ExitCode != 0 || execution.Result.WorktreePath == "") {
		execution.Err = errors.New("WorktreeCreate hook must return a worktree path")
	}
	if execution.Err != nil {
		execution.Err = fmt.Errorf("hook %q: %w", rule.ID, execution.Err)
		if hookFailureBlocks(rule, input) {
			execution.Result = exitTwoHookResult(input.Event, execution.Err.Error())
			if input.Event == contracts.HookPermissionRequest {
				execution.Result = HookResult{Deny: true, PermissionDecision: "deny", Reason: execution.Err.Error()}
			}
		}
	}
	if rule.Async || rule.AsyncRewake {
		// Decisions are too late to control the completed action. Keep only the
		// two notification fields delivered at the next session boundary.
		execution.Result = HookResult{Context: execution.Result.Context, SystemMessage: execution.Result.SystemMessage}
	}
	return execution, nil
}

func hookTimeout(rule contracts.HookRule) time.Duration {
	if rule.Event == contracts.HookSessionEnd {
		if rule.TimeoutSeconds == 0 {
			return 1500 * time.Millisecond
		}
		return time.Duration(min(rule.TimeoutSeconds, 60) * float64(time.Second))
	}
	if rule.TimeoutSeconds > 0 {
		return time.Duration(rule.TimeoutSeconds * float64(time.Second))
	}
	if rule.Type == "prompt" {
		return 30 * time.Second
	}
	if rule.Type == "agent" {
		return 60 * time.Second
	}
	switch rule.Event {
	case contracts.HookUserPromptSubmit, contracts.HookPreModelSwitch, contracts.HookPostModelSwitch:
		return 30 * time.Second
	case contracts.HookMessageDisplay:
		return 10 * time.Second
	default:
		return 600 * time.Second
	}
}

func hookFailureBlocks(rule contracts.HookRule, input HookInput) bool {
	if rule.Async || rule.AsyncRewake {
		return false
	}
	if input.Event == contracts.HookWorktreeCreate || input.Event == contracts.HookWorktreeRemove || input.Event == contracts.HookPreModelSwitch {
		return true
	}
	if slices.Contains([]contracts.HookEvent{contracts.HookStop, contracts.HookSubagentStop,
		contracts.HookTaskCompleted, contracts.HookTeammateIdle}, input.Event) {
		return false
	}
	if rule.OnFailure == "block" && (rule.Type == "command" || rule.Type == "http") {
		return true
	}
	// Current public Windows contract fails closed for shell-form permission
	// guards when Git Bash is absent, unless onFailure explicitly overrides it.
	if runtime.GOOS == "windows" && rule.Type == "command" && rule.Args == nil && rule.OnFailure == "" &&
		(input.Event == contracts.HookPreToolUse || input.Event == contracts.HookPermissionRequest) {
		_, err := hookGitBash()
		return err != nil
	}
	return false
}

func executeHookCommand(ctx context.Context, rule contracts.HookRule, input HookInput) (HookEvaluation, error) {
	data, err := json.Marshal(input)
	if err != nil {
		return HookEvaluation{}, err
	}
	program, args, err := hookCommand(rule, input)
	if err != nil {
		return HookEvaluation{}, err
	}
	cmd := exec.CommandContext(ctx, program, args...)
	cmd.Dir = input.CWD
	environmentInput := input
	if base := strings.ToLower(filepath.Base(program)); base == "pwsh" || base == "pwsh.exe" || base == "powershell" || base == "powershell.exe" {
		environmentInput.EnvFile = ""
	}
	cmd.Env = hookEnvironment(rule, environmentInput)
	cmd.Stdin = bytes.NewReader(append(data, '\n'))
	var stdout, stderr boundedOutput
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.WaitDelay = 2 * time.Second
	prepareProcess(cmd)
	err = cmd.Run()
	result := HookEvaluation{Output: stdout.String(), Stderr: stderr.String()}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		result.ExitCode = exit.ExitCode()
		result.ExitCodeKnown = true
		return result, nil
	}
	if err == nil {
		result.ExitCodeKnown = true
	}
	return result, err
}

func hookCommand(rule contracts.HookRule, input HookInput) (string, []string, error) {
	if rule.Args != nil {
		program := expandHookPaths(rule.Command, rule, input, true)
		args := make([]string, len(rule.Args))
		for i, arg := range rule.Args {
			args[i] = expandHookPaths(arg, rule, input, true)
		}
		if runtime.GOOS == "windows" {
			extension := strings.ToLower(filepath.Ext(program))
			if extension == ".cmd" || extension == ".bat" {
				return "", nil, errors.New("exec-form hooks require a real executable; use shell form for .cmd/.bat")
			}
		}
		return program, args, nil
	}
	if strings.Contains(rule.Command, "${user_config.") {
		return "", nil, errors.New("user_config placeholders require exec form; use CLAUDE_PLUGIN_OPTION_* in shell hooks")
	}
	command := expandHookPaths(rule.Command, rule, input, false)
	if runtime.GOOS != "windows" {
		if rule.Shell == "powershell" {
			return "", nil, errors.New("PowerShell hook shell is supported on Windows")
		}
		if rule.Shell == "bash" {
			return "bash", []string{"-c", command}, nil
		}
		return "sh", []string{"-c", command}, nil
	}
	if rule.Shell != "powershell" {
		bash, err := hookGitBash()
		if err == nil {
			return bash, []string{"-c", command}, nil
		}
		if rule.Shell == "bash" {
			return "", nil, err
		}
	}
	program, err := exec.LookPath("pwsh")
	if err != nil {
		program, err = exec.LookPath("powershell")
	}
	if err != nil {
		return "", nil, errors.New("command hook requires Git Bash or PowerShell")
	}
	return program, []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command",
		"$ErrorActionPreference='Stop'; $LASTEXITCODE=0; " + command + "; if ($LASTEXITCODE) { exit $LASTEXITCODE }"}, nil
}

func hookGitBash() (string, error) {
	if configured := os.Getenv("CLAUDE_CODE_GIT_BASH_PATH"); configured != "" {
		if info, err := os.Stat(configured); err == nil && info.Mode().IsRegular() {
			return configured, nil
		}
		return "", errors.New("CLAUDE_CODE_GIT_BASH_PATH does not identify a Bash executable")
	}
	return bashExecutable()
}

func expandHookPaths(text string, rule contracts.HookRule, input HookInput, userConfig bool) string {
	replacements := []string{"${CLAUDE_PROJECT_DIR}", input.CWD, "${CLAUDE_PLUGIN_ROOT}", rule.PluginRoot,
		"${CLAUDE_PLUGIN_DATA}", rule.PluginData}
	if userConfig {
		for key, value := range rule.UserConfig {
			replacements = append(replacements, "${user_config."+key+"}", value)
		}
	}
	return strings.NewReplacer(replacements...).Replace(text)
}

func hookEnvironment(rule contracts.HookRule, input HookInput) []string {
	var env []string
	owned := map[string]string{"CLAUDE_PROJECT_DIR": input.CWD}
	envFileAvailable := input.EnvFile != "" && slices.Contains([]contracts.HookEvent{contracts.HookSessionStart, contracts.HookSetup, contracts.HookCwdChanged, contracts.HookFileChanged}, input.Event)
	if envFileAvailable {
		owned["CLAUDE_ENV_FILE"] = input.EnvFile
	}
	if rule.PluginRoot != "" {
		owned["CLAUDE_PLUGIN_ROOT"] = rule.PluginRoot
	}
	if rule.PluginData != "" {
		owned["CLAUDE_PLUGIN_DATA"] = rule.PluginData
	}
	if input.Effort != nil {
		owned["CLAUDE_EFFORT"] = input.Effort.Level
	}
	for key, value := range rule.UserConfig {
		owned["CLAUDE_PLUGIN_OPTION_"+strings.ToUpper(key)] = value
	}
	for _, pair := range os.Environ() {
		key, _, _ := strings.Cut(pair, "=")
		if strings.HasPrefix(strings.ToUpper(key), "OTEL_") || !envFileAvailable && strings.EqualFold(key, "CLAUDE_ENV_FILE") {
			continue
		}
		env = append(env, pair)
	}
	host := maps.Clone(input.Environment)
	for key := range host {
		if strings.HasPrefix(strings.ToUpper(key), "OTEL_") || !envFileAvailable && strings.EqualFold(key, "CLAUDE_ENV_FILE") {
			delete(host, key)
		}
	}
	return processutil.Environment(env, host, owned)
}

func (h *HookRunner) executeHookHTTP(ctx context.Context, rule contracts.HookRule, input HookInput) (HookEvaluation, error) {
	data, err := json.Marshal(input)
	if err != nil {
		return HookEvaluation{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, expandHookPaths(rule.URL, rule, input, false), bytes.NewReader(data))
	if err != nil {
		return HookEvaluation{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	for key, value := range rule.Headers {
		value = os.Expand(value, func(name string) string {
			if slices.Contains(rule.AllowedEnvVars, name) {
				return os.Getenv(name)
			}
			return ""
		})
		request.Header.Set(key, value)
	}
	response, err := h.http.Do(request)
	if err != nil {
		return HookEvaluation{}, err
	}
	defer response.Body.Close()
	data, err = io.ReadAll(io.LimitReader(response.Body, 1<<20+1))
	result := HookEvaluation{Output: string(data)}
	if err != nil {
		return result, err
	}
	if len(data) > 1<<20 {
		return result, errors.New("HTTP hook response exceeds 1 MiB")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return result, fmt.Errorf("HTTP hook returned status %d", response.StatusCode)
	}
	return result, nil
}

func asyncHookJSON(text string) string {
	if json.Valid([]byte(strings.TrimSpace(text))) {
		return text
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "{") && json.Valid([]byte(line)) {
			return line
		}
	}
	return ""
}

func prepareHookEvaluation(rule contracts.HookRule, input HookInput) (contracts.HookRule, error) {
	if rule.Type == "prompt" || rule.Type == "agent" {
		data, err := json.Marshal(input)
		if err != nil {
			return rule, err
		}
		if strings.Contains(rule.Prompt, "$ARGUMENTS") {
			rule.Prompt = strings.ReplaceAll(rule.Prompt, "$ARGUMENTS", string(data))
		} else {
			rule.Prompt += "\n\n" + string(data)
		}
	}
	if rule.Type == "mcp_tool" && len(rule.ToolInput) > 0 {
		payload, err := substituteHookInput(rule.ToolInput, input)
		if err != nil {
			return rule, err
		}
		rule.ToolInput = payload
	}
	return rule, nil
}
