package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gcode/internal/agent"
	"gcode/internal/sessions"
	"gcode/internal/specialist"
	"gcode/internal/streamjson"
)

func (engine *Engine) childRunner(parent Input, parentCallbacks Callbacks, _ agent.Provider, _ string) specialist.RunChildFunc {
	return func(ctx context.Context, _ string, args []string, progress func(streamjson.Event)) (specialist.ChildRunResult, error) {
		child, err := parseChildArgs(args)
		if err != nil {
			return specialist.ChildRunResult{}, err
		}
		if child.SessionID == "" {
			return specialist.ChildRunResult{}, errors.New("specialist child session id is required")
		}
		if child.Cwd == "" {
			child.Cwd = parent.Cwd
		}
		if child.Model == "" {
			child.Model = parent.Provider.Model
		}
		if child.ProviderID == "" {
			child.ProviderID = parent.Provider.ID
		}
		if err := engine.ensureChildSession(child, parent); err != nil {
			return specialist.ChildRunResult{}, err
		}
		childRunID := newRunID()
		mode := child.PermissionMode
		fullAccess := mode == string(agent.PermissionModeUnsafe)
		if mode == "" {
			mode = string(agent.PermissionModeAuto)
		}
		input := Input{
			RunID:            childRunID,
			SessionID:        child.SessionID,
			CallingSessionID: parent.SessionID,
			CallingToolUseID: parent.CallingToolUseID,
			Depth:            child.Depth,
			Title:            child.Title,
			Prompt:           child.Prompt,
			Cwd:              child.Cwd,
			Provider:         parent.Provider,
			FullAccess:       fullAccess,
			PermissionMode:   mode,
			Autonomy:         child.Autonomy,
			EnabledTools:     child.EnabledTools,
		}
		input.Provider.Model = child.Model
		childEvents := make([]streamjson.Event, 0)
		emit := func(event streamjson.Event) {
			event.RunID = childRunID
			event.SchemaVersion = streamjson.SchemaVersion
			childEvents = append(childEvents, event)
			if progress != nil {
				progress(event)
			}
		}
		childCallbacks := Callbacks{
			OnText: func(value string) {
				emit(streamjson.Event{Type: streamjson.EventText, SessionID: child.SessionID, Delta: value})
			},
			OnReasoning: func(value string) {
				emit(streamjson.Event{Type: streamjson.EventReasoning, SessionID: child.SessionID, Delta: value})
			},
			OnToolCall: func(value ToolCall) {
				emit(streamjson.Event{Type: streamjson.EventToolCall, SessionID: child.SessionID, ID: value.ID, Name: value.Name, Args: value.Arguments})
			},
			OnPermission: func(value PermissionEvent) {
				emit(streamjson.Event{Type: streamjson.EventPermission, SessionID: child.SessionID, ID: value.ID, Name: value.Tool, Action: value.Action, Reason: value.Reason})
			},
			OnToolResult: func(value ToolResult) {
				emit(streamjson.Event{Type: streamjson.EventToolResult, SessionID: child.SessionID, ID: value.ID, Name: value.Name, Status: value.Status, Output: value.Output, ChangedFiles: value.ChangedFiles})
			},
			OnUsage: func(value Usage) {
				prompt, completion, total := value.PromptTokens, value.CompletionTokens, value.TotalTokens
				emit(streamjson.Event{Type: streamjson.EventUsage, SessionID: child.SessionID, PromptTokens: &prompt, CompletionTokens: &completion, TotalTokens: &total})
			},
			OnWarning: func(value error) {
				if value == nil {
					return
				}
				if parentCallbacks.OnWarning != nil {
					parentCallbacks.OnWarning(value)
				}
				emit(streamjson.Event{Type: streamjson.EventWarning, SessionID: child.SessionID, Message: value.Error()})
			},
			OnPermissionRequest: func(context.Context, PermissionRequest) (string, error) {
				return string(agent.PermissionDecisionDeny), errors.New("specialist child cannot request interactive permission")
			},
			OnAskUser: func(context.Context, AskRequest) ([]string, error) {
				return nil, errors.New("specialist child cannot ask the desktop user")
			},
		}
		result, runErr := engine.Run(ctx, input, childCallbacks)
		if answer := strings.TrimSpace(result.FinalAnswer); answer != "" {
			emit(streamjson.Event{Type: streamjson.EventFinal, SessionID: child.SessionID, Text: answer})
		}
		if runErr != nil {
			message := runErr.Error()
			emit(streamjson.Event{Type: streamjson.EventError, SessionID: child.SessionID, Message: message})
			code := 1
			emit(streamjson.Event{Type: streamjson.EventRunEnd, SessionID: child.SessionID, Status: "error", ExitCode: &code})
			return specialist.ChildRunResult{Events: childEvents, Started: true, ExitCode: code}, runErr
		}
		code := 0
		emit(streamjson.Event{Type: streamjson.EventRunEnd, SessionID: child.SessionID, Status: "success", ExitCode: &code})
		return specialist.ChildRunResult{Events: childEvents, Started: true, ExitCode: code}, nil
	}
}

