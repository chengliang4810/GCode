package remote

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/mock"
)

func TestScopedSnapshotSubscriptionAndUsageRoundTrip(t *testing.T) {
	backend, err := mock.New(mock.Options{Instant: true})
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()

	token := strings.Repeat("host", 8)
	handler, err := (Server{Core: backend, Token: token}).Handler()
	if err != nil {
		t.Fatal(err)
	}
	host := httptest.NewServer(handler)
	defer host.Close()

	client, err := NewClient(host.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	summary, err := client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Messages) != 0 {
		t.Fatalf("unscoped snapshot retained transcript bodies: %d sessions", len(summary.Messages))
	}

	detail, err := client.Snapshot(ctx, "chat-0")
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Messages["chat-0"]) == 0 || len(detail.Messages) != 1 {
		t.Fatalf("scoped snapshot returned the wrong transcript scope: %+v", detail.Messages)
	}
	if _, ok := detail.Messages["chat-1"]; ok {
		t.Fatal("scoped snapshot included an unrequested transcript")
	}

	events, err := client.Subscribe(ctx, "chat-0")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case event, ok := <-events:
		if !ok || event.Snapshot == nil {
			t.Fatalf("scoped subscription closed without an initial snapshot: %+v", event)
		}
		if len(event.Snapshot.Messages["chat-0"]) == 0 || len(event.Snapshot.Messages) != 1 {
			t.Fatalf("scoped subscription returned the wrong transcript scope: %+v", event.Snapshot.Messages)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for scoped subscription snapshot")
	}

	usage, err := client.ReadApplicationUsage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := contracts.UsageRecordsFromMessages(detail.Messages["chat-0"])
	if !reflect.DeepEqual(usage.Records["chat-0"], want) {
		t.Fatalf("usage metadata differs from scoped transcript projection:\ngot=%+v\nwant=%+v", usage.Records["chat-0"], want)
	}
	if _, ok := usage.Records["chat-1"]; !ok {
		t.Fatal("application usage omitted another resident session")
	}
	data, err := json.Marshal(usage)
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range detail.Messages["chat-0"] {
		for _, block := range message.Blocks {
			if block.Text != "" && strings.Contains(string(data), block.Text) {
				t.Fatalf("usage response leaked transcript text %q", block.Text)
			}
			if block.Tool != nil && block.Tool.Output != "" && strings.Contains(string(data), block.Tool.Output) {
				t.Fatalf("usage response leaked tool output %q", block.Tool.Output)
			}
		}
	}
}
