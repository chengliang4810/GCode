package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxMessageBytes = 24 << 20

type HTTPStatusError struct{ StatusCode int }

func (e *HTTPStatusError) Error() string { return fmt.Sprintf("MCP HTTP %d", e.StatusCode) }

func (c *Client) newRequest(ctx context.Context, method, target string, data []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if len(data) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range c.config.Headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("MCP-Protocol-Version", c.ProtocolVersion())
	c.mu.Lock()
	session, token := c.session, c.token
	c.mu.Unlock()
	if session != "" && c.ProtocolVersion() != ModernProtocolVersion {
		req.Header.Set("Mcp-Session-Id", session)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req, nil
}
func (c *Client) do(req *http.Request) (*http.Response, error) {
	response, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusUnauthorized && response.StatusCode != http.StatusForbidden {
		return response, nil
	}
	challenge := challengeFromResponse(c.config.Name, c.config.URL, response)
	if c.options.TokenProvider != nil && response.StatusCode == http.StatusUnauthorized {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		_ = response.Body.Close()
		token, err := c.options.TokenProvider(req.Context(), challenge)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(token) == "" {
			return nil, c.authRequired(challenge)
		}
		c.mu.Lock()
		c.token = token
		c.secrets = append(c.secrets, token)
		c.mu.Unlock()
		retry := req.Clone(req.Context())
		retry.Header = req.Header.Clone()
		retry.Header.Set("Authorization", "Bearer "+token)
		if req.GetBody != nil {
			retry.Body, err = req.GetBody()
			if err != nil {
				return nil, err
			}
		}
		response, err = c.http.Do(retry)
		if err != nil {
			return nil, err
		}
		if response.StatusCode != http.StatusUnauthorized && response.StatusCode != http.StatusForbidden {
			return response, nil
		}
		challenge = challengeFromResponse(c.config.Name, c.config.URL, response)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	_ = response.Body.Close()
	// Challenge fields are UI-facing. Do not expose configured query credentials.
	challenge.ResourceURL = c.scrub(challenge.ResourceURL)
	challenge.ResourceMetadataURL = c.scrub(challenge.ResourceMetadataURL)
	challenge.Scope = c.scrub(challenge.Scope)
	return nil, c.authRequired(challenge)
}
func (c *Client) post(ctx context.Context, message envelope) (envelope, error) {
	data, err := json.Marshal(message)
	if err != nil {
		return envelope{}, err
	}
	if len(data) > maxMessageBytes {
		return envelope{}, errors.New("MCP request exceeds message size limit")
	}
	c.mu.Lock()
	endpoint := c.endpoint
	c.mu.Unlock()
	req, err := c.newRequest(ctx, http.MethodPost, endpoint, data)
	if err != nil {
		return envelope{}, err
	}
	if c.ProtocolVersion() == ModernProtocolVersion {
		req.Header.Set("Mcp-Method", message.Method)
		if params, ok := message.Params.(map[string]any); ok {
			if name, ok := params["name"].(string); ok {
				req.Header.Set("Mcp-Name", headerValue(name))
			} else if uri, ok := params["uri"].(string); ok {
				req.Header.Set("Mcp-Name", headerValue(uri))
			}
		}
		if headers, ok := ctx.Value(toolHeadersKey{}).(http.Header); ok {
			for key, values := range headers {
				req.Header[key] = values
			}
		}
	}
	response, err := c.do(req)
	if err != nil {
		return envelope{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, maxMessageBytes+1))
		var rpc envelope
		if len(raw) <= maxMessageBytes && json.Unmarshal(raw, &rpc) == nil && rpc.Error != nil && rpc.ID == message.ID {
			return rpc, nil
		}
		return envelope{}, &HTTPStatusError{StatusCode: response.StatusCode}
	}
	if value := response.Header.Get("Mcp-Session-Id"); value != "" && c.ProtocolVersion() != ModernProtocolVersion {
		c.mu.Lock()
		c.session = value
		c.mu.Unlock()
	}
	if message.ID == 0 || c.config.Transport == "sse" {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return envelope{}, nil
	}
	media, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if media == "text/event-stream" {
		var result envelope
		found := false
		err = readSSE(response.Body, func(_ string, data []byte) (bool, error) {
			var msg envelope
			if err := json.Unmarshal(data, &msg); err != nil {
				return false, errors.New("MCP stream contains invalid JSON")
			}
			if msg.Method == "" && msg.ID == message.ID {
				result = msg
				found = true
				return true, nil
			}
			return false, c.dispatch(data)
		})
		if err != nil {
			return envelope{}, err
		}
		if !found {
			return envelope{}, errors.New("MCP stream ended without its response")
		}
		return result, nil
	}
	if media != "application/json" {
		return envelope{}, errors.New("MCP response requires application/json or text/event-stream")
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxMessageBytes+1))
	if err != nil {
		return envelope{}, err
	}
	if len(raw) > maxMessageBytes {
		return envelope{}, errors.New("MCP response exceeds message size limit")
	}
	var result envelope
	if err = json.Unmarshal(raw, &result); err != nil {
		return envelope{}, errors.New("MCP response contains invalid JSON")
	}
	if result.JSONRPC != "2.0" || result.ID != message.ID {
		return envelope{}, errors.New("MCP response ID or JSON-RPC version mismatch")
	}
	return result, nil
}
func (c *Client) sendHTTP(ctx context.Context, message any) error {
	raw, err := json.Marshal(message)
	if err != nil {
		return err
	}
	c.mu.Lock()
	endpoint := c.endpoint
	c.mu.Unlock()
	req, err := c.newRequest(ctx, http.MethodPost, endpoint, raw)
	if err != nil {
		return err
	}
	response, err := c.do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return &HTTPStatusError{StatusCode: response.StatusCode}
	}
	return nil
}

