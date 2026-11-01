package plugins

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAcceptsExplicitAllToolsHookMatcher(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, ".claude-plugin/plugin.json", `{"name":"all-tools"}`)
	writeTestFile(t, root, "hooks/hooks.json", `{"hooks":{"PreToolUse":[{"matcher":"*","hooks":[{"type":"command","command":"do-not-execute"}]}]}}`)
	bundle, err := Load(root)
	if err != nil || len(bundle.Hooks["PreToolUse"]) != 1 || bundle.Hooks["PreToolUse"][0].Matcher != "*" {
		t.Fatalf("public all-tools matcher was rejected or rewritten: %+v %v", bundle.Hooks, err)
	}
}

func TestOptionalArgumentHintsAndMalformedYAMLDoNotRejectSkillBody(t *testing.T) {
	component, err := ParseMarkdownText("---\nargument-hint: [project-name, optional-path]\n---\nCreate the project.", "fixture.md")
	if err != nil || component.Metadata.ArgumentHint != "project-name,optional-path" || component.Content != "Create the project." {
		t.Fatalf("optional argument hint rejected the skill: %+v %v", component, err)
	}
	content := "---\ndescription: broken: yaml\n---\nKeep this skill usable."
	component, err = ParseMarkdownText(content, "fixture.md")
	if err != nil || component.Content != content || len(component.Warnings) == 0 || len(component.Frontmatter) != 0 {
		t.Fatalf("invalid YAML did not remain a warned body: %+v %v", component, err)
	}
}

func TestCommandIdentityUsesPathAndAgentNameReplacesOnlyLastSegment(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, ".claude-plugin/plugin.json", `{"name":"demo"}`)
	writeTestFile(t, root, "commands/db/migrate.md", "---\nname: demo:display-only\n---\nMigrate fixture.")
	writeTestFile(t, root, "agents/db/check.md", "---\nname: reviewer\n---\nReview fixture.")
	bundle, err := Load(root)
	if err != nil || len(bundle.Commands) != 1 || bundle.Commands[0].Name != "demo:db:migrate" ||
		len(bundle.Agents) != 1 || bundle.Agents[0].Name != "demo:db:reviewer" {
		t.Fatalf("component naming rules were conflated: %+v %v", bundle, err)
	}
}

func writeTestFile(t *testing.T, root, name, text string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadComponentRules(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTestFile(t, root, ".claude-plugin/plugin.json", `{
		"name":"demo","commands":"./custom/run.md","skills":"./extra",
		"agents":["./custom/agent.md"],"outputStyles":"./custom/style.md",
		"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"echo inline"}]}]},
		"mcpServers":{"same":{"command":"inline"}}, "lspServers":{"lang":{"command":"gopls","extensionToLanguage":{".go":"go"}}},
		"userConfig":{"token":{"type":"string","title":"Token","description":"API token","sensitive":true}},
		"settings":{"agent":"demo:agent","env":{"BAD":"value"}}
	}`)
	for _, name := range []string{"commands/old.md", "agents/old.md", "output-styles/old.md", "custom/run.md", "custom/agent.md", "custom/style.md", "skills/default/SKILL.md", "extra/added/SKILL.md"} {
		writeTestFile(t, root, name, "---\ndescription: |\n  First line\n  Second line\nallowed-tools: [Read, 'Bash(git:*)']\ncontext: fork\nagent: Explore\nhooks:\n  Stop:\n    - hooks:\n        - type: command\n          command: echo done\n---\nHello $ARGUMENTS")
	}
	writeTestFile(t, root, "hooks/hooks.json", `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"echo default"}]}]}}`)
	writeTestFile(t, root, ".mcp.json", `{"same":{"command":"default"},"other":{"type":"http","url":"https://example.test/mcp"}}`)
	b, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Commands) != 1 || b.Commands[0].Name != "demo:run" {
		t.Fatalf("commands: %#v", b.Commands)
	}
	if len(b.Agents) != 1 || b.Agents[0].Name != "demo:agent" {
		t.Fatalf("agents: %#v", b.Agents)
	}
	if len(b.OutputStyles) != 1 || len(b.Skills) != 2 {
		t.Fatalf("replacement/addition failed: %+v", b)
	}
	if len(b.Hooks["SessionStart"]) != 2 || b.MCPServers["same"].Command != "inline" || len(b.MCPServers) != 2 {
		t.Fatalf("configuration merge failed: %+v", b)
	}
	skill := b.Skills[0]
	if skill.Metadata.Context != "fork" || skill.Metadata.Agent != "Explore" || len(skill.Metadata.Hooks["Stop"]) != 1 || len(skill.Metadata.AllowedTools) != 2 {
		t.Fatalf("frontmatter lost fields: %+v", skill.Metadata)
	}
	if !strings.Contains(skill.Metadata.Description, "Second line") {
		t.Fatal("YAML multiline description lost")
	}
	if b.Settings["agent"] != "demo:agent" || len(b.Settings) != 1 || len(b.Unsupported) != 1 {
		t.Fatalf("settings filtering: %+v", b.Settings)
	}
	clone := b.Clone()
	clone.Hooks["SessionStart"][0].Hooks[0].Command = "mutated"
	clone.Skills[0].Frontmatter["agent"] = "mutated"
	if b.Hooks["SessionStart"][0].Hooks[0].Command == "mutated" || b.Skills[0].Frontmatter["agent"] == "mutated" {
		t.Fatal("Clone aliases mutable state")
	}
}

