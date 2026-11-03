package desktop

import (
	"fmt"
	"time"

	"gcode/internal/appusage"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

type appUsageState struct {
	report    appusage.Report
	generated time.Time
	loading   bool
	rangeDays int
	heatMode  string
}

func (a *App) refreshVisibleAppUsage() {
	if a.route == "settings" && a.section == "Usage statistics" && !a.appUsage.loading {
		a.refreshAppUsage()
	}
}

func (a *App) refreshAppUsage() {
	state := &a.appUsage
	if state.loading {
		return
	}
	state.loading = true
	// Snapshots are immutable projections; event application copies changed
	// slices. The worker owns only that projection, never UI contexts/elements.
	messages, runs, revision, now := a.state.Messages, a.state.Runs, a.state.Revision, a.now()
	a.backgroundTask(func() func() {
		report := appusage.Build(messages, runs, now)
		return func() {
			state.report, state.generated, state.loading = report, now, false
			if a.state.Revision != revision {
				a.refreshVisibleAppUsage()
			}
		}
	})
}

func (a *App) appUsageSettings(c *ui.Context) {
	state := &a.appUsage
	if state.generated.IsZero() {
		ui.Text(c, tr(c, "Loading usage…")).TextColor(a.palette.Muted)
		return
	}
	// Refresh actions publish a complete report. Range and heatmap changes
	// operate on it locally and cannot reset the lifetime summary.
	summary := state.report.Summary
	ui.Row(c).FillWidth().Wrap().Background(a.palette.Card).Radius(12).Children(func() {
		for _, item := range []struct{ key, label, value string }{
			{"usage-total", "Total tokens", compactUsage(summary.Total)},
			{"usage-peak", "Peak daily tokens", compactUsage(summary.Peak)},
			{"usage-longest-session", "Longest session", a.usageDuration(summary.LongestSessionMS)},
			{"usage-current-streak", "Current streak", a.usageDays(summary.CurrentStreak)},
			{"usage-longest-streak", "Longest streak", a.usageDays(summary.LongestStreak)},
		} {
			ui.Column(c).Grow(1).MinWidth(110).Padding(12).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c.Key(item.key), item.value).FontSize(design.TextSize(c, design.TextXL)).FontWeight(500)
				ui.Text(c, tr(c, item.label)).TextColor(a.palette.Muted).FontSize(design.TextSize(c, design.TextSmall))
			})
		}
	})
	a.appUsageHeatmap(c, state.report.Heatmap(state.generated))
	ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(12).Children(func() {
		ui.Text(c, tr(c, "Application usage")).Grow(1).FontWeight(500)
		ui.Row(c).Gap(4).Children(func() {
			for _, days := range []int{7, 30} {
				selected := state.rangeDays == days || state.rangeDays == 0 && days == 7
				label := fmt.Sprintf(tr(c, "Last %d days"), days)
				controlNavButton(c.Key(fmt.Sprintf("usage-range-%d", days)), label, selected).
					Children(func() { ui.Text(c, label) }).OnClick(func() { state.rangeDays = days })
			}
		})
	})
	count := state.rangeDays
	if count == 0 {
		count = 7
	}
	days := state.report.Window(state.generated, count)
	models := appusage.Models(days)
	a.appUsageTrend(c, days, models)
	a.appUsageModels(c, models)
	if summary.UnknownModelTokens > 0 {
		ui.Text(c, tr(c, "Requests without a recorded model are grouped as Unknown model.")).
			FillWidth().FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
	}
	ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
		secondaryButton(c.Key("usage-refresh"), tr(c, "Refresh")).Disabled(state.loading).OnClick(a.refreshAppUsage)
	})
}

func (a *App) usageDays(days int) string {
	if days == 1 {
		return a.t("1 day")
	}
	return fmt.Sprintf(a.t("%d days"), days)
}

func (a *App) usageDuration(ms int64) string {
	minutes := max(0, ms) / 60000
	if minutes >= 1440 {
		return fmt.Sprintf(a.t("%dd %dh %dm"), minutes/1440, minutes%1440/60, minutes%60)
	}
	if minutes >= 60 {
		return fmt.Sprintf(a.t("%dh %dm"), minutes/60, minutes%60)
	}
	return fmt.Sprintf(a.t("%dm"), minutes)
}

func compactUsage(tokens int64) string {
	for _, unit := range []struct {
		divisor int64
		suffix  string
	}{{1_000_000_000, "B"}, {1_000_000, "M"}, {1_000, "K"}} {
		if tokens >= unit.divisor {
			return fmt.Sprintf("%.1f%s", float64(tokens)/float64(unit.divisor), unit.suffix)
		}
	}
	return fmt.Sprint(tokens)
}
