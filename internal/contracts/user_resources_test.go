package contracts

import "testing"

func TestUserResourcesCloneOwnsAllMutableFields(t *testing.T) {
	input := Snapshot{UserResources: UserResourceState{
		Skills: []UserSkill{{ID: "skill", Files: []string{"script.go"}}},
		MCPServers: []MCPServer{{ID: "server", Args: []string{"arg"}, EnvKeys: []string{"KEY"}, HeaderKeys: []string{"Authorization"},
			Test: MCPTestResult{State: "ready", Tools: []MCPToolSummary{{Name: "tool"}}}}},
	}}
	clone := CloneSnapshot(input)
	clone.UserResources.Skills[0].Files[0] = "changed"
	clone.UserResources.MCPServers[0].Args[0] = "changed"
	clone.UserResources.MCPServers[0].EnvKeys[0] = "changed"
	clone.UserResources.MCPServers[0].HeaderKeys[0] = "changed"
	clone.UserResources.MCPServers[0].Test.Tools[0].Name = "changed"
	if input.UserResources.Skills[0].Files[0] != "script.go" ||
		input.UserResources.MCPServers[0].Args[0] != "arg" ||
		input.UserResources.MCPServers[0].EnvKeys[0] != "KEY" ||
		input.UserResources.MCPServers[0].HeaderKeys[0] != "Authorization" ||
		input.UserResources.MCPServers[0].Test.Tools[0].Name != "tool" {
		t.Fatal("cloned resource snapshot aliases mutable state")
	}
	command := Command{Kind: "resource.mcp.save", Resource: &ResourceInput{MCP: &MCPServerInput{
		Env: map[string]string{"KEY": "secret"}, Args: []string{"arg"}, RemoveHeaderKeys: []string{"Authorization"},
	}}}
	copied := command.Clone()
	copied.Resource.MCP.Env["KEY"] = "changed"
	copied.Resource.MCP.Args[0] = "changed"
	copied.Resource.MCP.RemoveHeaderKeys[0] = "changed"
	if command.Resource.MCP.Env["KEY"] != "secret" || command.Resource.MCP.Args[0] != "arg" || command.Resource.MCP.RemoveHeaderKeys[0] != "Authorization" {
		t.Fatal("resource command does not own its mutable inputs")
	}
}

func TestUserResourceCommandsRejectAmbiguousPayloads(t *testing.T) {
	tests := []Command{
		{Kind: "resource.skill.save", Resource: &ResourceInput{Skill: &UserSkill{}}, Settings: &Settings{}},
		{Kind: "resource.skill.save", Resource: &ResourceInput{Skill: &UserSkill{}, MCP: &MCPServerInput{}}},
		{Kind: "resource.skill.import", Resource: &ResourceInput{Skill: &UserSkill{}}},
		{Kind: "resource.skill.delete"},
		{Kind: "resource.command.toggle", ID: "command", Resource: &ResourceInput{}},
		{Kind: "resource.unknown"},
		{Kind: "message.send", Resource: &ResourceInput{}},
	}
	for _, command := range tests {
		t.Run(command.Kind, func(t *testing.T) {
			if err := command.Validate(); err == nil {
				t.Fatal("ambiguous or incomplete resource command was accepted")
			}
		})
	}
}
