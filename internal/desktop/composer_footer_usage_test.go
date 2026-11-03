package desktop

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func footerNumber(value uint64) *uint64 { return &value }

func TestFooterAccountLimitUsesFirstActiveAndAllWindows(t *testing.T) {
	accounts := []contracts.ProviderAccount{
		{ID: "remote", TargetID: "remote", ProviderID: "claude", InUse: true, Usage: []contracts.UsageWindow{{UsedPercent: 100}}},
		{ID: "inactive", TargetID: "local", ProviderID: "claude", Usage: []contracts.UsageWindow{{UsedPercent: 99}}},
		{ID: "active", TargetID: "local", ProviderID: "claude", InUse: true, Usage: []contracts.UsageWindow{{UsedPercent: 10}, {UsedPercent: -10}, {UsedPercent: 88}}},
		{ID: "second-active", TargetID: "local", ProviderID: "claude", InUse: true, Usage: []contracts.UsageWindow{{UsedPercent: 100}}},
	}
	if fraction, ok := footerAccountFraction(accounts, "local", "claude"); !ok || fraction != .88 {
		t.Fatalf("active binding limit = %v, %v", fraction, ok)
	}
	accounts[2].Usage[2].UsedPercent = 120
	if fraction, _ := footerAccountFraction(accounts, "local", "claude"); fraction != 1 {
		t.Fatal("account ring did not clamp over-capacity windows")
	}
	accounts[2].Usage = nil
	if _, ok := footerAccountFraction(accounts, "local", "claude"); ok {
		t.Fatal("missing first active window borrowed another active account")
	}
	for _, provider := range []string{"antigravity", "mock", "unknown"} {
		accounts[0].ProviderID = provider
		if _, ok := footerAccountFraction(accounts, "remote", provider); ok {
			t.Fatalf("ineligible harness %s exposed an account ring", provider)
		}
	}
	if footerAccountPercent(.125) != "13%" || footerPercent(.125) != "12%" {
		t.Fatal("account round() and context precision formatting lost their distinct tie rules")
	}
}

func TestFooterContextDetailsKeepUnknownZeroAndOverCapacity(t *testing.T) {
	for _, tc := range []struct {
		usage contracts.ContextUsage
		want  string
	}{
		{contracts.ContextUsage{}, "Context usage not reported by this harness yet"},
		{contracts.ContextUsage{Tokens: footerNumber(0)}, "0 tokens used\nContext limit not reported"},
		{contracts.ContextUsage{Window: footerNumber(200000)}, "200,000 token capacity\nWaiting for context usage"},
		{contracts.ContextUsage{Tokens: footerNumber(0), Window: footerNumber(200000)}, "0 / 200,000 tokens\n200,000 tokens remaining"},
		{contracts.ContextUsage{Tokens: footerNumber(240000), Window: footerNumber(200000)}, "240,000 / 200,000 tokens\n0 tokens remaining"},
	} {
		if got := footerContextDetails(tc.usage); got != tc.want {
			t.Errorf("details = %q, want %q", got, tc.want)
		}
	}
	p := design.Palette("rcode-dark", true)
	for _, tc := range []struct {
		fraction float64
		account  bool
		want     ui.Color
	}{
		{.74, false, p.Muted}, {.75, false, p.Warning}, {.90, false, p.Danger},
		{.79, true, p.Accent}, {.80, true, p.Warning}, {.95, true, p.Danger},
	} {
		if footerUsageColor(p, tc.fraction, tc.account) != tc.want {
			t.Fatalf("wrong threshold %+v", tc)
		}
	}
}

func footerUsageTester(t *testing.T, usage contracts.ContextUsage) (*App, *ui.Tester) {
	t.Helper()
	a := testApp(t, "default")
	a.selectSession("chat-0")
	// This component fixture owns its account DTOs; refresh timing is tested
	// separately against the stateful backend rather than replacing these rows.
	a.service = &footerRecordingService{Service: a.service, skipRefresh: true}
	provider := a.selectedModel().ProviderID
	a.state.Integrations.Accounts = []contracts.ProviderAccount{
		{ID: "a", TargetID: "local", ProviderID: provider, InUse: true, Switchable: true, Email: "first@example.test", Plan: "Pro", Usage: []contracts.UsageWindow{{Label: "Session", UsedPercent: 20}, {Label: "Week", UsedPercent: 40}, {Label: "Month", UsedPercent: 90}}},
		{ID: "b", TargetID: "local", ProviderID: provider, Switchable: true, DisplayName: "Second account"},
	}
	a.state.ContextUsage[a.sessionID] = usage
	a.footerUsage = composerUsageState{session: a.sessionID, target: "local", provider: provider, loaded: true}
	tt := ui.NewTester(func(c *ui.Context) {
		a.palette = design.Palette("rcode-dark", true)
		c.SetTheme(a.palette.UI())
		c.Root().Font("Geist").FontSize(13).TextColor(a.palette.Text)
		design.ApplyReferenceTypography(c)
		ui.Column(c).Fill().Padding(24).Justify(ui.End).AlignItems(ui.End).Children(func() { a.composerUsage(c, a.currentSession(), &a.footerUsage, false) })
	}, 840, 560)
	tt.Advance(time.Second)
	return a, tt
}

