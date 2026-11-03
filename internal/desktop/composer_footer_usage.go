package desktop

import (
	"fmt"
	"math"
	"runtime"
	"strconv"
	"strings"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/i18n"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	footerUsageForceInterval = 30 * time.Second
	footerUsagePollInterval  = 5 * time.Minute
)

type composerUsageState struct {
	session, target, provider string
	accountsOpen, contextOpen bool
	loaded                    bool
	lastForced, nextPoll      time.Time
	errorText                 string
	sequence                  uint64
	refreshes                 map[uint64]bool
	switches                  map[uint64]footerAccountSwitch
	switchVersions            map[string]uint64
}

type footerAccountSwitch struct {
	target, provider, loginProvider string
	selected                        string
	previous                        map[string]bool
}

func (s footerAccountSwitch) group() string {
	return s.target + "\x00" + s.provider + "\x00" + s.loginProvider
}

// Frozen proto/providers.rs signs_in && reports_usage; the local catalog
// retains its original "openai" identifier for Codex.
func footerUsageProvider(id string) bool {
	switch id {
	case "claude", "openai", "codex", "cursor", "grok", "devin", "opencode", "pi", "hermes", "muse":
		return true
	}
	return false
}

func footerAccountFraction(accounts []contracts.ProviderAccount, target, provider string) (float64, bool) {
	if !footerUsageProvider(provider) {
		return 0, false
	}
	for _, account := range accounts {
		if account.TargetID != target || account.ProviderID != provider || !account.InUse {
			continue
		}
		if len(account.Usage) == 0 {
			return 0, false
		}
		fraction := float64(0)
		for _, window := range account.Usage {
			fraction = max(fraction, max(0, min(1, window.UsedPercent/100)))
		}
		return fraction, true
	}
	return 0, false
}

func footerPercent(fraction float64) string { return fmt.Sprintf("%.0f%%", fraction*100) }

// Agent usage uses Rust f32::round; context labels use formatted precision.
func footerAccountPercent(fraction float64) string {
	return fmt.Sprintf("%.0f%%", math.Round(fraction*100))
}

func footerCount(n uint64) string {
	s := strconv.FormatUint(n, 10)
	for at := len(s) - 3; at > 0; at -= 3 {
		s = s[:at] + "," + s[at:]
	}
	return s
}

func footerContextDetails(usage contracts.ContextUsage, language ...string) string {
	lang := "en"
	if len(language) > 0 {
		lang = language[0]
	}
	if usage.HasWindow() {
		if usage.Tokens != nil {
			remaining := uint64(0)
			if *usage.Window > *usage.Tokens {
				remaining = *usage.Window - *usage.Tokens
			}
			return i18n.In(lang, "%s / %s tokens\n%s tokens remaining", footerCount(*usage.Tokens), footerCount(*usage.Window), footerCount(remaining))
		}
		return i18n.In(lang, "%s token capacity\nWaiting for context usage", footerCount(*usage.Window))
	}
	if usage.Tokens != nil {
		return i18n.In(lang, "%s tokens used\nContext limit not reported", footerCount(*usage.Tokens))
	}
	return i18n.In(lang, "Context usage not reported by this harness yet")
}

func footerUsageColor(p design.Tokens, fraction float64, account bool) ui.Color {
	warn, danger := .75, .90
	if account {
		warn, danger = .80, .95
	}
	if fraction >= danger {
		return p.Danger
	}
	if fraction >= warn {
		return p.Warning
	}
	if account {
		return p.Accent
	}
	return p.Muted
}

func (a *App) trackFooterUsage(c *ui.Context, state *composerUsageState, session contracts.Session, provider string, side bool) {
	target := session.DeviceID
	if target == "" {
		target = a.deviceID
	}
	if target == "" {
		target = "local"
	}
	if side {
		provider = ""
	}
	if state.session != session.ID || state.target != target || state.provider != provider {
		state.accountsOpen, state.contextOpen = false, false
		state.errorText = ""
	}
	if state.target != target {
		state.loaded = false
		state.lastForced = time.Time{}
	}
	state.session, state.target, state.provider = session.ID, target, provider
	if side || !footerUsageProvider(provider) {
		return
	}
	now := c.Now()
	if state.nextPoll.IsZero() {
		state.nextPoll = now.Add(footerUsagePollInterval)
	}
	if !state.loaded {
		state.loaded = true
		a.refreshFooterUsage(state, now)
	}
	if !now.Before(state.nextPoll) {
		state.nextPoll = now.Add(footerUsagePollInterval)
		a.refreshFooterUsage(state, now)
	}
	c.After(max(time.Millisecond, state.nextPoll.Sub(now)))
}

