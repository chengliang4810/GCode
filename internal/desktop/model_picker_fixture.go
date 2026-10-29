//go:build uifixture

package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"gcode/internal/assets"
	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/mock"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// CompactPickerFixture is isolated test scaffolding for the frozen reference's
// compact-picker-fixture. It mounts the production model trigger/popover; it
// owns no alternate model-picker renderer or product entry point.
type CompactPickerFixture struct {
	app          *App
	appearance   string
	header       ui.Handle
	row          ui.Handle
	anchor       ui.Handle
	popup        ui.Handle
	built        CompactPickerFixtureState
	observed     CompactPickerFixtureState
	rendered     CompactPickerFixtureState
	settlePasses int
}

const compactPickerFixtureSettlePassLimit = 8

// CompactPickerFixtureBounds is a committed native layout box in client DIPs.
type CompactPickerFixtureBounds struct {
	X      float32 `json:"x"`
	Y      float32 `json:"y"`
	Width  float32 `json:"width"`
	Height float32 `json:"height"`
}

// CompactPickerFixtureGeometry contains the actual production control boxes
// and the source fixture scaffold boxes, all measured in the same frame.
type CompactPickerFixtureGeometry struct {
	Header CompactPickerFixtureBounds `json:"header"`
	Row    CompactPickerFixtureBounds `json:"row"`
	Anchor CompactPickerFixtureBounds `json:"anchor"`
	Popup  CompactPickerFixtureBounds `json:"popup"`
}

// Ready reports whether every open-scene box has committed nonempty geometry.
// It makes no assertion about reference parity.
func (g CompactPickerFixtureGeometry) Ready() bool {
	return g.Header.Width > 0 && g.Header.Height > 0 && g.Row.Width > 0 && g.Row.Height > 0 &&
		g.Anchor.Width > 0 && g.Anchor.Height > 0 && g.Popup.Width > 0 && g.Popup.Height > 0
}

type CompactPickerFixtureState struct {
	Appearance       string                       `json:"appearance"`
	Page             string                       `json:"page"`
	Width            float32                      `json:"logicalWidth"`
	Height           float32                      `json:"logicalHeight"`
	ProviderID       string                       `json:"providerId"`
	ModelID          string                       `json:"modelId"`
	Reasoning        string                       `json:"effectiveReasoning"`
	Fast             bool                         `json:"fast"`
	ContextWindow    string                       `json:"contextWindow"`
	PopoverOpen      bool                         `json:"popoverOpen"`
	Surface          string                       `json:"surface"`
	MotionPreference string                       `json:"motionPreference"`
	NativeWindow     bool                         `json:"nativeWindowAttached"`
	OneXOptionText   bool                         `json:"oneXOptionText"`
	Geometry         CompactPickerFixtureGeometry `json:"geometry"`
	GeometryStable   bool                         `json:"geometryStable"`
	LayoutError      string                       `json:"layoutError,omitempty"`
}

func NewCompactPickerFixture(appearance, page string) (*CompactPickerFixture, error) {
	if err := assets.RegisterFonts(); err != nil {
		return nil, err
	}
	backend := &compactPickerFixtureService{state: compactPickerFixtureCatalog()}
	app, err := NewWithOptions(backend, Options{Now: func() time.Time {
		return time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC)
	}})
	if err != nil {
		return nil, err
	}
	fixture := &CompactPickerFixture{app: app}
	if err := fixture.SetScene(appearance, page); err != nil {
		app.Close()
		return nil, err
	}
	return fixture, nil
}

// SetScene selects controlled fixture state. Call it on the UI thread once a
// native window exists, for example through Window.Update.
func (f *CompactPickerFixture) SetScene(appearance, page string) error {
	if appearance != "dark" && appearance != "light" {
		return fmt.Errorf("fixture appearance must be dark or light: %q", appearance)
	}
	if page != "panel" && page != "list" {
		return fmt.Errorf("fixture page must be panel or list: %q", page)
	}
	f.appearance = appearance
	f.app.state.Settings.Appearance.Mode = appearance
	f.app.state.Settings.Appearance.ThemeID = "zeron-" + appearance
	f.app.modelMenu, f.app.modelListPage = true, false
	f.app.modelQuery, f.app.modelProvider = "", ""
	if page == "list" {
		f.app.centralControls().openModelList()
	}
	return nil
}

func (f *CompactPickerFixture) Close() { f.app.Close() }

