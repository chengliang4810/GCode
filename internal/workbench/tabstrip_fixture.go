//go:build uifixture

package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sync"
	"time"

	"gcode/internal/assets"
	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/mock"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// TabStripFixtureScene describes the common controlled native comparison scene.
type TabStripFixtureScene struct {
	Appearance     string  `json:"appearance"`
	HostWidth      int     `json:"hostWidth"`
	Count          int     `json:"count"`
	ScrollFraction float32 `json:"scrollFraction"`
}

func (s TabStripFixtureScene) Validate() error {
	if s.Appearance != "dark" && s.Appearance != "light" {
		return errors.New("appearance must be dark or light")
	}
	if s.HostWidth != 400 && s.HostWidth != 220 {
		return errors.New("host width must be 400 or 220")
	}
	if s.Count != 0 && s.Count != 2 && s.Count != 6 {
		return errors.New("count must be 0, 2 or 6")
	}
	if math.IsNaN(float64(s.ScrollFraction)) || math.IsInf(float64(s.ScrollFraction), 0) || s.ScrollFraction < 0 || s.ScrollFraction > 1 {
		return errors.New("scroll fraction must be finite and between zero and one")
	}
	return nil
}

// TabStripFixtureBounds contains a measured committed border box in client DIPs.
type TabStripFixtureBounds struct {
	X      float32 `json:"x"`
	Y      float32 `json:"y"`
	Width  float32 `json:"width"`
	Height float32 `json:"height"`
}

func tabStripFixtureBounds(r ui.Rect) TabStripFixtureBounds {
	return TabStripFixtureBounds{r.X, r.Y, r.W, r.H}
}

type TabStripFixtureChip struct {
	SessionID string                `json:"sessionId"`
	Title     string                `json:"title"`
	Kind      string                `json:"kind"`
	Icon      string                `json:"icon"`
	Idle      bool                  `json:"idle"`
	Bounds    TabStripFixtureBounds `json:"bounds"`
}

type TabStripFixtureGeometry struct {
	Host           TabStripFixtureBounds  `json:"host"`
	ScrollViewport TabStripFixtureBounds  `json:"scrollViewport"`
	Chips          []TabStripFixtureChip  `json:"chips"`
	Plus           *TabStripFixtureBounds `json:"plus"`
	Expand         TabStripFixtureBounds  `json:"expand"`
}

func (g TabStripFixtureGeometry) Ready(count int) bool {
	if g.Host.Width <= 0 || g.Host.Height <= 0 || g.ScrollViewport.Width <= 0 || g.ScrollViewport.Height <= 0 || g.Expand.Width <= 0 || g.Expand.Height <= 0 || len(g.Chips) != count {
		return false
	}
	if count == 0 {
		return g.Plus == nil
	}
	if g.Plus == nil || g.Plus.Width <= 0 || g.Plus.Height <= 0 {
		return false
	}
	for _, chip := range g.Chips {
		if chip.Bounds.Width <= 0 || chip.Bounds.Height <= 0 {
			return false
		}
	}
	return true
}

// TabStripFixtureState is plain data from completed layout frames. It contains
// neither elements nor contexts; call State on the UI thread.
type TabStripFixtureState struct {
	Scene             TabStripFixtureScene    `json:"scene"`
	StyleCase         string                  `json:"styleCase,omitempty"`
	StyleEvidence     *TabStripStyleEvidence  `json:"styleEvidence,omitempty"`
	LogicalWidth      float32                 `json:"logicalWidth"`
	LogicalHeight     float32                 `json:"logicalHeight"`
	ParentSessionID   string                  `json:"parentSessionId"`
	ActiveIndex       *int                    `json:"activeIndex"`
	PositiveX         float32                 `json:"positiveX"`
	MaxX              float32                 `json:"maxX"`
	ScrollApplied     bool                    `json:"scrollApplied"`
	Expanded          bool                    `json:"expanded"`
	NativeWindow      bool                    `json:"nativeWindowAttached"`
	Geometry          TabStripFixtureGeometry `json:"geometry"`
	GeometryStable    bool                    `json:"geometryStable"`
	StableFrames      int                     `json:"stableFrames"`
	BuildPasses       int                     `json:"buildPasses"`
	FrameSamples      int                     `json:"distinctFrameSamples"`
	Surface           string                  `json:"surface"`
	MotionPreference  string                  `json:"motionPreference"`
	FontFamily        string                  `json:"fontFamily"`
	RootFontSize      float32                 `json:"rootFontSize"`
	EffectiveChipSize float32                 `json:"effectiveChipSize"`
	LayoutError       string                  `json:"layoutError,omitempty"`
	Pointer           TabStripFixturePointer  `json:"pointer"`
}

