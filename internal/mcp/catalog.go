package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"

	"gcode/internal/model"
)

// ToolNameLimitError reports the Claude-qualified namespace boundary.
// Names are never truncated or replaced by an undocumented alias.
type ToolNameLimitError struct {
	Name  string
	Limit int
}

func (e *ToolNameLimitError) Error() string {
	return fmt.Sprintf("MCP qualified tool name %q exceeds namespace limit of %d characters", e.Name, e.Limit)
}

func NormalizeName(name string) string {
	var out strings.Builder
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			out.WriteRune(r)
		} else {
			out.WriteByte('_')
			if utf16.RuneLen(r) == 2 {
				out.WriteByte('_')
			}
		}
	}
	normalized := out.String()
	if strings.HasPrefix(name, "claude.ai ") {
		normalized = strings.Trim(regexp.MustCompile(`_+`).ReplaceAllString(normalized, "_"), "_")
	}
	return normalized
}
func QualifiedToolName(config Config, tool string) (string, error) {
	if strings.TrimSpace(config.Name) == "" || strings.TrimSpace(tool) == "" {
		return "", errors.New("MCP server and tool names must not be empty")
	}
	server := config.Name
	if config.Plugin != "" {
		server = "plugin_" + config.Plugin + "_" + server
	}
	name := "mcp__" + NormalizeName(server) + "__" + NormalizeName(tool)
	if len(name) > 128 {
		return "", &ToolNameLimitError{Name: name, Limit: 128}
	}
	return name, nil
}

type RemoteTool struct {
	client     *Client
	original   string
	definition model.Tool
	headers    []headerProperty
}

// ToolDiagnostic records a tool excluded from the last discovery result.
// A rejected tool does not disable its server's other tools.
type ToolDiagnostic struct {
	Name    string
	Message string
	Length  int
	Limit   int
}

func (c *Client) ToolDiagnostics() []ToolDiagnostic {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]ToolDiagnostic(nil), c.toolDiagnostics...)
}

func (t RemoteTool) Definition() model.Tool { return t.definition }
func (t RemoteTool) Concurrent() bool       { return false }

