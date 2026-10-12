package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func editorCommand(options EditorOptions, path string) (string, []string, error) {
	commandLine := strings.TrimSpace(options.Command)
	if commandLine == "" {
		commandLine = strings.TrimSpace(os.Getenv("GCODE_EDITOR"))
	}
	if commandLine == "" {
		commandLine = strings.TrimSpace(os.Getenv("VISUAL"))
	}
	if commandLine == "" {
		commandLine = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if commandLine == "" {
		switch runtime.GOOS {
		case "darwin":
			return "open", []string{"-a", "TextEdit", path}, nil
		case "windows":
			return "cmd", []string{"/C", "start", "", path}, nil
		default:
			return "xdg-open", []string{path}, nil
		}
	}
	parts, err := splitCommandLine(commandLine)
	if err != nil {
		return "", nil, err
	}
	if len(parts) == 0 {
		return "", nil, errors.New("editor command is empty")
	}
	args := append([]string{}, parts[1:]...)
	args = append(args, options.Args...)
	found := false
	for index, arg := range args {
		if arg == "{file}" {
			args[index] = path
			found = true
		}
	}
	if !found {
		args = append(args, path)
		return parts[0], args, nil
	}
	return parts[0], args, nil
}

func splitCommandLine(input string) ([]string, error) {
	var parts []string
	var current strings.Builder
	var quote rune
	escaped := false
	for _, character := range input {
		if escaped {
			current.WriteRune(character)
			escaped = false
			continue
		}
		if character == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if character == quote {
				quote = 0
			} else {
				current.WriteRune(character)
			}
			continue
		}
		switch character {
		case '\'', '"':
			quote = character
		case ' ', '\t', '\n':
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(character)
		}
	}
	if escaped {
		current.WriteByte('\\')
	}
	if quote != 0 {
		return nil, errors.New("unterminated quote in editor command")
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts, nil
}

type EditorProcess struct {
	process *os.Process
	command *exec.Cmd
}

type EditorExitState struct {
	code int
}

func (state *EditorExitState) Success() bool {
	return state != nil && state.code == 0
}

func (state *EditorExitState) String() string {
	if state == nil {
		return "unknown status"
	}
	return fmt.Sprintf("exit status %d", state.code)
}

func (p *EditorProcess) Wait() (*EditorExitState, error) {
	if p == nil || p.command == nil {
		return nil, errors.New("editor process is nil")
	}
	err := p.command.Wait()
	if p.command.ProcessState == nil {
		return nil, err
	}
	return &EditorExitState{code: p.command.ProcessState.ExitCode()}, err
}

func launchEditor(ctx context.Context, name string, args ...string) (*EditorProcess, error) {
	command := exec.CommandContext(ctx, name, args...)
	if err := command.Start(); err != nil {
		return nil, err
	}
	return &EditorProcess{process: command.Process, command: command}, nil
}
