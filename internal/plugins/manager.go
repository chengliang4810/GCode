package plugins

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"gcode/internal/processutil"
)

type registry struct {
	Sources       []MarketplaceSource `json:"sources"`
	Catalogs      []Catalog           `json:"catalogs"`
	Installations []Installation      `json:"installations"`
}

// Manager serializes mutations and persists its registry only after a full operation
// succeeds. Installed directories are immutable and retained for running sessions.
type Manager struct {
	root string
	opMu sync.Mutex
	mu   sync.RWMutex
	data registry
}

func NewManager(root string) (*Manager, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	if err := validateStoredLinks(root); err != nil {
		return nil, err
	}
	m := &Manager{root: root}
	data, err := readDefinition(filepath.Join(root, "registry.json"))
	if err == nil {
		if err := json.Unmarshal(data, &m.data); err != nil {
			return nil, fmt.Errorf("plugin registry: %w", err)
		}
		for _, install := range m.data.Installations {
			if !inside(root, install.Path) {
				return nil, fmt.Errorf("installed path outside managed root: %s", install.Path)
			}
		}
		for _, catalog := range m.data.Catalogs {
			if !inside(root, catalog.Root) {
				return nil, fmt.Errorf("catalog path outside managed root: %s", catalog.Root)
			}
		}
		return m, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	m.data.Sources = []MarketplaceSource{
		{ID: "official", Name: "claude-plugins-official", Source: Source{Source: "github", Repo: "anthropics/claude-plugins-official"}, Builtin: true, Enabled: true, AutoUpdate: true},
		{ID: "community", Name: "claude-community", Source: Source{Source: "github", Repo: "anthropics/claude-plugins-community"}, Builtin: true, Enabled: true},
	}
	if err := m.save(m.data); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Manager) Sources() []MarketplaceSource {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return copyValue(m.data.Sources)
}

func (m *Manager) Catalogs() []Catalog {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return copyValue(m.data.Catalogs)
}

func (m *Manager) Installed() []Installation {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := copyValue(m.data.Installations)
	selected := selectInstallations(m.data.Installations)
	for i := range result {
		result[i].Issues = installationIssues(result[i], selected, map[string]bool{})
	}
	return result
}

func (m *Manager) PutSource(ctx context.Context, source MarketplaceSource) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validName.MatchString(source.ID) || !validName.MatchString(source.Name) {
		return errors.New("invalid marketplace source ID or name")
	}
	if source.Source.Relative != "" {
		return errors.New("marketplace source cannot be relative")
	}
	switch source.Source.Source {
	case "github", "git", "url", "directory", "file", "settings":
	default:
		return fmt.Errorf("unsupported marketplace source %q", source.Source.Source)
	}
	m.opMu.Lock()
	defer m.opMu.Unlock()
	next := copyValue(m.data)
	found := false
	for i, old := range next.Sources {
		if old.ID == source.ID {
			source.Builtin = old.Builtin
			next.Sources[i] = source
			found = true
		} else if old.Name == source.Name {
			return errors.New("marketplace name is already registered")
		}
	}
	if !found {
		next.Sources = append(next.Sources, source)
	}
	// A changed source must be refreshed before another installation can use its catalog.
	next.Catalogs = slices.DeleteFunc(next.Catalogs, func(c Catalog) bool { return c.SourceID == source.ID })
	return m.commit(next)
}

func (m *Manager) RemoveSource(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.opMu.Lock()
	defer m.opMu.Unlock()
	next := copyValue(m.data)
	before := len(next.Sources)
	next.Sources = slices.DeleteFunc(next.Sources, func(s MarketplaceSource) bool { return s.ID == id })
	if len(next.Sources) == before {
		return fmt.Errorf("unknown source %q", id)
	}
	next.Catalogs = slices.DeleteFunc(next.Catalogs, func(c Catalog) bool { return c.SourceID == id })
	// Existing installations remain usable after removing their marketplace.
	return m.commit(next)
}