func (c *Client) Tools(ctx context.Context) (listedTools []RemoteTool, resultErr error) {
	defer func() { resultErr = c.sanitizeError(resultErr) }()
	var diagnostics []ToolDiagnostic
	defer func() {
		c.mu.Lock()
		c.toolDiagnostics = diagnostics
		c.mu.Unlock()
	}()
	var tools []RemoteTool
	cursor := ""
	cursors := map[string]bool{}
	names := map[string]bool{}
	for page := 0; page < 100; page++ {
		var result struct {
			Tools []struct {
				Name        string `json:"name"`
				Title       string `json:"title"`
				Annotations struct {
					Title string `json:"title"`
				} `json:"annotations"`
				Description string         `json:"description"`
				Schema      map[string]any `json:"inputSchema"`
			} `json:"tools"`
			Next string `json:"nextCursor"`
		}
		params := map[string]string{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		if err := c.call(ctx, "tools/list", params, &result); err != nil {
			return nil, err
		}
		for _, tool := range result.Tools {
			if c.scrub(tool.Name) != tool.Name {
				return nil, errors.New("MCP tool name contains configured credentials; remove credentials from server metadata")
			}
			if schemaContainsCredentials(tool.Schema, c.config) || c.valueHasSecrets(tool.Schema) {
				return nil, errors.New("MCP tool schema contains configured credentials; remove credentials from server metadata")
			}
			if tool.Schema == nil {
				return nil, errors.New("MCP tool omitted inputSchema")
			}
			name, err := QualifiedToolName(c.config, tool.Name)
			if err != nil {
				var limit *ToolNameLimitError
				if errors.As(err, &limit) {
					diagnostic := ToolDiagnostic{Name: c.scrub(limit.Name), Message: c.scrub(err.Error()), Length: len(limit.Name), Limit: limit.Limit}
					diagnostics = append(diagnostics, diagnostic)
					c.publish("gcode/tool_rejected", diagnostic)
					continue
				}
				return nil, err
			}
			if names[name] {
				return nil, errors.New("MCP tools have colliding qualified names after normalization")
			}
			names[name] = true
			var headers []headerProperty
			if c.ProtocolVersion() == ModernProtocolVersion && c.config.URL != "" {
				headers, err = collectHeaderProperties(tool.Schema)
				if err != nil {
					c.publish("gcode/tool_rejected", map[string]any{"name": tool.Name, "reason": err.Error()})
					continue
				}
			}
			title := strings.TrimSpace(tool.Title)
			if title == "" {
				title = strings.TrimSpace(tool.Annotations.Title)
			}
			if title == "" {
				title = tool.Name
			}
			service := c.config.Name
			if c.config.Plugin != "" {
				service = c.config.Plugin + " · " + service
			}
			tools = append(tools, RemoteTool{client: c, original: tool.Name, headers: headers, definition: model.Tool{
				Name: name, DisplayName: c.scrub(service + " · " + title),
				Description: c.scrub(tool.Description), Parameters: tool.Schema,
			}})
		}
		if result.Next == "" {
			return tools, nil
		}
		if cursors[result.Next] {
			return nil, errors.New("MCP tool pagination repeated a cursor")
		}
		cursors[result.Next] = true
		cursor = result.Next
	}
	return nil, errors.New("MCP tool listing exceeded page limit")
}
func (c *Client) valueHasSecrets(value any) bool {
	switch v := value.(type) {
	case string:
		return c.scrub(v) != v
	case map[string]any:
		for k, child := range v {
			if c.scrub(k) != k || c.valueHasSecrets(child) {
				return true
			}
		}
	case []any:
		for _, child := range v {
			if c.valueHasSecrets(child) {
				return true
			}
		}
	}
	return false
}
func (c *Client) publish(method string, params any) {
	raw, _ := json.Marshal(params)
	c.enqueueNotification(Notification{Method: c.scrub(method), Params: c.sanitizeJSON(raw)})
}

// Resource is server metadata; reading it does not open a local file or URL.
type Resource struct {
	URI         string         `json:"uri"`
	Name        string         `json:"name"`
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description,omitempty"`
	MIMEType    string         `json:"mimeType,omitempty"`
	Size        int64          `json:"size,omitempty"`
	Metadata    map[string]any `json:"_meta,omitempty"`
}
type ResourceTemplate struct {
	URITemplate string `json:"uriTemplate"`
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	MIMEType    string `json:"mimeType,omitempty"`
}
type ResourceContents struct {
	URI      string         `json:"uri"`
	MIMEType string         `json:"mimeType,omitempty"`
	Text     string         `json:"text,omitempty"`
	Blob     string         `json:"blob,omitempty"`
	Metadata map[string]any `json:"_meta,omitempty"`
}
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}
type Prompt struct {
	Name        string           `json:"name"`
	Title       string           `json:"title,omitempty"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
}
type PromptMessage struct {
	Role    string  `json:"role"`
	Content Content `json:"content"`
}
type PromptResult struct {
	Description string          `json:"description,omitempty"`
	Messages    []PromptMessage `json:"messages"`
}

func listEntries[T any](ctx context.Context, c *Client, method, key string) ([]T, error) {
	var entries []T
	cursor := ""
	seen := map[string]bool{}
	for page := 0; page < 100; page++ {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		var result map[string]json.RawMessage
		if err := c.call(ctx, method, params, &result); err != nil {
			return nil, err
		}
		var values []T
		raw := result[key]
		if len(raw) == 0 {
			return nil, errors.New("MCP listing omitted its entries")
		}
		if err := json.Unmarshal(c.sanitizeJSON(raw), &values); err != nil {
			return nil, c.sanitizeError(err)
		}
		entries = append(entries, values...)
		var next string
		if raw := result["nextCursor"]; len(raw) > 0 {
			if err := json.Unmarshal(raw, &next); err != nil {
				return nil, err
			}
		}
		if next == "" {
			return entries, nil
		}
		if seen[next] {
			return nil, errors.New("MCP pagination repeated a cursor")
		}
		seen[next] = true
		cursor = next
	}
	return nil, errors.New("MCP listing exceeded page limit")
}
func (c *Client) Resources(ctx context.Context) ([]Resource, error) {
	return listEntries[Resource](ctx, c, "resources/list", "resources")
}
func (c *Client) ResourceTemplates(ctx context.Context) ([]ResourceTemplate, error) {
	return listEntries[ResourceTemplate](ctx, c, "resources/templates/list", "resourceTemplates")
}
func (c *Client) ReadResource(ctx context.Context, uri string) ([]ResourceContents, error) {
	var result struct {
		Contents []ResourceContents `json:"contents"`
	}
	var raw json.RawMessage
	if err := c.call(ctx, "resources/read", map[string]string{"uri": uri}, &raw); err != nil {
		return nil, err
	}
	safe, err := c.sanitizeContentJSON(raw)
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(safe, &result); err != nil {
		return nil, c.sanitizeError(err)
	}
	return result.Contents, nil
}
func (c *Client) Prompts(ctx context.Context) ([]Prompt, error) {
	return listEntries[Prompt](ctx, c, "prompts/list", "prompts")
}
func (c *Client) GetPrompt(ctx context.Context, name string, arguments map[string]string) (PromptResult, error) {
	var result PromptResult
	var raw json.RawMessage
	if err := c.call(ctx, "prompts/get", map[string]any{"name": name, "arguments": arguments}, &raw); err != nil {
		return result, err
	}
	safe, err := c.sanitizeContentJSON(raw)
	if err != nil {
		return result, err
	}
	err = json.Unmarshal(safe, &result)
	return result, c.sanitizeError(err)
}
func (c *Client) SubscribeResource(ctx context.Context, uri string) error {
	if c.ProtocolVersion() == ModernProtocolVersion {
		return errors.New("modern MCP resources use ListenNotifications resourceSubscriptions")
	}
	return c.call(ctx, "resources/subscribe", map[string]string{"uri": uri}, &struct{}{})
}
func (c *Client) UnsubscribeResource(ctx context.Context, uri string) error {
	if c.ProtocolVersion() == ModernProtocolVersion {
		return errors.New("modern MCP resources stop subscriptions by cancelling ListenNotifications")
	}
	return c.call(ctx, "resources/unsubscribe", map[string]string{"uri": uri}, &struct{}{})
}

type headerProperty struct {
	name string
	path []string
	kind string
}

var headerToken = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]+$")

func collectHeaderProperties(schema map[string]any) ([]headerProperty, error) {
	var out []headerProperty
	seen := map[string]bool{}
	var visit func(any, []string, bool, int) error
	visit = func(value any, path []string, reachable bool, depth int) error {
		if depth > 64 {
			return errors.New("MCP input schema exceeds header inspection depth")
		}
		switch v := value.(type) {
		case map[string]any:
			if annotation, exists := v["x-mcp-header"]; exists {
				name, ok := annotation.(string)
				kind, _ := v["type"].(string)
				if !ok || !headerToken.MatchString(name) || !reachable || len(path) == 0 || (kind != "string" && kind != "integer" && kind != "boolean") || seen[strings.ToLower(name)] {
					return errors.New("MCP tool has an invalid x-mcp-header annotation")
				}
				seen[strings.ToLower(name)] = true
				out = append(out, headerProperty{name: name, path: append([]string(nil), path...), kind: kind})
			}
			for key, child := range v {
				if key == "properties" {
					if props, ok := child.(map[string]any); ok {
						for name, property := range props {
							if err := visit(property, append(append([]string(nil), path...), name), reachable, depth+1); err != nil {
								return err
							}
						}
					}
					continue
				}
				if err := visit(child, path, false, depth+1); err != nil {
					return err
				}
			}
		case []any:
			for _, child := range v {
				if err := visit(child, path, false, depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	err := visit(schema, nil, true, 0)
	return out, err
}
func toolHeaderValues(properties []headerProperty, arguments map[string]any) (http.Header, error) {
	headers := http.Header{}
	for _, property := range properties {
		var value any = arguments
		present := true
		for _, part := range property.path {
			object, ok := value.(map[string]any)
			if !ok {
				present = false
				break
			}
			value, present = object[part]
			if !present {
				break
			}
		}
		if !present || value == nil {
			continue
		}
		var text string
		switch property.kind {
		case "string":
			v, ok := value.(string)
			if !ok {
				return nil, errors.New("MCP header parameter must be a string")
			}
			text = v
		case "boolean":
			v, ok := value.(bool)
			if !ok {
				return nil, errors.New("MCP header parameter must be boolean")
			}
			text = strconv.FormatBool(v)
		case "integer":
			v, ok := value.(json.Number)
			if !ok {
				return nil, errors.New("MCP header parameter must be an integer")
			}
			n, err := strconv.ParseInt(string(v), 10, 64)
			if err != nil || n > 9007199254740991 || n < -9007199254740991 {
				return nil, errors.New("MCP header integer is outside the safe range")
			}
			text = strconv.FormatInt(n, 10)
		}
		headers.Set("Mcp-Param-"+property.name, headerValue(text))
	}
	return headers, nil
}