func (a *App) refreshFooterUsage(state *composerUsageState, now time.Time) {
	if !state.lastForced.IsZero() && now.Sub(state.lastForced) < footerUsageForceInterval {
		return
	}
	state.lastForced = now
	state.sequence++
	if state.refreshes == nil {
		state.refreshes = make(map[uint64]bool)
	}
	state.refreshes[state.sequence] = true
	a.dispatch(contracts.Command{Kind: "provider.accounts.refresh", Version: state.sequence, Integration: &contracts.IntegrationRequest{TargetID: state.target, Value: "force"}})
}

func (a *App) footerUsageCommandFinished(command contracts.Command, err error) bool {
	state := &a.footerUsage
	if command.Integration == nil || command.Version == 0 {
		return false
	}
	if command.Kind == "provider.accounts.refresh" && state.refreshes[command.Version] {
		delete(state.refreshes, command.Version)
		// Source account_usage::load retains its previous cache on list errors.
		return true
	}
	pending, ok := state.switches[command.Version]
	if command.Kind != "provider.account.switch" || !ok {
		return false
	}
	delete(state.switches, command.Version)
	nextVersion := uint64(0)
	for version, later := range state.switches {
		if version > command.Version && later.group() == pending.group() && (nextVersion == 0 || version < nextVersion) {
			nextVersion = version
		}
	}
	if nextVersion != 0 && err != nil {
		// The next choice was made against this optimistic choice. If this
		// request failed, its rollback baseline must skip the failed choice.
		next := state.switches[nextVersion]
		next.previous = pending.previous
		state.switches[nextVersion] = next
	}
	if state.switchVersions[pending.group()] > command.Version {
		return true
	}
	if err != nil {
		for i := range a.state.Integrations.Accounts {
			account := &a.state.Integrations.Accounts[i]
			if account.TargetID == pending.target && account.ProviderID == pending.provider && account.LoginProvider == pending.loginProvider {
				if active, exists := pending.previous[account.ID]; exists {
					account.InUse = active
				}
			}
		}
		if state.target == pending.target && state.provider == pending.provider {
			state.errorText = err.Error()
		}
	}
	return true
}

func (a *App) switchFooterAccount(state *composerUsageState, selected contracts.ProviderAccount) {
	if selected.InUse || !selected.Switchable {
		return
	}
	previous := make(map[string]bool)
	for i := range a.state.Integrations.Accounts {
		account := &a.state.Integrations.Accounts[i]
		if account.TargetID == selected.TargetID && account.ProviderID == selected.ProviderID && account.LoginProvider == selected.LoginProvider {
			previous[account.ID] = account.InUse
			account.InUse = account.ID == selected.ID
		}
	}
	state.sequence++
	if state.switches == nil {
		state.switches = make(map[uint64]footerAccountSwitch)
	}
	state.switches[state.sequence] = footerAccountSwitch{target: selected.TargetID, provider: selected.ProviderID, loginProvider: selected.LoginProvider, selected: selected.ID, previous: previous}
	if state.switchVersions == nil {
		state.switchVersions = make(map[string]uint64)
	}
	state.switchVersions[state.switches[state.sequence].group()] = state.sequence
	state.errorText = ""
	a.dispatch(contracts.Command{Kind: "provider.account.switch", Version: state.sequence, Integration: &contracts.IntegrationRequest{TargetID: selected.TargetID, ProviderID: selected.ProviderID, AccountID: selected.ID}})
}

// Preserve a visible optimistic choice when an unrelated snapshot arrives
// before its command response. The cache remains the shared integrations DTO.
func (a *App) projectFooterAccountSwitches(snapshot *contracts.Snapshot) {
	latest := make(map[string]uint64)
	for version, pending := range a.footerUsage.switches {
		key := pending.group()
		latest[key] = max(latest[key], version)
	}
	for _, version := range latest {
		pending := a.footerUsage.switches[version]
		for i := range snapshot.Integrations.Accounts {
			account := &snapshot.Integrations.Accounts[i]
			if account.TargetID == pending.target && account.ProviderID == pending.provider && account.LoginProvider == pending.loginProvider {
				account.InUse = account.ID == pending.selected
			}
		}
	}
}

