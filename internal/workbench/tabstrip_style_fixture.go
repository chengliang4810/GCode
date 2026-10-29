//go:build uifixture

package workbench

import (
	"fmt"
	"reflect"
	"time"

	"github.com/egoist/mygo/ui"
)

// TabStripStyleEvidence describes observed production widget state. It is absent
// from the unchanged baseline matrix and is not a visual acceptance assertion.
type TabStripStyleEvidence struct {
	HoveredTabID string                       `json:"hoveredTabId"`
	DirtyFileIDs []string                     `json:"dirtyFileIds"`
	Edge         *TabStripFixtureEdgeEvidence `json:"edge,omitempty"`
}

type TabStripFixtureEdgeEvidence struct {
	FilePaths []string                       `json:"filePaths"`
	Tooltip   TabStripFixtureTooltipEvidence `json:"tooltip"`
}

// Tooltip is a tagged observation of the production manager and its selected
// overlay's durable Handle, not an alternate visibility or layout decision.
type TabStripFixtureTooltipEvidence struct {
	ShownOwnerIndex     int                   `json:"shownOwnerIndex"`
	ManagerVisible      bool                  `json:"managerVisible"`
	Phase               string                `json:"phase"`
	SlotIndex           int                   `json:"slotIndex"`
	Close               bool                  `json:"close"`
	Text                string                `json:"text"`
	CapturedOwnerBounds TabStripFixtureBounds `json:"capturedOwnerBounds"`
	ShowPoint           [2]float32            `json:"showPoint"`
	CommittedBounds     TabStripFixtureBounds `json:"committedBounds"`
	BoundsSource        string                `json:"boundsSource"`
	ObservedAtBuildPass int                   `json:"observedAtBuildPass"`
	ObservedAt          time.Time             `json:"observedAt"`
}

// SetStyleCase opts into a separate two-theme supplement. The caller must use
// the fixed 400-DIP, two-tab, unscrolled scene; the R14 matrix remains separate.
func (f *TabStripFixture) SetStyleCase(style string) error {
	if style != "" && style != "inactive-hover" && style != "dirty-file" && style != "file-hover" && style != "file-hover-edge" {
		return fmt.Errorf("unsupported tabstrip style case %q", style)
	}
	if style != "" && (f.scene.HostWidth != 400 || f.scene.Count != 2 || f.scene.ScrollFraction != 0) {
		return fmt.Errorf("style supplement requires width400, count2 and scroll0")
	}
	f.styleCase = style
	return f.SetScene(f.scene)
}

// TabStripFixturePointerTarget reports a point in the actual committed chip,
// rather than assuming its layout. Baseline and dirty cases stay outside it.
func TabStripFixturePointerTarget(state TabStripFixtureState) (float32, float32, bool) {
	if state.StyleCase == "inactive-hover" || tabStripStyleHoversFile(state.StyleCase) {
		if len(state.Geometry.Chips) != 2 {
			return 0, 0, false
		}
		chip := state.Geometry.Chips[1].Bounds
		return chip.X + chip.Width/2, chip.Y + chip.Height/2, chip.Width > 0 && chip.Height > 0
	}
	return 10, 10, true
}

// TabStripFixturePointerReady checks received input and actual production hover.
// Synthetic message coordinates alone are never evidence of widget hover.
func TabStripFixturePointerReady(state TabStripFixtureState) bool {
	x, y, ok := TabStripFixturePointerTarget(state)
	if !ok || !state.Pointer.MoveSeen || state.Pointer.LastMoveX != x || state.Pointer.LastMoveY != y {
		return false
	}
	if state.StyleCase == "inactive-hover" {
		return state.Pointer.AnyControlHovered && state.StyleEvidence != nil && state.StyleEvidence.HoveredTabID == "side-02"
	}
	if tabStripStyleUsesFiles(state.StyleCase) {
		if state.StyleEvidence == nil || len(state.StyleEvidence.DirtyFileIDs) != 1 || state.StyleEvidence.DirtyFileIDs[0] != "file-01" {
			return false
		}
		if tabStripStyleHoversFile(state.StyleCase) {
			return state.Pointer.AnyControlHovered && state.StyleEvidence.HoveredTabID == "file-02"
		}
		if state.StyleEvidence.HoveredTabID != "" {
			return false
		}
	}
	return !state.Pointer.AnyControlHovered
}

func tabStripStyleUsesFiles(style string) bool {
	return style == "dirty-file" || tabStripStyleHoversFile(style)
}

func tabStripStyleHoversFile(style string) bool {
	return style == "file-hover" || style == "file-hover-edge"
}

func tabStripFixtureHostOrigin(style string) (float32, float32) {
	if style == "file-hover-edge" {
		return 400, 510
	}
	return 40, 40
}

func (f *TabStripFixture) edgeEvidence(c *ui.Context) *TabStripFixtureEdgeEvidence {
	if f.styleCase != "file-hover-edge" {
		return nil
	}
	edge := &TabStripFixtureEdgeEvidence{FilePaths: []string{}}
	for _, tab := range f.workspace.tabs {
		if document := f.workspace.documents[tab.id]; document != nil {
			edge.FilePaths = append(edge.FilePaths, document.file.Path)
		}
	}
	manager := &f.workspace.tabTooltip
	tooltip := TabStripFixtureTooltipEvidence{
		ShownOwnerIndex: manager.shown, Phase: "none", SlotIndex: -1,
		BoundsSource: "owner.card.Handle.Bounds(c): previous committed layout read during tagged fixture build; observedAt is the first observation of this value, not its layout time",
	}
	if index := manager.shown; index >= 0 && index < len(manager.owners) {
		owner := manager.owners[index]
		if owner != nil {
			tooltip.SlotIndex, tooltip.Close = index/2, owner.close
			tooltip.Text, tooltip.CapturedOwnerBounds = owner.text, tabStripFixtureBounds(owner.bounds)
			tooltip.ShowPoint = [2]float32{owner.x, owner.y}
			tooltip.CommittedBounds = tabStripFixtureBounds(owner.card.Bounds(c))
			switch owner.phase {
			case tabTooltipIdle:
				tooltip.Phase = "idle"
			case tabTooltipWaiting:
				tooltip.Phase = "waiting"
			case tabTooltipVisible:
				tooltip.Phase, tooltip.ManagerVisible = "visible", true
			}
		}
	}
	if f.edgeTooltip != nil {
		previous := *f.edgeTooltip
		previous.ObservedAt, previous.ObservedAtBuildPass = time.Time{}, 0
		if reflect.DeepEqual(previous, tooltip) {
			edge.Tooltip = *f.edgeTooltip
			return edge
		}
	}
	tooltip.ObservedAt, tooltip.ObservedAtBuildPass = c.Now().UTC(), f.buildPasses
	f.edgeTooltip = &tooltip
	edge.Tooltip = tooltip
	return edge
}