func TestFooterUsageCardsUseSharedCacheAndOwnContext(t *testing.T) {
	a, tt := footerUsageTester(t, contracts.ContextUsage{Tokens: footerNumber(240000), Window: footerNumber(200000)})
	if !tt.HasText("90%") || !tt.HasText("120%") {
		t.Fatal("account binding window or unclamped context label missing")
	}
	if err := tt.Click("Account usage"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	panel, ok := tt.Find("Account usage menu")
	if !ok || panel.W != 400 {
		t.Fatalf("account card width: %+v, %v", panel, ok)
	}
	if !tt.HasText("Second account") || tt.HasText("Month") || tt.HasText("Settings") {
		t.Fatalf("account card content: %v", tt.Texts())
	}
	first, _ := tt.Find("Use account: first@example.test")
	second, _ := tt.Find("Use account: Second account")
	accountChip, _ := tt.Find("Account usage")
	contextChip, _ := tt.Find("Context usage")
	heading, _ := tt.Find(trackedHeading(a.state.Providers[0].Name + " accounts"))
	t.Logf("production footer boxes: account=%+v context=%+v card=%+v heading=%+v first=%+v second=%+v", accountChip, contextChip, panel, heading, first, second)
	if first.Y >= second.Y {
		t.Fatal("account rows no longer follow backend order")
	}
	if err := tt.Click("Context usage"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if _, ok := tt.Find("Account usage menu"); ok {
		t.Fatal("two footer popovers stayed open")
	}
	if !tt.HasText("240,000/200,000 (120.0%)") {
		t.Fatal("context card borrowed billing token totals")
	}
	a.state.ContextUsage[a.sessionID] = contracts.ContextUsage{Window: footerNumber(200000)}
	tt.Frame()
	if !tt.HasText("—") || !tt.HasText("—/200,000") {
		t.Fatal("capacity-only context was rendered as zero")
	}
	a.state.ContextUsage[a.sessionID] = contracts.ContextUsage{Tokens: footerNumber(0), Window: footerNumber(200000)}
	tt.Frame()
	if !tt.HasText("0%") || !tt.HasText("0/200,000 (0.0%)") {
		t.Fatal("reported zero was rendered as unknown")
	}
	a.state.ContextUsage[a.sessionID] = contracts.ContextUsage{}
	tt.Frame()
	if _, ok := tt.Find("Context usage"); ok || a.footerUsage.contextOpen {
		t.Fatal("unknown capacity retained the chip or orphaned popover")
	}
}

func TestFooterUsageIsIndependentAcrossMainAndSide(t *testing.T) {
	a, tt, child := sideViewTester(t)
	a.state.ContextUsage[a.sessionID] = contracts.ContextUsage{Tokens: footerNumber(150000), Window: footerNumber(200000)}
	a.state.ContextUsage[child.ID] = contracts.ContextUsage{Tokens: footerNumber(18000), Window: footerNumber(200000)}
	tt.Frame()
	if err := tt.Click("Context usage in " + child.Title); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if !tt.HasText("18,000/200,000 (9.0%)") || tt.HasText("150,000/200,000 (75.0%)") {
		t.Fatal("side card borrowed the main conversation's context")
	}
	if a.footerUsage.contextOpen || !a.sideComposer(child.ID).FooterUsage.contextOpen {
		t.Fatal("side popup changed main popup state")
	}
	a.state.ContextUsage[child.ID] = contracts.ContextUsage{}
	tt.Frame()
	if _, ok := tt.Find("Context usage in " + child.Title); ok {
		t.Fatal("side ring fell back to parent when its snapshot disappeared")
	}
}

func TestFooterUsagePopoversSnapOnlyAtActualWindowOverflow(t *testing.T) {
	type windowCase struct {
		name          string
		width, height int
		wantX, wantY  float32
	}
	for _, card := range []struct {
		name, trigger, panel string
		width, height        float32
		cases                []windowCase
	}{
		{"accounts", "Account usage", "Account usage menu", 400, 128, []windowCase{
			{"ordinary", 840, 560, 357, 378},
			{"three-pixel-gap-is-not-overflow", 486, 560, 3, 378},
			{"exact-left-edge-is-not-overflow", 483, 560, 0, 378},
			{"one-pixel-overflow-snaps", 482, 560, 8, 378},
			{"wide-card-visible-region-starts-at-eight", 360, 560, 8, 378},
			{"above-card-shifts-down-without-flipping", 840, 140, 357, 8},
			{"tall-card-visible-region-starts-at-eight", 840, 80, 357, 8},
		}},
		{"context", "Context usage", "Context window menu", 360, 74, []windowCase{
			{"ordinary", 840, 560, 456, 432},
			{"three-and-four-pixel-gaps-are-not-overflow", 387, 132, 3, 4},
			{"exact-window-edges-are-not-overflow", 384, 128, 0, 0},
			{"one-pixel-overflow-snaps-both-axes", 383, 127, 8, 8},
			{"oversized-card-aligns-top-left", 160, 80, 8, 8},
		}},
	} {
		t.Run(card.name, func(t *testing.T) {
			_, tt := footerUsageTester(t, contracts.ContextUsage{Tokens: footerNumber(42000), Window: footerNumber(200000)})
			if err := tt.Click(card.trigger); err != nil {
				t.Fatal(err)
			}
			for _, tc := range card.cases {
				t.Run(tc.name, func(t *testing.T) {
					tt.SetSize(tc.width, tc.height)
					tt.Advance(time.Second)
					panel, ok := tt.Find(card.panel)
					want := ui.Rect{X: tc.wantX, Y: tc.wantY, W: card.width, H: card.height}
					// Tester.Find reports the visible label intersection, unlike
					// Handle.Bounds. Oversized raw boxes are covered by the
					// framework's attachment tests; this checks the visible card.
					want.W = min(want.W, float32(tc.width)-want.X)
					want.H = min(want.H, float32(tc.height)-want.Y)
					if !ok || panel != want {
						t.Fatalf("source overflow-only placement = %+v, %v; want %+v", panel, ok, want)
					}
				})
			}
			// Reopening after the window moves must use the new anchor, not
			// the earlier card box or a retained UI Element.
			tt.Key(0, ui.KeyEscape)
			tt.SetSize(840, 560)
			if err := tt.Click(card.trigger); err != nil {
				t.Fatal(err)
			}
			tt.Advance(time.Second)
			if panel, ok := tt.Find(card.panel); !ok || panel.W != card.width || panel.H != card.height {
				t.Fatalf("reopened card lost production geometry: %+v, %v", panel, ok)
			}
		})
	}
}

type footerRecordingService struct {
	contracts.Service
	commands    []contracts.Command
	switchErr   error
	skipRefresh bool
}

func (s *footerRecordingService) Execute(ctx context.Context, command contracts.Command) error {
	s.commands = append(s.commands, command.Clone())
	if command.Kind == "provider.accounts.refresh" && s.skipRefresh {
		return nil
	}
	if command.Kind == "provider.account.switch" && s.switchErr != nil {
		return s.switchErr
	}
	return s.Service.Execute(ctx, command)
}

func TestFooterUsagePollAndForceThrottleAreTargetScoped(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	a.footerUsage = composerUsageState{}
	recorder := &footerRecordingService{Service: a.service}
	a.service = recorder
	tt := ui.NewTester(a.View, 1320, 880)
	countRefresh := func() int {
		n := 0
		for _, command := range recorder.commands {
			if command.Kind == "provider.accounts.refresh" {
				n++
				if command.Integration.TargetID != "local" || command.Integration.ProviderID != "" {
					t.Fatal("footer failed to list the whole target")
				}
			}
		}
		return n
	}
	if countRefresh() != 1 {
		t.Fatal("first eligible session did not load exactly once")
	}
	if err := tt.Click("Account usage"); err != nil {
		t.Fatal(err)
	}
	if countRefresh() != 1 {
		t.Fatal("opening immediately bypassed 30 second throttle")
	}
	tt.Advance(31 * time.Second)
	tt.ClickAt(700, 100)
	if err := tt.Click("Account usage"); err != nil {
		t.Fatal(err)
	}
	if countRefresh() != 2 {
		t.Fatal("opening after throttle did not force refresh")
	}
	a.navigate("settings/Appearance")
	tt.Frame()
	tt.Advance(5 * time.Minute)
	if countRefresh() != 3 {
		t.Fatal("live composer stopped background polling while settings was open")
	}
}

func TestFooterAccountSwitchFailureRestoresOnlyLoginGroup(t *testing.T) {
	a, tt := footerUsageTester(t, contracts.ContextUsage{})
	provider := a.selectedModel().ProviderID
	a.state.Integrations.Accounts = append(a.state.Integrations.Accounts, contracts.ProviderAccount{ID: "other", TargetID: "local", ProviderID: provider, LoginProvider: "other", InUse: true})
	recorder := &footerRecordingService{Service: a.service, switchErr: errors.New("Switch refused")}
	a.service = recorder
	if err := tt.Click("Account usage"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if err := tt.Click("Use account: Second account"); err != nil {
		t.Fatal(err)
	}
	if !a.state.Integrations.Accounts[0].InUse || a.state.Integrations.Accounts[1].InUse || !a.state.Integrations.Accounts[2].InUse {
		t.Fatal("failure lost the original login group or changed another group")
	}
	if !tt.HasText("Switch refused") || a.errorText != "" {
		t.Fatal("footer switch failure was routed to another screen")
	}
	if got := recorder.commands[len(recorder.commands)-1]; got.Kind != "provider.account.switch" || got.Integration.AccountID != "b" {
		t.Fatalf("wrong switch command %+v", got)
	}
}

func TestFooterPendingSwitchSurvivesUnrelatedSnapshotAndRejectsOldFailure(t *testing.T) {
	a, _ := footerUsageTester(t, contracts.ContextUsage{})
	a.closed = true // Hold command responses while exercising the optimistic state.
	first := a.state.Integrations.Accounts[0]
	second := a.state.Integrations.Accounts[1]
	a.switchFooterAccount(&a.footerUsage, second)
	oldVersion := a.footerUsage.sequence
	// Choose the first account again before the previous response arrives.
	first.InUse = false
	a.switchFooterAccount(&a.footerUsage, first)
	newVersion := a.footerUsage.sequence
	snapshot := a.state
	snapshot.Revision++
	snapshot.Integrations.Accounts = append([]contracts.ProviderAccount(nil), snapshot.Integrations.Accounts...)
	snapshot.Integrations.Accounts[0].InUse, snapshot.Integrations.Accounts[1].InUse = false, true
	a.apply(snapshot)
	if !a.state.Integrations.Accounts[0].InUse || a.state.Integrations.Accounts[1].InUse {
		t.Fatal("unrelated snapshot erased latest optimistic selection")
	}
	command := contracts.Command{Kind: "provider.account.switch", Version: oldVersion, Integration: &contracts.IntegrationRequest{}}
	if !a.footerUsageCommandFinished(command, errors.New("late failure")) || a.footerUsage.errorText != "" {
		t.Fatal("old failed selection overwrote the newer action")
	}
	command.Version = newVersion
	a.footerUsageCommandFinished(command, nil)
	if len(a.footerUsage.switches) != 0 {
		t.Fatal("acknowledged switches leaked pending state")
	}
}

func TestFooterResetTooltipAndAccountNameMatchSource(t *testing.T) {
	now := time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC)
	windows := []contracts.UsageWindow{
		{Label: "Session", ResetsAt: now.Add(time.Hour)}, {Label: "Week", ResetsAt: now.Add(48 * time.Hour)},
		{Label: "Month", ResetsAt: now.Add(30 * 24 * time.Hour)}, {Label: "Unknown"},
	}
	want := "Session: resets " + windows[0].ResetsAt.Local().Format("3:04 PM") + " · Week: resets " + windows[1].ResetsAt.Local().Format("Mon") + " · Month: resets " + windows[2].ResetsAt.Local().Format("Jan 2") + " · Unknown"
	if got := footerAccountResets(windows, now); got != want {
		t.Fatalf("tooltip = %q", got)
	}
	if got := footerAccountName(contracts.ProviderAccount{DisplayName: "Name"}); got != "Name" {
		t.Fatal(got)
	}
	if got := footerAccountName(contracts.ProviderAccount{Email: "Email", DisplayName: "Name"}); got != "Email" {
		t.Fatal(got)
	}
	if got := trackedHeading("Context window"); strings.ReplaceAll(got, "\u200a", "") != "CONTEXT WINDOW" {
		t.Fatal(got)
	}
}

func TestFooterRepeatedFailedSwitchRebasesRollback(t *testing.T) {
	a, _ := footerUsageTester(t, contracts.ContextUsage{})
	a.closed = true
	a.state.Integrations.Accounts = append(a.state.Integrations.Accounts, contracts.ProviderAccount{ID: "c", TargetID: "local", ProviderID: a.selectedModel().ProviderID, Switchable: true})
	a.switchFooterAccount(&a.footerUsage, a.state.Integrations.Accounts[1])
	firstVersion := a.footerUsage.sequence
	a.switchFooterAccount(&a.footerUsage, a.state.Integrations.Accounts[2])
	secondVersion := a.footerUsage.sequence
	for _, version := range []uint64{firstVersion, secondVersion} {
		a.footerUsageCommandFinished(contracts.Command{Kind: "provider.account.switch", Version: version, Integration: &contracts.IntegrationRequest{}}, errors.New("refused"))
	}
	for _, account := range a.state.Integrations.Accounts {
		if account.InUse != (account.ID == "a") {
			t.Fatalf("both requests failed, but active account is %+v", account)
		}
	}
}