func (a *App) composerUsage(c *ui.Context, session contracts.Session, state *composerUsageState, side bool) {
	provider := a.selectedModel().ProviderID
	a.trackFooterUsage(c, state, session, provider, side)
	now := c.Now()
	c.Key("composer-usage:" + session.ID)
	ui.Row(c).Shrink(0).Gap(4).AlignItems(ui.Center).Children(func() {
		if fraction, exists := footerAccountFraction(a.state.Integrations.Accounts, state.target, state.provider); exists {
			color := footerUsageColor(a.palette, fraction, true)
			textColor := color
			if fraction < .8 {
				textColor = a.palette.Muted
			}
			trigger := a.footerRing(c, "Account usage", footerAccountPercent(fraction), fraction, color, textColor, state.accountsOpen)
			trigger.OnClick(func() {
				state.accountsOpen = !state.accountsOpen
				state.contextOpen = false
				if state.accountsOpen {
					a.refreshFooterUsage(state, now)
				}
			})
			a.footerPopover(c, trigger, &state.accountsOpen, "Account usage menu", 400, func() { a.footerAccountsCard(c, state) })
		} else {
			state.accountsOpen = false
		}
		usage := a.state.ContextUsage[session.ID]
		if !usage.HasWindow() {
			state.contextOpen = false
			return
		}
		fraction, known := usage.Fraction()
		label, color := "—", a.palette.Faint
		if known {
			label, color = footerPercent(fraction), footerUsageColor(a.palette, fraction, false)
		}
		contextLabel := tr(c, "Context usage")
		if side {
			contextLabel += tr(c, " in ") + session.Title
		}
		trigger := a.footerRing(c, contextLabel, label, fraction, color, color, state.contextOpen)
		trigger.OnClick(func() { state.contextOpen = !state.contextOpen; state.accountsOpen = false })
		a.footerPopover(c, trigger, &state.contextOpen, "Context window menu", 360, func() {
			a.footerContextCard(c, usage)
		})
	})
}

// Context counts follow the active language; unknown usage remains unknown.
func footerContextSummary(usage contracts.ContextUsage, language string) string {
	count := func(n uint64) string {
		if language == "zh-CN" && n >= 10000 {
			return strings.TrimSuffix(strconv.FormatFloat(float64(n)/10000, 'f', 1, 64), ".0") + "万"
		}
		return footerCount(n)
	}
	if !usage.HasWindow() {
		return "—"
	}
	if usage.Tokens == nil {
		return "—/" + count(*usage.Window)
	}
	fraction, _ := usage.Fraction()
	return fmt.Sprintf("%s/%s (%.1f%%)", count(*usage.Tokens), count(*usage.Window), fraction*100)
}

func (a *App) footerContextCard(c *ui.Context, usage contracts.ContextUsage) {
	fraction, known := usage.Fraction()
	ui.Column(c).FillWidth().Padding(8).Gap(16).Children(func() {
		ui.Row(c).FillWidth().Height(24).AlignItems(ui.Center).Gap(12).Children(func() {
			ui.Text(c, tr(c, "Context window")).FontSize(design.TextSize(c, design.TextBase)).FontWeight(500).TextColor(a.palette.Text)
			ui.Box(c).Grow(1)
			ui.Text(c, footerContextSummary(usage, a.state.Settings.General.Language)).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted).SingleLine()
		})
		ui.Box(c).Key("context-capacity-progress").Label("context-capacity-progress").FillWidth().Height(8).Radius(4).Background(a.palette.Faint.Alpha(.12)).Clip().Tooltip(footerContextDetails(usage, a.state.Settings.General.Language)).Draw(func(p *ui.Painter, bounds ui.Rect) {
			if !known || fraction <= 0 {
				return
			}
			bounds.W *= float32(min(1, max(0, fraction)))
			p.FillGradient(bounds, ui.LinearGradient{Angle: 90, Stops: []ui.GradientStop{{At: 0, Color: a.palette.Accent}, {At: .72, Color: a.palette.Accent}, {At: 1, Color: a.palette.Accent.Alpha(.6)}}}, 4)
		})
	})
}

