package desktop

import (
	"testing"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func TestComposerModelAvailabilityUsesTargetDevice(t *testing.T) {
	for _, tc := range []struct {
		name, target, provider string
	}{
		{"local ignores global flags", "local", "openai"},
		{"remote ignores local flags", "remote", "claude"},
		{"cloud uses its own flags", "cloud-1", "openai"},
		{"missing target has no local fallback", "missing", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := &App{deviceID: tc.target, state: contracts.Snapshot{
				Providers:    []contracts.Provider{{ID: "claude", Enabled: true}, {ID: "openai"}, {ID: "cursor", Enabled: true}},
				Models:       []contracts.Model{{ID: "claude-model", ProviderID: "claude"}, {ID: "openai-model", ProviderID: "openai"}, {ID: "cursor-model", ProviderID: "cursor"}},
				ModelCatalog: contracts.ModelCatalogState{State: contracts.CatalogNoAgents},
				Integrations: contracts.IntegrationState{Providers: []contracts.ProviderDetails{
					{TargetID: "local", ProviderID: "claude", Installed: true},
					{TargetID: "local", ProviderID: "openai", Installed: true, Enabled: true},
					{TargetID: "local", ProviderID: "cursor", Enabled: true},
					{TargetID: "remote", ProviderID: "claude", Installed: true, Enabled: true},
					{TargetID: "remote", ProviderID: "openai", Installed: true},
					{TargetID: "remote", ProviderID: "cursor", Enabled: true},
					{TargetID: "cloud-1", ProviderID: "openai", Installed: true, Enabled: true},
				}},
			}}
			a.restoreModelSelection()
			if got := a.selectedModel().ProviderID; got != tc.provider {
				t.Fatalf("selected provider = %q, want %q", got, tc.provider)
			}
			if got := a.modelNoAgents(); got != (tc.provider == "") {
				t.Fatalf("no agents = %v for target %q", got, tc.target)
			}
			rows := a.modelRows()
			if tc.provider == "" && len(rows) != 0 || tc.provider != "" && len(rows) != 1 {
				t.Fatalf("target %q offered %d model rows", tc.target, len(rows))
			}
			for _, row := range rows {
				if row.provider.ID != tc.provider {
					t.Fatalf("target %q offered provider %q", tc.target, row.provider.ID)
				}
			}
		})
	}
}

