package corerunner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/modelcatalog"
	"gcode/internal/plugins"
	"gcode/internal/storage"
	"gcode/scripts/plugin-acceptance/acceptance"
)

// InProcessRunner uses the public typed core service. It never opens the user's
// profile and never creates a provider from implicit environment credentials.
type InProcessRunner struct{}

var environmentMu sync.Mutex

func (InProcessRunner) Run(ctx context.Context, request acceptance.RunRequest) (acceptance.RuntimeEvidence, error) {
	evidence := acceptance.RuntimeEvidence{SchemaVersion: 1, RunID: request.RunID, PluginID: request.PluginID, MarketplaceCommit: request.MarketplaceCommit,
		SourceCommit: request.SourceCommit, ArtifactDigest: request.ArtifactDigest, Platform: request.Platform, Model: request.Model}
	plan, err := readPlan(request)
	if err != nil {
		return blockedEvidence(evidence, request.Components, err.Error()), nil
	}
	actual := acceptance.ComponentsForBundle(request.Bundle)
	if len(actual) != len(request.Components) {
		return blockedEvidence(evidence, request.Components, "probe request omits a declared bundle component"), nil
	}
	for _, component := range actual {
		found := false
		for _, requested := range request.Components {
			if requested.ID == component.ID && requested.Digest == component.Digest {
				found = true
				break
			}
		}
		if !found {
			return blockedEvidence(evidence, request.Components, "probe component inventory does not match the actual bundle"), nil
		}
	}
	if request.Platform != runtime.GOOS+"/"+runtime.GOARCH {
		return blockedEvidence(evidence, request.Components, "runtime platform differs from current host"), nil
	}
	if request.Provider.ModelID != request.Model || !request.Provider.Provider.Ready() {
		return blockedEvidence(evidence, request.Components, "explicit test provider/model is not ready"), nil
	}
	maxOutput := 0
	selectedModel := strings.TrimPrefix(request.Provider.ModelID, request.Provider.Provider.ID+"/")
	for _, configured := range request.Provider.Provider.Models {
		if configured.ID == selectedModel {
			maxOutput = configured.MaxOutput
		}
	}
	if err := saveJSON(filepath.Join(request.EvidenceDir, "runtime-settings.json"), map[string]any{
		"modelId": request.Model, "protocol": request.Provider.Provider.Protocol,
		"reasoning": request.Provider.Reasoning, "configuredMaxOutput": maxOutput,
	}); err != nil {
		return evidence, err
	}
	for _, component := range request.Components {
		var probePlan ProbePlan
		for _, candidate := range plan.Probes {
			if candidate.ComponentID == component.ID {
				probePlan = candidate
				break
			}
		}
		probe := acceptance.Probe{ID: probePlan.ID, ComponentID: component.ID, ComponentDigest: component.Digest, Operation: operation(component.Kind), Outcome: "blocked", StartedAt: time.Now().UTC()}
		if !supported(component.Kind) {
			probe.Error = component.Kind + " runtime is not connected to this acceptance runner"
			probe.FinishedAt = time.Now().UTC()
			evidence.Probes = append(evidence.Probes, probe)
			continue
		}
		// The standalone acceptance command owns this process. Serializing its
		// temporary environment keeps HOME and credentials isolated for every core.
		environmentMu.Lock()
		probe, e := runProbe(ctx, request, component, probePlan, probe)
		environmentMu.Unlock()
		if e != nil {
			probe.Outcome = "failed"
			probe.Error = e.Error()
			for _, secret := range probeSecrets(request) {
				if secret != "" {
					probe.Error = strings.ReplaceAll(probe.Error, secret, "[REDACTED]")
				}
			}
		}
		probe.FinishedAt = time.Now().UTC()
		evidence.Probes = append(evidence.Probes, probe)
	}
	return evidence, nil
}