// Pointer records fixture input observations, never the real OS cursor.
type TabStripFixturePointer struct {
	Source            string  `json:"source"`
	MoveSeen          bool    `json:"moveSeen"`
	LastMoveX         float32 `json:"lastMoveX"`
	LastMoveY         float32 `json:"lastMoveY"`
	AnyControlHovered bool    `json:"anyControlHovered"`
	MoveCount         int     `json:"moveCount"`
}

func (s TabStripFixtureState) Ready() bool {
	active := s.Scene.Count == 0 && s.ActiveIndex == nil || s.Scene.Count > 0 && s.ActiveIndex != nil && *s.ActiveIndex == 0
	return active && math.Abs(float64(s.PositiveX-s.Scene.ScrollFraction*s.MaxX)) < .01 &&
		s.LogicalWidth == 840 && s.LogicalHeight == 560 && s.ParentSessionID == "parent" &&
		s.ScrollApplied && s.GeometryStable && s.StableFrames >= 2 && s.LayoutError == "" && s.Geometry.Ready(s.Scene.Count)
}

// TabStripFixture mounts the actual State.TabStrip. Only scaffolding and typed
// fixture data live here; there is no alternate chip, icon or fade renderer.
type TabStripFixture struct {
	workspace     *State
	service       *tabStripFixtureService
	scene         TabStripFixtureScene
	window        *mygo.Window
	host          ui.Handle
	chips         []ui.Handle
	plus, expand  ui.Handle
	built         TabStripFixtureState
	observed      TabStripFixtureState
	rendered      TabStripFixtureState
	scrollApplied bool
	stableFrames  int
	settlePasses  int
	buildPasses   int
	frameSamples  int
	lastSampleAt  time.Time
	pointer       TabStripFixturePointer
	hovered       bool
	styleCase     string
	hoveredTab    string
	edgeTooltip   *TabStripFixtureTooltipEvidence
}

type tabStripObserver struct {
	handles    map[string]*ui.Handle
	hovered    *bool
	hoveredTab *string
}

func (o tabStripObserver) bind(kind, id string, element ui.Element) {
	if o.hovered != nil && element.Hovered() {
		*o.hovered = true
	}
	if kind == "chip" && o.hoveredTab != nil && element.Hovered() {
		*o.hoveredTab = id
	}
	if handle := o.handles[kind+":"+id]; handle != nil {
		element.Bind(handle)
	}
}

func NewTabStripFixture(scene TabStripFixtureScene) (*TabStripFixture, error) {
	if err := scene.Validate(); err != nil {
		return nil, err
	}
	if err := assets.RegisterFonts(); err != nil {
		return nil, err
	}
	f := &TabStripFixture{service: &tabStripFixtureService{}}
	if err := f.SetScene(scene); err != nil {
		return nil, err
	}
	return f, nil
}