// UseNativeWindow selects the same display-dependent rendering branches as
// App.Attach without starting the application's subscriptions in this fixture.
// Call it on the UI thread before showing the fixture window.
func (f *CompactPickerFixture) UseNativeWindow(window *mygo.Window) {
	f.app.win = window
}

// State returns plain scene data paired with its committed layout boxes, not
// a newly requested scene awaiting its first frame. Call it on the UI thread.
func (f *CompactPickerFixture) State() CompactPickerFixtureState { return f.rendered }

func (f *CompactPickerFixture) View(c *ui.Context) {
	a := f.app
	width, height := c.Size()
	// Handles read the committed frame at the start of this build. Pair them
	// with that frame's plain scene state, rather than with the new request.
	observed := f.built
	observed.Geometry = CompactPickerFixtureGeometry{
		Header: compactPickerFixtureBounds(f.header.Bounds(c)),
		Row:    compactPickerFixtureBounds(f.row.Bounds(c)),
		Anchor: compactPickerFixtureBounds(f.anchor.Bounds(c)),
		Popup:  compactPickerFixtureBounds(f.popup.Bounds(c)),
	}
	stable := observed == f.observed && observed.Geometry.Ready()
	f.observed, f.rendered = observed, observed
	f.rendered.GeometryStable = stable
	a.palette = design.Palette("zeron-"+f.appearance, f.appearance == "dark")
	c.SetTheme(a.palette.UI())
	c.SetMotionPreferences(&ui.MotionPreferences{Motion: ui.MotionReduced})
	c.DisableAnimations()
	c.Root().Background(a.palette.Shell).Font("Geist").FontSize(13).TextColor(a.palette.Text)
	design.ApplyReferenceTypography(c)
	// Same shell as the frozen example: p24, bottom-aligned title and 48px
	// anchor row, gap16. The page header itself uses an additional 8px inset.
	ui.Column(c).Fill().Padding(24).Justify(ui.End).Gap(16).Children(func() {
		ui.Row(c).Bind(&f.header).Label("Fixture header").FillWidth().Padding(0, 8).Height(26).AlignItems(ui.Baseline).Gap(10).Children(func() {
			ui.Text(c, "Compact picker · "+f.appearance).FontSize(20).FontWeight(500).FixedLineHeight(26)
		})
		ui.Row(c).Bind(&f.row).Label("Fixture trigger row").FillWidth().Height(48).AlignItems(ui.Center).Children(func() {
			// The reference's Pickers root is a min-width-zero flex row. Its
			// only child here is the model chip: fixture access modes are empty.
			ui.Row(c).MinWidth(0).AlignItems(ui.Center).Gap(4).Children(func() {
				// These are the product elements, including its modelPopover.
				anchor, popup := a.centralControls().modelTrigger(c)
				anchor.Bind(&f.anchor)
				popup.Bind(&f.popup)
			})
		})
	})
	page := "panel"
	if a.modelListPage {
		page = "list"
	}
	controls := a.centralControls()
	model := controls.selectedModel()
	_, _, _, fast := controls.compactFastOption(model)
	contextWindow := ""
	for _, option := range model.Options {
		if option.ID == "contextWindow" {
			contextWindow = controls.modelOptionValue(model, option)
			break
		}
	}
	next := CompactPickerFixtureState{
		Appearance: f.appearance, Page: page, Width: width, Height: height,
		ProviderID: model.ProviderID,
		ModelID:    a.modelID, Reasoning: a.reasoning, PopoverOpen: a.modelMenu,
		Fast: fast, ContextWindow: contextWindow,
		Surface: "opaque", MotionPreference: "reduced",
		NativeWindow: a.win != nil, OneXOptionText: a.centralControls().modelOptionPixelSnap(),
	}
	if f.built != next {
		f.settlePasses = 0
		c.After(time.Millisecond)
	} else if next.PopoverOpen && !stable {
		// Popover budgeting reads the preceding anchor. Require two equal
		// committed samples, but stop if a missing box cannot settle.
		if f.settlePasses < compactPickerFixtureSettlePassLimit {
			f.settlePasses++
			c.After(time.Millisecond)
		} else {
			f.rendered.LayoutError = "fixture geometry did not stabilize within 8 extra frames"
		}
	}
	f.built = next
}

func compactPickerFixtureBounds(r ui.Rect) CompactPickerFixtureBounds {
	return CompactPickerFixtureBounds{X: r.X, Y: r.Y, Width: r.W, Height: r.H}
}