func blockedEvidence(e acceptance.RuntimeEvidence, components []acceptance.Component, reason string) acceptance.RuntimeEvidence {
	for i, component := range components {
		now := time.Now().UTC()
		e.Probes = append(e.Probes, acceptance.Probe{ID: fmt.Sprintf("blocked-%d", i), ComponentID: component.ID, ComponentDigest: component.Digest,
			Operation: operation(component.Kind), Outcome: "blocked", StartedAt: now, FinishedAt: now, Error: reason})
	}
	return e
}
func operation(kind string) string {
	return map[string]string{"skill": "skill-invoke", "command": "command-invoke", "agent": "agent-run", "hook": "hook-event", "mcp": "mcp-tool-call", "lsp": "lsp-request", "output-style": "output-style-apply", "settings": "settings-apply", "dependency": "dependency-capability", "user-config": "user-config-resolve", "channel": "channel-message"}[kind]
}
func supported(kind string) bool {
	return kind == "skill" || kind == "command" || kind == "agent" || kind == "hook" || kind == "mcp"
}

type observation struct {
	events    []contracts.Event
	state     contracts.Snapshot
	wire      []wireCall
	assistant string
	workspace string
}

func runProbe(ctx context.Context, request acceptance.RunRequest, component acceptance.Component, plan ProbePlan, probe acceptance.Probe) (result acceptance.Probe, resultErr error) {
	probeRoot, err := os.MkdirTemp(request.Workspace, "probe-")
	if err != nil {
		return probe, err
	}
	probeRoot, err = filepath.EvalSymlinks(probeRoot)
	if err != nil {
		return probe, err
	}
	home, cwd := filepath.Join(probeRoot, "home"), filepath.Join(probeRoot, "cwd")
	if err := os.MkdirAll(home, 0700); err != nil {
		return probe, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, removeOwnedProbeHome(probeRoot, home))
	}()
	if err := os.MkdirAll(cwd, 0700); err != nil {
		return probe, err
	}
	restore := privateEnvironment(home)
	defer restore()
	// Prevent Git tools from treating the user's ancestor checkout as this fixture.
	init := exec.CommandContext(ctx, "git", "-c", "core.hooksPath=", "-c", "init.templateDir=", "init", "-q", "--initial-branch=acceptance", cwd)
	if output, err := init.CombinedOutput(); err != nil {
		return probe, fmt.Errorf("initialize private fixture repository: %w: %s", err, output)
	}
	for _, fixture := range plan.Fixtures {
		path, err := workspacePath(cwd, fixture.Path)
		if err != nil {
			return probe, err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return probe, err
		}
		if err := os.WriteFile(path, []byte(fixture.Text), 0600); err != nil {
			return probe, err
		}
	}
	store, err := storage.Open(storage.Options{Home: home})
	if err != nil {
		return probe, err
	}
	defer func() { resultErr = errors.Join(resultErr, store.Close()) }()
	if err := installCandidate(ctx, store, request); err != nil {
		return probe, err
	}
	client := model.NewClient()
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	wire := &wireRecorder{base: transport}
	client.HTTP.Transport = wire
	defer transport.CloseIdleConnections()
	catalog, err := modelcatalog.Open(modelcatalog.Options{Directory: store.Directory(), Client: &http.Client{Transport: rejectNetwork{}}})
	if err != nil {
		return probe, err
	}
	core, err := engine.Open(engine.Options{Storage: store, Client: client, Catalog: catalog,
		ToolGuard: fixtureToolGuard(cwd, []string{request.Bundle.Root, filepath.Join(store.ConfigDirectory(), "plugins", "versions")}, plan, component, request.Bundle.Manifest.Name)})
	if err != nil {
		return probe, err
	}
	defer func() { resultErr = errors.Join(resultErr, core.Close()) }()
	if err := core.Execute(ctx, contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: request.Provider.Provider}}); err != nil {
		return probe, err
	}
	if config := request.Provider.PluginConfigs[request.PluginID]; len(config) != 0 {
		if err := core.Execute(ctx, contracts.Command{Kind: "plugins.configure", ID: request.PluginID, RequestID: "acceptance-config", Plugin: &contracts.PluginInput{Configuration: config}}); err != nil {
			return probe, err
		}
	}
	if err := core.Execute(ctx, contracts.Command{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: cwd, Name: "Acceptance fixture"}}); err != nil {
		return probe, err
	}
	state, err := core.Snapshot(ctx)
	if err != nil {
		return probe, err
	}
	if len(state.Projects) != 1 {
		return probe, errors.New("isolated core did not create exactly one fixture project")
	}
	eventCtx, eventCancel := context.WithCancel(ctx)
	events, err := core.Subscribe(eventCtx)
	if err != nil {
		eventCancel()
		return probe, err
	}
	recorder := &eventRecorder{}
	recorderDone := make(chan struct{})
	go func() {
		defer close(recorderDone)
		for event := range events {
			recorder.add(event)
		}
	}()
	defer func() { eventCancel(); <-recorderDone }()
	send := contracts.SendRequest{ProjectID: state.Projects[0].ID, ModelID: request.Provider.ModelID, ReasoningLevel: request.Provider.Reasoning, Text: plan.Prompt, RequestID: probe.ID}
	start := time.Now().UTC()
	if err := core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &send}); err != nil {
		return probe, err
	}
	state, runErr := waitForRuns(ctx, core)
	if runErr != nil {
		for _, session := range state.Sessions {
			_ = core.Execute(context.Background(), contracts.Command{Kind: "message.stop", SessionID: session.ID})
		}
	}
	// All final message/tool state is authoritative even if the event stream lost a projection.
	eventCancel()
	<-recorderDone
	eventsSnapshot := recorder.snapshot()
	seen := observation{events: eventsSnapshot, state: state, wire: wire.snapshot(), assistant: assistantText(state), workspace: cwd}
	secrets := probeSecrets(request)
	paths := map[string]any{"events.json": eventsSnapshot, "snapshot.json": state, "model-http.json": seen.wire}
	for name, value := range paths {
		if err := saveRedacted(filepath.Join(probeRoot, name), value, secrets); err != nil {
			return probe, err
		}
	}
	if runErr != nil {
		return probe, runErr
	}
	matched, err := triggerEvidence(request, component, plan, seen)
	if err != nil {
		return probe, err
	}
	results, err := assertBusiness(component, plan, seen, request.PluginID)
	if err != nil {
		return probe, err
	}
	finish := time.Now().UTC()
	input, output := probePayload(component, plan, seen, matched, results)
	if err := writeBoundTrace(request, component, probe, probeRoot, start, finish, input, output, &probe, secrets); err != nil {
		return probe, err
	}
	probe.Outcome = "passed"
	return probe, nil
}

