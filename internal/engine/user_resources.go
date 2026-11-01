package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"gcode/internal/contracts"
	"gcode/internal/mcp"
	"gcode/internal/storage"
)

const (
	resourceCatalogFile = "user-resources.json"
	maxResourceText     = 256 << 10
	maxSkillBytes       = 8 << 20
	maxSkillFiles       = 256
)

var resourceIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,50}$`)
var mcpIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)

type userResourceCatalog struct {
	Skills   []contracts.UserSkill   `json:"skills"`
	Commands []contracts.UserCommand `json:"commands"`
	MCP      []mcp.Config            `json:"mcp"`
}

func readResourceCatalog(root string) (userResourceCatalog, error) {
	var catalog userResourceCatalog
	path := filepath.Join(root, resourceCatalogFile)
	if err := rejectResourceLinks(path); err != nil {
		return catalog, err
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return catalog, nil
	}
	if err != nil {
		return catalog, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 16<<20+1))
	if err != nil {
		return catalog, err
	}
	if len(data) > 16<<20 {
		return catalog, errors.New("user resource catalog exceeds 16 MiB")
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		return catalog, fmt.Errorf("read user resources: %w", err)
	}
	if len(catalog.Skills)+len(catalog.Commands)+len(catalog.MCP) > 500 {
		return catalog, errors.New("at most 500 user resources are supported")
	}
	seen := map[string]bool{}
	for _, skill := range catalog.Skills {
		if err := validateSkill(skill); err != nil {
			return catalog, err
		}
		if seen["skill:"+skill.ID] {
			return catalog, errors.New("duplicate skill ID")
		}
		seen["skill:"+skill.ID] = true
	}
	for _, command := range catalog.Commands {
		if err := validateUserCommand(command); err != nil {
			return catalog, err
		}
		if seen["command:"+command.ID] || seen["command-name:"+command.Name] {
			return catalog, errors.New("duplicate command ID or name")
		}
		seen["command:"+command.ID], seen["command-name:"+command.Name] = true, true
	}
	for _, server := range catalog.MCP {
		if err := validateMCPConfig(server); err != nil {
			return catalog, err
		}
		if seen["mcp:"+server.ID] || seen["mcp-name:"+server.Name] {
			return catalog, errors.New("duplicate MCP server ID or name")
		}
		seen["mcp:"+server.ID], seen["mcp-name:"+server.Name] = true, true
	}
	return catalog, nil
}

// Every managed operation rejects symlinks, including existing parents. OpenRoot
// additionally confines file operations to the selected managed directory.
func rejectResourceLinks(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	for current := absolute; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return errors.New("user resource paths must not contain symbolic links")
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if filepath.Dir(current) == current {
			return nil
		}
	}
}

func validateSkill(skill contracts.UserSkill) error {
	if !resourceIDPattern.MatchString(skill.ID) {
		return errors.New("skill ID must use 1–50 letters, numbers, underscores or hyphens")
	}
	if strings.TrimSpace(skill.Name) == "" || len(skill.Name) > 200 || len(skill.Description) > 4096 {
		return errors.New("skill name is required and its metadata must fit the size limit")
	}
	if strings.TrimSpace(skill.Content) == "" || len(skill.Content) > maxResourceText || !utf8.ValidString(skill.Content) {
		return errors.New("skill content must be nonempty UTF-8 text of at most 256 KiB")
	}
	if len(skill.Files) > maxSkillFiles {
		return errors.New("skill has too many support files")
	}
	for _, name := range skill.Files {
		if !fs.ValidPath(name) || strings.Contains(name, `\`) || strings.Contains(name, ":") {
			return errors.New("skill support file path is invalid")
		}
	}
	return nil
}

func validateUserCommand(command contracts.UserCommand) error {
	if !resourceIDPattern.MatchString(command.ID) || !resourceIDPattern.MatchString(command.Name) {
		return errors.New("command ID and name must use 1–50 letters, numbers, underscores or hyphens")
	}
	if command.Name == "init" || command.Name == "goal" || command.Name == "compact" || command.Name == "skill" {
		return errors.New("command name is reserved")
	}
	if strings.TrimSpace(command.Content) == "" || len(command.Content) > maxResourceText || !utf8.ValidString(command.Content) {
		return errors.New("command template must be nonempty UTF-8 text of at most 256 KiB")
	}
	if len(command.Description) > 4096 || len(command.ArgumentHint) > 1024 {
		return errors.New("command metadata exceeds the size limit")
	}
	return nil
}

func validateMCPConfig(config mcp.Config) error {
	if !resourceIDPattern.MatchString(config.ID) {
		return errors.New("MCP server ID must use 1–50 letters, numbers, underscores or hyphens")
	}
	if !mcpIDPattern.MatchString(config.Name) {
		return errors.New("MCP server name must use 1–40 letters, numbers, underscores or hyphens")
	}
	if (strings.TrimSpace(config.Command) == "") == (strings.TrimSpace(config.URL) == "") {
		return errors.New("MCP server requires exactly one executable or HTTP URL")
	}
	if len(config.Args) > 100 || len(config.Env) > 100 || len(config.Headers) > 100 {
		return errors.New("MCP server configuration exceeds the item limit")
	}
	if config.URL != "" {
		parsed, err := url.Parse(config.URL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.Fragment != "" {
			return errors.New("MCP URL must be an HTTP(S) URL without credentials or a fragment")
		}
		if config.Command != "" || len(config.Args) != 0 || len(config.Env) != 0 {
			return errors.New("HTTP MCP servers do not accept executable arguments or environment variables")
		}
	} else if len(config.Headers) != 0 {
		return errors.New("stdio MCP servers do not accept HTTP headers")
	}
	for key, value := range config.Env {
		if key == "" || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(value, 0) {
			return errors.New("invalid MCP environment variable")
		}
	}
	for key, value := range config.Headers {
		if key == "" || strings.ContainsAny(key, "\r\n: \t") || strings.ContainsAny(value, "\r\n") {
			return errors.New("invalid MCP HTTP header")
		}
	}
	return nil
}

func cloneResourceCatalog(c userResourceCatalog) userResourceCatalog {
	c.Skills = contracts.UserResourceState{Skills: c.Skills}.Clone().Skills
	c.Commands = slices.Clone(c.Commands)
	c.MCP = slices.Clone(c.MCP)
	for i := range c.MCP {
		c.MCP[i].Args = slices.Clone(c.MCP[i].Args)
		c.MCP[i].Env, c.MCP[i].Headers = maps.Clone(c.MCP[i].Env), maps.Clone(c.MCP[i].Headers)
	}
	return c
}

func (e *Engine) refreshUserResourcesLocked() {
	previous := e.state.UserResources.MCPServers
	public := contracts.UserResourceState{Skills: e.resources.Skills, Commands: e.resources.Commands}.Clone()
	for i := range public.Skills {
		public.Skills[i].Path = filepath.Join(e.storage.ConfigDirectory(), "skills", public.Skills[i].ID, "SKILL.md")
	}
	for _, config := range e.resources.MCP {
		server := contracts.MCPServer{
			ID: config.ID, Name: config.Name, Transport: "stdio", Command: config.Command,
			Args: slices.Clone(config.Args), URL: config.URL, Enabled: !config.Disabled,
			EnvKeys: slices.Sorted(maps.Keys(config.Env)), HeaderKeys: slices.Sorted(maps.Keys(config.Headers)),
			Test: contracts.MCPTestResult{State: "idle"},
		}
		if config.URL != "" {
			server.Transport = "http"
		}
		for _, old := range previous {
			if old.ID == server.ID {
				server.Test = old.Test
				break
			}
		}
		public.MCPServers = append(public.MCPServers, server)
	}
	e.state.UserResources = public
}

func (e *Engine) resourceCommandLocked(command contracts.Command) error {
	if command.Kind == "resource.refresh" {
		catalog, err := readResourceCatalog(e.storage.ConfigDirectory())
		if err != nil {
			return err
		}
		e.resources = catalog
		e.refreshUserResourcesLocked()
		for i := range e.state.UserResources.MCPServers {
			e.state.UserResources.MCPServers[i].Test = contracts.MCPTestResult{State: "idle"}
		}
		e.publishLocked(contracts.Event{Kind: "state.changed"})
		return nil
	}
	if command.Kind == "resource.mcp.test" {
		return e.startMCPTestLocked(command.ID)
	}
	next := cloneResourceCatalog(e.resources)
	var apply func() (func(bool), error)
	switch command.Kind {
	case "resource.skill.save", "resource.skill.import":
		skill := *command.Resource.Skill
		skill.Path = ""
		skill.ID, skill.Name = strings.TrimSpace(skill.ID), strings.TrimSpace(skill.Name)
		skill.Description = strings.TrimSpace(skill.Description)
		index := slices.IndexFunc(next.Skills, func(s contracts.UserSkill) bool { return s.ID == skill.ID })
		var source string
		if command.Kind == "resource.skill.import" {
			source = command.Resource.ImportPath
			data, files, err := readSkillImport(source)
			if err != nil {
				return err
			}
			skill.Content, skill.Files = string(data["SKILL.md"]), files
			if skill.Name == "" {
				skill.Name = skillMetadata(skill.Content, "name")
				if skill.Name == "" {
					skill.Name = skill.ID
				}
			}
			if skill.Description == "" {
				skill.Description = skillMetadata(skill.Content, "description")
			}
			apply = func() (func(bool), error) { return e.replaceSkillDirectory(skill.ID, data) }
		} else {
			if index >= 0 {
				skill.Files = slices.Clone(next.Skills[index].Files)
				source = filepath.Join(e.storage.ConfigDirectory(), "skills", skill.ID)
			} else {
				skill.Files = nil
			}
			apply = func() (func(bool), error) {
				files := map[string][]byte{}
				if source != "" {
					var err error
					files, _, err = readSkillImport(source)
					if err != nil {
						return nil, err
					}
				}
				files["SKILL.md"] = []byte(skill.Content)
				return e.replaceSkillDirectory(skill.ID, files)
			}
		}
		if err := validateSkill(skill); err != nil {
			return err
		}
		if index < 0 {
			next.Skills = append(next.Skills, skill)
		} else {
			next.Skills[index] = skill
		}
	case "resource.skill.delete", "resource.skill.toggle":
		index := slices.IndexFunc(next.Skills, func(s contracts.UserSkill) bool { return s.ID == command.ID })
		if index < 0 {
			return errors.New("skill not found")
		}
		if command.Kind == "resource.skill.toggle" {
			next.Skills[index].Enabled = command.Enabled
		} else {
			next.Skills = slices.Delete(next.Skills, index, index+1)
			apply = func() (func(bool), error) { return e.replaceSkillDirectory(command.ID, nil) }
		}
	case "resource.command.save":
		value := *command.Resource.Command
		value.ID, value.Name = strings.TrimSpace(value.ID), strings.TrimSpace(value.Name)
		if err := validateUserCommand(value); err != nil {
			return err
		}
		index := slices.IndexFunc(next.Commands, func(c contracts.UserCommand) bool { return c.ID == value.ID })
		for i, other := range next.Commands {
			if i != index && other.Name == value.Name {
				return errors.New("command name already exists")
			}
		}
		if index < 0 {
			next.Commands = append(next.Commands, value)
		} else {
			next.Commands[index] = value
		}
	case "resource.command.delete", "resource.command.toggle":
		index := slices.IndexFunc(next.Commands, func(c contracts.UserCommand) bool { return c.ID == command.ID })
		if index < 0 {
			return errors.New("command not found")
		}
		if command.Kind == "resource.command.toggle" {
			next.Commands[index].Enabled = command.Enabled
		} else {
			next.Commands = slices.Delete(next.Commands, index, index+1)
		}
	case "resource.mcp.save":
		value := command.Resource.MCP
		name := strings.TrimSpace(value.Name)
		config := mcp.Config{ID: value.ID, Name: name, Command: strings.TrimSpace(value.Command), URL: strings.TrimSpace(value.URL),
			Args: slices.Clone(value.Args), Disabled: !value.Enabled, Env: map[string]string{}, Headers: map[string]string{}}
		index := slices.IndexFunc(next.MCP, func(c mcp.Config) bool { return c.ID == value.ID })
		for i, other := range next.MCP {
			if i != index && other.Name == name {
				return errors.New("MCP server name already exists")
			}
		}
		if index >= 0 {
			config.Env, config.Headers = maps.Clone(next.MCP[index].Env), maps.Clone(next.MCP[index].Headers)
		}
		if config.Env == nil {
			config.Env = map[string]string{}
		}
		if config.Headers == nil {
			config.Headers = map[string]string{}
		}
		if value.Transport == "stdio" {
			if config.URL != "" {
				return errors.New("stdio MCP servers do not accept a URL")
			}
			config.Headers = map[string]string{}
		} else if value.Transport == "http" {
			if config.Command != "" || len(config.Args) != 0 {
				return errors.New("HTTP MCP servers do not accept an executable")
			}
			config.Env = map[string]string{}
		} else {
			return errors.New("MCP transport must be stdio or http")
		}
		maps.Copy(config.Env, value.Env)
		maps.Copy(config.Headers, value.Headers)
		for _, key := range value.RemoveEnvKeys {
			delete(config.Env, key)
		}
		for _, key := range value.RemoveHeaderKeys {
			delete(config.Headers, key)
		}
		if err := validateMCPConfig(config); err != nil {
			return err
		}
		if index < 0 {
			next.MCP = append(next.MCP, config)
		} else {
			next.MCP[index] = config
		}
	case "resource.mcp.delete", "resource.mcp.toggle":
		index := slices.IndexFunc(next.MCP, func(c mcp.Config) bool { return c.ID == command.ID })
		if index < 0 {
			return errors.New("MCP server not found")
		}
		if command.Kind == "resource.mcp.toggle" {
			next.MCP[index].Disabled = !command.Enabled
		} else {
			next.MCP = slices.Delete(next.MCP, index, index+1)
		}
	default:
		return errors.New("unsupported user resource operation")
	}
	if len(next.Skills)+len(next.Commands)+len(next.MCP) > 500 {
		return errors.New("at most 500 user resources are supported")
	}
	encoded, err := json.Marshal(next)
	if err != nil || len(encoded) > 16<<20 {
		return errors.New("user resource catalog exceeds 16 MiB")
	}
	root := e.storage.ConfigDirectory()
	if err := rejectResourceLinks(filepath.Join(root, resourceCatalogFile)); err != nil {
		return err
	}
	var finish func(bool)
	if apply != nil {
		var err error
		finish, err = apply()
		if err != nil {
			return err
		}
	}
	if err := storage.WriteJSON(filepath.Join(root, resourceCatalogFile), next); err != nil {
		if finish != nil {
			finish(false)
		}
		return err
	}
	if finish != nil {
		finish(true)
	}
	e.resources = next
	e.refreshUserResourcesLocked()
	if strings.HasPrefix(command.Kind, "resource.mcp.") {
		for i := range e.state.UserResources.MCPServers {
			if e.state.UserResources.MCPServers[i].ID == command.ID || command.Kind == "resource.mcp.save" && e.state.UserResources.MCPServers[i].ID == command.Resource.MCP.ID {
				e.state.UserResources.MCPServers[i].Test = contracts.MCPTestResult{State: "idle"}
			}
		}
	}
	e.publishLocked(contracts.Event{Kind: "state.changed"})
	return nil
}

func skillMetadata(content, key string) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return ""
	}
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			break
		}
		if value, ok := strings.CutPrefix(line, key+":"); ok {
			return strings.Trim(strings.TrimSpace(value), `"'`)
		}
	}
	return ""
}

