//go:build uifixture

package desktop

import (
	"fmt"
	"strings"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// FooterUsageFixture embeds the actual product ring cluster and cards. Scene
// changes are controlled DTO state, not evidence of native input acceptance.
type FooterUsageFixture struct {
	app               *App
	appearance, scene string
	row               ui.Handle
	rendered          FooterUsageFixtureState
	previous          FooterUsageFixtureState
	settlePasses      int
}

type FooterUsageFixtureState struct {
	Appearance     string                      `json:"appearance"`
	Scene          string                      `json:"scene"`
	Width          float32                     `json:"logicalWidth"`
	Height         float32                     `json:"logicalHeight"`
	Context        contracts.ContextUsage      `json:"contextUsage"`
	Accounts       []contracts.ProviderAccount `json:"accounts"`
	Row            CompactPickerFixtureBounds  `json:"row"`
	AccountsOpen   bool                        `json:"accountsOpen"`
	ContextOpen    bool                        `json:"contextOpen"`
	GeometryStable bool                        `json:"geometryStable"`
	NativeWindow   bool                        `json:"nativeWindowAttached"`
	LayoutError    string                      `json:"layoutError,omitempty"`
}

func NewFooterUsageFixture(appearance, scene string) (*FooterUsageFixture, error) {
	// Reuse the fixture's deterministic typed service and frozen Codex catalog.
	compact, err := NewCompactPickerFixture(appearance, "panel")
	if err != nil {
		return nil, err
	}
	f := &FooterUsageFixture{app: compact.app}
	f.app.modelMenu = false
	f.app.sessionID = "footer-chat"
	f.app.state.Sessions = []contracts.Session{{ID: "footer-chat", Title: "Footer fixture", DeviceID: "local", ModelID: "gpt-5.4"}}
	f.app.state.ContextUsage = make(map[string]contracts.ContextUsage)
	f.app.state.Integrations.Accounts = []contracts.ProviderAccount{
		{ID: "a", TargetID: "local", ProviderID: "codex", Email: "first@example.test", Plan: "Pro", InUse: true, Switchable: true, Usage: []contracts.UsageWindow{{Label: "Session", UsedPercent: 20}, {Label: "Week", UsedPercent: 40}, {Label: "Month", UsedPercent: 90}}},
		{ID: "b", TargetID: "local", ProviderID: "codex", DisplayName: "Second account", Switchable: true},
	}
	if err := f.SetScene(appearance, scene); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

func (f *FooterUsageFixture) Close() { f.app.Close() }

// UseNativeWindow reads display metrics from the actual fixture window while
// keeping deterministic fixture state separate from App.Attach subscriptions.
// Call it on the UI thread before showing the fixture window.
func (f *FooterUsageFixture) UseNativeWindow(window *mygo.Window) {
	f.app.win = window
}

func (f *FooterUsageFixture) SetScene(appearance, scene string) error {
	if appearance != "dark" && appearance != "light" {
		return fmt.Errorf("invalid footer appearance %q", appearance)
	}
	usage := contracts.ContextUsage{Tokens: footerFixtureCount(42000), Window: footerFixtureCount(200000)}
	base := strings.TrimSuffix(strings.TrimSuffix(scene, "-context"), "-accounts")
	switch base {
	case "known":
	case "unknown":
		if strings.HasSuffix(scene, "-context") {
			return fmt.Errorf("unknown capacity has no context chip")
		}
		usage = contracts.ContextUsage{}
	case "capacity":
		usage.Tokens = nil
	case "zero":
		usage.Tokens = footerFixtureCount(0)
	case "over":
		usage.Tokens = footerFixtureCount(240000)
	case "warning":
		usage.Tokens = footerFixtureCount(150000)
	case "danger":
		usage.Tokens = footerFixtureCount(180000)
	default:
		return fmt.Errorf("invalid footer scene %q", scene)
	}
	f.appearance, f.scene = appearance, scene
	f.app.state.ContextUsage[f.app.sessionID] = usage
	f.app.footerUsage = composerUsageState{session: f.app.sessionID, target: "local", provider: "codex", loaded: true, accountsOpen: strings.HasSuffix(scene, "-accounts"), contextOpen: strings.HasSuffix(scene, "-context")}
	f.settlePasses = 0
	return nil
}

func footerFixtureCount(value uint64) *uint64 { return &value }

func (f *FooterUsageFixture) State() FooterUsageFixtureState {
	state := f.rendered
	state.Context = state.Context.Clone()
	state.Accounts = append([]contracts.ProviderAccount(nil), state.Accounts...)
	for i := range state.Accounts {
		state.Accounts[i].Usage = append([]contracts.UsageWindow(nil), state.Accounts[i].Usage...)
	}
	return state
}

func (f *FooterUsageFixture) View(c *ui.Context) {
	a := f.app
	width, height := c.Size()
	row := compactPickerFixtureBounds(f.row.Bounds(c))
	stable := row.Width > 0 && row == f.previous.Row && f.previous.Appearance == f.appearance && f.previous.Scene == f.scene
	a.palette = design.Palette("zeron-"+f.appearance, f.appearance == "dark")
	c.SetTheme(a.palette.UI())
	c.SetMotionPreferences(&ui.MotionPreferences{Motion: ui.MotionReduced})
	c.DisableAnimations()
	c.Root().Background(a.palette.Shell).Font("Geist").FontSize(13).TextColor(a.palette.Text)
	design.ApplyReferenceTypography(c)
	ui.Column(c).Fill().Padding(24).Justify(ui.End).Children(func() {
		ui.Row(c).Bind(&f.row).FillWidth().Height(24).AlignItems(ui.Center).Justify(ui.End).Children(func() { a.composerUsage(c, a.currentSession(), &a.footerUsage, false) })
	})
	f.rendered = FooterUsageFixtureState{
		Appearance: f.appearance, Scene: f.scene, Width: width, Height: height,
		Context: a.state.ContextUsage[a.sessionID].Clone(), Accounts: a.state.Integrations.Accounts,
		Row: row, AccountsOpen: a.footerUsage.accountsOpen, ContextOpen: a.footerUsage.contextOpen,
		GeometryStable: stable,
		NativeWindow:   a.win != nil,
	}
	if f.settlePasses < 8 {
		f.settlePasses++
		c.After(time.Millisecond)
	} else if !stable {
		f.rendered.LayoutError = "footer fixture geometry did not stabilize within 8 extra frames"
	}
	f.previous = f.rendered
}