func (m *Manager) Refresh(ctx context.Context, id string) (Catalog, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	var source *MarketplaceSource
	for i := range m.data.Sources {
		if m.data.Sources[i].ID == id {
			source = &m.data.Sources[i]
			break
		}
	}
	if source == nil {
		return Catalog{}, fmt.Errorf("unknown marketplace source %q", id)
	}
	if !source.Enabled {
		return Catalog{}, errors.New("marketplace source is disabled")
	}
	catalog, err := m.prepareCatalog(ctx, *source)
	if err != nil {
		return Catalog{}, err
	}
	next := copyValue(m.data)
	next.Catalogs = slices.DeleteFunc(next.Catalogs, func(c Catalog) bool { return c.SourceID == id })
	next.Catalogs = append(next.Catalogs, catalog)
	if err := m.commit(next); err != nil {
		return Catalog{}, err
	}
	return copyValue(catalog), nil
}

// AddSource identifies a marketplace from its location and publishes its source
// and validated catalog together. Failed additions never register an empty market.
func (m *Manager) AddSource(ctx context.Context, location string) (MarketplaceSource, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	origin, err := parseMarketplaceLocation(location)
	if err != nil {
		return MarketplaceSource{}, err
	}
	source := MarketplaceSource{Source: origin, Enabled: true}
	catalog, err := m.prepareCatalog(ctx, source)
	if err != nil {
		return MarketplaceSource{}, err
	}
	source.ID, source.Name = catalog.Marketplace.Name, catalog.Marketplace.Name
	catalog.SourceID = source.ID
	next := copyValue(m.data)
	next.Sources = append(next.Sources, source)
	next.Catalogs = append(next.Catalogs, catalog)
	if err := m.commit(next); err != nil {
		return MarketplaceSource{}, err
	}
	return source, nil
}

func (m *Manager) prepareCatalog(ctx context.Context, source MarketplaceSource) (Catalog, error) {
	stage, err := m.stage()
	if err != nil {
		return Catalog{}, err
	}
	defer os.RemoveAll(stage)
	tree := filepath.Join(stage, "tree")
	materialSource := source.Source
	if materialSource.Source == "url" {
		materialSource.Source = "marketplace-url"
	}
	sourceTree, commit, err := snapshotMarketplace(ctx, materialSource, tree, source.Headers)
	if err != nil {
		return Catalog{}, err
	}
	manifestPath := ".claude-plugin/marketplace.json"
	if source.Source.Path != "" && source.Source.Source != "directory" && source.Source.Source != "file" {
		manifestPath = source.Source.Path
	}
	if source.Source.Source == "url" || source.Source.Source == "file" {
		manifestPath = "marketplace.json"
	}
	data, err := readSourceDefinition(sourceTree, manifestPath)
	if err != nil {
		return Catalog{}, err
	}
	var marketplace Marketplace
	if err := json.Unmarshal(data, &marketplace); err != nil {
		return Catalog{}, fmt.Errorf("marketplace manifest: %w", err)
	}
	if !validName.MatchString(marketplace.Name) || marketplace.Owner.Name == "" {
		return Catalog{}, errors.New("marketplace requires a valid name and owner")
	}
	if source.ID == "" {
		for _, registered := range m.data.Sources {
			if registered.ID == marketplace.Name || registered.Name == marketplace.Name {
				return Catalog{}, errors.New("marketplace is already registered")
			}
		}
	}
	seen := map[string]bool{}
	for _, entry := range marketplace.Plugins {
		if !validName.MatchString(entry.Name) || seen[entry.Name] {
			return Catalog{}, fmt.Errorf("invalid or duplicate plugin name %q", entry.Name)
		}
		seen[entry.Name] = true
		if entry.Source.Relative != "" {
			if !strings.HasPrefix(entry.Source.Relative, "./") {
				return Catalog{}, errors.New("relative plugin sources must start with ./")
			}
			if source.Source.Source == "url" || source.Source.Source == "file" || source.Source.Source == "settings" {
				return Catalog{}, errors.New("URL/file/inline marketplaces require remote plugin sources")
			}
			relative := entry.Source.Relative
			if marketplace.Metadata.PluginRoot != "" {
				relative = filepath.ToSlash(filepath.Join(marketplace.Metadata.PluginRoot, relative))
			}
			if _, err := contained(tree, relative); err != nil {
				return Catalog{}, err
			}
		}
	}
	for _, other := range m.data.Catalogs {
		if other.SourceID != source.ID && other.Marketplace.Name == marketplace.Name {
			return Catalog{}, errors.New("another source already claims this marketplace name")
		}
	}
	final, err := m.publish(tree, "catalogs")
	if err != nil {
		return Catalog{}, err
	}
	catalog := Catalog{SourceID: source.ID, Marketplace: marketplace, Root: final, Links: sourceTree.Links, Commit: commit, RefreshedAt: time.Now().UTC()}
	return catalog, nil
}

