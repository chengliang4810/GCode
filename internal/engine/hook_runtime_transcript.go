package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
)

func (e *Engine) writeHookTranscript(sessionID string) (string, error) {
	redact := e.hookTextRedactor(contracts.HookRule{})
	e.mu.Lock()
	index := e.sessionIndex(sessionID)
	if index < 0 {
		e.mu.Unlock()
		return "", errors.New("hook transcript session was not found")
	}
	rootID := e.state.Sessions[index].StorageRootID
	history := model.CloneMessages(e.history[sessionID])
	e.mu.Unlock()
	root, err := e.storage.Resolve(rootID)
	if err != nil {
		return "", fmt.Errorf("resolve hook transcript storage root: %w", err)
	}
	var data bytes.Buffer
	encoder := json.NewEncoder(&data)
	for index, message := range history {
		role := message.Role
		if role == "tool" {
			role = "user"
		}
		content := make([]map[string]any, 0)
		if message.Role == "tool" {
			content = append(content, map[string]any{"type": "tool_result", "tool_use_id": message.CallID, "content": redact(message.Text), "is_error": message.IsError})
		} else {
			if message.Text != "" {
				content = append(content, map[string]any{"type": "text", "text": redact(message.Text)})
			}
			for _, call := range message.Calls {
				var input any
				if err := json.Unmarshal([]byte(call.Arguments), &input); err != nil {
					input = map[string]any{"unparsed_arguments": call.Arguments}
				}
				content = append(content, map[string]any{"type": "tool_use", "id": call.ID, "name": call.Name, "input": redactHookTranscriptValue(input, redact)})
			}
		}
		record := map[string]any{"type": role, "uuid": sessionID + "-" + strconv.Itoa(index), "sessionId": sessionID,
			"message": map[string]any{"role": role, "content": content}}
		if index > 0 {
			record["parentUuid"] = sessionID + "-" + strconv.Itoa(index-1)
		} else {
			record["parentUuid"] = nil
		}
		if err := encoder.Encode(record); err != nil {
			return "", fmt.Errorf("encode hook transcript: %w", err)
		}
	}
	if filepath.Base(sessionID) != sessionID || !filepath.IsLocal(sessionID) || sessionID == "." {
		return "", errors.New("invalid hook transcript session identifier")
	}
	handle, err := os.OpenRoot(root)
	if err != nil {
		return "", err
	}
	defer handle.Close()
	folder, err := openSessionDirectory(handle, sessionID)
	if err != nil {
		return "", err
	}
	defer folder.Close()
	var file *os.File
	var temporary string
	for {
		temporary = ".transcript-" + newShortID()
		file, err = folder.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		break
	}
	defer folder.Remove(temporary)
	if _, err := file.WriteString(data.String()); err != nil {
		_ = file.Close()
		return "", err
	}
	if err := errors.Join(file.Sync(), file.Close()); err != nil {
		return "", err
	}
	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		err = folder.Rename(temporary, "hook-transcript.jsonl")
		if err == nil {
			return filepath.Join(root, "sessions", sessionID, "hook-transcript.jsonl"), nil
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("replace hook transcript: %w", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func redactHookTranscriptValue(value any, redact func(string) string) any {
	switch typed := value.(type) {
	case string:
		return redact(typed)
	case map[string]any:
		for key, item := range typed {
			typed[key] = redactHookTranscriptValue(item, redact)
		}
	case []any:
		for index, item := range typed {
			typed[index] = redactHookTranscriptValue(item, redact)
		}
	}
	return value
}
