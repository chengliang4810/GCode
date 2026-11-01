package engine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/plugins"
)

func skillInstructions(root, _ string) string {
	catalog, err := readResourceCatalog(root)
	if err != nil {
		return ""
	}
	var out strings.Builder
	for _, skill := range catalog.Skills {
		if !skill.Enabled {
			continue
		}
		if out.Len() == 0 {
			out.WriteString("\n\nAvailable user skills (read the matching SKILL.md with the read tool only when relevant):")
		}
		out.WriteString("\n- " + skill.ID + ": " + skill.Description + " (" + filepath.Join(root, "skills", skill.ID, "SKILL.md") + ")")
		if out.Len() > 32000 {
			break
		}
	}
	return out.String()
}

func expandPrompt(root, cwd string, text string) (string, error) {
	plain, additions, err := promptResources(root, text)
	if err != nil {
		return "", err
	}
	prompt, err := expandCommand(root, cwd, plain)
	if err != nil {
		return "", err
	}
	if additions != "" {
		prompt += "\n\n" + additions
	}
	return prompt, nil
}

func promptResources(root, text string) (string, string, error) {
	var additions []string
	var catalog *userResourceCatalog
	seen := make(map[string]bool)
	for _, part := range contracts.ParsePrompt(text) {
		ref := part.Reference
		if ref == nil || ref.Kind == "command" {
			continue
		}
		if seen[ref.Kind+":"+ref.ID] {
			continue
		}
		seen[ref.Kind+":"+ref.ID] = true
		if catalog == nil {
			value, err := readResourceCatalog(root)
			if err != nil {
				return "", "", err
			}
			catalog = &value
		}
		found := false
		switch ref.Kind {
		case "skill":
			for _, skill := range catalog.Skills {
				if skill.ID == ref.ID && skill.Enabled {
					additions = append(additions, "User-selected skill "+skill.ID+":\n"+skill.Content)
					found = true
				}
			}
		case "mcp":
			for _, server := range catalog.MCP {
				if server.ID == ref.ID && !server.Disabled {
					additions = append(additions, "The user referenced MCP server "+server.ID+" ("+server.Name+"). Use its available tools when relevant; this reference does not enable, install or authenticate a server.")
					found = true
				}
			}
		}
		if !found {
			return "", "", errors.New("referenced " + ref.Kind + " not found or disabled: " + ref.ID)
		}
	}
	return contracts.PlainPrompt(text), strings.Join(additions, "\n\n"), nil
}

func expandCommand(root, cwd string, text string) (string, error) {
	fields := strings.Fields(text)
	if len(fields) == 0 || !strings.HasPrefix(fields[0], "/") {
		return text, nil
	}
	name := strings.TrimPrefix(fields[0], "/")
	if name == "init" {
		return "Inspect this workspace and create or update its AGENTS.md with concise, accurate build, test and development instructions. Read existing instructions first, preserve user-owned guidance, and verify every repository-specific claim.\n\n" +
			strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), fields[0])), nil
	}
	skill := strings.HasPrefix(name, "skill:")
	// Plugin and project invocation metadata is expanded on the run worker
	// after SessionStart and input admission hooks.
	if strings.Contains(name, ":") && !skill {
		return text, nil
	}
	name = strings.TrimPrefix(name, "skill:")
	if !resourceIDPattern.MatchString(name) {
		return "", errors.New("invalid user command or skill name")
	}
	catalog, err := readResourceCatalog(root)
	if err != nil {
		return "", err
	}
	arguments := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), fields[0]))
	if skill {
		for _, entry := range catalog.Skills {
			if entry.ID == name && entry.Enabled {
				return entry.Content + "\n\n" + arguments, nil
			}
		}
		return "", errors.New("user skill not found or disabled")
	}
	if cwd != "" {
		for _, path := range []string{filepath.Join(cwd, ".claude", "commands", name+".md"), filepath.Join(cwd, ".claude", "skills", name, "SKILL.md")} {
			if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
				return text, nil
			}
		}
	}
	for _, entry := range catalog.Commands {
		if entry.Name != name || !entry.Enabled {
			continue
		}
		component, err := plugins.ParseMarkdownText(entry.Content, filepath.Join(root, "commands", entry.Name+".md"))
		if err != nil {
			return "", err
		}
		return substituteInvocation(component.Content, arguments, component.Metadata.Arguments)
	}
	for _, entry := range catalog.Skills {
		if entry.ID == name && entry.Enabled {
			return text, nil
		}
	}
	return "", errors.New("user command not found or disabled")
}