func (a *App) footerRing(c *ui.Context, label, value string, fraction float64, arc, text ui.Color, open bool) ui.Element {
	button := ui.ButtonBase(c).Height(24).Padding(0, 6).Radius(6).Gap(5).AlignItems(ui.Center).Label(tr(c, label)).Tooltip(tr(c, label) + ": " + tr(c, value)).TextColor(text)
	if open || button.Hovered() {
		ink := ui.Hex("#000000")
		if a.palette.Dark {
			ink = ui.Hex("#ffffff")
		}
		button.Background(ink.Alpha(.05))
	}
	track := a.palette.Faint.Alpha(.25)
	button.Children(func() {
		ui.Box(c).Size(16, 16).Draw(func(p *ui.Painter, bounds ui.Rect) {
			paintFooterArc(p, bounds, 1, track)
			paintFooterArc(p, bounds, max(0, min(1, fraction)), arc)
		})
		ui.Text(c, value).Width(footerTextWidth(c, value, design.TextSize(c, design.TextSmall), 400)).FontSize(design.TextSize(c, design.TextSmall)).SingleLine()
	})
	return button
}

func paintFooterArc(p *ui.Painter, bounds ui.Rect, fraction float64, color ui.Color) {
	if fraction <= 0 {
		return
	}
	steps := max(2, int(math.Ceil(64*fraction)))
	path := ui.Path{}
	for i := 0; i <= steps; i++ {
		angle := -math.Pi/2 + 2*math.Pi*fraction*float64(i)/float64(steps)
		x, y := bounds.X+bounds.W/2+6*float32(math.Cos(angle)), bounds.Y+bounds.H/2+6*float32(math.Sin(angle))
		if i == 0 {
			path.MoveTo(x, y)
		} else {
			path.LineTo(x, y)
		}
	}
	p.StrokePath(&path, 1.8, color)
}

func (a *App) footerPopover(c *ui.Context, anchor ui.Element, open *bool, label string, width float32, body func()) {
	ui.PopoverBase(c, anchor, open, func(panel ui.Element) {
		panel.Label(tr(c, label)).AttachTo(anchor, ui.AnchorTopEnd, ui.AnchorBottomEnd).SnapToWindow(8).Margin(0, 0, 6, 0).Padding(4).Gap(2).Radius(12)
		if !a.frosted(c) {
			panel.Shadow(0, 10, 30, -3, ui.RGBA(0, 0, 0, .1)).Shadow(0, 4, 12, -4, ui.RGBA(0, 0, 0, .1))
		}
		a.floatingSurface(c, panel, 12)
		if width > 0 {
			panel.Width(width)
		}
		panel.Children(body)
	})
}

func (a *App) footerHeading(c *ui.Context, label string) {
	size := design.TextSize(c, design.TextXS)
	ui.Text(c, trackedHeading(tr(c, label))).Padding(6, 8, 4, 8).FontSize(size).FixedLineHeight(a.footerTextHeight(size)).TextColor(a.palette.Muted).FontWeight(500)
}

// GPUI's inherited phi leading is snapped to physical pixels before the
// column is sized. Unattached fixtures use their explicitly validated 1x grid.
func (a *App) footerTextHeight(size float32) float32 {
	scale := float32(1)
	if a.win != nil {
		scale = max(1, float32(mygo.Screen.DisplayMatching(a.win.Bounds()).ScaleFactor))
	}
	return float32(math.Ceil(float64(size*1.618034*scale)-.5)) / scale
}

func trackedHeading(label string) string {
	return strings.Join(strings.Split(strings.ToUpper(label), ""), "\u200a")
}

func footerTextWidth(c *ui.Context, text string, size float32, weight int) float32 {
	features := ""
	if runtime.GOOS == "windows" {
		features = "kern=0"
	}
	width := float32(0)
	for _, line := range strings.Split(text, "\n") {
		advance, _ := c.MeasureText(0, ui.Span{Text: line, Font: c.Theme().Font, Size: size, Weight: weight, Features: features})
		width = max(width, float32(math.Ceil(float64(advance))))
	}
	return width
}

