package corerunner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gcode/internal/engine"
	"gcode/internal/mcp"
	"gcode/scripts/plugin-acceptance/acceptance"
)

// fixtureToolGuard is the test owner's policy at the actual core execution
// boundary. Direct Hook/MCP programs remain outside this model-tool policy.
func fixtureToolGuard(root string, readRoots []string, plan ProbePlan, component acceptance.Component, pluginName string) engine.ToolGuard {
	mcpTool, _ := mcp.QualifiedToolName(mcp.Config{Name: component.Name, Plugin: pluginName}, plan.MCPTool)
	return func(ctx context.Context, cwd, name string, raw json.RawMessage) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !under(root, cwd) {
			return errors.New("probe tool working directory escapes its private workspace")
		}
		var args map[string]json.RawMessage
		if err := json.Unmarshal(raw, &args); err != nil {
			return err
		}
		switch name {
		case "Read", "Write", "Edit", "ListDirectory", "Glob", "Grep":
			key := "file_path"
			if name == "ListDirectory" || name == "Glob" || name == "Grep" {
				key = "path"
			}
			var path string
			if value := args[key]; len(value) != 0 {
				if err := json.Unmarshal(value, &path); err != nil {
					return err
				}
			}
			if path == "" {
				if name == "Read" || name == "Write" || name == "Edit" {
					return errors.New("probe file tool requires an explicit file path")
				}
				path = cwd
			} else if !filepath.IsAbs(path) {
				path = filepath.Join(cwd, path)
			}
			path = filepath.Clean(path)
			writable := name == "Write" || name == "Edit"
			allowed := under(root, path)
			if !writable {
				for _, readRoot := range readRoots {
					allowed = allowed || under(readRoot, path)
				}
			}
			if !allowed {
				return errors.New("probe file access escapes its private workspace and read-only plugin sources")
			}
			if err := noSymlinks(path); err != nil {
				return err
			}
			if writable {
				for _, fixture := range plan.Fixtures {
					if immutableFixture(plan, fixture) && samePath(path, filepath.Join(root, filepath.FromSlash(fixture.Path))) {
						return errors.New("probe fixtures are immutable")
					}
				}
			}
			for _, output := range plan.VerifierOutputs {
				if samePath(path, filepath.Join(root, filepath.FromSlash(output))) {
					return errors.New("verifier outputs are owned by the explicitly allowed verifier process")
				}
			}
			if name == "Glob" {
				var pattern string
				_ = json.Unmarshal(args["pattern"], &pattern)
				if filepath.IsAbs(pattern) || strings.Contains(pattern, "..") || strings.ContainsAny(pattern, "\\:") {
					return errors.New("probe glob pattern must stay relative to its allowed path")
				}
			}
			return nil
		case "Bash":
			var command string
			if err := json.Unmarshal(args["command"], &command); err != nil {
				return err
			}
			if !contains(plan.AllowCommands, command) {
				return errors.New("probe shell command is not explicitly allowed by its business plan")
			}
			for _, fixture := range plan.Fixtures {
				if !immutableFixture(plan, fixture) {
					continue
				}
				path := filepath.Join(root, filepath.FromSlash(fixture.Path))
				if err := noSymlinks(path); err != nil {
					return err
				}
				data, err := os.ReadFile(path)
				if err != nil || string(data) != fixture.Text {
					return errors.New("immutable probe fixture changed before an allowed shell command")
				}
			}
			return nil
		case "Agent", "Skill", "TodoWrite", "TaskCreate", "TaskUpdate", "TaskGet", "TaskList":
			return nil
		default:
			if component.Kind == "mcp" && mcpTool != "" && name == mcpTool {
				return nil
			}
			return fmt.Errorf("tool %q is not authorized by this fixture plan", name)
		}
	}
}

func immutableFixture(plan ProbePlan, fixture Fixture) bool {
	for _, assertion := range plan.Assertions {
		if assertion.Kind == "file" && assertion.Path == fixture.Path && assertion.Equals != nil && *assertion.Equals == fixture.Text {
			return true
		}
	}
	return false
}

func under(root, path string) bool {
	if root == "" {
		return false
	}
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func samePath(first, second string) bool {
	if filepath.Separator == '\\' {
		return strings.EqualFold(filepath.Clean(first), filepath.Clean(second))
	}
	return filepath.Clean(first) == filepath.Clean(second)
}

func noSymlinks(path string) error {
	for {
		info, err := os.Lstat(path)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return errors.New("probe file access contains a symbolic link")
		}
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		parent := filepath.Dir(path)
		if parent == path {
			return nil
		}
		path = parent
	}
}
