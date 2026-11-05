// Package mcp connects the Go core to user-configured MCP servers.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gcode/internal/processutil"
)

const LatestHandshakeVersion = "2025-11-25"
const ModernProtocolVersion = "2026-07-28"

// Config describes an external server. Plugin is its owning plugin namespace.
// Transport is stdio, http (Streamable HTTP), or sse (HTTP+SSE).
type Config struct {
	// SensitiveValues is host-owned runtime metadata, never persisted or sent to a server.
	SensitiveValues []string `json:"-"`
	// ProcessEnv holds nonsecret host defaults for the stdio child process.
	// Explicit Env values take priority and retain their credential semantics.
	ProcessEnv map[string]string `json:"-"`
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Transport  string            `json:"transport,omitempty"`
	Plugin     string            `json:"plugin,omitempty"`
	Command    string            `json:"command,omitempty"`
	Args       []string          `json:"args,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	URL        string            `json:"url,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	Disabled   bool              `json:"disabled"`
}

type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string { return fmt.Sprintf("MCP error %d: %s", e.Code, e.Message) }

type envelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      uint64          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  any             `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// Options holds runtime services, never persisted credentials or UI objects.
// ProtocolVersion defaults to the latest handshake revision. "auto" probes
// modern discovery and uses a handshake only when the server is an older era.
// An explicit modern revision never silently falls back to another lifecycle.
type Options struct {
	HTTPClient      *http.Client
	TokenProvider   func(context.Context, AuthChallenge) (string, error)
	ProtocolVersion string
	RequestTimeout  time.Duration
}

type Notification struct {
	Method string
	Params json.RawMessage
}

type Client struct {
	ownedHTTP           bool
	config              Config
	options             Options
	next                atomic.Uint64
	mu                  sync.Mutex
	writeMu             sync.Mutex
	pending             map[uint64]chan envelope
	input               io.WriteCloser
	stderr              *stderrCapture
	processDone         chan struct{}
	http                *http.Client
	session             string
	endpoint            string
	protocol            string
	capabilities        map[string]any
	token               string
	secrets             []string
	toolDiagnostics     []ToolDiagnostic
	done                chan struct{}
	closeOnce           sync.Once
	shutdownOnce        sync.Once
	shutdownErr         error
	cancel              context.CancelFunc
	lifetime            context.Context
	workers             sync.WaitGroup
	notifications       chan Notification
	dropped             atomic.Uint64
	notificationMu      sync.Mutex
	notificationsClosed bool
}

func Connect(ctx context.Context, config Config, cwd string) (*Client, error) {
	return ConnectWithOptions(ctx, config, cwd, Options{})
}

func ConnectWithOptions(ctx context.Context, config Config, cwd string, options Options) (connectedClient *Client, resultErr error) {
	defer func() { resultErr = sanitizeError(resultErr, config) }()
	if SanitizeText(config.Name, config) != config.Name || SanitizeText(config.Plugin, config) != config.Plugin {
		return nil, errors.New("MCP namespace contains configured credentials")
	}
	if config.Disabled {
		return nil, errors.New("MCP server is disabled")
	}
	if strings.TrimSpace(config.Name) == "" || len(config.Name) > 512 {
		return nil, errors.New("MCP server name must contain 1 to 512 bytes")
	}
	if (config.Command == "") == (config.URL == "") {
		return nil, errors.New("MCP server requires exactly one of command or URL")
	}
	if config.Transport == "" {
		if config.Command != "" {
			config.Transport = "stdio"
		} else {
			config.Transport = "http"
		}
	}
	switch config.Transport {
	case "stdio":
		if config.Command == "" {
			return nil, errors.New("MCP stdio transport requires command")
		}
	case "http", "streamable-http", "sse":
		if config.URL == "" {
			return nil, errors.New("MCP HTTP transport requires URL")
		}
		parsed, err := url.Parse(config.URL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.Fragment != "" {
			return nil, errors.New("MCP URL must be an HTTP(S) URL without user information or fragment")
		}
	default:
		return nil, fmt.Errorf("unsupported MCP transport %q", config.Transport)
	}
	config.Env = maps.Clone(config.Env)
	config.ProcessEnv = maps.Clone(config.ProcessEnv)
	config.Headers = maps.Clone(config.Headers)
	config.Args = append([]string(nil), config.Args...)
	config.SensitiveValues = append([]string(nil), config.SensitiveValues...)
	if options.RequestTimeout <= 0 {
		options.RequestTimeout = 2 * time.Minute
	}
	if options.ProtocolVersion == "" {
		options.ProtocolVersion = LatestHandshakeVersion
	}
	if options.ProtocolVersion != "auto" && options.ProtocolVersion != ModernProtocolVersion && !handshakeVersion(options.ProtocolVersion) {
		return nil, errors.New("unsupported MCP protocol version")
	}
	hc := &http.Client{}
	ownedHTTP := true
	if options.HTTPClient != nil {
		*hc = *options.HTTPClient
	}
	// A redirected MCP endpoint must be explicitly configured, rather than copying
	// bearer/custom headers to an endpoint chosen by an untrusted response.
	switch transport := hc.Transport.(type) {
	case nil:
		if base, ok := http.DefaultTransport.(*http.Transport); ok {
			hc.Transport = base.Clone()
		} else {
			hc.Transport = http.DefaultTransport
			ownedHTTP = false
		}
	case *http.Transport:
		hc.Transport = transport.Clone()
	default:
		ownedHTTP = false
	}
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	life, cancel := context.WithCancel(ctx)
	c := &Client{ownedHTTP: ownedHTTP, config: config, options: options, pending: map[uint64]chan envelope{}, http: hc, endpoint: config.URL, protocol: LatestHandshakeVersion, done: make(chan struct{}), cancel: cancel, lifetime: life, notifications: make(chan Notification, 64)}
	connected := false
	defer func() {
		if !connected {
			_ = c.Close()
		}
		if resultErr != nil {
			resultErr = c.sanitizeError(c.withStderr(resultErr))
		}
	}()
	if config.Command != "" {
		command := exec.CommandContext(life, config.Command, config.Args...)
		command.Dir = cwd
		command.WaitDelay = 2 * time.Second
		command.Env = processutil.Environment(os.Environ(), config.ProcessEnv, config.Env)
		stdout, err := command.StdoutPipe()
		if err != nil {
			return nil, err
		}
		c.input, err = command.StdinPipe()
		if err != nil {
			return nil, err
		}
		c.stderr = &stderrCapture{}
		command.Stderr = c.stderr
		prepareCommand(command)
		if err = command.Start(); err != nil {
			return nil, err
		}
		c.processDone = make(chan struct{})
		c.workers.Add(2)
		go func() { defer c.workers.Done(); c.read(stdout) }()
		go func() { defer c.workers.Done(); _ = command.Wait(); close(c.processDone); c.fail() }()
	}
	handshakeCtx, stop := context.WithTimeout(ctx, 20*time.Second)
	defer stop()
	if config.Transport == "sse" {
		if err := c.openLegacySSE(handshakeCtx); err != nil {
			return nil, err
		}
	}
	modern := config.Transport != "sse" && (options.ProtocolVersion == ModernProtocolVersion || options.ProtocolVersion == "auto")
	if modern {
		c.setProtocol(ModernProtocolVersion)
		var discovery struct {
			ProtocolVersions []string       `json:"supportedVersions"`
			Capabilities     map[string]any `json:"capabilities"`
		}
		probeCtx, stopProbe := context.WithTimeout(handshakeCtx, 1500*time.Millisecond)
		err := c.call(probeCtx, "server/discover", map[string]any{}, &discovery)
		stopProbe()
		if err == nil {
			if !contains(discovery.ProtocolVersions, ModernProtocolVersion) {
				err = errors.New("MCP discovery does not offer the requested modern revision")
			} else {
				c.setCapabilities(discovery.Capabilities)
			}
		}
		if err == nil {
			connected = true
			return c, nil
		}
		var rpc *RPCError
		modernError := errors.As(err, &rpc) && rpc.Code <= -32020 && rpc.Code >= -32022
		if modernError && rpc.Code == -32022 && options.ProtocolVersion == "auto" {
			var offered struct {
				Supported []string `json:"supported"`
			}
			if json.Unmarshal(rpc.Data, &offered) == nil && !contains(offered.Supported, ModernProtocolVersion) {
				for _, version := range offered.Supported {
					if handshakeVersion(version) {
						modernError = false
						break
					}
				}
			}
		}
		if options.ProtocolVersion == ModernProtocolVersion || modernError {
			return nil, err
		}
		var auth *AuthRequiredError
		if errors.As(err, &auth) {
			return nil, err
		}
		// A declined or timed-out discovery probe determines an older server era.
		// It does not retry a failed tool or business operation.
		c.setProtocol(LatestHandshakeVersion)
	}
	if handshakeVersion(options.ProtocolVersion) {
		c.setProtocol(options.ProtocolVersion)
	}
	if config.Transport == "sse" && options.ProtocolVersion == ModernProtocolVersion {
		return nil, errors.New("modern MCP does not use legacy HTTP+SSE transport")
	}
	var initialized struct {
		Protocol     string         `json:"protocolVersion"`
		Capabilities map[string]any `json:"capabilities"`
	}
	if err := c.call(handshakeCtx, "initialize", map[string]any{"protocolVersion": c.ProtocolVersion(), "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "gcode", "version": "0.1.0"}}, &initialized); err != nil {
		return nil, err
	}
	if !handshakeVersion(initialized.Protocol) {
		return nil, errors.New("MCP server negotiated an unsupported handshake protocol version")
	}
	c.setProtocol(initialized.Protocol)
	c.setCapabilities(initialized.Capabilities)
	if err := c.notify(handshakeCtx, "notifications/initialized", map[string]any{}); err != nil {
		return nil, err
	}
	connected = true
	return c, nil
}

func handshakeVersion(version string) bool {
	switch version {
	case "2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05":
		return true
	}
	return false
}
func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func (c *Client) ProtocolVersion() string    { c.mu.Lock(); defer c.mu.Unlock(); return c.protocol }
func (c *Client) setProtocol(version string) { c.mu.Lock(); c.protocol = version; c.mu.Unlock() }
func (c *Client) setCapabilities(capabilities map[string]any) {
	if capabilities == nil {
		c.capabilities = nil
		return
	}
	safe, _ := c.sanitizeValue(capabilities).(map[string]any)
	c.capabilities = safe
}
func (c *Client) Capabilities() map[string]any {
	data, _ := json.Marshal(c.capabilities)
	var result map[string]any
	_ = json.Unmarshal(data, &result)
	return result
}

// Notifications is bounded. A UI observing dropped notifications should refresh
// its complete projection; the reader never blocks an RPC on a slow observer.
func (c *Client) Notifications() <-chan Notification { return c.notifications }
func (c *Client) DroppedNotifications() uint64       { return c.dropped.Load() }
func (c *Client) Done() <-chan struct{}              { return c.done }

func (c *Client) call(ctx context.Context, method string, params any, result any) (resultErr error) {
	defer func() {
		select {
		case <-c.done:
			resultErr = c.withStderr(resultErr)
		default:
		}
		resultErr = c.sanitizeError(resultErr)
	}()
	var stop context.CancelFunc
	if long, _ := ctx.Value(longLivedRequestKey{}).(bool); long {
		ctx, stop = context.WithCancel(ctx)
	} else {
		ctx, stop = context.WithTimeout(ctx, c.options.RequestTimeout)
	}
	defer stop()
	linked, unlink := context.WithCancel(ctx)
	defer unlink()
	stopLife := context.AfterFunc(c.lifetime, unlink)
	defer stopLife()
	ctx = linked
	select {
	case <-c.done:
		return errors.New("MCP connection closed")
	default:
	}
	if c.ProtocolVersion() == ModernProtocolVersion {
		p := map[string]any{}
		if params != nil {
			raw, err := json.Marshal(params)
			if err != nil {
				return err
			}
			decoder := json.NewDecoder(strings.NewReader(string(raw)))
			decoder.UseNumber()
			if err = decoder.Decode(&p); err != nil {
				return err
			}
		}
		p["_meta"] = map[string]any{"io.modelcontextprotocol/protocolVersion": ModernProtocolVersion, "io.modelcontextprotocol/clientCapabilities": map[string]any{}, "io.modelcontextprotocol/clientInfo": map[string]string{"name": "gcode", "version": "0.1.0"}}
		params = p
	}
	request := envelope{JSONRPC: "2.0", ID: c.next.Add(1), Method: method, Params: params}
	var response envelope
	if c.config.URL != "" && c.config.Transport != "sse" {
		var err error
		response, err = c.post(ctx, request)
		if err != nil {
			if ctx.Err() != nil && c.ProtocolVersion() != ModernProtocolVersion && method != "initialize" {
				c.cancelRequest(ctx, request.ID)
			}
			return err
		}
	} else {
		reply := make(chan envelope, 1)
		c.mu.Lock()
		if len(c.pending) >= 128 {
			c.mu.Unlock()
			return errors.New("MCP in-flight request limit reached")
		}
		c.pending[request.ID] = reply
		c.mu.Unlock()
		defer func() { c.mu.Lock(); delete(c.pending, request.ID); c.mu.Unlock() }()
		var err error
		if c.config.Transport == "sse" {
			_, err = c.post(ctx, request)
		} else {
			err = c.write(ctx, request)
		}
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			if method != "initialize" {
				c.cancelRequest(ctx, request.ID)
			}
			return ctx.Err()
		case <-c.done:
			select {
			case response = <-reply:
			default:
				return errors.New("MCP connection closed")
			}
		case response = <-reply:
		}
	}
	if response.Error != nil {
		safe := *response.Error
		safe.Message = c.scrub(safe.Message)
		safe.Data = c.sanitizeJSON(safe.Data)
		return &safe
	}
	if len(response.Result) == 0 {
		return errors.New("MCP response omitted its result")
	}
	var status struct {
		ResultType string `json:"resultType"`
	}
	if err := json.Unmarshal(response.Result, &status); err != nil {
		return err
	}
	if status.ResultType != "" && status.ResultType != "complete" {
		return fmt.Errorf("MCP result type %q requires an unsupported client interaction", status.ResultType)
	}
	if result == nil {
		return nil
	}
	return json.Unmarshal(response.Result, result)
}
func (c *Client) cancelRequest(parent context.Context, id uint64) {
	ctx, stop := context.WithTimeout(context.WithoutCancel(parent), time.Second)
	defer stop()
	_ = c.notify(ctx, "notifications/cancelled", map[string]any{"requestId": id, "reason": "client cancelled"})
}
func (c *Client) notify(ctx context.Context, method string, params any) error {
	message := envelope{JSONRPC: "2.0", Method: method, Params: params}
	if c.config.URL != "" {
		_, err := c.post(ctx, message)
		return err
	}
	return c.write(ctx, message)
}
func (c *Client) write(ctx context.Context, message any) error {
	if c.input == nil {
		return errors.New("MCP stdin is not open")
	}
	completed := make(chan error, 1)
	go func() {
		c.writeMu.Lock()
		defer c.writeMu.Unlock()
		completed <- json.NewEncoder(c.input).Encode(message)
	}()
	select {
	case err := <-completed:
		return err
	case <-ctx.Done():
		_ = c.input.Close()
		c.fail()
		return ctx.Err()
	case <-c.done:
		return errors.New("MCP connection closed")
	}
}
func (c *Client) read(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), maxMessageBytes)
	for scanner.Scan() {
		if err := c.dispatch(scanner.Bytes()); err != nil {
			c.fail()
			return
		}
	}
	c.fail()
}
func (c *Client) dispatch(data []byte) error {
	var header struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return errors.New("MCP received invalid JSON")
	}
	if header.JSONRPC != "2.0" {
		return errors.New("MCP received an invalid JSON-RPC version")
	}
	if header.Method != "" {
		if len(header.ID) > 0 && string(header.ID) != "null" {
			response := map[string]any{"jsonrpc": "2.0", "id": header.ID}
			if header.Method == "ping" {
				response["result"] = map[string]any{}
			} else {
				response["error"] = &RPCError{Code: -32601, Message: "client capability not supported"}
			}
			ctx, stop := context.WithTimeout(c.lifetime, 2*time.Second)
			defer stop()
			if c.config.URL != "" {
				return c.sendHTTP(ctx, response)
			}
			return c.write(ctx, response)
		}
		params := c.sanitizeJSON(header.Params)
		c.enqueueNotification(Notification{Method: c.scrub(header.Method), Params: params})
		return nil
	}
	var response envelope
	if err := json.Unmarshal(data, &response); err != nil {
		return err
	}
	c.mu.Lock()
	reply := c.pending[response.ID]
	c.mu.Unlock()
	if reply != nil {
		select {
		case reply <- response:
		default:
		}
	}
	return nil
}
func (c *Client) fail() { c.closeOnce.Do(func() { close(c.done); c.cancel() }) }
func (c *Client) Close() error {
	c.shutdownOnce.Do(func() {
		if c.config.URL != "" && c.config.Transport != "sse" && c.ProtocolVersion() != ModernProtocolVersion {
			c.deleteSession()
		}
		if c.input != nil {
			_ = c.input.Close()
		}
		c.fail()
		if c.processDone != nil {
			select {
			case <-c.processDone:
			case <-time.After(3 * time.Second):
				c.shutdownErr = errors.New("MCP process did not exit after cancellation")
			}
		}
		// Do not turn a failed process termination into an unbounded Close.
		if c.shutdownErr == nil {
			c.workers.Wait()
		}
		if c.ownedHTTP {
			c.http.CloseIdleConnections()
		}
		c.closeNotifications()
	})
	return c.shutdownErr
}
func (c *Client) scrub(text string) string {
	c.mu.Lock()
	extra := append([]string(nil), c.secrets...)
	c.mu.Unlock()
	return sanitizeSecrets(text, append(configuredSecrets(c.config), extra...))
}
func (c *Client) sanitizeError(err error) error {
	if err == nil {
		return nil
	}
	return sanitizedError{cause: err, text: c.scrub(err.Error())}
}
func (c *Client) sanitizeJSON(raw json.RawMessage) json.RawMessage {
	var value any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return nil
	}
	value = c.sanitizeValue(value)
	data, _ := json.Marshal(value)
	return data
}
func (c *Client) sanitizeValue(value any) any {
	switch v := value.(type) {
	case string:
		return c.scrub(v)
	case []any:
		for i := range v {
			v[i] = c.sanitizeValue(v[i])
		}
		return v
	case map[string]any:
		out := map[string]any{}
		for k, child := range v {
			out[c.scrub(k)] = c.sanitizeValue(child)
		}
		return out
	default:
		return value
	}
}
