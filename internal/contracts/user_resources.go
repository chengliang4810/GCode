package contracts

import (
	"errors"
	"maps"
	"slices"
	"strings"
	"time"
)

type UserResourceState struct {
	Skills     []UserSkill   `json:"skills"`
	Commands   []UserCommand `json:"commands"`
	MCPServers []MCPServer   `json:"mcpServers"`
}

type UserSkill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Content     string   `json:"content"`
	Enabled     bool     `json:"enabled"`
	Files       []string `json:"files"`
	// Path is a host-generated display location, never a file operation target.
	Path string `json:"path,omitempty"`
}

type UserCommand struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	ArgumentHint string `json:"argumentHint"`
	Content      string `json:"content"`
	Enabled      bool   `json:"enabled"`
}

// MCPServer is a public projection. Credential values never enter snapshots.
type MCPServer struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	Transport  string        `json:"transport"`
	Command    string        `json:"command,omitempty"`
	Args       []string      `json:"args,omitempty"`
	URL        string        `json:"url,omitempty"`
	EnvKeys    []string      `json:"envKeys,omitempty"`
	HeaderKeys []string      `json:"headerKeys,omitempty"`
	Enabled    bool          `json:"enabled"`
	Test       MCPTestResult `json:"test"`
}

type MCPTestResult struct {
	State    string           `json:"state"`
	Error    string           `json:"error,omitempty"`
	Tools    []MCPToolSummary `json:"tools,omitempty"`
	TestedAt time.Time        `json:"testedAt,omitempty"`
}

type MCPToolSummary struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Omitted credential keys are unchanged. Explicit removal uses Remove*Keys.
type MCPServerInput struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Transport        string            `json:"transport"`
	Command          string            `json:"command,omitempty"`
	Args             []string          `json:"args,omitempty"`
	URL              string            `json:"url,omitempty"`
	Enabled          bool              `json:"enabled"`
	Env              map[string]string `json:"env,omitempty"`
	Headers          map[string]string `json:"headers,omitempty"`
	RemoveEnvKeys    []string          `json:"removeEnvKeys,omitempty"`
	RemoveHeaderKeys []string          `json:"removeHeaderKeys,omitempty"`
}

type ResourceInput struct {
	Skill      *UserSkill      `json:"skill,omitempty"`
	Command    *UserCommand    `json:"command,omitempty"`
	MCP        *MCPServerInput `json:"mcp,omitempty"`
	ImportPath string          `json:"importPath,omitempty"`
}

func (r UserResourceState) Clone() UserResourceState {
	r.Skills = slices.Clone(r.Skills)
	for i := range r.Skills {
		r.Skills[i].Files = slices.Clone(r.Skills[i].Files)
	}
	r.Commands = slices.Clone(r.Commands)
	r.MCPServers = slices.Clone(r.MCPServers)
	for i := range r.MCPServers {
		s := &r.MCPServers[i]
		s.Args, s.EnvKeys, s.HeaderKeys = slices.Clone(s.Args), slices.Clone(s.EnvKeys), slices.Clone(s.HeaderKeys)
		s.Test.Tools = slices.Clone(s.Test.Tools)
	}
	return r
}

func (r *ResourceInput) Clone() *ResourceInput {
	if r == nil {
		return nil
	}
	v := *r
	v.Skill = copyPointer(r.Skill)
	if v.Skill != nil {
		v.Skill.Files = slices.Clone(v.Skill.Files)
	}
	v.Command = copyPointer(r.Command)
	v.MCP = copyPointer(r.MCP)
	if v.MCP != nil {
		v.MCP.Args = slices.Clone(v.MCP.Args)
		v.MCP.Env, v.MCP.Headers = maps.Clone(v.MCP.Env), maps.Clone(v.MCP.Headers)
		v.MCP.RemoveEnvKeys = slices.Clone(v.MCP.RemoveEnvKeys)
		v.MCP.RemoveHeaderKeys = slices.Clone(v.MCP.RemoveHeaderKeys)
	}
	return &v
}

func (c Command) validateResource() error {
	if c.Resource != nil && !strings.HasPrefix(c.Kind, "resource.") {
		return errors.New("resource payload does not match command")
	}
	if strings.HasPrefix(c.Kind, "resource.") &&
		(c.Goal != nil || c.Provider != nil || c.Git != nil || c.Send != nil || c.Settings != nil ||
			c.Session != nil || c.Attachment != nil || c.Integration != nil || c.ProjectCreate != nil ||
			c.Adjustment != nil || c.TerminalSize != nil) {
		return errors.New("resource commands accept only their resource payload")
	}
	switch c.Kind {
	case "resource.skill.save", "resource.skill.import":
		if c.Resource == nil || c.Resource.Skill == nil || c.Resource.Command != nil || c.Resource.MCP != nil {
			return errors.New("skill payload is required")
		}
		if c.Kind == "resource.skill.import" && strings.TrimSpace(c.Resource.ImportPath) == "" {
			return errors.New("skill import directory is required")
		}
	case "resource.command.save":
		if c.Resource == nil || c.Resource.Command == nil || c.Resource.Skill != nil || c.Resource.MCP != nil {
			return errors.New("command payload is required")
		}
	case "resource.mcp.save":
		if c.Resource == nil || c.Resource.MCP == nil || c.Resource.Skill != nil || c.Resource.Command != nil {
			return errors.New("MCP server payload is required")
		}
	case "resource.skill.delete", "resource.skill.toggle", "resource.command.delete", "resource.command.toggle",
		"resource.mcp.delete", "resource.mcp.toggle", "resource.mcp.test":
		if c.ID == "" || c.Resource != nil {
			return errors.New("resource ID and no resource payload are required")
		}
	case "resource.refresh":
		if c.Resource != nil {
			return errors.New("refresh does not accept a resource payload")
		}
	default:
		if strings.HasPrefix(c.Kind, "resource.") {
			return errors.New("unsupported resource operation")
		}
	}
	return nil
}
