package workflow

import (
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"

	"gcode/internal/contracts"
)

// The workflow hub groups saved definitions and prepares a launch from one of
// them. Everything here is a pure rule: it decides what the hub shows and what
// a launch sends, never how it looks.

// ProjectRef is the project fact the hub groups definitions by. Remote is the
// host identity: a definition saved on another host is not the same thing as
// one saved here.
type ProjectRef struct {
	ID     string
	Name   string
	Remote bool
}

// DefinitionProjectGroup is one project's saved definitions.
type DefinitionProjectGroup struct {
	ProjectID string
	Name      string
	// Current marks the project the user is looking at right now.
	Current bool
	// Remote keeps the host identity: a remote project is not a local path.
	Remote bool
	// HostOnly marks definitions whose project is gone from the snapshot. They
	// are still listed: a definition that cannot show its project is not gone.
	Gone        bool
	Definitions []contracts.WorkflowDefinition
}

// DefinitionGroups is the hub's two groups: project definitions and the ones
// that belong to this machine.
type DefinitionGroups struct {
	Projects []DefinitionProjectGroup
	Global   []contracts.WorkflowDefinition
}

// GroupDefinitions splits saved definitions by scope. The current project's
// group comes first, then the other local projects, then remote ones, then
// definitions whose project is no longer known; the global group is last.
func GroupDefinitions(definitions []contracts.WorkflowDefinition, projects []ProjectRef, currentProjectID string) DefinitionGroups {
	groups := DefinitionGroups{}
	byProject := map[string][]contracts.WorkflowDefinition{}
	global := make([]contracts.WorkflowDefinition, 0, len(definitions))
	for _, definition := range definitions {
		if definition.Scope == SavedScopeGlobal {
			global = append(global, definition)
			continue
		}
		byProject[definition.ProjectID] = append(byProject[definition.ProjectID], definition)
	}
	known := make(map[string]ProjectRef, len(projects))
	for _, project := range projects {
		known[project.ID] = project
	}
	ordered := make([]DefinitionProjectGroup, 0, len(projects)+len(byProject))
	for _, project := range projects {
		saved, ok := byProject[project.ID]
		if !ok {
			continue
		}
		ordered = append(ordered, DefinitionProjectGroup{
			ProjectID:   project.ID,
			Name:        project.Name,
			Current:     project.ID == currentProjectID && project.ID != "",
			Remote:      project.Remote,
			Definitions: sortDefinitions(saved),
		})
		delete(byProject, project.ID)
	}
	// 定义所属的项目已经不在快照里：照样列出来，不静默消失。
	for _, projectID := range sortedKeys(byProject) {
		group := DefinitionProjectGroup{ProjectID: projectID, Name: projectID, Gone: true}
		if project, ok := known[projectID]; ok {
			group.Name, group.Remote = project.Name, project.Remote
		}
		group.Definitions = sortDefinitions(byProject[projectID])
		ordered = append(ordered, group)
	}
	// 当前项目排最前，其余按项目顺序；远端项目排在后面，它们的路径不是本机路径。
	groups.Projects = placeCurrentFirst(ordered, currentProjectID)
	groups.Global = sortDefinitions(global)
	return groups
}

// Count reports how many definitions the groups hold together.
func (g DefinitionGroups) Count() int {
	total := len(g.Global)
	for _, group := range g.Projects {
		total += len(group.Definitions)
	}
	return total
}

// placeCurrentFirst keeps the current project at the top without reordering
// the rest relative to each other.
func placeCurrentFirst(groups []DefinitionProjectGroup, currentProjectID string) []DefinitionProjectGroup {
	if currentProjectID == "" {
		return groups
	}
	for i, group := range groups {
		if group.ProjectID != currentProjectID {
			continue
		}
		out := make([]DefinitionProjectGroup, 0, len(groups))
		out = append(out, group)
		out = append(out, groups[:i]...)
		return append(out, groups[i+1:]...)
	}
	return groups
}