// SetScene is called on the UI thread after a native window exists.
func (f *TabStripFixture) SetScene(scene TabStripFixtureScene) error {
	if err := scene.Validate(); err != nil {
		return err
	}
	if f.styleCase != "" && (scene.HostWidth != 400 || scene.Count != 2 || scene.ScrollFraction != 0) {
		return errors.New("style supplement requires width400, count2 and scroll0")
	}
	f.service.setScene(scene, f.styleCase)
	snapshot, err := f.service.Bootstrap(context.Background())
	if err != nil {
		return err
	}
	if f.workspace != nil {
		_ = f.workspace.Close()
	}
	s := New(f.window, f.service)
	s.Update(snapshot)
	s.SwitchSession("parent")
	// A side renderer enables the production session-tab path. No transcript
	// is mounted because this fixture intentionally renders only TabStrip.
	s.RenderSession = func(*ui.Context, string) {}
	s.OnToggleExpand = func() {}
	for _, session := range snapshot.Sessions {
		if session.ParentSessionID == "parent" {
			s.OpenSession(session.ID)
		}
	}
	if scene.Count > 0 && !tabStripStyleUsesFiles(f.styleCase) {
		s.OpenSession("side-01")
	}
	if tabStripStyleUsesFiles(f.styleCase) {
		for _, file := range snapshot.Files {
			s.OpenFile(file.ID)
		}
		s.OpenFile("file-01")
		s.documents["file-01"].editor.Buffer.Set("unsaved fixture edit\n")
	}
	f.workspace, f.scene = s, scene
	f.host, f.plus, f.expand = ui.Handle{}, ui.Handle{}, ui.Handle{}
	f.chips = make([]ui.Handle, scene.Count)
	s.tabObserver.handles = map[string]*ui.Handle{"plus:": &f.plus, "expand:": &f.expand}
	s.tabObserver.hovered = &f.hovered
	s.tabObserver.hoveredTab = &f.hoveredTab
	for i := range f.chips {
		s.tabObserver.handles["chip:"+s.tabs[i].id] = &f.chips[i]
	}
	f.built, f.observed, f.rendered = TabStripFixtureState{}, TabStripFixtureState{}, TabStripFixtureState{}
	f.scrollApplied, f.stableFrames, f.settlePasses = false, 0, 0
	f.buildPasses, f.frameSamples, f.lastSampleAt = 0, 0, time.Time{}
	f.pointer = TabStripFixturePointer{}
	f.hoveredTab = ""
	f.edgeTooltip = nil
	return nil
}

func (f *TabStripFixture) UseNativeWindow(window *mygo.Window) {
	f.window = window
	f.workspace.win = window
}

func (f *TabStripFixture) Close() {
	if f.workspace != nil {
		_ = f.workspace.Close()
	}
	_ = f.service.Close()
}

func (f *TabStripFixture) State() TabStripFixtureState {
	s := f.rendered
	// State is read between frames on the UI thread. Pair the last received
	// plain input with hover from the most recent completed build. Handle
	// boxes remain observations from completed layouts; no Element escapes.
	if f.built.Scene == s.Scene {
		renderedPointer := s.Pointer
		s.Pointer = f.pointer
		s.Pointer.Source = f.built.Pointer.Source
		s.Pointer.AnyControlHovered = f.built.Pointer.AnyControlHovered
		if s.Pointer != renderedPointer {
			// A received message is not a committed frame. Keep readiness
			// false until this input has passed the stable-frame sampling.
			s.GeometryStable, s.StableFrames = false, 0
		}
	}
	s.Geometry.Chips = append([]TabStripFixtureChip{}, s.Geometry.Chips...)
	if s.ActiveIndex != nil {
		active := *s.ActiveIndex
		s.ActiveIndex = &active
	}
	if s.Geometry.Plus != nil {
		plus := *s.Geometry.Plus
		s.Geometry.Plus = &plus
	}
	if s.StyleEvidence != nil {
		style := *s.StyleEvidence
		style.DirtyFileIDs = append([]string{}, style.DirtyFileIDs...)
		if style.Edge != nil {
			edge := *style.Edge
			edge.FilePaths = append([]string{}, edge.FilePaths...)
			style.Edge = &edge
		}
		s.StyleEvidence = &style
	}
	return s
}