func compactPickerFixtureCatalog() contracts.Snapshot {
	settings := mock.DefaultSettings()
	settings.General.Language = "en" // Frozen reference fixture language.
	settings.Appearance.Mode, settings.Appearance.ThemeID = "dark", "zeron-dark"
	settings.Appearance.Surface, settings.Appearance.MotionPreference = "opaque", "reduced"
	settings.Appearance.SmoothScrolling = false
	settings.General.CompactModelPicker = true
	settings.ModelPicker = contracts.ModelPickerSettings{
		ProviderID: "codex", Options: map[string]string{},
		Selections: map[string]contracts.ModelSelection{"codex": {ModelID: "gpt-5.4", Reasoning: "High"}},
	}
	return contracts.Snapshot{
		SchemaVersion: contracts.SchemaVersion, Revision: 1, Settings: settings,
		ModelCatalog: contracts.ModelCatalogState{State: contracts.CatalogReady},
		Devices:      []contracts.Device{{ID: "local", Name: "This device", Current: true, Status: "connected"}},
		Providers: []contracts.Provider{{
			ID: "codex", Name: "Codex", Icon: "openai-mark", Status: "connected", Enabled: true,
			Models: []string{"gpt-5.4", "gpt-5.3-codex", "gpt-5.2", "gpt-5.1-codex-mini"},
		}},
		Integrations: contracts.IntegrationState{
			ProvidersState: contracts.LoadReady,
			Providers:      []contracts.ProviderDetails{{ProviderID: "codex", TargetID: "local", Installed: true, Enabled: true}},
		},
		Models: []contracts.Model{
			{ID: "gpt-5.4", ProviderID: "codex", Label: "GPT-5.4", Description: "Best for complex coding and reasoning",
				ReasoningLevels: []string{"Low", "Medium", "High", "X-High"},
				Options: []contracts.ModelOption{
					{ID: "serviceTier", Label: "Service tier", DefaultChoice: "standard", Choices: []string{"standard", "fast"},
						ChoiceLabels: map[string]string{"standard": "Standard", "fast": "Fast"}},
					{ID: "contextWindow", Label: "Context window", DefaultChoice: "standard", Choices: []string{"standard", "1m"},
						ChoiceLabels: map[string]string{"standard": "Standard", "1m": "1M tokens"}},
				}},
			{ID: "gpt-5.3-codex", ProviderID: "codex", Label: "GPT-5.3 Codex", Description: "Optimized for agentic coding"},
			{ID: "gpt-5.2", ProviderID: "codex", Label: "GPT-5.2", Description: "General purpose reasoning"},
			{ID: "gpt-5.1-codex-mini", ProviderID: "codex", Label: "GPT-5.1 Codex Mini", Description: "Fast, efficient coding"},
		},
		Messages: map[string][]contracts.Message{},
	}
}

// The fixture's typed backend is in-memory only. It accepts the product
// settings mutation path so option/favorite interactions retain real state.
type compactPickerFixtureService struct {
	mu     sync.Mutex
	state  contracts.Snapshot
	closed bool
}

var _ contracts.Service = (*compactPickerFixtureService)(nil)

func (s *compactPickerFixtureService) Bootstrap(ctx context.Context) (contracts.Snapshot, error) {
	return s.Snapshot(ctx)
}

func (s *compactPickerFixtureService) Snapshot(ctx context.Context) (contracts.Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return contracts.Snapshot{}, err
	}
	if s.closed {
		return contracts.Snapshot{}, errors.New("fixture backend is closed")
	}
	data, err := json.Marshal(s.state)
	if err != nil {
		return contracts.Snapshot{}, err
	}
	var snapshot contracts.Snapshot
	err = json.Unmarshal(data, &snapshot)
	return snapshot, err
}

func (s *compactPickerFixtureService) Execute(ctx context.Context, command contracts.Command) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.closed {
		return errors.New("fixture backend is closed")
	}
	if command.Kind != "settings.update" || command.Settings == nil {
		return fmt.Errorf("unsupported fixture command: %s", command.Kind)
	}
	s.state.Settings = *command.Clone().Settings
	s.state.Revision++
	return nil
}

func (s *compactPickerFixtureService) Subscribe(ctx context.Context) (<-chan contracts.Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	events := make(chan contracts.Event)
	close(events)
	return events, nil
}

func (s *compactPickerFixtureService) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	return nil
}
