package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"gcode/internal/model"
)

// RunShell runs command in the requested shell without loading user profiles.
// Environment overrides belong only to this child process.
func RunShell(ctx context.Context, cwd, shell, command string, env map[string]string, timeout time.Duration) (model.ToolOutput, error) {
	if timeout <= 0 {
		return model.ToolOutput{}, errors.New("shell timeout must be positive")
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd, err := newShellCommand(runCtx, cwd, shell, command, env)
	if err != nil {
		return model.ToolOutput{}, err
	}
	var output, stdout, stderr boundedOutput
	cmd.Stdout, cmd.Stderr = io.MultiWriter(&output, &stdout), io.MultiWriter(&output, &stderr)
	err = cmd.Run()
	if runCtx.Err() != nil {
		err = runCtx.Err()
	}
	return model.ToolOutput{Text: output.String(), Data: map[string]any{"stdout": stdout.String(), "stderr": stderr.String(), "interrupted": runCtx.Err() != nil}}, err
}

func newShellCommand(ctx context.Context, cwd, shell, command string, env map[string]string) (*exec.Cmd, error) {
	if strings.TrimSpace(command) == "" {
		return nil, errors.New("shell command is required")
	}
	var program string
	var argv []string
	switch shell {
	case "bash":
		path, err := bashExecutable()
		if err != nil {
			return nil, err
		}
		program = path
		if env["RCODE_SESSION_ENV_FILE"] != "" {
			command = "if [ -f \"$RCODE_SESSION_ENV_FILE\" ]; then source \"$RCODE_SESSION_ENV_FILE\" || exit $?; fi\n" + command
		}
		argv = []string{"--noprofile", "--norc", "-o", "pipefail", "-c", command}
	case "powershell":
		if env["RCODE_SESSION_ENV_FILE"] != "" {
			return nil, errors.New("session Bash environment files are not supported by PowerShell")
		}
		program = "pwsh"
		if _, err := exec.LookPath(program); err != nil {
			program = "powershell"
			if _, err := exec.LookPath(program); err != nil {
				return nil, errors.New("PowerShell shell requires pwsh or powershell on PATH")
			}
		}
		argv = []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", "$ErrorActionPreference='Stop'; $LASTEXITCODE=0;\n" + command + "\nif ($LASTEXITCODE) { exit $LASTEXITCODE }"}
	default:
		return nil, fmt.Errorf("unsupported shell %q; expected bash or powershell", shell)
	}
	cmd := exec.CommandContext(ctx, program, argv...)
	cmd.Dir = cwd
	merged := map[string]string{}
	keys := map[string]string{}
	normalize := func(key string) string {
		if runtime.GOOS == "windows" {
			return strings.ToUpper(key)
		}
		return key
	}
	for _, entry := range os.Environ() {
		name, value, ok := strings.Cut(entry, "=")
		if ok {
			key := normalize(name)
			merged[key] = value
			keys[key] = name
		}
	}
	for name, value := range env {
		if name == "" || strings.ContainsAny(name, "=\x00") || strings.ContainsRune(value, '\x00') {
			return nil, fmt.Errorf("invalid shell environment variable %q", name)
		}
		key := normalize(name)
		if name == "RCODE_SESSION_ENV_FILE" {
			value = filepath.ToSlash(value)
		}
		merged[key] = value
		keys[key] = name
	}
	for key, value := range merged {
		name := keys[key]
		if strings.EqualFold(name, "BASH_ENV") || strings.EqualFold(name, "ENV") {
			continue
		}
		cmd.Env = append(cmd.Env, name+"="+value)
	}
	cmd.WaitDelay = 2 * time.Second
	prepareProcess(cmd)
	return cmd, nil
}

func bashExecutable() (string, error) {
	if runtime.GOOS != "windows" {
		path, err := exec.LookPath("bash")
		if err != nil {
			return "", errors.New("Bash requires bash on PATH")
		}
		return path, nil
	}
	if configured := os.Getenv("RCODE_GIT_BASH_PATH"); configured != "" {
		if info, err := os.Stat(configured); err != nil || info.IsDir() {
			return "", fmt.Errorf("RCODE_GIT_BASH_PATH does not identify a Bash executable: %q", configured)
		}
		return configured, nil
	}
	candidates := []string{}
	if git, err := exec.LookPath("git"); err == nil {
		root := filepath.Dir(filepath.Dir(git))
		candidates = append(candidates, filepath.Join(root, "bin", "bash.exe"))
	}
	for _, root := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs")} {
		if root != "" {
			candidates = append(candidates, filepath.Join(root, "Git", "bin", "bash.exe"))
		}
	}
	if path, err := exec.LookPath("bash"); err == nil {
		// Windows' System32 bash.exe is a WSL launcher and has different path
		// semantics. Do not silently run a Git Bash script through WSL.
		windows := strings.ToLower(filepath.Clean(os.Getenv("WINDIR")))
		if !strings.HasPrefix(strings.ToLower(path), windows+string(filepath.Separator)) {
			candidates = append(candidates, path)
		}
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return candidate, nil
		}
	}
	return "", errors.New("Bash requires Git Bash on Windows; install Git for Windows or set RCODE_GIT_BASH_PATH")
}