type childArgs struct {
	SessionID      string
	Prompt         string
	Cwd            string
	Model          string
	ProviderID     string
	PermissionMode string
	Autonomy       string
	EnabledTools   []string
	Depth          int
	Title          string
}

func parseChildArgs(args []string) (childArgs, error) {
	child := childArgs{}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		next := func() (string, error) {
			if index+1 >= len(args) {
				return "", fmt.Errorf("missing value after %s", arg)
			}
			index++
			return args[index], nil
		}
		switch arg {
		case "--init-session-id":
			value, err := next()
			if err != nil {
				return child, err
			}
			child.SessionID = value
		case "--model":
			value, err := next()
			if err != nil {
				return child, err
			}
			child.Model = value
		case "--cwd":
			value, err := next()
			if err != nil {
				return child, err
			}
			child.Cwd = value
		case "--auto":
			value, err := next()
			if err != nil {
				return child, err
			}
			child.Autonomy = value
			if value == "high" {
				child.PermissionMode = string(agent.PermissionModeUnsafe)
			} else if value == "member" {
				child.PermissionMode = string(agent.PermissionModeMemberAuto)
			} else {
				child.PermissionMode = string(agent.PermissionModeAuto)
			}
		case "--permission-mode":
			value, err := next()
			if err != nil {
				return child, err
			}
			child.PermissionMode = value
		case "--enabled-tools":
			value, err := next()
			if err != nil {
				return child, err
			}
			child.EnabledTools = splitList(value)
		case "--depth":
			value, err := next()
			if err != nil {
				return child, err
			}
			parsed, parseErr := strconv.Atoi(value)
			if parseErr != nil {
				return child, parseErr
			}
			child.Depth = parsed
		case "--session-title":
			value, err := next()
			if err != nil {
				return child, err
			}
			child.Title = value
		case "--file":
			value, err := next()
			if err != nil {
				return child, err
			}
			data, readErr := os.ReadFile(value)
			if readErr != nil {
				return child, readErr
			}
			child.Prompt = string(data)
		case "--calling-session-id", "--calling-tool-use-id", "--reasoning-effort", "--output-format", "--tag":
			if _, err := next(); err != nil {
				return child, err
			}
		default:
			if strings.HasPrefix(arg, "-") || arg == "exec" || arg == "--resume" {
				continue
			}
			if child.Prompt == "" {
				child.Prompt = arg
			}
		}
	}
	if strings.TrimSpace(child.Prompt) == "" {
		return child, errors.New("specialist child prompt is required")
	}
	return child, nil
}

func splitList(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}

func (engine *Engine) ensureChildSession(child childArgs, parent Input) error {
	if engine.store == nil {
		return nil
	}
	metadata, err := engine.store.Get(child.SessionID)
	if err != nil {
		return err
	}
	if metadata != nil {
		return nil
	}
	_, err = engine.store.Create(sessions.CreateInput{
		SessionID:       child.SessionID,
		SessionKind:     sessions.SessionKindChild,
		Title:           child.Title,
		Cwd:             child.Cwd,
		ModelID:         child.Model,
		Provider:        parent.Provider.ID,
		ParentSessionID: parent.SessionID,
		RootSessionID:   parent.SessionID,
		Depth:           child.Depth,
		Tag:             specialist.SessionTagSpecialist,
	})
	return err
}
