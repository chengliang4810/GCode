package remote

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"gcode/internal/contracts"
)

type Client struct {
	url, token string
	http       *http.Client
	done       chan struct{}
	once       sync.Once
}

func NewClient(address, token string) (*Client, error) {
	u, err := url.Parse(address)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("remote address must be an HTTP(S) URL")
	}
	if len(token) < 24 {
		return nil, errors.New("remote API token must contain at least 24 characters")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 30 * time.Second
	return &Client{url: strings.TrimRight(address, "/"), token: token, http: &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, done: make(chan struct{})}, nil
}
func (c *Client) request(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, c.url+path, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		_ = response.Body.Close()
		return nil, fmt.Errorf("remote host returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	return response, nil
}

const maxSessionScopeIDs = 128

func scopedPath(path string, sessionIDs ...string) (string, error) {
	if len(sessionIDs) > maxSessionScopeIDs {
		return "", fmt.Errorf("session scope contains too many IDs (maximum %d)", maxSessionScopeIDs)
	}
	query := make(url.Values)
	for _, id := range sessionIDs {
		if id == "" {
			return "", errors.New("session scope contains an empty ID")
		}
		query.Add("sessionId", id)
	}
	if encoded := query.Encode(); encoded != "" {
		return path + "?" + encoded, nil
	}
	return path, nil
}

func (c *Client) Bootstrap(ctx context.Context) (contracts.Snapshot, error) { return c.Snapshot(ctx) }
func (c *Client) Snapshot(ctx context.Context, sessionIDs ...string) (contracts.Snapshot, error) {
	path, err := scopedPath("/v1/snapshot", sessionIDs...)
	if err != nil {
		return contracts.Snapshot{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	response, err := c.request(ctx, http.MethodGet, path, nil)
	if err != nil {
		return contracts.Snapshot{}, err
	}
	defer response.Body.Close()
	var snapshot contracts.Snapshot
	err = json.NewDecoder(io.LimitReader(response.Body, 64<<20)).Decode(&snapshot)
	return snapshot, err
}
func (c *Client) Execute(ctx context.Context, command contracts.Command) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	response, err := c.request(ctx, http.MethodPost, "/v1/commands", command)
	if err != nil {
		return err
	}
	return response.Body.Close()
}
func (c *Client) ListDirectories(ctx context.Context, request contracts.DirectoryRequest) (contracts.DirectoryListing, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	response, err := c.request(ctx, http.MethodPost, "/v1/directories", request)
	if err != nil {
		return contracts.DirectoryListing{}, err
	}
	defer response.Body.Close()
	var result contracts.DirectoryListing
	err = json.NewDecoder(response.Body).Decode(&result)
	return result, err
}
func (c *Client) ListDirectoryLocations(ctx context.Context, request contracts.DirectoryRequest) (contracts.DirectoryLocations, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	response, err := c.request(ctx, http.MethodPost, "/v1/directory-locations", request)
	if err != nil {
		return contracts.DirectoryLocations{}, err
	}
	defer response.Body.Close()
	var result contracts.DirectoryLocations
	err = json.NewDecoder(response.Body).Decode(&result)
	return result, err
}
func (c *Client) Subscribe(ctx context.Context, sessionIDs ...string) (<-chan contracts.Event, error) {
	path, err := scopedPath("/v1/events", sessionIDs...)
	if err != nil {
		return nil, err
	}
	streamCtx, cancel := context.WithCancel(ctx)
	go func() {
		select {
		case <-c.done:
			cancel()
		case <-streamCtx.Done():
		}
	}()
	response, err := c.request(streamCtx, http.MethodGet, path, nil)
	if err != nil {
		cancel()
		return nil, err
	}
	events := make(chan contracts.Event, 256)
	go func() {
		defer close(events)
		defer cancel()
		publishConnection := func(kind string) bool {
			select {
			case <-streamCtx.Done():
				return false
			case events <- contracts.Event{SchemaVersion: contracts.SchemaVersion, Kind: kind}:
				return true
			}
		}
		var reconnect reconnectBackoff
		for {
			scanner := bufio.NewScanner(response.Body)
			scanner.Buffer(make([]byte, 4096), 64<<20)
			for scanner.Scan() {
				line := scanner.Text()
				if !strings.HasPrefix(line, "data: ") {
					continue
				}
				var event contracts.Event
				if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
					break
				}
				reconnect.reset()
				select {
				case events <- event:
				case <-streamCtx.Done():
					_ = response.Body.Close()
					return
				}
			}
			_ = response.Body.Close()
			if streamCtx.Err() != nil || !publishConnection("connection.disconnected") {
				return
			}
			for {
				if !waitReconnect(streamCtx, reconnect.next()) {
					return
				}
				var err error
				response, err = c.request(streamCtx, http.MethodGet, path, nil)
				if err == nil {
					break
				}
			}
			if !publishConnection("connection.connected") {
				_ = response.Body.Close()
				return
			}
			// Every connection begins with the server's authoritative snapshot,
			// covering missed events without replaying any task or command.
		}
	}()
	return events, nil
}

type reconnectBackoff struct {
	delay time.Duration
}

func (b *reconnectBackoff) reset() { b.delay = 0 }

func (b *reconnectBackoff) next() time.Duration {
	if b.delay == 0 {
		b.delay = time.Second
		return b.delay
	}
	b.delay = min(b.delay*2, 30*time.Second)
	// Equal jitter spreads clients across the upper half of each growing
	// interval. The maximum delay stays bounded even during a long outage.
	half := b.delay / 2
	return half + time.Duration(rand.Int64N(int64(b.delay-half)+1))
}

func waitReconnect(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (c *Client) Close() error {
	c.once.Do(func() { close(c.done); c.http.CloseIdleConnections() })
	return nil
}
