package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gcode/internal/model"
)

// ValidateHookToolOutput checks builtin result shapes before a hook replaces
// them. MCP servers own their output schemas and pass through valid JSON.
func ValidateHookToolOutput(name string, raw json.RawMessage, original any) error {
	if !json.Valid(raw) {
		return errors.New("replacement is not valid JSON")
	}
	if strings.HasPrefix(name, "mcp__") {
		return nil
	}
	var value map[string]any
	if json.Unmarshal(raw, &value) != nil || value == nil {
		return errors.New("builtin tool output must be an object")
	}
	require := func(fields map[string]string) error {
		for key, kind := range fields {
			item, exists := value[key]
			if !exists {
				return fmt.Errorf("%s is required", key)
			}
			if !hookOutputKind(item, kind) {
				return fmt.Errorf("%s must be %s", key, kind)
			}
		}
		return nil
	}
	var err error
	switch name {
	case "Bash":
		err = require(map[string]string{"stdout": "string", "stderr": "string", "interrupted": "bool"})
		if item, exists := value["isImage"]; exists && !hookOutputKind(item, "bool") {
			return errors.New("isImage must be bool")
		}
	case "Write":
		err = require(map[string]string{"type": "string", "filePath": "string", "content": "string", "originalFile": "nullableString", "structuredPatch": "array"})
		if value["type"] != "create" && value["type"] != "update" {
			return errors.New("Write.type must be create or update")
		}
	case "Edit":
		err = require(map[string]string{"filePath": "string", "oldString": "string", "newString": "string", "originalFile": "string", "replaceAll": "bool", "structuredPatch": "array"})
	case "Read":
		if value["type"] != "text" {
			return errors.New("this host supports Read text result rewrites only")
		}
		file, ok := value["file"].(map[string]any)
		if !ok {
			return errors.New("Read.file must be an object")
		}
		for key, kind := range map[string]string{"filePath": "string", "content": "string", "numLines": "number", "startLine": "number", "totalLines": "number"} {
			if !hookOutputKind(file[key], kind) {
				return fmt.Errorf("Read.file.%s must be %s", key, kind)
			}
		}
	default:
		if original == nil {
			return fmt.Errorf("%s does not expose a replaceable output schema on this host", name)
		}
		data, marshalErr := json.Marshal(original)
		if marshalErr != nil {
			return marshalErr
		}
		var sample any
		if err := json.Unmarshal(data, &sample); err != nil {
			return err
		}
		if !hookOutputSameShape(sample, value) {
			return fmt.Errorf("%s replacement does not match its typed result", name)
		}
	}
	if err != nil {
		return err
	}
	if patch, ok := value["structuredPatch"].([]any); ok {
		for _, rawHunk := range patch {
			hunk, ok := rawHunk.(map[string]any)
			if !ok {
				return errors.New("structuredPatch entries must be objects")
			}
			for _, key := range []string{"oldStart", "oldLines", "newStart", "newLines"} {
				if !hookOutputKind(hunk[key], "number") {
					return fmt.Errorf("structuredPatch.%s must be number", key)
				}
			}
			lines, ok := hunk["lines"].([]any)
			if !ok {
				return errors.New("structuredPatch.lines must be an array")
			}
			for _, line := range lines {
				if !hookOutputKind(line, "string") {
					return errors.New("structuredPatch.lines entries must be strings")
				}
			}
		}
	}
	return nil
}

func hookOutputKind(value any, kind string) bool {
	switch kind {
	case "string":
		_, ok := value.(string)
		return ok
	case "nullableString":
		if value == nil {
			return true
		}
		_, ok := value.(string)
		return ok
	case "number":
		_, ok := value.(float64)
		return ok
	case "bool":
		_, ok := value.(bool)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	}
	return false
}

func hookOutputSameShape(sample, value any) bool {
	switch typed := sample.(type) {
	case map[string]any:
		object, ok := value.(map[string]any)
		if !ok {
			return false
		}
		for key, item := range typed {
			candidate, exists := object[key]
			if !exists || !hookOutputSameShape(item, candidate) {
				return false
			}
		}
		return true
	case []any:
		array, ok := value.([]any)
		if !ok {
			return false
		}
		if len(typed) > 0 {
			for _, item := range array {
				if !hookOutputSameShape(typed[0], item) {
					return false
				}
			}
		}
		return true
	case nil:
		return value == nil || hookOutputKind(value, "string")
	case string:
		return hookOutputKind(value, "string")
	case bool:
		return hookOutputKind(value, "bool")
	case float64:
		return hookOutputKind(value, "number")
	default:
		return false
	}
}

func applyHookToolOutput(name string, output model.ToolOutput, raw json.RawMessage) model.ToolOutput {
	if ValidateHookToolOutput(name, raw, output.Data) != nil {
		return output
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return output
	}
	output.Data = value
	output.Images = nil
	// A replacement may redact the original stream. Do not expose a path to
	// the pre-hook content as if it were the approved model output.
	output.OutputFile = ""
	output.HookContext = ""
	if text, ok := value.(string); ok {
		output.Text = text
		return output
	}
	if object, ok := value.(map[string]any); ok {
		if name == "Bash" {
			output.Text = object["stdout"].(string) + object["stderr"].(string)
			return output
		}
		if name == "Read" {
			file := object["file"].(map[string]any)
			var text strings.Builder
			start := int(file["startLine"].(float64))
			lines := strings.Split(strings.TrimSuffix(file["content"].(string), "\n"), "\n")
			for index, line := range lines {
				fmt.Fprintf(&text, "%d: %s\n", start+index, line)
			}
			output.Text = text.String()
			return output
		}
	}
	output.Text = string(raw)
	return output
}
