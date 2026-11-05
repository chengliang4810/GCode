package agent

import (
	"encoding/json"
	"regexp"
	"strings"

	"gcode/internal/contracts"
)

var hookInputPlaceholder = regexp.MustCompile(`\$\{([A-Za-z0-9_.]+)\}`)

func substituteHookInput(raw json.RawMessage, input HookInput) (json.RawMessage, error) {
	var arguments any
	if err := json.Unmarshal(raw, &arguments); err != nil {
		return nil, err
	}
	data, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	var visit func(any) any
	visit = func(value any) any {
		switch typed := value.(type) {
		case string:
			return hookInputPlaceholder.ReplaceAllStringFunc(typed, func(placeholder string) string {
				path := strings.TrimSuffix(strings.TrimPrefix(placeholder, "${"), "}")
				var field any = fields
				for _, segment := range strings.Split(path, ".") {
					object, ok := field.(map[string]any)
					if !ok {
						return ""
					}
					field = object[segment]
				}
				if field == nil {
					return ""
				}
				if text, ok := field.(string); ok {
					return text
				}
				encoded, err := json.Marshal(field)
				if err != nil {
					return ""
				}
				return string(encoded)
			})
		case map[string]any:
			for key, item := range typed {
				typed[key] = visit(item)
			}
		case []any:
			for i, item := range typed {
				typed[i] = visit(item)
			}
		}
		return value
	}
	return json.Marshal(visit(arguments))
}

type hookCondition struct {
	source   string
	wildcard *regexp.Regexp
}

func compileHookCondition(condition string) hookCondition {
	result := hookCondition{source: condition}
	_, pattern, ok := strings.Cut(condition, "(")
	if ok && strings.HasSuffix(pattern, ")") {
		pattern = strings.TrimSuffix(pattern, ")")
		expression := "^" + strings.ReplaceAll(regexp.QuoteMeta(pattern), `\*`, ".*") + "$"
		result.wildcard, _ = regexp.Compile(expression)
	}
	return result
}

// The table belongs to a runner's current rules. Replacing it on rule changes
// keeps active Run snapshots immutable and releases patterns of popped skills.
func compileHookConditions(rules []contracts.HookRule) map[string]hookCondition {
	result := make(map[string]hookCondition, len(rules))
	for _, rule := range rules {
		if _, exists := result[rule.If]; !exists {
			result[rule.If] = compileHookCondition(rule.If)
		}
	}
	return result
}

func (condition hookCondition) matches(input HookInput) bool {
	source := condition.source
	if source == "" {
		return true
	}
	switch input.Event {
	case contracts.HookPreToolUse, contracts.HookPostToolUse, contracts.HookPostToolUseFailure,
		contracts.HookPermissionRequest, contracts.HookPermissionDenied:
	default:
		return false
	}
	name, pattern, hasPattern := strings.Cut(source, "(")
	if !hasPattern {
		return strings.TrimSpace(source) == input.ToolName || source == "*"
	}
	if !strings.HasSuffix(pattern, ")") || strings.TrimSpace(name) != input.ToolName {
		return false
	}
	pattern = strings.TrimSuffix(pattern, ")")
	var args map[string]any
	if json.Unmarshal(input.ToolInput, &args) != nil {
		return false
	}
	switch input.ToolName {
	case "Bash", "PowerShell":
		command, _ := args["command"].(string)
		_, tail, hasTail := strings.Cut(strings.TrimSpace(pattern), " ")
		if hasTail && strings.TrimSpace(tail) != "*" && hookShellExpansion(command) {
			return true
		}
		for _, part := range hookShellSubcommands(command) {
			part = stripHookShellAssignments(part)
			first, _, _ := strings.Cut(part, " ")
			if strings.HasPrefix(first, "$") || strings.HasPrefix(first, "`") || first == "eval" || first == "source" || first == "env" {
				return true
			}
			if strings.HasSuffix(pattern, ":*") {
				prefix := strings.TrimSuffix(pattern, ":*")
				if part == prefix || strings.HasPrefix(part, prefix+" ") {
					return true
				}
			} else if condition.wildcard != nil && condition.wildcard.MatchString(part) {
				return true
			}
		}
	default:
		for _, key := range []string{"file_path", "path", "url", "query"} {
			if value, ok := args[key].(string); ok && condition.wildcard != nil && condition.wildcard.MatchString(strings.ReplaceAll(value, `\`, "/")) {
				return true
			}
		}
	}
	return false
}

func stripHookShellAssignments(command string) string {
	for {
		command = strings.TrimSpace(command)
		end := len(command)
		var quote byte
		escaped := false
		for i := 0; i < len(command); i++ {
			c := command[i]
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' && quote != '\'' {
				escaped = true
				continue
			}
			if c == '\'' || c == '"' {
				if quote == 0 {
					quote = c
				} else if quote == c {
					quote = 0
				}
				continue
			}
			if quote == 0 && (c == ' ' || c == '\t' || c == '\n') {
				end = i
				break
			}
		}
		name, _, assignment := strings.Cut(command[:end], "=")
		if !assignment || name == "" {
			return command
		}
		for i, c := range name {
			if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '_' || i > 0 && c >= '0' && c <= '9') {
				return command
			}
		}
		command = command[end:]
	}
}

func hookShellExpansion(command string) bool {
	var quote rune
	escaped := false
	for _, c := range command {
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if c == '\'' || c == '"' {
			if quote == 0 {
				quote = c
			} else if quote == c {
				quote = 0
			}
			continue
		}
		if quote != '\'' && (c == '$' || c == '`') {
			return true
		}
	}
	return false
}

// Split only syntactic shell separators, retaining quoted literal text. Command
// substitutions are additional candidate commands, never executed by matching.
func hookShellSubcommands(command string) []string {
	var result []string
	start := 0
	var quote rune
	escaped := false
	for i, c := range command {
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if c == '\'' || c == '"' || c == '`' {
			if quote == 0 {
				quote = c
			} else if quote == c {
				quote = 0
			}
			continue
		}
		if quote == 0 && (c == ';' || c == '|' || c == '&' || c == '\n') {
			if part := strings.TrimSpace(command[start:i]); part != "" {
				result = append(result, part)
			}
			start = i + 1
		}
	}
	if part := strings.TrimSpace(command[start:]); part != "" {
		result = append(result, part)
	}
	// Nested substitutions are extracted by a small balanced scanner. Ignore
	// single-quoted text; expansion in double quotes still executes commands.
	quote, escaped = 0, false
	for i := 0; i < len(command); i++ {
		c := command[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if c == '\'' || c == '"' {
			if quote == 0 {
				quote = rune(c)
			} else if quote == rune(c) {
				quote = 0
			}
			continue
		}
		if quote == '\'' {
			continue
		}
		if c == '`' {
			end := strings.IndexByte(command[i+1:], '`')
			if end >= 0 {
				result = append(result, hookShellSubcommands(command[i+1:i+1+end])...)
				i += end + 1
			}
		} else if c == '$' && i+1 < len(command) && command[i+1] == '(' {
			depth, end := 1, i+2
			for ; end < len(command); end++ {
				if command[end] == '(' {
					depth++
				}
				if command[end] == ')' {
					depth--
					if depth == 0 {
						break
					}
				}
			}
			if depth == 0 {
				result = append(result, hookShellSubcommands(command[i+2:end])...)
				i = end
			}
		}
	}
	return result
}