func readSkillImport(source string) (map[string][]byte, []string, error) {
	if !filepath.IsAbs(source) {
		return nil, nil, errors.New("skill import directory must be absolute")
	}
	if err := rejectResourceLinks(source); err != nil {
		return nil, nil, err
	}
	root, err := os.OpenRoot(source)
	if err != nil {
		return nil, nil, err
	}
	defer root.Close()
	files := map[string][]byte{}
	var names []string
	var total int
	var visited int
	err = fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		visited++
		if visited > 1024 || strings.Count(path, "/") > 16 {
			return errors.New("skill import directory tree exceeds the traversal limit")
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() && !entry.Type().IsRegular() {
			return errors.New("skill import may contain only regular files and directories")
		}
		if entry.IsDir() {
			return nil
		}
		if !fs.ValidPath(path) || strings.ContainsAny(path, `\:`) {
			return errors.New("invalid skill support file path")
		}
		if len(files) >= maxSkillFiles {
			return errors.New("skill import exceeds the 256-file limit")
		}
		f, err := root.Open(filepath.FromSlash(path))
		if err != nil {
			return err
		}
		data, readErr := io.ReadAll(io.LimitReader(f, int64(maxSkillBytes-total+1)))
		err = errors.Join(readErr, f.Close())
		if err != nil {
			return err
		}
		total += len(data)
		if total > maxSkillBytes {
			return errors.New("skill import exceeds the 8 MiB limit")
		}
		files[path] = data
		if path != "SKILL.md" {
			names = append(names, path)
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if data, ok := files["SKILL.md"]; !ok || len(data) > maxResourceText || !utf8.Valid(data) {
		return nil, nil, errors.New("skill import requires a UTF-8 SKILL.md of at most 256 KiB")
	}
	sort.Strings(names)
	return files, names, nil
}

// replaceSkillDirectory returns a rollback callback. The catalog is the commit
// point; directories are staged on the same filesystem, never merged in place.
func (e *Engine) replaceSkillDirectory(skillID string, files map[string][]byte) (func(bool), error) {
	if !resourceIDPattern.MatchString(skillID) {
		return nil, errors.New("invalid skill ID")
	}
	base := filepath.Join(e.storage.ConfigDirectory(), "skills")
	target := filepath.Join(base, skillID)
	if err := rejectResourceLinks(target); err != nil {
		return nil, err
	}
	if _, err := os.Stat(target); err == nil {
		if _, _, err := readSkillImport(target); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	root, err := os.OpenRoot(base)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	stage, backup := ".stage-"+id("skill"), ".backup-"+id("skill")
	if files != nil {
		if err := root.Mkdir(stage, 0700); err != nil {
			return nil, err
		}
		defer root.RemoveAll(stage)
		for name, data := range files {
			path := filepath.Join(stage, filepath.FromSlash(name))
			if err := root.MkdirAll(filepath.Dir(path), 0700); err != nil {
				return nil, err
			}
			if err := root.WriteFile(path, data, 0600); err != nil {
				return nil, err
			}
		}
	}
	existed := false
	if _, err := root.Stat(skillID); err == nil {
		if err := root.Rename(skillID, backup); err != nil {
			return nil, err
		}
		existed = true
	}
	if files != nil {
		if err := root.Rename(stage, skillID); err != nil {
			if existed {
				_ = root.Rename(backup, skillID)
			}
			return nil, err
		}
	}
	finish := func(committed bool) {
		r, err := os.OpenRoot(base)
		if err != nil {
			return
		}
		defer r.Close()
		if committed {
			if existed {
				_ = r.RemoveAll(backup)
			}
			return
		}
		_ = r.RemoveAll(skillID)
		if existed {
			_ = r.Rename(backup, skillID)
		}
	}
	return finish, nil
}

func (e *Engine) startMCPTestLocked(serverID string) error {
	index := slices.IndexFunc(e.resources.MCP, func(c mcp.Config) bool { return c.ID == serverID })
	if index < 0 {
		return errors.New("MCP server not found")
	}
	publicIndex := slices.IndexFunc(e.state.UserResources.MCPServers, func(c contracts.MCPServer) bool { return c.ID == serverID })
	if e.state.UserResources.MCPServers[publicIndex].Test.State == "testing" {
		return errors.New("MCP server test is already running")
	}
	config := cloneResourceCatalog(userResourceCatalog{MCP: []mcp.Config{e.resources.MCP[index]}}).MCP[0]
	fingerprint, _ := json.Marshal(config)
	environment, err := e.processEnvironment(e.storage.ActiveRootID(), "mcp-test-"+newShortID())
	if err != nil {
		return err
	}
	processConfig := mcpProcessEnvironment(config, environment)
	started := time.Now()
	e.state.UserResources.MCPServers[publicIndex].Test = contracts.MCPTestResult{State: "testing", TestedAt: started}
	e.publishLocked(contracts.Event{Kind: "state.changed"})
	root := e.storage.ConfigDirectory()
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		go func() {
			select {
			case <-e.done:
				cancel()
			case <-ctx.Done():
			}
		}()
		result := contracts.MCPTestResult{State: "ready", TestedAt: time.Now()}
		client, err := connectMCP(ctx, processConfig, root)
		if err == nil {
			tools, listErr := client.Tools(ctx)
			err = listErr
			if err == nil {
				if len(tools) > 1000 {
					err = errors.New("MCP test tool list exceeds 1000 tools")
				}
				for _, tool := range tools {
					if err != nil {
						break
					}
					definition := tool.Definition()
					result.Tools = append(result.Tools, contracts.MCPToolSummary{
						Name: redactMCPError(definition.Name, config), Description: redactMCPError(definition.Description, config),
					})
				}
			}
			_ = client.Close()
		}
		if err != nil {
			result.State = "error"
			result.Error = redactMCPError(err.Error(), config)
		}
		e.mu.Lock()
		defer e.mu.Unlock()
		if e.closed {
			return
		}
		current := slices.IndexFunc(e.resources.MCP, func(c mcp.Config) bool { return c.ID == serverID })
		if current < 0 {
			return
		}
		now, _ := json.Marshal(e.resources.MCP[current])
		if !bytes.Equal(now, fingerprint) {
			return
		}
		for i := range e.state.UserResources.MCPServers {
			if e.state.UserResources.MCPServers[i].ID == serverID {
				current := e.state.UserResources.MCPServers[i].Test
				if current.State == "testing" && current.TestedAt.Equal(started) {
					e.state.UserResources.MCPServers[i].Test = result
				}
			}
		}
		e.publishLocked(contracts.Event{Kind: "state.changed"})
	}()
	return nil
}

func redactMCPError(message string, config mcp.Config) string {
	message = mcp.SanitizeText(message, config)
	if len(message) > 4096 {
		message = message[:4096] + "…"
	}
	return message
}