func TestLoadInlineCommandAndMissingManifest(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	manifest := Manifest{Name: "inline", Commands: []byte(`{"help":{"content":"---\ndescription: Help\n---\nExplain ${CLAUDE_PLUGIN_ROOT}","allowedTools":["Read"]}}`)}
	b, err := LoadWithManifest(root, manifest, Source{})
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Commands) != 1 || b.Commands[0].Name != "inline:help" || b.Commands[0].Content != "Explain ${CLAUDE_PLUGIN_ROOT}" {
		t.Fatalf("inline content: %+v", b.Commands)
	}
	if !b.Commands[0].Metadata.UserInvocable {
		t.Fatal("default user-invocable must be true")
	}
}

func TestLoadRejectsUnsafeDeclarations(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, manifest string }{
		{"path escape", `{"name":"demo","skills":["./../../outside"]}`},
		{"absolute path", `{"name":"demo","commands":"C:/outside.md"}`},
		{"missing file", `{"name":"demo","commands":"./missing.md"}`},
		{"agents directory", `{"name":"demo","agents":"./agents"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeTestFile(t, root, ".claude-plugin/plugin.json", tc.manifest)
			writeTestFile(t, root, "agents/example.md", "agent")
			writeTestFile(t, root, "skills/example/SKILL.md", "---\nallowed-tools: [Read\n---\nbody")
			if _, err := Load(root); err == nil {
				t.Fatal("unsafe definition accepted")
			}
		})
	}
}

func TestParseMarkdownRealYAML(t *testing.T) {
	t.Parallel()
	c, err := parseMarkdown([]byte("\ufeff---\r\nname: quoted\r\ndescription: >-\r\n  Folded text with a colon:\r\n  remains valid\r\nallowed-tools: Read, Bash(git:*)\r\nuser-invocable: false\r\n---\r\n# Body\r\n"), "example.md")
	if err != nil {
		t.Fatal(err)
	}
	if c.Metadata.UserInvocable || c.Metadata.Description != "Folded text with a colon: remains valid" || len(c.Metadata.AllowedTools) != 2 || !strings.HasPrefix(c.Content, "# Body") {
		t.Fatalf("incorrect YAML parsing: %+v", c)
	}
}

func TestParseMarkdownDisallowedTools(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"Read, Bash(git status *)", "[Read, 'Bash(git status *)']"} {
		component, err := ParseMarkdownText("---\ndisallowed-tools: "+value+"\n---\nbody", "inline.md")
		if err != nil {
			t.Fatal(err)
		}
		if len(component.Metadata.DisallowedTools) != 2 || component.Metadata.DisallowedTools[1] != "Bash(git status *)" {
			t.Fatalf("disallowed-tools parsing: %+v", component.Metadata.DisallowedTools)
		}
	}
	if _, err := ParseMarkdownText("---\ndisallowed-tools: [Read, 17]\n---\nbody", "inline.md"); err == nil {
		t.Fatal("numeric disallowed-tools entry accepted")
	}
}

func TestMarketplaceRootEntrySkillsAreExplicitWhileManifestSkillsAddDefaults(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTestFile(t, root, "skills/first/SKILL.md", "first")
	writeTestFile(t, root, "skills/second/SKILL.md", "second")
	fallback := Manifest{Name: "subset", Skills: []byte(`["./skills/first"]`)}
	rootBundle, err := LoadWithManifest(root, fallback, Source{Relative: "./"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rootBundle.Skills) != 1 || rootBundle.Skills[0].Name != "subset:first" {
		t.Fatalf("marketplace-root entry exposed unrelated skills: %+v", rootBundle.Skills)
	}
	writeTestFile(t, root, ".claude-plugin/marketplace.json", `{"name":"unrelated-market-file"}`)
	writeTestFile(t, root, ".claude-plugin/plugin.json", `{"name":"subset","skills":["./skills/first"]}`)
	manifestBundle, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifestBundle.Skills) != 2 {
		t.Fatal("presence of a marketplace file incorrectly changed manifest skills semantics")
	}
}

func TestMarketplaceHooksOverrideByEventWithoutDuplicatingFallback(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTestFile(t, root, ".claude-plugin/plugin.json", `{"name":"hooks","hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"original"}]}],"Stop":[{"hooks":[{"type":"command","command":"keep"}]}]}}`)
	fallback := Manifest{Name: "hooks", Hooks: []byte(`{"SessionStart":[{"hooks":[{"type":"command","command":"entry"}]}]}`)}
	bundle, err := LoadWithManifest(root, fallback, Source{})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Hooks["SessionStart"]) != 1 || bundle.Hooks["SessionStart"][0].Hooks[0].Command != "entry" || len(bundle.Hooks["Stop"]) != 1 {
		t.Fatalf("marketplace event overlay incorrect: %+v", bundle.Hooks)
	}
	writeTestFile(t, root, ".claude-plugin/plugin.json", `{"name":"hooks"}`)
	bundle, err = LoadWithManifest(root, fallback, Source{})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Hooks["SessionStart"]) != 1 {
		t.Fatal("fallback hook was merged twice")
	}
}

