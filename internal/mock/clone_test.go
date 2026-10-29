package mock

import (
	"context"
	"testing"

	"gcode/internal/contracts"
)

func TestSnapshotOwnsModelOptionChoiceLabels(t *testing.T) {
	s := newTestServer(t, Options{})
	s.mu.Lock()
	s.state.Models[0].Options = []contracts.ModelOption{{
		ID: "contextWindow", Choices: []string{"standard", "1m"}, DefaultChoice: "standard",
		ChoiceLabels: map[string]string{"standard": "Standard", "1m": "1M tokens"},
	}}
	s.mu.Unlock()
	copy, err := s.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	copy.Models[0].Options[0].ChoiceLabels["1m"] = "Changed by caller"
	copy.Models[0].Options[0].Choices[1] = "changed-id"
	after, err := s.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	option := after.Models[0].Options[0]
	if option.ChoiceLabels["1m"] != "1M tokens" || option.Choices[1] != "1m" {
		t.Fatal("caller mutation escaped through model option metadata")
	}
}
