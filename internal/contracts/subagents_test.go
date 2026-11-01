package contracts

import (
	"reflect"
	"testing"

	"gcode/internal/model"
)

func TestEffectiveSubagentsIncludeBuiltinsAndOwnedOverrides(t *testing.T) {
	for _, test := range []struct {
		name       string
		configured []SubagentProfile
	}{
		{name: "no saved profiles"},
		{name: "custom only", configured: []SubagentProfile{{ID: "review", Name: "Review", Description: "Review changes"}}},
		{name: "disabled builtin", configured: []SubagentProfile{{ID: "Explore", Name: "Research", Description: "Read files", Disabled: true,
			Model: &ModelSelection{ModelID: "p/m", Reasoning: "Max"}, Tools: []string{"Read"}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := CloneSubagentProfiles(test.configured)
			profiles := EffectiveSubagentProfiles(test.configured)
			if len(profiles) < 4 || profiles[0].ID != "general-purpose" || profiles[1].ID != "Explore" || profiles[2].ID != "Plan" || profiles[3].ID != "claude" {
				t.Fatal("built-in catalog disappeared")
			}
			if !IsBuiltinSubagent(profiles[0].ID) || IsBuiltinSubagent("review") {
				t.Fatal("built-in identity depends on saved configuration")
			}
			for _, configured := range test.configured {
				found := false
				for _, actual := range profiles {
					if actual.ID == configured.ID {
						found = true
						if !reflect.DeepEqual(actual, configured) {
							t.Fatal("saved override or custom profile was replaced")
						}
					}
				}
				if !found {
					t.Fatal("saved profile was omitted")
				}
				if configured.Disabled {
					for _, enabled := range EnabledSubagentProfiles(profiles) {
						if enabled.ID == configured.ID {
							t.Fatal("built-in catalog re-enabled an explicitly disabled profile")
						}
					}
				}
			}
			for i := range profiles {
				if profiles[i].Model != nil {
					profiles[i].Model.ModelID = "changed"
					profiles[i].Tools[0] = "ls"
				}
			}
			if !reflect.DeepEqual(test.configured, before) || DefaultSubagentProfiles()[1].Disabled {
				t.Fatal("effective profiles share mutable configuration or built-in catalog")
			}
		})
	}
}

func TestSubagentSettingsAndModelLevelsAreOwnedSnapshots(t *testing.T) {
	settings := DefaultSettings()
	settings.Subagents[0].Model = &ModelSelection{ModelID: "p/m", Reasoning: "Max"}
	source := Snapshot{
		Settings: settings,
		CustomProviders: []ProviderConfiguration{{Provider: model.Provider{
			ID: "p", Models: []model.Model{{ID: "m", ReasoningLevels: []string{"low", "max"}}},
		}}},
	}
	cloned := CloneSnapshot(source)
	cloned.Settings.Subagents[0].Model.Reasoning = "Low"
	cloned.CustomProviders[0].Provider.Models[0].ReasoningLevels[0] = "none"
	if source.Settings.Subagents[0].Model.Reasoning != "Max" ||
		source.CustomProviders[0].Provider.Models[0].ReasoningLevels[0] != "low" {
		t.Fatal("snapshot shares mutable subagent or model configuration")
	}
	command := Command{Kind: "settings.update", Settings: &settings}
	frozen := command.Clone()
	frozen.Settings.Subagents[0].Model.ModelID = "other/model"
	if settings.Subagents[0].Model.ModelID != "p/m" {
		t.Fatal("queued settings command shares its profile selection")
	}
}

func TestSubagentProfilesRejectAmbiguousConfiguration(t *testing.T) {
	for _, profiles := range [][]SubagentProfile{
		{{ID: "", Name: "A", Description: "Task"}},
		{{ID: "a/b", Name: "A", Description: "Task"}},
		{{ID: "a", Name: "", Description: "Task"}},
		{{ID: "a", Name: "A"}},
		{{ID: "a", Name: "A", Description: "Task", Model: &ModelSelection{}}},
		{{ID: "a", Name: "A", Description: "Task"}, {ID: "a", Name: "B", Description: "Task"}},
	} {
		settings := Settings{Subagents: profiles}
		if err := (Command{Kind: "settings.update", Settings: &settings}).Validate(); err == nil {
			t.Fatalf("invalid subagent profiles accepted: %+v", profiles)
		}
	}
}