// sortDefinitions keeps the most recently changed definition first and breaks
// ties by name, so the order never depends on map iteration.
func sortDefinitions(definitions []contracts.WorkflowDefinition) []contracts.WorkflowDefinition {
	out := append([]contracts.WorkflowDefinition(nil), definitions...)
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// DefinitionRuns returns the runs a definition launched, newest first. It is
// the hub's history, so it counts every run, not only the settled ones.
func DefinitionRuns(definitionID string, runs []contracts.WorkflowRun, limit int) []contracts.WorkflowRun {
	if definitionID == "" {
		return nil
	}
	out := make([]contracts.WorkflowRun, 0, len(runs))
	for _, run := range runs {
		if run.DefinitionID == definitionID {
			out = append(out, run)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// DefinitionLastRun is the most recent run of a definition.
func DefinitionLastRun(definitionID string, runs []contracts.WorkflowRun) (contracts.WorkflowRun, bool) {
	found := DefinitionRuns(definitionID, runs, 1)
	if len(found) == 0 {
		return contracts.WorkflowRun{}, false
	}
	return found[0], true
}

// DefinitionArtifacts picks the delivered artifacts the hub shows. They come
// from the most recent run that succeeded and really published something: a
// newer failure must not hide what an earlier run already delivered.
func DefinitionArtifacts(definitionID string, runs []contracts.WorkflowRun) ArtifactSlice {
	for _, run := range DefinitionRuns(definitionID, runs, 0) {
		if run.Status != contracts.WorkflowStatusCompleted || len(run.Artifacts) == 0 {
			continue
		}
		return CardArtifacts(run.Artifacts)
	}
	return ArtifactSlice{}
}

// ArgumentPreview is the bounded argument line of a hub card: at most limit
// declared names, then how many were left out.
func ArgumentPreview(arguments []contracts.WorkflowArgument, limit int) ([]string, int) {
	names := make([]string, 0, len(arguments))
	for _, argument := range arguments {
		names = append(names, argument.Name)
	}
	if limit <= 0 || len(names) <= limit {
		return names, 0
	}
	return names[:limit], len(names) - limit
}

// LaunchIssueKind names why a launch value is unusable. The interface turns it
// into a sentence; the rule does not own wording.
const (
	LaunchIssueRequired = "required"
	LaunchIssueNumber   = "number"
	LaunchIssueJSON     = "json"
	LaunchIssueUnknown  = "unknown"
)

// LaunchIssue is the first unusable value in declaration order. The form
// scrolls to and focuses that field.
type LaunchIssue struct {
	Name string
	Kind string
}

// LaunchPrefill is the starting value of every declared argument for a new
// launch. A run of the same definition provides what it actually used; keys
// the definition no longer declares are dropped, and missing ones fall back to
// the declaration's own default.
func LaunchPrefill(definition contracts.WorkflowDefinition, run *contracts.WorkflowRun) map[string]string {
	used := map[string]string{}
	if run != nil {
		for _, value := range run.Args {
			if value.Text == "" {
				continue
			}
			used[value.Name] = value.Text
		}
	}
	values := make(map[string]string, len(definition.Arguments))
	for _, argument := range definition.Arguments {
		text, ok := used[argument.Name]
		if !ok {
			text = argument.Default
		}
		values[argument.Name] = text
	}
	return values
}

// LaunchArgs turns the form's values into the arguments a launch sends. It
// refuses what the core would refuse later anyway: an unknown name, a number
// that is not finite, JSON that does not parse, or a missing required value.
// Empty string, number and JSON values are not sent at all; a boolean is
// always sent as an explicit true or false.
func LaunchArgs(definition contracts.WorkflowDefinition, values map[string]string) ([]contracts.WorkflowArgValue, *LaunchIssue) {
	declared := make(map[string]bool, len(definition.Arguments))
	for _, argument := range definition.Arguments {
		declared[argument.Name] = true
	}
	for _, name := range sortedKeys(values) {
		if !declared[name] && strings.TrimSpace(values[name]) != "" {
			return nil, &LaunchIssue{Name: name, Kind: LaunchIssueUnknown}
		}
	}
	out := make([]contracts.WorkflowArgValue, 0, len(definition.Arguments))
	for _, argument := range definition.Arguments {
		text := strings.TrimSpace(values[argument.Name])
		if text == "" && argument.HasDefault {
			text = argument.Default
		}
		kind := argument.Type
		switch kind {
		case "boolean":
			boolean := text == "true"
			if text != "true" && text != "false" && text != "" {
				return nil, &LaunchIssue{Name: argument.Name, Kind: LaunchIssueUnknown}
			}
			out = append(out, contracts.WorkflowArgValue{Name: argument.Name, Kind: kind, Text: strconv.FormatBool(boolean)})
			continue
		case "number":
			if text == "" {
				if argument.Required {
					return nil, &LaunchIssue{Name: argument.Name, Kind: LaunchIssueRequired}
				}
				continue
			}
			number, err := strconv.ParseFloat(text, 64)
			if err != nil || math.IsInf(number, 0) || math.IsNaN(number) {
				return nil, &LaunchIssue{Name: argument.Name, Kind: LaunchIssueNumber}
			}
		case "json":
			if text == "" {
				if argument.Required {
					return nil, &LaunchIssue{Name: argument.Name, Kind: LaunchIssueRequired}
				}
				continue
			}
			var parsed any
			if err := json.Unmarshal([]byte(text), &parsed); err != nil {
				return nil, &LaunchIssue{Name: argument.Name, Kind: LaunchIssueJSON}
			}
		default:
			if text == "" {
				if argument.Required {
					return nil, &LaunchIssue{Name: argument.Name, Kind: LaunchIssueRequired}
				}
				continue
			}
		}
		out = append(out, contracts.WorkflowArgValue{Name: argument.Name, Kind: kind, Text: text})
	}
	return out, nil
}