func (m *Manager) Install(ctx context.Context, marketplace, name string) (Installation, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	return m.install(ctx, marketplace, name)
}

func (m *Manager) prepare(ctx context.Context, marketplace, name string, constraints []string) (Installation, error) {
	var catalog *Catalog
	var entry *MarketplaceEntry
	for i := range m.data.Catalogs {
		c := &m.data.Catalogs[i]
		if c.Marketplace.Name == marketplace || c.SourceID == marketplace {
			catalog = c
			for j := range c.Marketplace.Plugins {
				if c.Marketplace.Plugins[j].Name == name {
					entry = &c.Marketplace.Plugins[j]
					break
				}
			}
			break
		}
	}
	if catalog == nil || entry == nil {
		return Installation{}, fmt.Errorf("plugin %s@%s not found in refreshed catalogs", name, marketplace)
	}
	for _, source := range m.data.Sources {
		if source.ID == catalog.SourceID && !source.Enabled {
			return Installation{}, errors.New("marketplace source is disabled")
		}
	}
	stage, err := m.stage()
	if err != nil {
		return Installation{}, err
	}
	defer os.RemoveAll(stage)
	tree := filepath.Join(stage, "tree")
	commit := catalog.Commit
	pluginSource := entry.Source
	if entry.Source.Relative != "" {
		catalogRoot := catalog.Root
		catalogLinks := catalog.Links
		if len(constraints) != 0 {
			for _, source := range m.data.Sources {
				if source.ID != catalog.SourceID || (source.Source.Source != "github" && source.Source.Source != "git") {
					continue
				}
				tagSource, found, err := resolveDependencyTag(ctx, source.Source, name, constraints)
				if err != nil {
					return Installation{}, err
				}
				if found {
					catalogRoot = filepath.Join(stage, "tagged-marketplace")
					var tagged SourceTree
					tagged, commit, err = snapshotMarketplace(ctx, tagSource, catalogRoot, nil)
					if err != nil {
						return Installation{}, err
					}
					catalogLinks = tagged.Links
				}
				break
			}
		}
		relative := entry.Source.Relative
		if catalog.Marketplace.Metadata.PluginRoot != "" {
			relative = filepath.ToSlash(filepath.Join(catalog.Marketplace.Metadata.PluginRoot, relative))
		}
		path, err := contained(catalogRoot, relative)
		if err != nil {
			return Installation{}, err
		}
		relative, err = filepath.Rel(catalogRoot, path)
		if err != nil {
			return Installation{}, err
		}
		if err := MaterializePluginSource(ctx, SourceTree{Root: catalogRoot, Links: catalogLinks}, filepath.ToSlash(relative), tree); err != nil {
			return Installation{}, err
		}
	} else {
		if len(constraints) != 0 && (pluginSource.Source == "github" || pluginSource.Source == "url" || pluginSource.Source == "git" || pluginSource.Source == "git-subdir") {
			var found bool
			pluginSource, found, err = resolveDependencyTag(ctx, pluginSource, name, constraints)
			if err != nil {
				return Installation{}, err
			}
			if !found {
				return Installation{}, fmt.Errorf("dependency %s@%s has no git tag satisfying %s", name, catalog.Marketplace.Name, strings.Join(constraints, " AND "))
			}
		}
		commit, err = materialize(ctx, pluginSource, tree, nil)
		if err != nil {
			return Installation{}, err
		}
	}
	if entry.Strict != nil && !*entry.Strict {
		if _, err := os.Stat(filepath.Join(tree, ".claude-plugin", "plugin.json")); err == nil && hasComponents(entry.Manifest) {
			return Installation{}, errors.New("strict:false marketplace components conflict with plugin.json")
		}
	}
	origin := entry.Source
	if entry.Origin != nil {
		origin = *entry.Origin
	}
	bundle, err := LoadWithManifest(tree, entry.Manifest, origin)
	if err != nil {
		return Installation{}, err
	}
	final, err := m.publish(tree, "versions")
	if err != nil {
		return Installation{}, err
	}
	now := time.Now().UTC()
	install := Installation{
		ID: name + "@" + catalog.Marketplace.Name, Name: name, Marketplace: catalog.Marketplace.Name,
		SourceID: catalog.SourceID, Path: final, Version: bundle.Manifest.Version, Commit: commit,
		Enabled:     bundle.Manifest.DefaultEnabled == nil || *bundle.Manifest.DefaultEnabled,
		InstalledAt: now, UpdatedAt: now, Manifest: bundle.Manifest, EntryManifest: entry.Manifest, Source: origin,
	}
	return copyValue(install), nil
}

