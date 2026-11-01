package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
)

// NotificationFilter is the opt-in filter used by the modern listen method.
// The server acknowledges the honored subset on Notifications().
type NotificationFilter struct {
	ToolsListChanged      bool     `json:"toolsListChanged,omitempty"`
	PromptsListChanged    bool     `json:"promptsListChanged,omitempty"`
	ResourcesListChanged  bool     `json:"resourcesListChanged,omitempty"`
	ResourceSubscriptions []string `json:"resourceSubscriptions,omitempty"`
}

// ListenNotifications runs until the caller cancels, the client closes, or the
// server ends its notification stream. The owner runs it in its own goroutine
// and must refetch state after an unexpected disconnect; no events are replayed.
// stdio/HTTP+SSE already deliver handshake-era notices on their open reader.
func (c *Client) ListenNotifications(ctx context.Context, filter NotificationFilter) (resultErr error) {
	defer func() { resultErr = c.sanitizeError(resultErr) }()
	linked, stop := context.WithCancel(ctx)
	defer stop()
	stopLife := context.AfterFunc(c.lifetime, stop)
	defer stopLife()
	ctx = linked
	if c.ProtocolVersion() == ModernProtocolVersion {
		return c.call(context.WithValue(ctx, longLivedRequestKey{}, true), "subscriptions/listen", map[string]any{"notifications": filter}, &struct{}{})
	}
	if c.config.URL == "" || c.config.Transport == "sse" {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.done:
			return errors.New("MCP connection closed")
		}
	}
	req, err := c.newRequest(ctx, http.MethodGet, c.config.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	response, err := c.do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return &HTTPStatusError{StatusCode: response.StatusCode}
	}
	media, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if media != "text/event-stream" {
		return errors.New("MCP notification stream requires text/event-stream")
	}
	err = readSSE(response.Body, func(_ string, data []byte) (bool, error) { return false, c.dispatch(data) })
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return err
	}
	return errors.New("MCP notification stream ended; refresh state before reconnecting")
}

type longLivedRequestKey struct{}

func (c *Client) enqueueNotification(n Notification) {
	c.notificationMu.Lock()
	defer c.notificationMu.Unlock()
	if c.notificationsClosed {
		return
	}
	select {
	case c.notifications <- n:
	default:
		c.dropped.Add(1)
	}
}
func (c *Client) closeNotifications() {
	c.notificationMu.Lock()
	defer c.notificationMu.Unlock()
	if !c.notificationsClosed {
		close(c.notifications)
		c.notificationsClosed = true
	}
}
func (c *Client) publishNotification(method string, params json.RawMessage) {
	c.enqueueNotification(Notification{Method: c.scrub(method), Params: c.sanitizeJSON(params)})
}
