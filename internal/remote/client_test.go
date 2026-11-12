package remote

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestClientRejectsPlaintextRemoteHosts(t *testing.T) {
	t.Parallel()
	token := strings.Repeat("x", 24)
	if _, err := NewClient("http://host.example:27655", token); err == nil {
		t.Fatal("plaintext remote host accepted")
	}
	if _, err := NewClient("http://127.0.0.1:27655", token); err != nil {
		t.Fatalf("loopback plaintext host rejected: %v", err)
	}
	if _, err := NewClient("https://host.example:27655", strings.Repeat("x", 8)); err == nil {
		t.Fatal("short token accepted")
	}
}

func TestReconnectBackoffIsBoundedAndResets(t *testing.T) {
	t.Parallel()
	var backoff reconnectBackoff
	if got := backoff.next(); got != time.Second {
		t.Fatalf("first retry = %v", got)
	}
	ceiling := 2 * time.Second
	for range 100 {
		delay := backoff.next()
		if delay < ceiling/2 || delay > ceiling {
			t.Fatalf("retry %v outside [%v, %v]", delay, ceiling/2, ceiling)
		}
		ceiling = min(ceiling*2, 30*time.Second)
	}
	backoff.reset()
	if got := backoff.next(); got != time.Second {
		t.Fatalf("first retry after recovery = %v", got)
	}
}

func TestReconnectWaitStopsOnCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan bool, 1)
		go func() { done <- waitReconnect(ctx, 30*time.Second) }()
		synctest.Wait()
		started := time.Now()
		cancel()
		if <-done {
			t.Fatal("canceled reconnect wait reported timer completion")
		}
		if time.Since(started) != 0 {
			t.Fatal("cancellation waited for the retry timer")
		}
	})
}

func TestSubscriptionBackoffRecoversAndClientCloseCancels(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client, err := NewClient("https://fixture.invalid", strings.Repeat("x", 24))
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()
		var attempts []time.Time
		client.http.Transport = subscriptionTransport(func(request *http.Request) (*http.Response, error) {
			attempts = append(attempts, time.Now())
			if len(attempts) == 2 || len(attempts) == 3 {
				return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("unavailable"))}, nil
			}
			fixture := `data: {"kind":"snapshot","snapshot":{"revision":1}}` + "\n\n"
			var body io.ReadCloser = io.NopCloser(strings.NewReader(fixture))
			if len(attempts) > 1 {
				body = &subscriptionBody{ctx: request.Context(), reader: strings.NewReader(fixture)}
			}
			return &http.Response{StatusCode: 200, Body: body}, nil
		})
		events, err := client.Subscribe(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, kind := range []string{"snapshot", "connection.disconnected", "connection.connected", "snapshot"} {
			if event := <-events; event.Kind != kind {
				t.Fatalf("event kind = %q, want %q", event.Kind, kind)
			}
		}
		if len(attempts) != 4 {
			t.Fatalf("recovery attempts = %d", len(attempts))
		}
		for index, bounds := range [][2]time.Duration{
			{time.Second, time.Second}, {time.Second, 2 * time.Second}, {2 * time.Second, 4 * time.Second},
		} {
			elapsed := attempts[index+1].Sub(attempts[index])
			if elapsed < bounds[0] || elapsed > bounds[1] {
				t.Fatalf("retry interval %d = %v, want %v", index, elapsed, bounds)
			}
		}
		// No caller cancellation: Client.Close owns cancellation of its stream.
		_ = client.Close()
		for range events {
		}
		time.Sleep(time.Minute)
		if len(attempts) != 4 {
			t.Fatal("closed client continued reconnecting")
		}
	})
}

type subscriptionTransport func(*http.Request) (*http.Response, error)

func (transport subscriptionTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

type subscriptionBody struct {
	ctx    context.Context
	reader *strings.Reader
}

func (body *subscriptionBody) Read(data []byte) (int, error) {
	if body.reader.Len() > 0 {
		return body.reader.Read(data)
	}
	<-body.ctx.Done()
	return 0, body.ctx.Err()
}

func (*subscriptionBody) Close() error { return nil }