func (a *App) footerAccountsCard(c *ui.Context, state *composerUsageState) {
	name := "Agent"
	for _, provider := range a.state.Providers {
		if provider.ID == state.provider {
			name = provider.Name
			break
		}
	}
	a.footerHeading(c, name+tr(c, " accounts"))
	for _, account := range a.state.Integrations.Accounts {
		if account.TargetID != state.target || account.ProviderID != state.provider {
			continue
		}
		account := account
		c.Key("footer-account:" + account.TargetID + ":" + account.ID)
		row := ui.ButtonBase(c).FillWidth().Padding(6, 8).Radius(7).Gap(10).AlignItems(ui.Center).Justify(ui.Start).TextColor(a.palette.Text.Alpha(.9)).Label(tr(c, "Use account: ") + footerAccountName(account))
		if account.InUse || row.Hovered() {
			row.Background(modelSelectionColor(a.centralControls().modelPopupPalette(c))).TextColor(a.palette.Text)
		}
		if !account.InUse && account.Switchable {
			row.OnClick(func() { a.switchFooterAccount(state, account) })
		}
		row.Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
				ui.Text(c, footerAccountName(account)).FontSize(design.TextSize(c, design.TextBase)).LineHeight(1.5).FontWeight(500).SingleLine()
				if account.Plan != "" || account.InUse || len(account.Usage) == 0 && account.UsageError != "" {
					ui.Row(c).Wrap().GapX(8).GapY(2).FontSize(design.TextSize(c, design.TextBase)).LineHeight(1.5).Children(func() {
						if account.Plan != "" {
							ui.Text(c, account.Plan).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted)
						}
						if account.InUse {
							if account.Plan != "" {
								ui.Text(c, "·").TextColor(a.palette.Muted.Alpha(.3))
							}
							ui.Text(c, tr(c, "In use")).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Accent)
						} else if len(account.Usage) == 0 && account.UsageError != "" {
							if account.Plan != "" {
								ui.Text(c, "·").TextColor(a.palette.Muted.Alpha(.3))
							}
							ui.Text(c, account.UsageError).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted)
						}
					})
				}
			})
			ui.Column(c).Shrink(0).Gap(2).Tooltip(tr(c, footerAccountResets(account.Usage, a.now(), a.state.Settings.General.Language))).Children(func() {
				for i, window := range account.Usage {
					if i == 2 {
						break
					}
					a.footerUsageMeter(c, window)
				}
			})
		})
	}
	if state.errorText != "" {
		ui.Text(c, state.errorText).Padding(4, 8).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Danger)
	}
}

func footerAccountName(account contracts.ProviderAccount) string {
	if account.Email != "" {
		return account.Email
	}
	if account.DisplayName != "" {
		return account.DisplayName
	}
	return "Unknown account"
}

func footerAccountResets(windows []contracts.UsageWindow, now time.Time, language ...string) string {
	lang := "en"
	if len(language) > 0 {
		lang = language[0]
	}
	parts := make([]string, 0, len(windows))
	for _, window := range windows {
		label := i18n.In(lang, window.Label)
		if !window.ResetsAt.IsZero() {
			format := "Jan 2"
			hours := int64(window.ResetsAt.Sub(now) / time.Hour)
			if hours < 22 {
				format = "3:04 PM"
			} else if hours < 24*7 {
				format = "Mon"
			}
			label += i18n.In(lang, ": resets ") + i18n.Date(lang, window.ResetsAt.Local(), format)
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, " · ")
}

func (a *App) footerUsageMeter(c *ui.Context, window contracts.UsageWindow) {
	fraction := max(0, min(1, window.UsedPercent/100))
	color, alpha := footerUsageColor(a.palette, fraction, true), float32(.8)
	if fraction >= .8 {
		alpha = .9
	}
	textColor := a.palette.Muted
	if fraction >= .8 {
		textColor = color
	}
	wash := ui.Hex("#000000")
	if a.palette.Dark {
		wash = ui.Hex("#ffffff")
	}
	ui.Row(c).Height(16).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, window.Label).Width(52).Shrink(0).SingleLine().FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted)
		ui.Box(c).Width(88).Height(4).Shrink(0).Radius(2).Background(wash.Alpha(.08)).Clip().Children(func() {
			if fraction > 0 {
				ui.Box(c).Width(88 * float32(max(.015, fraction))).Height(4).Radius(2).Background(color.Alpha(alpha))
			}
		})
		ui.Text(c, footerAccountPercent(fraction)).Width(34).Shrink(0).TextAlign(ui.End).FontSize(design.TextSize(c, design.TextBase)).TextColor(textColor)
	})
}
