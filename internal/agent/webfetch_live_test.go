package agent

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestLiveWebFetchAgainstRealEndpoint is opt-in: it reaches the public network,
// so it only runs when GCODE_LIVE_WEB=1 is set.
func TestLiveWebFetchAgainstRealEndpoint(t *testing.T) {
	if os.Getenv("GCODE_LIVE_WEB") != "1" {
		t.Skip("set GCODE_LIVE_WEB=1 to reach the network")
	}
	output, err := (webFetchTool{}).Execute(context.Background(), "", json.RawMessage(`{"url":"https://example.com"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(output.Text), "example domain") {
		t.Fatalf("unexpected body: %q", output.Text)
	}
	if strings.Contains(output.Text, "<html") || strings.Contains(output.Text, "<div") {
		t.Fatalf("markup was not stripped: %q", output.Text)
	}
	t.Logf("fetched %d characters", len(output.Text))
}