func TestHookExecFormAndFutureFieldsRemainVisible(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTestFile(t, root, ".claude-plugin/plugin.json", `{"name":"hooks"}`)
	writeTestFile(t, root, "hooks/hooks.json", `{"hooks":{"PreToolUse":[{"matcher":"Read","hooks":[
		{"type":"command","command":"script","args":[],"shell":"powershell","onFailure":"block","futurePolicy":{"mode":"audit"}},
		{"type":"mcp_tool","server":"plugin:hooks:server","tool":"audit","input":{"file":"${tool_input.file_path}"}}
	]}]}}`)
	bundle, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	hook := bundle.Hooks["PreToolUse"][0].Hooks[0]
	if hook.Args == nil || hook.Shell != "powershell" || hook.OnFailure != "block" {
		t.Fatalf("exec form fields lost: %+v", hook)
	}
	if len(bundle.Unsupported) != 1 || hook.Unknown["futurePolicy"] == nil {
		t.Fatal("future hook field was silently dropped")
	}
	encoded, err := json.Marshal(hook)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"args":[]`) || !strings.Contains(string(encoded), `"futurePolicy"`) {
		t.Fatalf("hook marshal loses exec form or unknown data: %s", encoded)
	}
	mcp := bundle.Hooks["PreToolUse"][0].Hooks[1]
	if mcp.Server != "plugin:hooks:server" || mcp.Tool != "audit" || mcp.Input["file"] != "${tool_input.file_path}" {
		t.Fatalf("MCP hook fields lost: %+v", mcp)
	}
}