func TestComposerDeviceChoiceResolvesRemoteModelAndSends(t *testing.T) {
	a := testApp(t, "default")
	for i := range a.state.Providers {
		a.state.Providers[i].Enabled = false
	}
	a.state.Integrations.Providers = []contracts.ProviderDetails{
		{TargetID: "local", ProviderID: "claude", Installed: true, Enabled: true},
		{TargetID: "macbook", ProviderID: "openai", Installed: true, Enabled: true},
	}
	a = modelSnapshotApp(t, a.state)
	recorder := &sideRecordingService{Service: a.service}
	a.service = recorder
	tt := ui.NewTester(a.View, 1320, 880)
	if err := tt.Click("Select device"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("MacBook Pro"); err != nil {
		t.Fatal(err)
	}
	if a.deviceID != "macbook" || a.selectedModel().ProviderID != "openai" {
		t.Fatalf("device selection retained a local model: %s / %s", a.deviceID, a.modelID)
	}
	if err := openTestModelList(tt, "Select model"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("GPT-5.4") || tt.HasText("Opus 4.6") || tt.HasText("No models configured") {
		t.Fatalf("remote picker used local catalog status: %v", tt.Texts())
	}
	tt.Key(0, ui.KeyEscape)
	tt.Key(0, ui.KeyEscape)
	if err := tt.Click("Message"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Use the remote agent")
	tt.Key(0, ui.KeyEnter)
	for _, command := range recorder.commands {
		if command.Kind == "message.send" {
			if command.Send.DeviceID != "macbook" || command.Send.ModelID != "gpt-5.4" {
				t.Fatalf("send used local configuration: %+v", command.Send)
			}
			return
		}
	}
	t.Fatal("remote agent was blocked by local no-agents state")
}

func TestSideComposerProviderFlagsUseItsSessionDevice(t *testing.T) {
	a := testApp(t, "transcript")
	child := sideSession(t, a, "showcase-agent")
	for i := range a.state.Sessions {
		if a.state.Sessions[i].ID == child.ID {
			a.state.Sessions[i].DeviceID = "macbook"
		}
	}
	a.state.Integrations.Providers = []contracts.ProviderDetails{
		{TargetID: "local", ProviderID: "claude", Installed: true, Enabled: true},
		{TargetID: "macbook", ProviderID: "claude", Installed: true},
		{TargetID: "macbook", ProviderID: "openai", Installed: true, Enabled: true},
		{TargetID: "macbook", ProviderID: "cursor", Enabled: true},
	}
	a = modelSnapshotApp(t, a.state)
	a.selectSession("chat-0")
	a.integrationUI.ProviderTarget = "cloud-1"
	a.workspace.OpenSession(child.ID)
	tt := ui.NewTester(a.View, 1600, 1000)
	if err := openTestModelList(tt, "Select model in "+child.Title); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Codex"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("GPT-5.4") {
		t.Fatal("side picker omitted the remote provider")
	}
	state := a.sideComposer(child.ID)
	side := a.sideControls(child.ID, state)
	if side.modelNoAgents() {
		t.Fatal("side availability used the central or Settings device")
	}
	for _, row := range side.modelRows() {
		if row.provider.ID != "claude" && row.provider.ID != "openai" {
			t.Fatalf("side exposed an uninstalled provider: %s", row.provider.ID)
		}
	}
	for _, row := range a.modelRows() {
		if row.provider.ID != "claude" {
			t.Fatalf("main composer adopted side device flags: %s", row.provider.ID)
		}
	}
	if err := tt.Click("GPT-5.4"); err != nil {
		t.Fatal(err)
	}
	if state.modelID != "gpt-5.4" || a.modelID != "claude-opus-4-6" {
		t.Fatalf("side selection lost ownership: %s / %s", state.modelID, a.modelID)
	}
	for _, row := range side.modelRows() {
		if row.provider.ID != "openai" {
			t.Fatalf("side retained a disabled provider after switching: %s", row.provider.ID)
		}
	}
}

func TestComposerRejectsProviderSelectionAfterDeviceFlagsChange(t *testing.T) {
	a := testApp(t, "default")
	controls := a.centralControls()
	controls.modelProvider = "openai"
	rows := controls.modelRows()
	index := -1
	for i, row := range rows {
		if row.provider.ID == "openai" {
			index = i
			break
		}
	}
	if index < 0 {
		t.Fatal("fixture did not offer Codex before its flag changed")
	}
	for i := range a.state.Integrations.Providers {
		provider := &a.state.Integrations.Providers[i]
		if provider.TargetID == "local" && provider.ProviderID == "openai" {
			provider.Enabled = false
		}
	}
	before := a.modelID
	controls.activateModelRow(rows, index)
	if a.modelID != before {
		t.Fatalf("a stale picker action selected a disabled provider: %s", a.modelID)
	}
}

func TestExistingComposerRetainsOnlyInstalledSelectedProvider(t *testing.T) {
	for _, side := range []bool{false, true} {
		name, target := "main", "local"
		if side {
			name, target = "side", "remote"
		}
		t.Run(name, func(t *testing.T) {
			for _, tc := range []struct {
				name                         string
				installed, enabled, metadata bool
				wantOffered                  bool
			}{
				{"disabled but installed", true, false, true, true},
				{"enabled but uninstalled", false, true, true, false},
				{"disabled and uninstalled", false, false, true, false},
				{"missing target metadata", false, false, false, false},
			} {
				t.Run(tc.name, func(t *testing.T) {
					provider := contracts.Provider{ID: "claude", Enabled: true}
					a := &App{deviceID: "local", sessionID: "chat", state: contracts.Snapshot{
						Providers: []contracts.Provider{provider},
						Models: []contracts.Model{
							{ID: "selected", ProviderID: "claude"},
							{ID: "alternative", ProviderID: "claude"},
						},
						Sessions: []contracts.Session{{ID: "chat", DeviceID: target}},
					}}
					if side {
						a.state.Integrations.Providers = append(a.state.Integrations.Providers, contracts.ProviderDetails{TargetID: "local", ProviderID: "claude", Installed: true, Enabled: true})
					}
					if tc.metadata {
						a.state.Integrations.Providers = append(a.state.Integrations.Providers, contracts.ProviderDetails{TargetID: target, ProviderID: "claude", Installed: tc.installed, Enabled: tc.enabled})
					}
					controls := a.centralControls()
					if side {
						controls = a.sideControls("chat", &sideComposerState{})
					}
					controls.modelID = "selected"
					rows := controls.modelRows()
					if got := len(rows) > 0; got != tc.wantOffered {
						t.Fatalf("selected provider offered = %v, want %v", got, tc.wantOffered)
					}
					if controls.modelProviderOffered(provider) != tc.wantOffered {
						t.Fatal("provider rail disagreed with model rows")
					}
					if !tc.wantOffered {
						controls.activateModelRow([]modelRow{{model: a.state.Models[1], provider: provider}}, 0)
						if controls.modelID != "selected" {
							t.Fatal("a stale action selected an uninstalled provider model")
						}
					}
				})
			}
		})
	}
}
