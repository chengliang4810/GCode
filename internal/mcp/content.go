package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"maps"
	"net/url"
	"strings"

	"gcode/internal/model"
	_ "golang.org/x/image/webp"
)

// Content retains all server-defined fields, including image/audio payloads,
// embedded resources, resource links, annotations and extension metadata.
type Content map[string]any

type CallResult struct {
	Content           []Content      `json:"content"`
	StructuredContent any            `json:"structuredContent,omitempty"`
	IsError           bool           `json:"isError,omitempty"`
	Metadata          map[string]any `json:"_meta,omitempty"`
	// Raw preserves top-level extension fields as well as recognized content.
	Raw map[string]any `json:"-"`
}

func (result CallResult) MarshalJSON() ([]byte, error) {
	value := maps.Clone(result.Raw)
	if value == nil {
		value = map[string]any{}
	}
	value["content"] = result.Content
	if result.StructuredContent != nil {
		value["structuredContent"] = result.StructuredContent
	}
	if result.IsError {
		value["isError"] = true
	}
	if result.Metadata != nil {
		value["_meta"] = result.Metadata
	}
	return json.Marshal(value)
}

type ToolError struct{}

func (*ToolError) Error() string { return "MCP tool returned an error" }

func (c *Client) CallTool(ctx context.Context, name string, arguments map[string]any) (CallResult, error) {
	var raw json.RawMessage
	if err := c.call(ctx, "tools/call", map[string]any{"name": name, "arguments": arguments}, &raw); err != nil {
		return CallResult{}, err
	}
	safe, err := c.sanitizeContentJSON(raw)
	if err != nil {
		return CallResult{}, err
	}
	var result CallResult
	decoder := json.NewDecoder(bytes.NewReader(safe))
	decoder.UseNumber()
	if err = decoder.Decode(&result); err != nil {
		return CallResult{}, c.sanitizeError(err)
	}
	decoder = json.NewDecoder(bytes.NewReader(safe))
	decoder.UseNumber()
	if err = decoder.Decode(&result.Raw); err != nil {
		return CallResult{}, c.sanitizeError(err)
	}
	if result.IsError {
		return result, &ToolError{}
	}
	return result, nil
}
func (t RemoteTool) Execute(ctx context.Context, _ string, args json.RawMessage) (output model.ToolOutput, resultErr error) {
	defer func() { resultErr = t.client.sanitizeError(resultErr); output.Text = t.client.scrub(output.Text) }()
	decoder := json.NewDecoder(bytes.NewReader(args))
	decoder.UseNumber()
	var arguments map[string]any
	if err := decoder.Decode(&arguments); err != nil {
		return output, err
	}
	if arguments == nil {
		return output, errors.New("MCP tool arguments must be an object")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return output, errors.New("MCP tool arguments contain trailing JSON")
	}
	if len(t.headers) > 0 {
		headers, err := toolHeaderValues(t.headers, arguments)
		if err != nil {
			return output, err
		}
		ctx = context.WithValue(ctx, toolHeadersKey{}, headers)
	}
	result, err := t.client.CallTool(ctx, t.original, arguments)
	if err != nil && !result.IsError {
		return output, err
	}
	output.Data = result
	var text strings.Builder
	for _, content := range result.Content {
		kind, _ := content["type"].(string)
		switch kind {
		case "text":
			value, _ := content["text"].(string)
			text.WriteString(value)
			text.WriteByte('\n')
		case "image":
			img, imageErr := contentImage(content)
			if imageErr != nil {
				return model.ToolOutput{}, imageErr
			}
			output.Images = append(output.Images, img)
		case "resource":
			resource, _ := content["resource"].(map[string]any)
			if resource == nil {
				return model.ToolOutput{}, errors.New("MCP embedded resource omitted its contents")
			}
			if value, ok := resource["text"].(string); ok {
				text.WriteString(value)
				text.WriteByte('\n')
			}
			if blob, ok := resource["blob"].(string); ok {
				mimeType, _ := resource["mimeType"].(string)
				if strings.HasPrefix(mimeType, "image/") {
					img, imageErr := contentImage(map[string]any{"data": blob, "mimeType": mimeType})
					if imageErr != nil {
						return model.ToolOutput{}, imageErr
					}
					output.Images = append(output.Images, img)
				} else {
					fmt.Fprintf(&text, "[MCP resource %s (%s); binary content retained in tool response]\n", stringField(resource, "uri"), mimeType)
				}
			}
		case "resource_link":
			fmt.Fprintf(&text, "[MCP resource link: %s — %s]\n", stringField(content, "name"), stringField(content, "uri"))
		case "audio":
			fmt.Fprintf(&text, "[MCP audio (%s); audio content retained in tool response]\n", stringField(content, "mimeType"))
		default:
			// Preserve an extension block, and make its presence visible to models that
			// only consume text/images. Never pretend an unsupported modality was read.
			fmt.Fprintf(&text, "[MCP %s content retained in tool response]\n", kind)
		}
	}
	if result.StructuredContent != nil {
		encoded, encodeErr := json.Marshal(result.StructuredContent)
		if encodeErr != nil {
			return model.ToolOutput{}, encodeErr
		}
		text.Write(encoded)
	}
	output.Text = text.String()
	return output, err
}
func stringField(value map[string]any, key string) string { s, _ := value[key].(string); return s }
func contentImage(content map[string]any) (model.Image, error) {
	data, ok := content["data"].(string)
	if !ok {
		return model.Image{}, errors.New("MCP image omitted base64 data")
	}
	mimeType, _ := content["mimeType"].(string)
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil || len(raw) > 16<<20 {
		return model.Image{}, errors.New("MCP image has invalid or oversized base64 data")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return model.Image{}, errors.New("MCP image payload is not a supported image")
	}
	expected := map[string]string{"png": "image/png", "jpeg": "image/jpeg", "gif": "image/gif", "webp": "image/webp"}[format]
	if mimeType != expected && !(mimeType == "image/jpg" && expected == "image/jpeg") {
		return model.Image{}, errors.New("MCP image MIME type does not match its payload")
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 64_000_000 {
		return model.Image{}, errors.New("MCP image dimensions exceed the supported limit")
	}
	return model.Image{MIME: expected, Data: data, Width: config.Width, Height: config.Height}, nil
}

// Only binary data/blob fields escape text replacement, after strict decoding.
// Configured secrets found in decoded payloads are rejected rather than silently
// corrupting an image or returning credentials in another encoding.
func (c *Client) sanitizeContentJSON(raw json.RawMessage) (json.RawMessage, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, c.sanitizeError(err)
	}
	safe, err := c.sanitizeContentValue(value, 0)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(safe)
	return encoded, c.sanitizeError(err)
}
func (c *Client) sanitizeContentValue(value any, depth int) (any, error) {
	if depth > 128 {
		return nil, errors.New("MCP content nesting exceeds limit")
	}
	switch v := value.(type) {
	case string:
		return c.scrub(v), nil
	case []any:
		for i, child := range v {
			safe, err := c.sanitizeContentValue(child, depth+1)
			if err != nil {
				return nil, err
			}
			v[i] = safe
		}
		return v, nil
	case map[string]any:
		out := map[string]any{}
		for key, child := range v {
			if (key == "data" || key == "blob") && isBinaryContent(v, key) {
				encoded, ok := child.(string)
				if !ok {
					return nil, errors.New("MCP binary content must contain base64 text")
				}
				decoded, err := base64.StdEncoding.DecodeString(encoded)
				if err != nil || len(decoded) > 16<<20 {
					return nil, errors.New("MCP binary content is invalid or oversized")
				}
				if c.binaryHasSecrets(decoded) {
					return nil, errors.New("MCP binary content contains configured credentials")
				}
				out[key] = encoded
				continue
			}
			safe, err := c.sanitizeContentValue(child, depth+1)
			if err != nil {
				return nil, err
			}
			out[c.scrub(key)] = safe
		}
		return out, nil
	default:
		return value, nil
	}
}
func isBinaryContent(value map[string]any, key string) bool {
	if key == "blob" {
		_, ok := value["uri"].(string)
		return ok
	}
	kind, _ := value["type"].(string)
	return kind == "image" || kind == "audio"
}
func (c *Client) binaryHasSecrets(data []byte) bool {
	secrets := append(argumentSecrets(c.config.Args), c.config.SensitiveValues...)
	sensitiveName := func(name string) bool {
		name = strings.ToLower(name)
		for _, part := range []string{"secret", "token", "password", "authorization", "cookie", "api_key", "api-key", "apikey", "access_key", "access-key"} {
			if strings.Contains(name, part) {
				return true
			}
		}
		return name == "key" || name == "auth"
	}
	for name, value := range c.config.Env {
		if len(value) >= 8 || sensitiveName(name) {
			secrets = append(secrets, value)
		}
	}
	for name, value := range c.config.Headers {
		if len(value) >= 8 || sensitiveName(name) {
			secrets = append(secrets, value)
			if _, token, ok := strings.Cut(value, " "); ok {
				secrets = append(secrets, token)
			}
		}
	}
	if parsed, err := url.Parse(c.config.URL); err == nil {
		for name, values := range parsed.Query() {
			for _, value := range values {
				if len(value) >= 8 || sensitiveName(name) {
					secrets = append(secrets, value)
				}
			}
		}
	}
	c.mu.Lock()
	secrets = append(secrets, c.secrets...)
	c.mu.Unlock()
	for _, secret := range secrets {
		if secret != "" && bytes.Contains(data, []byte(secret)) {
			return true
		}
	}
	return false
}