func (f *TabStripFixture) View(c *ui.Context) {
	s := f.workspace
	f.buildPasses++
	observed := f.built
	observed.PositiveX, observed.MaxX = s.tabScroll.X, s.tabScroll.MaxX
	observed.Geometry = f.geometry(c)
	valid := observed.ScrollApplied && observed.Geometry.Ready(f.scene.Count)
	distinct := !c.Now().Equal(f.lastSampleAt)
	if distinct {
		f.frameSamples++
		f.lastSampleAt = c.Now()
		if valid && reflect.DeepEqual(tabStripFixtureStabilitySnapshot(observed), tabStripFixtureStabilitySnapshot(f.observed)) {
			f.stableFrames = min(f.stableFrames+1, 2)
		} else {
			f.stableFrames = 0
		}
		f.observed = observed
	}
	f.rendered = observed
	f.rendered.StableFrames = f.stableFrames
	f.rendered.GeometryStable = f.stableFrames >= 2
	f.rendered.BuildPasses, f.rendered.FrameSamples = f.buildPasses, f.frameSamples
	if !f.scrollApplied && observed.Geometry.Host.Width > 0 && observed.Geometry.ScrollViewport.Width > 0 {
		s.tabScroll.X = f.scene.ScrollFraction * s.tabScroll.MaxX
		f.scrollApplied = true
	}
	p := s.palette()
	c.SetTheme(p.UI())
	c.SetMotionPreferences(&ui.MotionPreferences{Motion: ui.MotionReduced})
	c.DisableAnimations()
	c.Root().Background(p.Shell).Font("Geist").FontSize(14).FontWeight(400).TextColor(p.Text)
	c.Root().HandleInput(func(event ui.InputEvent) bool {
		if event.Kind == ui.InputPointerMove {
			f.pointer.MoveSeen, f.pointer.LastMoveX, f.pointer.LastMoveY = true, event.X, event.Y
			f.pointer.MoveCount++
			f.settlePasses = 0
			return true
		}
		return false
	})
	design.ApplyReferenceTypography(c)
	f.hovered = false
	f.hoveredTab = ""
	hostX, hostY := tabStripFixtureHostOrigin(f.styleCase)
	ui.Box(c).Label("Fixture host").Bind(&f.host).Absolute().Left(hostX).Top(hostY).Width(float32(f.scene.HostWidth)).Height(38).Children(func() {
		s.TabStrip(c, false)
	})
	width, height := c.Size()
	next := TabStripFixtureState{
		Scene: f.scene, LogicalWidth: width, LogicalHeight: height, ParentSessionID: s.scopeID,
		ScrollApplied: f.scrollApplied, NativeWindow: f.window != nil,
		Surface: "opaque", MotionPreference: "reduced", FontFamily: "Geist", RootFontSize: 14,
		EffectiveChipSize: design.UISize(c, 11.5),
	}
	if len(s.tabs) > 0 {
		active := s.active
		next.ActiveIndex = &active
	}
	next.Pointer = f.pointer
	next.Pointer.Source = "headless Tester input"
	if f.window != nil {
		next.Pointer.Source = "native input observations; CLI requests synthetic child-HWND messages; not OS cursor position"
	}
	next.Pointer.AnyControlHovered = f.hovered
	if f.styleCase != "" {
		next.StyleCase = f.styleCase
		next.StyleEvidence = &TabStripStyleEvidence{HoveredTabID: f.hoveredTab, DirtyFileIDs: []string{}}
		next.StyleEvidence.Edge = f.edgeEvidence(c)
		for _, tab := range s.tabs {
			if document := s.documents[tab.id]; document != nil && document.editor.Buffer.Modified() {
				next.StyleEvidence.DirtyFileIDs = append(next.StyleEvidence.DirtyFileIDs, tab.id)
			}
		}
	}
	f.built = next
	needsSettle := !f.rendered.Ready() || f.rendered.Pointer != next.Pointer || !reflect.DeepEqual(f.rendered.StyleEvidence, next.StyleEvidence)
	if needsSettle && f.settlePasses < 8 {
		if distinct {
			f.settlePasses++
		}
		c.After(time.Millisecond)
	} else if needsSettle {
		f.rendered.LayoutError = "fixture did not reach two stable completed geometry frames"
	}
}

// A tooltip is a separate overlay with its own production lifecycle. Its
// diagnostic manager/card observations may change while the strip's committed
// geometry, typed files and actual hover/input stay fixed. Keep those complete
// observations in the evidence, but do not count them as strip instability.
func tabStripFixtureStabilitySnapshot(s TabStripFixtureState) TabStripFixtureState {
	if s.StyleEvidence == nil || s.StyleEvidence.Edge == nil {
		return s
	}
	style := *s.StyleEvidence
	edge := *style.Edge
	edge.Tooltip = TabStripFixtureTooltipEvidence{}
	style.Edge = &edge
	s.StyleEvidence = &style
	return s
}

