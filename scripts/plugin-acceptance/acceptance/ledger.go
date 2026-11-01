package acceptance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"gcode/internal/plugins"
)

var namePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

// Execute creates a fresh evidence directory. No source scripts or provider calls are
// executed unless Run is explicit and a real RuntimeRunner has been supplied.
func Execute(ctx context.Context, cfg Config, runner RuntimeRunner) (result Report, resultErr error) {
	if cfg.MarketplaceFile == "" {
		cfg.MarketplaceFile = DefaultMarketplaceFile
		cfg.RequireOfficialBaseline = true
	}
	if cfg.MarketplaceCommit == "" {
		cfg.MarketplaceCommit = OfficialCommit
	}
	if cfg.MarketplaceRepository == "" {
		cfg.MarketplaceRepository = OfficialRepository
	}
	if cfg.Platform == "" {
		cfg.Platform = runtime.GOOS + "/" + runtime.GOARCH
	}
	if cfg.Workers == 0 {
		cfg.Workers = 4
	}
	if cfg.Workers < 1 || cfg.Workers > 16 {
		return Report{}, errors.New("workers must be between 1 and 16")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 2 * time.Minute
	}
	if cfg.OutputDir == "" {
		cfg.OutputDir = "output/plugin-acceptance"
	}
	if cfg.Run && cfg.ProviderConfig == "" {
		return Report{}, errors.New("--run requires an explicit --provider-config private test account file")
	}
	if (cfg.Run || len(cfg.RuntimeEvidence) != 0) && cfg.Model == "" {
		return Report{}, errors.New("runtime execution/import requires an explicit model identity")
	}
	if cfg.Run && cfg.Platform != runtime.GOOS+"/"+runtime.GOARCH {
		return Report{}, errors.New("--run platform must equal the current operating system and architecture")
	}
	if cfg.SourceEvidenceDir == "" {
		cfg.SourceEvidenceDir = filepath.Dir(cfg.MarketplaceFile)
	}
	data, err := os.ReadFile(cfg.MarketplaceFile)
	if err != nil {
		return Report{}, err
	}
	var marketplace struct {
		Name       string            `json:"name"`
		Owner      plugins.Author    `json:"owner"`
		Plugins    []json.RawMessage `json:"plugins"`
		AllowCross []string          `json:"allowCrossMarketplaceDependenciesOn"`
		Metadata   struct {
			PluginRoot string `json:"pluginRoot"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(data, &marketplace); err != nil {
		return Report{}, err
	}
	if !namePattern.MatchString(marketplace.Name) || len(marketplace.Plugins) == 0 {
		return Report{}, errors.New("marketplace must have a name and plugin entries")
	}
	digest := hash(data)
	if cfg.RequireOfficialBaseline && (marketplace.Name != OfficialMarketplace || cfg.MarketplaceCommit != OfficialCommit || digest != OfficialManifestSHA256 || len(marketplace.Plugins) != 315) {
		return Report{}, errors.New("official acceptance baseline must be the exact frozen 315-entry manifest at " + OfficialCommit)
	}
	if err := os.MkdirAll(cfg.OutputDir, 0700); err != nil {
		return Report{}, err
	}
	runDir, err := os.MkdirTemp(cfg.OutputDir, "run-")
	if err != nil {
		return Report{}, err
	}
	runDir, err = filepath.Abs(runDir)
	if err != nil {
		return Report{}, err
	}
	report := Report{
		SchemaVersion: 1, RunID: filepath.Base(runDir), CreatedAt: time.Now().UTC(), Directory: runDir,
		Marketplace: marketplace.Name, MarketplaceRepository: cfg.MarketplaceRepository,
		MarketplaceCommit: cfg.MarketplaceCommit, MarketplaceSHA256: digest, OfficialBaseline: cfg.RequireOfficialBaseline,
		Platform: cfg.Platform, Model: cfg.Model, Denominator: len(marketplace.Plugins), RequiredPasses: (len(marketplace.Plugins)*9 + 9) / 10,
	}
	defer func() {
		recount(&report)
		if err := WriteReport(report); err != nil && resultErr == nil {
			resultErr = err
		}
		result = report
	}()
	seen := map[string]bool{}
	for _, raw := range marketplace.Plugins {
		var entry plugins.MarketplaceEntry
		if err := json.Unmarshal(raw, &entry); err != nil {
			return Report{}, fmt.Errorf("marketplace entry: %w", err)
		}
		id := entry.Name + "@" + marketplace.Name
		if !namePattern.MatchString(entry.Name) || seen[id] {
			return Report{}, fmt.Errorf("empty or duplicate marketplace entry %q", id)
		}
		seen[id] = true
		row := Row{
			ID: id, Name: entry.Name, Marketplace: marketplace.Name, MarketplaceCommit: cfg.MarketplaceCommit,
			EntrySHA256: canonicalHash(raw), Platform: cfg.Platform, Model: cfg.Model, Version: entry.Version,
			Source: Stage{Status: "not_tested"}, Install: Stage{Status: "not_tested"}, Discover: Stage{Status: "not_tested"}, Runtime: Stage{Status: "not_tested"},
			Components: []Component{}, Probes: []Probe{}, Blocked: []string{}, Evidence: []string{}, Errors: []string{}, entry: entry,
		}
		row.FixedSource = sourceFromEntry(entry.Source, cfg.MarketplaceRepository, cfg.MarketplaceCommit)
		if entry.Source.Relative != "" && marketplace.Metadata.PluginRoot != "" {
			row.FixedSource.Path = filepath.ToSlash(filepath.Join(marketplace.Metadata.PluginRoot, row.FixedSource.Path))
		}
		attachSnapshot(&row, cfg.SourceEvidenceDir)
		report.Rows = append(report.Rows, row)
	}
	cache, err := loadCache(cfg)
	if err != nil {
		return report, err
	}
	if cfg.InstallAndDiscover || cfg.Run {
		exporter := sourceExporter{cache: cache, directory: filepath.Join(runDir, "source-roots"), exports: map[string]*sourceExport{}}
		parallelRows(ctx, cfg.Workers, report.Rows, func(ctx context.Context, row *Row) {
			rowCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
			defer cancel()
			target := filepath.Join(runDir, "sources", hash([]byte(row.ID))[:20])
			if err := exporter.prepare(rowCtx, row, target); err != nil {
				row.Source = Stage{Status: "blocked", Method: "local-git-object", Error: err.Error()}
				row.Blocked = append(row.Blocked, err.Error())
				return
			}
			row.Source = Stage{Status: "verified", Method: "local-git-archive-at-fixed-commit", Evidence: []string{target}}
			row.ArtifactRoot = target
		})
		for i := range report.Rows {
			row := &report.Rows[i]
			if row.Source.Status != "verified" {
				continue
			}
			rowCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
			// Every candidate has an independent installation registry. Other
			// candidates must not introduce namespace or enabled-state conflicts.
			manager, err := localManager(rowCtx, filepath.Join(runDir, "installations", hash([]byte(row.ID))[:20]), report.Rows, marketplace.Name, marketplace.Owner, marketplace.AllowCross)
			if err != nil {
				cancel()
				row.Install = Stage{Status: "failed", Method: "native-plugin-manager", Error: err.Error()}
				row.Errors = append(row.Errors, err.Error())
				continue
			}
			installation, err := manager.Install(rowCtx, "acceptance", row.Name)
			cancel()
			if err != nil {
				row.Install = Stage{Status: "failed", Method: "native-plugin-manager", Error: err.Error()}
				row.Errors = append(row.Errors, err.Error())
				continue
			}
			row.Install = Stage{Status: "passed", Method: "native-plugin-manager", Evidence: []string{installation.Path}}
			row.ArtifactRoot = installation.Path
			bundle, err := plugins.LoadInstallation(installation)
			if err != nil {
				row.Discover = Stage{Status: "failed", Method: "native-bundle-loader", Error: err.Error()}
				row.Errors = append(row.Errors, err.Error())
				continue
			}
			bundle.ID = row.ID
			row.Version = bundle.Manifest.Version
			row.ArtifactDigest, err = treeDigest(installation.Path)
			if err != nil {
				row.Discover = Stage{Status: "failed", Error: err.Error()}
				continue
			}
			row.Components = components(bundle)
			row.Discover = Stage{Status: "passed", Method: "native-bundle-loader", Evidence: []string{installation.Path}}
			componentIDs := map[string]bool{}
			for _, component := range row.Components {
				if componentIDs[component.ID] {
					row.Blocked = append(row.Blocked, "ambiguous duplicate component ID: "+component.ID)
				}
				componentIDs[component.ID] = true
			}
			if len(row.Components) == 0 {
				row.Blocked = append(row.Blocked, "no executable component or declared capability was discovered")
			}
			for _, issue := range bundle.Unsupported {
				if executableUnsupported(issue) {
					row.Blocked = append(row.Blocked, "unsupported "+issue.Component+": "+issue.Message)
				}
			}
			inventory := filepath.Join(runDir, "inventory", hash([]byte(row.ID))[:20]+".json")
			if err := writeJSON(inventory, map[string]any{"pluginId": row.ID, "source": row.FixedSource, "artifactDigest": row.ArtifactDigest, "components": row.Components}); err != nil {
				return report, err
			}
			row.Evidence = append(row.Evidence, inventory)
		}
	}
	if len(cfg.RuntimeEvidence) != 0 {
		for _, path := range cfg.RuntimeEvidence {
			if err := ImportEvidence(&report, path); err != nil {
				return report, err
			}
		}
	}
	if cfg.Run {
		var provider ProviderConfig
		secret, err := os.ReadFile(cfg.ProviderConfig)
		if err != nil {
			return report, err
		}
		if err := json.Unmarshal(secret, &provider); err != nil {
			return report, fmt.Errorf("test provider config: %w", err)
		}
		if provider.ModelID != cfg.Model {
			return report, errors.New("provider-config model differs from --model")
		}
		if err := provider.Provider.Validate(); err != nil {
			return report, fmt.Errorf("test provider: %w", err)
		}
		if !strings.HasPrefix(provider.ModelID, provider.Provider.ID+"/") {
			return report, errors.New("test modelId must select the explicitly configured provider")
		}
		// The linked core runner owns a private process environment per probe.
		// Start each candidate deadline when it actually begins, not while it
		// waits for another candidate's environment lease.
		parallelRows(ctx, 1, report.Rows, func(ctx context.Context, row *Row) {
			if row.Discover.Status != "passed" || len(row.Blocked) != 0 {
				return
			}
			if runner == nil {
				row.Runtime = Stage{Status: "blocked", Error: "real RuntimeRunner is not linked"}
				row.Blocked = append(row.Blocked, "real RuntimeRunner is not linked")
				return
			}
			rowCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
			defer cancel()
			bundle, err := plugins.LoadWithManifest(row.ArtifactRoot, row.entry.Manifest, row.entry.Source)
			if err != nil {
				row.Runtime = Stage{Status: "failed", Error: err.Error()}
				return
			}
			bundle.ID, bundle.Origin = row.ID, row.Marketplace
			workspace := filepath.Join(runDir, "runtime", hash([]byte(row.ID))[:20])
			if err := os.MkdirAll(workspace, 0700); err != nil {
				row.Runtime = Stage{Status: "failed", Error: err.Error()}
				return
			}
			evidence, err := runner.Run(rowCtx, RunRequest{RunID: report.RunID, PluginID: row.ID, MarketplaceCommit: row.MarketplaceCommit, SourceCommit: row.FixedSource.Commit,
				ArtifactDigest: row.ArtifactDigest, Platform: row.Platform, Model: row.Model, Bundle: bundle, Components: slices.Clone(row.Components),
				Workspace: workspace, EvidenceDir: workspace, Provider: provider, ProbeDir: cfg.ProbeDir, EntryManifest: row.entry.Manifest, EntrySource: row.entry.Source})
			if err != nil {
				row.Runtime = Stage{Status: "failed", Error: err.Error()}
				row.Errors = append(row.Errors, err.Error())
				return
			}
			file := filepath.Join(workspace, "runtime-evidence.json")
			if err := writeJSON(file, evidence); err != nil {
				row.Runtime = Stage{Status: "failed", Error: err.Error()}
				return
			}
			if err := applyEvidence(row, evidence, filepath.Dir(file)); err != nil {
				row.Runtime = Stage{Status: "rejected", Error: err.Error()}
				row.Errors = append(row.Errors, err.Error())
				return
			}
			row.Evidence = append(row.Evidence, file)
		})
	}
	recount(&report)
	return report, nil
}

func parallelRows(ctx context.Context, workers int, rows []Row, fn func(context.Context, *Row)) {
	jobs := make(chan *Row)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for row := range jobs {
				fn(ctx, row)
			}
		}()
	}
	for i := range rows {
		jobs <- &rows[i]
	}
	close(jobs)
	wg.Wait()
}

func sourceFromEntry(source plugins.Source, marketRepo, marketCommit string) FixedSource {
	fixed := FixedSource{Repository: source.Repo, Commit: source.SHA, Path: source.Path}
	if source.Relative != "" {
		fixed = FixedSource{Repository: marketRepo, Commit: marketCommit, Path: strings.TrimPrefix(source.Relative, "./")}
	}
	if fixed.Repository == "" {
		address := strings.TrimSuffix(source.URL, ".git")
		address = strings.TrimPrefix(strings.TrimPrefix(address, "https://github.com/"), "http://github.com/")
		if !strings.Contains(address, "://") && strings.Count(address, "/") == 1 {
			fixed.Repository = address
		}
	}
	return fixed
}

func attachSnapshot(row *Row, dir string) {
	path := filepath.Join(dir, row.Marketplace+"--"+row.Name+"--source-evidence.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var snapshot struct {
		Repository string            `json:"repo"`
		Commit     string            `json:"commit"`
		Path       string            `json:"path"`
		Archive    string            `json:"archive_sha256"`
		Config     map[string]string `json:"config"`
	}
	if json.Unmarshal(data, &snapshot) != nil {
		return
	}
	// A source snapshot must not override an explicit marketplace commit pin.
	if row.FixedSource.Commit != "" && row.FixedSource.Commit != snapshot.Commit {
		return
	}
	if row.FixedSource.Repository != "" && !strings.EqualFold(row.FixedSource.Repository, snapshot.Repository) {
		return
	}
	if row.FixedSource.Path != "" && strings.Trim(filepath.ToSlash(row.FixedSource.Path), "./") != strings.Trim(filepath.ToSlash(snapshot.Path), "./") {
		return
	}
	row.FixedSource = FixedSource{Repository: snapshot.Repository, Commit: snapshot.Commit, Path: snapshot.Path, ArchiveSHA256: snapshot.Archive}
	if manifest := snapshot.Config[".claude-plugin/plugin.json"]; manifest != "" {
		row.FixedSource.ManifestTextSHA256 = manifestTextHash([]byte(manifest))
	}
	row.Source = Stage{Status: "snapshot_only", Method: "previous-static-investigation", Evidence: []string{path}}
	row.Evidence = append(row.Evidence, path)
}

func localManager(ctx context.Context, runDir string, rows []Row, name string, owner plugins.Author, allow []string) (*plugins.Manager, error) {
	root := filepath.Join(runDir, "catalog-input")
	market := plugins.Marketplace{Name: name, Owner: owner, AllowCrossMarketplaceDependenciesOn: allow}
	if market.Owner.Name == "" {
		market.Owner.Name = "Acceptance fixture"
	}
	for _, row := range rows {
		entry := row.entry
		originalSource := entry.Source
		entry.Origin = &originalSource
		path := row.ArtifactRoot
		if row.Source.Status != "verified" {
			path = filepath.Join(runDir, "unavailable", hash([]byte(row.ID))[:20])
		}
		entry.Source = plugins.Source{Source: "directory", Path: path}
		market.Plugins = append(market.Plugins, entry)
	}
	if err := writeJSON(filepath.Join(root, ".claude-plugin", "marketplace.json"), market); err != nil {
		return nil, err
	}
	manager, err := plugins.NewManager(filepath.Join(runDir, "manager"))
	if err != nil {
		return nil, err
	}
	for _, source := range manager.Sources() {
		if err := manager.RemoveSource(ctx, source.ID); err != nil {
			return nil, err
		}
	}
	if err := manager.PutSource(ctx, plugins.MarketplaceSource{ID: "acceptance", Name: name, Enabled: true, Source: plugins.Source{Source: "directory", Path: root}}); err != nil {
		return nil, err
	}
	_, err = manager.Refresh(ctx, "acceptance")
	return manager, err
}

func components(bundle plugins.Bundle) []Component {
	var result []Component
	add := func(kind, name string, value any) {
		data, _ := json.Marshal(value)
		result = append(result, Component{ID: kind + ":" + name, Kind: kind, Name: name, Digest: hash(data), Status: "not_tested"})
	}
	for _, group := range []struct {
		kind   string
		values []plugins.MarkdownComponent
	}{{"skill", bundle.Skills}, {"command", bundle.Commands}, {"agent", bundle.Agents}, {"output-style", bundle.OutputStyles}} {
		for _, component := range group.values {
			// Installation paths vary per run and must not change the component fingerprint.
			component.Path = ""
			add(group.kind, component.Name, component)
			if group.kind == "skill" || group.kind == "command" {
				for event, matchers := range component.Metadata.Hooks {
					for mi, matcher := range matchers {
						for hi, hook := range matcher.Hooks {
							add("hook", fmt.Sprintf("%s/%s/%s/%d/%d", group.kind, component.Name, event, mi, hi), map[string]any{"owner": component.Name, "event": event, "matcher": matcher.Matcher, "hook": hook})
						}
					}
				}
			}
		}
	}
	for event, matchers := range bundle.Hooks {
		for mi, matcher := range matchers {
			for hi, hook := range matcher.Hooks {
				add("hook", fmt.Sprintf("%s/%d/%d", event, mi, hi), map[string]any{"event": event, "matcher": matcher.Matcher, "hook": hook})
			}
		}
	}
	for name, server := range bundle.MCPServers {
		add("mcp", name, server)
	}
	for name, server := range bundle.LSPServers {
		add("lsp", name, server)
	}
	for name, value := range bundle.Settings {
		add("settings", name, value)
	}
	for _, dependency := range bundle.Manifest.Dependencies {
		add("dependency", dependency.Name, dependency)
	}
	for name, value := range bundle.Manifest.UserConfig {
		add("user-config", name, value)
	}
	for _, channel := range bundle.Manifest.Channels {
		add("channel", channel.Server, channel)
	}
	slices.SortFunc(result, func(a, b Component) int { return strings.Compare(a.ID, b.ID) })
	return result
}

// ComponentsForBundle returns the same complete inventory used by the ledger.
func ComponentsForBundle(bundle plugins.Bundle) []Component { return components(bundle) }

func executableUnsupported(issue plugins.Issue) bool {
	switch issue.Component {
	case "icon", "displayName", "documentationUrl", "supportUrl", "privacyPolicyUrl", "termsOfServiceUrl", "keywords", "repository", "license":
		return false
	default:
		return true
	}
}

func recount(report *Report) {
	report.Passed = 0
	for i := range report.Rows {
		row := &report.Rows[i]
		row.Passed = row.Source.Status == "verified" && row.Install.Status == "passed" && row.Discover.Status == "passed" && row.Runtime.Status == "passed" && len(row.Blocked) == 0 && len(row.Components) > 0
		if row.Passed {
			for _, component := range row.Components {
				if component.Status != "passed" {
					row.Passed = false
					break
				}
			}
		}
		if row.Passed {
			report.Passed++
		}
	}
	if report.Denominator > 0 {
		report.Percent = float64(report.Passed) * 100 / float64(report.Denominator)
	}
	report.TargetMet = report.Passed >= report.RequiredPasses && report.Denominator > 0
}

func hash(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func canonicalHash(data []byte) string {
	var value any
	if json.Unmarshal(data, &value) == nil {
		data, _ = json.Marshal(value)
	}
	return hash(data)
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