func hasComponents(m Manifest) bool {
	return len(m.Commands)+len(m.Agents)+len(m.Skills)+len(m.Hooks)+len(m.MCPServers)+len(m.LSPServers)+len(m.OutputStyles) != 0
}

func (m *Manager) SetEnabled(ctx context.Context, id string, enabled bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.opMu.Lock()
	defer m.opMu.Unlock()
	next := copyValue(m.data)
	for i := range next.Installations {
		if next.Installations[i].ID == id {
			next.Installations[i].Enabled = enabled
			if err := validateDependencyTransition(m.data, next); err != nil {
				return err
			}
			return m.commit(next)
		}
	}
	return fmt.Errorf("plugin installation %q not found", id)
}

func (m *Manager) Update(ctx context.Context, id string) (Installation, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	for _, old := range m.data.Installations {
		if old.ID == id {
			return m.install(ctx, old.Marketplace, old.Name)
		}
	}
	return Installation{}, fmt.Errorf("plugin installation %q not found", id)
}

func (m *Manager) Uninstall(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.opMu.Lock()
	defer m.opMu.Unlock()
	next := copyValue(m.data)
	before := len(next.Installations)
	next.Installations = slices.DeleteFunc(next.Installations, func(i Installation) bool { return i.ID == id })
	if len(next.Installations) == before {
		return fmt.Errorf("plugin installation %q not found", id)
	}
	if err := validateDependencyTransition(m.data, next); err != nil {
		return err
	}
	// Do not remove immutable version directories: active sessions may still use them.
	return m.commit(next)
}

func (m *Manager) Bundles() ([]Bundle, error) {
	m.mu.RLock()
	installations := copyValue(m.data.Installations)
	m.mu.RUnlock()
	selected := selectInstallations(installations)
	if conflicts := namespaceConflicts(selected); len(conflicts) != 0 {
		return nil, fmt.Errorf("plugin configuration conflict: %s", strings.Join(conflicts, "; "))
	}
	keys := make([]string, 0, len(selected))
	for id := range selected {
		keys = append(keys, id)
	}
	slices.Sort(keys)
	bundles := make([]Bundle, 0, len(keys))
	for _, id := range keys {
		installation := selected[id]
		if !installation.Enabled {
			continue
		}
		if len(installationIssues(installation, selected, map[string]bool{})) != 0 {
			continue
		}
		bundle, err := LoadInstallation(installation)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		bundle.ID, bundle.Origin, bundle.Installation = id, installation.Marketplace, &installation
		bundles = append(bundles, bundle)
	}
	return bundles, nil
}

func inside(root, path string) bool {
	if root == "" {
		return false
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (m *Manager) stage() (string, error) {
	path := filepath.Join(m.root, "staging")
	if err := os.MkdirAll(path, 0700); err != nil {
		return "", err
	}
	return os.MkdirTemp(path, "operation-")
}

func (m *Manager) publish(tree, category string) (string, error) {
	parent := filepath.Join(m.root, category)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return "", err
	}
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", err
	}
	final := filepath.Join(parent, hex.EncodeToString(token[:]))
	if err := renameWithRetry(tree, final); err != nil {
		return "", err
	}
	return final, nil
}

func (m *Manager) commit(next registry) error {
	if err := m.save(next); err != nil {
		return err
	}
	m.mu.Lock()
	m.data = next
	m.mu.Unlock()
	return nil
}