// readSSE assembles complete events, including multiline data, before decoding
// JSON. A long-lived stream has a per-event bound rather than a lifetime byte cap.
func readSSE(reader io.Reader, consume func(string, []byte) (bool, error)) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), maxMessageBytes)
	event := ""
	var data strings.Builder
	flush := func() (bool, error) {
		if data.Len() == 0 {
			event = ""
			return false, nil
		}
		payload := strings.TrimSuffix(data.String(), "\n")
		done, err := consume(event, []byte(payload))
		event = ""
		data.Reset()
		return done, err
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			done, err := flush()
			if done || err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		key, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch key {
		case "event":
			event = value
		case "data":
			if data.Len()+len(value)+1 > maxMessageBytes {
				return errors.New("MCP SSE event exceeds message size limit")
			}
			data.WriteString(value)
			data.WriteByte('\n')
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	_, err := flush()
	return err
}
func (c *Client) openLegacySSE(ctx context.Context) error {
	ready := make(chan error, 1)
	c.workers.Add(1)
	go func() {
		defer c.workers.Done()
		req, err := c.newRequest(c.lifetime, http.MethodGet, c.config.URL, nil)
		if err != nil {
			ready <- err
			return
		}
		req.Header.Set("Accept", "text/event-stream")
		response, err := c.do(req)
		if err != nil {
			ready <- err
			return
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			ready <- &HTTPStatusError{StatusCode: response.StatusCode}
			return
		}
		media, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
		if media != "text/event-stream" {
			ready <- errors.New("MCP SSE endpoint requires text/event-stream")
			return
		}
		received := false
		err = readSSE(response.Body, func(event string, data []byte) (bool, error) {
			if event == "endpoint" {
				if received {
					return false, errors.New("MCP SSE endpoint changed during a connection")
				}
				base, _ := url.Parse(c.config.URL)
				relative, err := url.Parse(string(data))
				if err != nil {
					return false, err
				}
				target := base.ResolveReference(relative)
				if target.Scheme != base.Scheme || !strings.EqualFold(target.Host, base.Host) || target.User != nil || target.Fragment != "" {
					return false, errors.New("MCP SSE POST endpoint must use the configured origin")
				}
				c.mu.Lock()
				c.endpoint = target.String()
				for _, values := range target.Query() {
					c.secrets = append(c.secrets, values...)
				}
				c.mu.Unlock()
				received = true
				ready <- nil
				return false, nil
			}
			if !received {
				return false, errors.New("MCP SSE stream omitted its endpoint event")
			}
			if event == "" || event == "message" {
				return false, c.dispatch(data)
			}
			return false, nil
		})
		if !received {
			if err == nil {
				err = errors.New("MCP SSE stream ended before its endpoint")
			}
			select {
			case ready <- err:
			default:
			}
		}
		c.fail()
	}()
	select {
	case err := <-ready:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return errors.New("MCP SSE connection closed")
	}
}
func (c *Client) deleteSession() {
	c.mu.Lock()
	session := c.session
	c.mu.Unlock()
	if session == "" {
		return
	}
	ctx, stop := context.WithTimeout(context.WithoutCancel(c.lifetime), time.Second)
	defer stop()
	req, err := c.newRequest(ctx, http.MethodDelete, c.config.URL, nil)
	if err != nil {
		return
	}
	response, err := c.do(req)
	if err == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		_ = response.Body.Close()
	}
}
func headerValue(value string) string {
	safe := strings.TrimSpace(value) == value && !(strings.HasPrefix(value, "=?base64?") && strings.HasSuffix(value, "?="))
	for _, r := range value {
		if r < 0x20 && r != '\t' || r > 0x7e {
			safe = false
		}
	}
	if safe {
		return value
	}
	return "=?base64?" + base64.StdEncoding.EncodeToString([]byte(value)) + "?="
}

type toolHeadersKey struct{}

func (c *Client) authRequired(challenge AuthChallenge) error {
	challenge.Server = c.scrub(challenge.Server)
	challenge.ResourceURL = c.scrub(challenge.ResourceURL)
	challenge.ResourceMetadataURL = c.scrub(challenge.ResourceMetadataURL)
	challenge.Scope = c.scrub(challenge.Scope)
	return &AuthRequiredError{Challenge: challenge}
}