func installCandidate(ctx context.Context, store *storage.Store, request acceptance.RunRequest) error {
	manager, err := plugins.NewManager(filepath.Join(store.ConfigDirectory(), "plugins"))
	if err != nil {
		return err
	}
	for _, source := range manager.Sources() {
		if err := manager.RemoveSource(ctx, source.ID); err != nil {
			return err
		}
	}
	catalogRoot := filepath.Join(store.Directory(), "acceptance-catalog")
	entry := plugins.MarketplaceEntry{Manifest: request.EntryManifest, Source: plugins.Source{Source: "directory", Path: request.Bundle.Root}}
	entry.Origin = &request.EntrySource
	name, marketplace, ok := strings.Cut(request.PluginID, "@")
	entry.Name = name
	market := plugins.Marketplace{Name: marketplace, Owner: plugins.Author{Name: "Isolated acceptance"}, Plugins: []plugins.MarketplaceEntry{entry}}
	if !ok || market.Name == "" {
		return errors.New("candidate plugin has no marketplace identity")
	}
	if err := saveJSON(filepath.Join(catalogRoot, ".claude-plugin", "marketplace.json"), market); err != nil {
		return err
	}
	if err := manager.PutSource(ctx, plugins.MarketplaceSource{ID: "acceptance", Name: market.Name, Enabled: true, Source: plugins.Source{Source: "directory", Path: catalogRoot}}); err != nil {
		return err
	}
	if _, err := manager.Refresh(ctx, "acceptance"); err != nil {
		return err
	}
	installed, err := manager.Install(ctx, "acceptance", entry.Name)
	if err != nil {
		return err
	}
	digest, err := acceptance.ArtifactDigest(installed.Path)
	if err != nil {
		return err
	}
	if digest != request.ArtifactDigest {
		return errors.New("core-owned candidate bytes differ from the acceptance artifact")
	}
	return nil
}

