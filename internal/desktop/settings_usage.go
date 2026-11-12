package desktop

import (
	"fmt"
	"time"

	"gcode/internal/appusage"
	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/i18n"
	"github.com/egoist/mygo/ui"
)

type appUsageState struct {
	report       appusage.Report
	generated    time.Time
	errorText    string
	loading      bool
	rangeDays    int
	heatMode     string
	heatmap      []appusage.Day
	heatmapAt    time.Time
	window       []appusage.Day
	windowModels []appusage.Model
	windowAt     time.Time
	windowDays   int
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
	reader, ok := a.service.(contracts.ApplicationUsageService)
	if !ok {
		state.errorText = "Application usage is unavailable: service does not support usage statistics."
		return
	}
	state.loading = true
	state.errorText = ""
	// Snapshots are immutable projections; event application copies changed
	// slices. The worker owns only that projection, never UI contexts/elements.
	revision := a.state.Revision
	a.backgroundTask(func() func() {
		usage, err := reader.ReadApplicationUsage(a.ctx)
		return func() {
			state.loading = false
			if err != nil {
				state.errorText = err.Error()
				return
			}
			state.report, state.generated, state.errorText = appusage.FromContractReport(usage.Report), usage.GeneratedAt, ""
			state.invalidateDerivedUsage()
			if a.state.Revision != revision {
				a.refreshVisibleAppUsage()
			}
		}
	})
}

func (a *App) appUsageSettings(c *ui.Context) {
	state := &a.appUsage
	if state.errorText != "" {
		ui.Text(c, state.errorText).FillWidth().TextColor(a.palette.Danger)
		ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
			secondaryButton(c.Key("usage-refresh"), tr(c, "Refresh")).Disabled(state.loading).OnClick(a.refreshAppUsage)
		})
		return
	}
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
			minWidth := float32(110)
			if item.key == "usage-longest-session" {
				minWidth = 260
			}
			ui.Column(c).Grow(1).MinWidth(minWidth).Padding(12).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c.Key(item.key), item.value).FontSize(design.TextSize(c, design.TextXL)).FontWeight(500)
				ui.Text(c, tr(c, item.label)).TextColor(a.palette.Muted).FontSize(design.TextSize(c, design.TextSmall))
			})
		}
	})
	a.appUsageHeatmap(c, state.usageHeatmap())
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
	days, models := state.usageWindow(count)
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

func (s *appUsageState) invalidateDerivedUsage() {
	s.heatmap = nil
	s.heatmapAt = time.Time{}
	s.window = nil
	s.windowModels = nil
	s.windowAt = time.Time{}
	s.windowDays = 0
}

func (s *appUsageState) usageHeatmap() []appusage.Day {
	if s.heatmap == nil || !s.heatmapAt.Equal(s.generated) {
		s.heatmap = s.report.Heatmap(s.generated)
		s.heatmapAt = s.generated
	}
	return s.heatmap
}

func (s *appUsageState) usageWindow(count int) ([]appusage.Day, []appusage.Model) {
	if s.window == nil || s.windowDays != count || !s.windowAt.Equal(s.generated) {
		s.window = s.report.Window(s.generated, count)
		s.windowModels = appusage.Models(s.window)
		s.windowDays = count
		s.windowAt = s.generated
	}
	return s.window, s.windowModels
}

func (a *App) usageDays(days int) string {
	if days == 1 {
		return a.t("1 day")
	}
	return fmt.Sprintf(a.t("%d days"), days)
}

func (a *App) usageDuration(ms int64) string {
	return i18n.Duration(a.state.Settings.General.Language, ms)
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
