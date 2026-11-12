package mock

import (
	"context"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/model"
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

// The mock backend must own its snapshots exactly like the real engine, so a
// caller that edits a projection cannot mutate backend state.
func TestCloneSnapshotMatchesContractOwnership(t *testing.T) {
	goal := &contracts.SessionGoal{ID: "goal", Status: contracts.GoalActive}
	source := contracts.Snapshot{
		Sessions:        []contracts.Session{{ID: "session", Title: "title", Goal: goal}},
		SessionGroups:   []contracts.SessionGroup{{ID: "group", Title: "Group"}},
		Settings:        contracts.Settings{Hooks: []contracts.HookRule{{ID: "hook", Args: []string{"one"}}}},
		CustomProviders: []contracts.ProviderConfiguration{{Provider: model.Provider{ID: "custom", Name: "Custom"}}},
	}
	cloned := cloneSnapshot(source)
	cloned.Sessions[0].Goal.Status = contracts.GoalPaused
	cloned.Sessions[0].Title = "changed"
	cloned.SessionGroups[0].Title = "changed"
	cloned.Settings.Hooks[0].Args[0] = "changed"
	cloned.CustomProviders[0].Provider.Name = "changed"
	if source.Sessions[0].Goal.Status != contracts.GoalActive || source.Sessions[0].Title != "title" {
		t.Fatal("caller mutation escaped through session state")
	}
	if source.SessionGroups[0].Title != "Group" {
		t.Fatal("caller mutation escaped through session groups")
	}
	if source.Settings.Hooks[0].Args[0] != "one" {
		t.Fatal("caller mutation escaped through hook rules")
	}
	if source.CustomProviders[0].Provider.Name != "Custom" {
		t.Fatal("caller mutation escaped through custom providers")
	}
}