func privateEnvironment(home string) func() {
	before := os.Environ()
	keep := map[string]string{}
	for _, key := range []string{"PATH", "Path", "SystemRoot", "SYSTEMROOT", "WINDIR", "COMSPEC", "ComSpec", "PATHEXT", "TEMP", "TMP", "TMPDIR", "LANG", "LC_ALL", "TZ", "SSL_CERT_FILE", "SSL_CERT_DIR"} {
		if value, ok := os.LookupEnv(key); ok {
			keep[key] = value
		}
	}
	os.Clearenv()
	for key, value := range keep {
		_ = os.Setenv(key, value)
	}
	for _, key := range []string{"HOME", "USERPROFILE", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "APPDATA", "LOCALAPPDATA"} {
		_ = os.Setenv(key, home)
	}
	return func() {
		os.Clearenv()
		for _, entry := range before {
			key, value, ok := strings.Cut(entry, "=")
			if ok {
				_ = os.Setenv(key, value)
			}
		}
	}
}

type rejectNetwork struct{}

func (rejectNetwork) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("model metadata network is disabled during acceptance")
}

type eventRecorder struct {
	mu     sync.Mutex
	events []contracts.Event
}

func (r *eventRecorder) add(event contracts.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}
func (r *eventRecorder) snapshot() []contracts.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]contracts.Event(nil), r.events...)
}

func waitForRuns(ctx context.Context, core contracts.Service) (contracts.Snapshot, error) {
	ticker := time.NewTicker(15 * time.Millisecond)
	defer ticker.Stop()
	var state contracts.Snapshot
	for {
		next, err := core.Snapshot(ctx)
		if err != nil {
			return state, err
		}
		state = next
		done := len(state.Runs) > 0
		for _, run := range state.Runs {
			if run.Status == "running" {
				done = false
			}
		}
		if done {
			for _, run := range state.Runs {
				if run.Status != "completed" {
					if run.Error != "" {
						return state, fmt.Errorf("core run %s ended as %s: %s", run.ID, run.Status, run.Error)
					}
					return state, fmt.Errorf("core run %s ended as %s", run.ID, run.Status)
				}
			}
			return state, nil
		}
		for _, session := range state.Sessions {
			if session.Status == contracts.AwaitingInput {
				return state, errors.New("probe requires an explicit user permission or answer")
			}
		}
		select {
		case <-ctx.Done():
			return state, ctx.Err()
		case <-ticker.C:
		}
	}
}

func probeSecrets(request acceptance.RunRequest) []string {
	values := []string{request.Provider.Provider.APIKey}
	for _, config := range request.Provider.PluginConfigs {
		for _, value := range config {
			values = append(values, value)
		}
	}
	return values
}

func assistantText(state contracts.Snapshot) string {
	var text strings.Builder
	for _, messages := range state.Messages {
		for _, message := range messages {
			if message.Role == "assistant" {
				for _, block := range message.Blocks {
					if block.Kind == "text" {
						text.WriteString(block.Text)
						text.WriteByte('\n')
					}
				}
			}
		}
	}
	return text.String()
}

func saveJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
