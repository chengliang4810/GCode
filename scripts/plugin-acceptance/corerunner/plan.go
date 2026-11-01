// Package corerunner runs explicit plugin acceptance probes against the actual Go core.
package corerunner

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gcode/scripts/plugin-acceptance/acceptance"
)

type Plan struct {
	PluginID     string      `json:"pluginId"`
	SourceCommit string      `json:"sourceCommit"`
	AllowHooks   []string    `json:"allowHooks"`
	Probes       []ProbePlan `json:"probes"`
}

type ProbePlan struct {
	ID              string      `json:"id"`
	ComponentID     string      `json:"componentId"`
	Prompt          string      `json:"prompt"`
	MCPTool         string      `json:"mcpTool,omitempty"`
	AllowCommands   []string    `json:"allowCommands,omitempty"`
	VerifierOutputs []string    `json:"verifierOutputs,omitempty"`
	Fixtures        []Fixture   `json:"fixtures,omitempty"`
	Assertions      []Assertion `json:"assertions"`
}

type Fixture struct {
	Path string `json:"path"`
	Text string `json:"text"`
}
type Assertion struct {
	Kind     string  `json:"kind"`
	Path     string  `json:"path,omitempty"`
	Source   string  `json:"source,omitempty"`
	ToolName string  `json:"toolName,omitempty"`
	Equals   *string `json:"equals,omitempty"`
	Contains string  `json:"contains,omitempty"`
	Regex    string  `json:"regex,omitempty"`
	ExitCode *int    `json:"exitCode,omitempty"`
}
type AssertionResult struct {
	Kind    string `json:"kind"`
	Path    string `json:"path,omitempty"`
	Actual  string `json:"actual,omitempty"`
	Matched bool   `json:"matched"`
}

func readPlan(request acceptance.RunRequest) (Plan, error) {
	if request.ProbeDir == "" {
		return Plan{}, errors.New("no --probe-dir; explicit per-plugin business probes are required")
	}
	if strings.ContainsAny(request.PluginID, "/\\:") {
		return Plan{}, errors.New("invalid plugin probe identity")
	}
	path := filepath.Join(request.ProbeDir, request.PluginID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return Plan{}, fmt.Errorf("read explicit plugin probe plan: %w", err)
	}
	if len(data) > 2<<20 {
		return Plan{}, errors.New("probe plan exceeds 2 MiB")
	}
	var plan Plan
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&plan); err != nil {
		return plan, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return plan, errors.New("probe plan must contain exactly one JSON document")
	}
	if plan.PluginID != request.PluginID || plan.SourceCommit != request.SourceCommit {
		return plan, errors.New("probe plan does not match the exact plugin/source commit")
	}
	components := map[string]acceptance.Component{}
	for _, component := range request.Components {
		components[component.ID] = component
	}
	seen, ids := map[string]bool{}, map[string]bool{}
	for _, probe := range plan.Probes {
		if _, exists := components[probe.ComponentID]; !exists || seen[probe.ComponentID] || probe.ID == "" || ids[probe.ID] {
			return plan, errors.New("probe plans must identify each exact component once with a unique probe ID")
		}
		seen[probe.ComponentID], ids[probe.ID] = true, true
		if len(probe.Assertions) == 0 || strings.TrimSpace(probe.Prompt) == "" {
			return plan, errors.New("every probe requires an explicit task and business assertions")
		}
		if components[probe.ComponentID].Kind == "mcp" && probe.MCPTool == "" {
			return plan, errors.New("MCP probe requires the exact server tool name")
		}
		for _, assertion := range probe.Assertions {
			switch assertion.Kind {
			case "file", "file-exists", "file-absent":
				if _, err := workspacePath("workspace", assertion.Path); err != nil {
					return plan, err
				}
			case "text":
				if assertion.Source != "" && assertion.Source != "assistant" && assertion.Source != "tool" &&
					!(assertion.Source == "hook" && components[probe.ComponentID].Kind == "hook") {
					return plan, errors.New("text source must be assistant or tool")
				}
			case "exit":
				if assertion.ExitCode == nil {
					return plan, errors.New("exit assertion requires an explicit exitCode")
				}
			default:
				return plan, fmt.Errorf("unknown business assertion %q", assertion.Kind)
			}
			if (assertion.Kind == "text" || assertion.Kind == "file") && assertion.Equals == nil && assertion.Contains == "" && assertion.Regex == "" {
				return plan, errors.New("text/file assertions require expected business content")
			}
			if assertion.Regex != "" {
				if _, err := regexp.Compile(assertion.Regex); err != nil {
					return plan, err
				}
			}
		}
		for _, fixture := range probe.Fixtures {
			if _, err := workspacePath("workspace", fixture.Path); err != nil {
				return plan, err
			}
			if len(fixture.Text) > 1<<20 {
				return plan, errors.New("fixture exceeds 1 MiB")
			}
		}
		for _, path := range probe.VerifierOutputs {
			if _, err := workspacePath("workspace", path); err != nil {
				return plan, err
			}
		}
	}
	for _, component := range request.Components {
		if !seen[component.ID] {
			return plan, fmt.Errorf("declared component %s has no explicit probe plan", component.ID)
		}
		if component.Kind == "hook" && !contains(plan.AllowHooks, component.ID) {
			return plan, fmt.Errorf("hook %s is not explicitly approved by allowHooks", component.ID)
		}
	}
	return plan, nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func workspacePath(root, name string) (string, error) {
	if name == "" || strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\:") || !filepath.IsLocal(filepath.FromSlash(name)) {
		return "", fmt.Errorf("probe resource path must remain inside its private workspace: %q", name)
	}
	path := filepath.Join(root, filepath.FromSlash(name))
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("probe path escapes private workspace")
	}
	current := root
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("probe resource path contains a symlink")
		}
	}
	return path, nil
}
