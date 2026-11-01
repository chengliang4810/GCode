package contracts

import (
	"fmt"
	"slices"
	"strings"
)

// SubagentProfile is a task identity and an optional explicit model selection.
// A nil Model inherits the invoking session's model and reasoning together.
type SubagentProfile struct {
	ID                      string          `json:"id"`
	Name                    string          `json:"name"`
	Description             string          `json:"description"`
	Prompt                  string          `json:"prompt"`
	Model                   *ModelSelection `json:"model,omitempty"`
	Disabled                bool            `json:"disabled"`
	Tools                   []string        `json:"tools,omitempty"`
	DisallowedTools         []string        `json:"disallowedTools,omitempty"`
	SkipProjectInstructions bool            `json:"skipProjectInstructions"`
}

func CloneSubagentProfiles(profiles []SubagentProfile) []SubagentProfile {
	result := slices.Clone(profiles)
	for i := range result {
		result[i].Model = copyPointer(result[i].Model)
		result[i].Tools = slices.Clone(result[i].Tools)
		result[i].DisallowedTools = slices.Clone(result[i].DisallowedTools)
	}
	return result
}

func ValidateSubagentProfiles(profiles []SubagentProfile) error {
	seen := make(map[string]bool, len(profiles))
	for _, profile := range profiles {
		if strings.TrimSpace(profile.ID) == "" || seen[profile.ID] ||
			strings.ContainsAny(profile.ID, "/\\ \t\r\n") {
			return fmt.Errorf("subagent IDs must be nonempty, unique and contain no spaces or slashes")
		}
		seen[profile.ID] = true
		if strings.TrimSpace(profile.Name) == "" || strings.TrimSpace(profile.Description) == "" {
			return fmt.Errorf("subagent %q needs a name and description", profile.ID)
		}
		if profile.Model != nil && strings.TrimSpace(profile.Model.ModelID) == "" {
			return fmt.Errorf("subagent %q needs a configured model or an inherited selection", profile.ID)
		}
		toolSeen := map[string]bool{}
		for _, name := range profile.Tools {
			if strings.TrimSpace(name) == "" || toolSeen[name] {
				return fmt.Errorf("subagent %q tool names must be nonempty and unique", profile.ID)
			}
			toolSeen[name] = true
			if IsReadOnlySubagent(profile.ID) && !slices.Contains([]string{"Read", "ListDirectory", "Glob", "Grep"}, name) {
				return fmt.Errorf("%s only supports Read, ListDirectory, Glob and Grep", profile.ID)
			}
		}
	}
	return nil
}

func EnabledSubagentProfiles(profiles []SubagentProfile) []SubagentProfile {
	result := CloneSubagentProfiles(profiles)
	return slices.DeleteFunc(result, func(profile SubagentProfile) bool { return profile.Disabled })
}

// EffectiveSubagentProfiles combines the built-in catalog with saved overrides
// and custom profiles. Omitting a built-in configuration keeps its definition;
// Disabled is the explicit way to make a built-in unavailable.
func EffectiveSubagentProfiles(configured []SubagentProfile) []SubagentProfile {
	profiles := DefaultSubagentProfiles()
	for _, profile := range CloneSubagentProfiles(configured) {
		index := slices.IndexFunc(profiles, func(candidate SubagentProfile) bool { return candidate.ID == profile.ID })
		if index < 0 {
			profiles = append(profiles, profile)
		} else {
			profiles[index] = profile
		}
	}
	return profiles
}

func IsBuiltinSubagent(id string) bool {
	return slices.ContainsFunc(DefaultSubagentProfiles(), func(profile SubagentProfile) bool { return profile.ID == id })
}

func IsReadOnlySubagent(id string) bool { return id == "Explore" || id == "Plan" }

func DefaultSubagentProfiles() []SubagentProfile {
	return []SubagentProfile{
		{
			ID: "general-purpose", Name: "General purpose",
			Description: "Implement, investigate or verify an independent coding task.",
			Prompt:      "Complete the assigned task using the available tools. Read the relevant files, preserve unrelated changes, verify your work and return a concise report with evidence.",
		},
		{
			ID: "Explore", Name: "Explore",
			Description:             "Read-only codebase investigation, file search and evidence collection.",
			Prompt:                  "Investigate the assigned question without changing project files. Return the findings, relevant paths and any limits of the evidence.",
			SkipProjectInstructions: true,
		},
		{
			ID: "Plan", Name: "Plan",
			Description:             "Investigate requirements and design an implementation plan without modifying files.",
			Prompt:                  "Inspect the relevant code and project instructions. Explain an actionable implementation plan, important files, dependencies and checks. This is a read-only planning task; do not execute changes.",
			SkipProjectInstructions: true,
		},
		{
			ID: "claude", Name: "General",
			Description: "Handle a task with the invoking conversation's model and a separate execution context.",
			Prompt:      "Complete the delegated task using the available tools. Preserve unrelated work, verify observable results and report evidence and limitations.",
		},
	}
}