func (m *Manager) save(next registry) error {
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(m.root, ".registry-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if err := file.Chmod(0600); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return renameWithRetry(name, filepath.Join(m.root, "registry.json"))
}

func snapshotMarketplace(ctx context.Context, source Source, target string, headers map[string]string) (SourceTree, string, error) {
	switch source.Source {
	case "url", "github", "git", "git-subdir":
		return ExportGitSource(ctx, source, target)
	default:
		commit, err := materialize(ctx, source, target, headers)
		return SourceTree{Root: target}, commit, err
	}
}

func materialize(ctx context.Context, source Source, target string, headers map[string]string) (commit string, resultErr error) {
	defer func() { resultErr = redactSourceError(resultErr, []string{source.URL}, headers) }()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	switch source.Source {
	case "settings":
		if source.Name == "" || source.Owner == nil {
			return "", errors.New("inline marketplace requires name and owner")
		}
		marketplace := Marketplace{Name: source.Name, Owner: *source.Owner, Plugins: source.Plugins}
		data, err := json.Marshal(marketplace)
		if err != nil {
			return "", err
		}
		if err := os.MkdirAll(filepath.Join(target, ".claude-plugin"), 0700); err != nil {
			return "", err
		}
		return "", os.WriteFile(filepath.Join(target, ".claude-plugin", "marketplace.json"), data, 0600)
	case "directory":
		return "", copyTree(ctx, source.Path, target)
	case "file":
		if err := os.MkdirAll(target, 0700); err != nil {
			return "", err
		}
		data, err := readDefinition(source.Path)
		if err != nil {
			return "", err
		}
		return "", os.WriteFile(filepath.Join(target, "marketplace.json"), data, 0600)
	case "marketplace-url":
		if err := os.MkdirAll(target, 0700); err != nil {
			return "", err
		}
		ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, source.URL, nil)
		if err != nil {
			return "", err
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		response, err := (&http.Client{Timeout: 90 * time.Second}).Do(req)
		if err != nil {
			return "", err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return "", fmt.Errorf("marketplace returned HTTP %d", response.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(response.Body, maxDefinitionSize+1))
		if err != nil {
			return "", err
		}
		if len(data) > maxDefinitionSize {
			return "", errors.New("marketplace manifest exceeds size limit")
		}
		return "", os.WriteFile(filepath.Join(target, "marketplace.json"), data, 0600)
	case "url", "github", "git", "git-subdir":
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return "", err
		}
		export, err := os.MkdirTemp(filepath.Dir(target), "plugin-source-")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(export)
		tree, commit, err := ExportGitSource(ctx, source, export)
		if err != nil {
			return "", err
		}
		relative := "."
		if source.Source == "git-subdir" && source.Path == "" {
			return "", errors.New("git-subdir source requires path")
		}
		if source.Path != "" {
			relative = filepath.ToSlash(source.Path)
		}
		if err := MaterializePluginSource(ctx, tree, relative, target); err != nil {
			return "", err
		}
		return commit, nil
	default:
		return "", fmt.Errorf("unsupported source type %q", source.Source)
	}
}

func gitAddress(source Source) (string, error) {
	address := source.URL
	if source.Source == "github" {
		if strings.Count(source.Repo, "/") != 1 || strings.ContainsAny(source.Repo, "\\ :\r\n") {
			return "", errors.New("invalid GitHub owner/repository")
		}
		address = "https://github.com/" + source.Repo + ".git"
	}
	if source.Source == "git-subdir" && !strings.Contains(address, "://") && strings.Count(address, "/") == 1 {
		address = "https://github.com/" + address + ".git"
	}
	return address, validateGitAddress(address)
}

func validateGitAddress(address string) error {
	if address == "" || strings.HasPrefix(address, "-") || strings.ContainsAny(address, "\r\n\x00") {
		return errors.New("invalid Git URL")
	}
	if strings.HasPrefix(address, "git@") && strings.Contains(address, ":") {
		return nil
	}
	if filepath.IsAbs(address) {
		return nil
	}
	u, err := url.Parse(address)
	if err != nil {
		return err
	}
	switch u.Scheme {
	case "https", "http", "ssh", "file":
		return nil
	case "":
		if filepath.IsAbs(address) {
			return nil
		}
	}
	return errors.New("Git source must be HTTPS, SSH, file URL, or absolute local repository path")
}

func git(ctx context.Context, dir string, args ...string) (result string, resultErr error) {
	defer func() { resultErr = redactSourceError(resultErr, args, nil) }()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}
	cmd := gitCommand(ctx, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("git operation failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func gitCommand(ctx context.Context, args ...string) *exec.Cmd {
	args = append([]string{
		"-c", "core.longpaths=true", "-c", "core.hooksPath=", "-c", "protocol.ext.allow=never",
	}, args...)
	command := exec.CommandContext(ctx, "git", args...)
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=Never", "GIT_CONFIG_NOSYSTEM=1")
	command.WaitDelay = 2 * time.Second
	processutil.HideConsole(command)
	return command
}