func (f *TabStripFixture) geometry(c *ui.Context) TabStripFixtureGeometry {
	g := TabStripFixtureGeometry{
		Host:           tabStripFixtureBounds(f.host.Bounds(c)),
		ScrollViewport: tabStripFixtureBounds(f.workspace.tabScroll.Bounds(c)),
		Expand:         tabStripFixtureBounds(f.expand.Bounds(c)),
		Chips:          make([]TabStripFixtureChip, 0, len(f.chips)),
	}
	if len(f.chips) > 0 {
		plus := tabStripFixtureBounds(f.plus.Bounds(c))
		g.Plus = &plus
	}
	for _, tab := range f.workspace.tabs {
		var bounds TabStripFixtureBounds
		if handle := f.workspace.tabObserver.handles["chip:"+tab.id]; handle != nil {
			bounds = tabStripFixtureBounds(handle.Bounds(c))
		}
		idle := false
		icon := tabIcon(tab.kind)
		if tab.kind == "file" && f.workspace.documents[tab.id] != nil {
			icon = "file-type-icon"
		}
		for _, session := range f.workspace.snapshot.Sessions {
			if session.ID == tab.id {
				idle = session.Status == contracts.Idle
				break
			}
		}
		g.Chips = append(g.Chips, TabStripFixtureChip{
			SessionID: tab.id, Title: tab.title, Kind: tab.kind, Icon: icon, Idle: idle,
			Bounds: bounds,
		})
	}
	return g
}

type tabStripFixtureService struct {
	mu     sync.Mutex
	state  contracts.Snapshot
	closed bool
}

var _ contracts.Service = (*tabStripFixtureService)(nil)

func (s *tabStripFixtureService) setScene(scene TabStripFixtureScene, styleCase string) {
	settings := mock.DefaultSettings()
	settings.General.Language = "en" // Frozen reference fixture language.
	settings.Appearance.Mode, settings.Appearance.ThemeID = scene.Appearance, "zeron-"+scene.Appearance
	settings.Appearance.Surface, settings.Appearance.MotionPreference = "opaque", "reduced"
	settings.Appearance.SmoothScrolling = false
	settings.Appearance.Font, settings.Appearance.FontSize = "Geist", 13
	state := contracts.Snapshot{SchemaVersion: contracts.SchemaVersion, Revision: 1, Settings: settings}
	state.Sessions = []contracts.Session{{ID: "parent", Title: "Parent", Status: contracts.Idle}}
	if tabStripStyleUsesFiles(styleCase) {
		state.Settings.Files.Autosave = false
		for i := 1; i <= 2; i++ {
			name := fmt.Sprintf("Tab%02d.txt", i)
			filePath := name
			if styleCase == "file-hover-edge" {
				filePath = "workspace/long-component-path/long-component-path/long-component-path/" + name
			}
			state.Files = append(state.Files, contracts.File{ID: fmt.Sprintf("file-%02d", i), Name: name, Path: filePath, Kind: "file", Content: "saved fixture text\n", Version: 1})
		}
	}
	for i := 1; i <= scene.Count && !tabStripStyleUsesFiles(styleCase); i++ {
		state.Sessions = append(state.Sessions, contracts.Session{ID: fmt.Sprintf("side-%02d", i), Title: fmt.Sprintf("Tab%02d", i), ParentSessionID: "parent", SideChat: true, Status: contracts.Idle})
	}
	s.mu.Lock()
	s.state = state
	s.mu.Unlock()
}

func (s *tabStripFixtureService) Bootstrap(ctx context.Context) (contracts.Snapshot, error) {
	return s.Snapshot(ctx)
}

func (s *tabStripFixtureService) Snapshot(ctx context.Context) (contracts.Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return contracts.Snapshot{}, err
	}
	if s.closed {
		return contracts.Snapshot{}, errors.New("fixture service is closed")
	}
	data, err := json.Marshal(s.state)
	if err != nil {
		return contracts.Snapshot{}, err
	}
	var state contracts.Snapshot
	err = json.Unmarshal(data, &state)
	return state, err
}

func (s *tabStripFixtureService) Execute(ctx context.Context, command contracts.Command) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return fmt.Errorf("tabstrip fixture has no mutating backend command: %s", command.Kind)
}

func (s *tabStripFixtureService) Subscribe(ctx context.Context) (<-chan contracts.Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ch := make(chan contracts.Event)
	close(ch)
	return ch, nil
}

func (s *tabStripFixtureService) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	return nil
}
