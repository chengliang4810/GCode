package desktop

import (
	"fmt"
	"math"
	"strings"

	"gcode/internal/appusage"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

const usageChartModelLimit = 6

func (a *App) usageChartCard(c *ui.Context, title string, draw func()) {
	ui.Column(c).FillWidth().Padding(16).Gap(16).Radius(12).Background(a.palette.Card).Children(func() {
		ui.Text(c, tr(c, title)).FontWeight(500)
		draw()
	})
}

func (a *App) usageModelName(id string) string {
	if id == "" {
		return a.t("Unknown model")
	}
	for _, model := range a.state.Models {
		if model.ID == id {
			return a.runtimeModelLabel(model)
		}
	}
	// Recorded usage outlives model configuration. Resolve its provider while
	// retaining the complete request model ID, including any embedded slashes.
	if providerID, modelID, ok := strings.Cut(id, "/"); ok {
		for _, provider := range a.state.Providers {
			if provider.ID == providerID {
				return provider.Name + "/" + modelID
			}
		}
	}
	return id
}

func (a *App) usageChartColor(index int) ui.Color {
	// Theme semantic colors keep charts legible in both modes and follow the
	// user's accent. The same index is used by trend lines and model shares.
	colors := []ui.Color{a.palette.Accent, a.palette.Success, a.palette.Warning, a.palette.Danger, a.palette.Muted, a.palette.AccentStrong}
	return colors[index%len(colors)]
}

func (a *App) appUsageHeatmap(c *ui.Context, days []appusage.Day) {
	state := &a.appUsage
	ui.Column(c).FillWidth().Padding(16).Gap(14).Radius(12).Background(a.palette.Card).Children(func() {
		ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, tr(c, "Token activity")).Grow(1).FontWeight(500)
			ui.Row(c).Gap(4).Children(func() {
				for _, option := range []struct{ key, label string }{{"daily", "Daily"}, {"weekly", "Weekly"}, {"cumulative", "Cumulative"}} {
					selected := state.heatMode == option.key || state.heatMode == "" && option.key == "daily"
					controlNavButton(c.Key("usage-heatmap-"+option.key), tr(c, option.label), selected).
						Children(func() { ui.Text(c, tr(c, option.label)) }).
						OnClick(func() { state.heatMode = option.key })
				}
			})
		})
		var values [52][7]int64
		var peak, cumulative int64
		for week := range 52 {
			var total int64
			for row := range 7 {
				total += days[week*7+row].Tokens
			}
			cumulative += total
			for row := range 7 {
				value := days[week*7+row].Tokens
				switch state.heatMode {
				case "weekly":
					value = total
				case "cumulative":
					value = cumulative
				}
				values[week][row] = value
				peak = max(peak, value)
			}
		}
		ui.Row(c.Key("usage-heatmap")).FillWidth().Gap(3).Children(func() {
			for week := range 52 {
				ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
					for row := range 7 {
						day := days[week*7+row]
						value := values[week][row]
						fraction := float64(value) / float64(max(int64(1), peak))
						level := math.Ceil(fraction * 4)
						color := a.palette.Border.Alpha(.5)
						if level > 0 {
							color = a.palette.Accent.Alpha(float32(.2 + level*.2))
						}
						if state.heatMode == "weekly" || state.heatMode == "cumulative" {
							// Column height is relative to the selected mode's peak.
							if float64(7-row) > math.Ceil(fraction*7) {
								color = a.palette.Border.Alpha(.5)
							}
						}
						hint := fmt.Sprintf("%s · %s Tokens · %s",
							appusage.Date(day.Date), fmt.Sprint(value), fmt.Sprintf(tr(c, "%d turns, %d tools"), day.Turns, day.Tools))
						if state.heatMode == "weekly" || state.heatMode == "cumulative" {
							hint = fmt.Sprintf("%s – %s · %d Tokens", appusage.Date(days[week*7].Date),
								appusage.Date(days[week*7+6].Date), value)
						}
						ui.Box(c.Key(fmt.Sprintf("usage-day-%s", appusage.Date(day.Date)))).
							FillWidth().Height(10).Radius(2).Background(color).Label(hint).Tooltip(hint)
					}
				})
			}
		})
		ui.Row(c).FillWidth().Justify(ui.SpaceBetween).Children(func() {
			ui.Box(c.Key("usage-months")).Grow(1).Height(18).Draw(func(p *ui.Painter, bounds ui.Rect) {
				type marker struct {
					week, month int
					text        string
				}
				var markers []marker
				for week := range 52 {
					date := days[week*7].Date
					for row := range 7 {
						if days[week*7+row].Date.Day() == 1 {
							date = days[week*7+row].Date
							break
						}
					}
					if week == 0 || markers[len(markers)-1].month != int(date.Month()) {
						text := date.Format("Jan")
						if a.state.Settings.General.Language == "zh-CN" {
							text = fmt.Sprintf("%d月", date.Month())
						}
						markers = append(markers, marker{week, int(date.Month()), text})
					}
				}
				if len(markers) > 12 {
					markers = markers[len(markers)-12:]
				}
				for _, marker := range markers {
					x := min(bounds.X+bounds.W*float32(marker.week)/52, bounds.X+bounds.W-28)
					p.Text(x, bounds.Y, marker.text,
						design.TextSize(c, design.TextSmall), a.palette.Muted)
				}
			})
		})
		ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(5).Children(func() {
				ui.Text(c, tr(c, "Less")).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
				for i := range 5 {
					color := a.palette.Border.Alpha(.5)
					if i > 0 {
						color = a.palette.Accent.Alpha(.2 + float32(i)*.2)
					}
					ui.Box(c).Size(10, 10).Radius(2).Background(color)
				}
				ui.Text(c, tr(c, "More")).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
			})
		})
	})
}

// Keep charts and legends bounded while retaining every model's consumption.
// The final slice explicitly represents the remaining models.
func usageChartModels(models []appusage.Model) []appusage.Model {
	if len(models) <= usageChartModelLimit {
		return models
	}
	result := append([]appusage.Model(nil), models[:usageChartModelLimit-1]...)
	other := appusage.Model{ID: "\x00other"}
	for _, model := range models[usageChartModelLimit-1:] {
		other.Tokens += model.Tokens
	}
	return append(result, other)
}

func (a *App) usageChartModelName(id string) string {
	if id == "\x00other" {
		return a.t("Other models")
	}
	return a.usageModelName(id)
}

func (a *App) appUsageTrend(c *ui.Context, days []appusage.Day, models []appusage.Model) {
	chartModels := usageChartModels(models)
	a.usageChartCard(c, "Daily model usage", func() {
		if len(models) == 0 {
			ui.Text(c.Key("usage-empty"), tr(c, "No recorded usage in this period")).Padding(24, 0).TextColor(a.palette.Muted)
			return
		}
		series := make([][]int64, len(chartModels))
		var peak int64
		for modelIndex, model := range chartModels {
			series[modelIndex] = make([]int64, len(days))
			for i, day := range days {
				value := day.Models[model.ID]
				if model.ID == "\x00other" {
					value = day.Tokens
					for _, top := range chartModels[:usageChartModelLimit-1] {
						value -= day.Models[top.ID]
					}
				}
				series[modelIndex][i] = value
				peak = max(peak, value)
			}
		}
		muted, border := a.palette.Muted, a.palette.Border
		fontSize := design.TextSize(c, design.TextSmall)
		ui.Box(c.Key("usage-trend")).FillWidth().Height(200).Label(tr(c, "Daily model usage")).
			Draw(func(p *ui.Painter, bounds ui.Rect) {
				plot := ui.Rect{X: bounds.X + 48, Y: bounds.Y + 10, W: max(1, bounds.W-60), H: bounds.H - 40}
				for i := range 4 {
					y := plot.Y + plot.H*float32(i)/3
					p.Line(plot.X, y, plot.X+plot.W, y, 1, border.Alpha(.5))
					p.Text(bounds.X, y-6, compactUsage(peak*int64(3-i)/3), fontSize, muted)
				}
				for modelIndex := range chartModels {
					points := make([][2]float32, len(days))
					for i, value := range series[modelIndex] {
						points[i] = [2]float32{plot.X + plot.W*float32(i)/float32(len(days)-1),
							plot.Y + plot.H*(1-float32(value)/float32(max(int64(1), peak)))}
					}
					p.StrokePolyline(points, 2, a.usageChartColor(modelIndex))
				}
				for i := 0; i < len(days); i++ {
					if i != 0 && i != len(days)-1 && i%max(1, len(days)/5) != 0 {
						continue
					}
					x := plot.X + plot.W*float32(i)/float32(len(days)-1)
					p.Text(x-15, plot.Y+plot.H+10, days[i].Date.Format("01/02"), fontSize, muted)
				}
			}).Children(func() {
			// Transparent hit targets lie over the plotted days, so hover
			// exposes precise values without adding another visible control.
			ui.Row(c.Key("usage-daily-values")).Fill().Padding(10, 12, 30, 48).Children(func() {
				for _, day := range days {
					hint := fmt.Sprintf("%s · %d Tokens", appusage.Date(day.Date), day.Tokens)
					for _, model := range chartModels {
						value := day.Models[model.ID]
						if model.ID == "\x00other" {
							value = day.Tokens
							for _, top := range chartModels[:usageChartModelLimit-1] {
								value -= day.Models[top.ID]
							}
						}
						hint += fmt.Sprintf("\n%s · %d", a.usageChartModelName(model.ID), value)
					}
					ui.Box(c.Key("usage-value-" + appusage.Date(day.Date))).Grow(1).FillHeight().Label(hint).Tooltip(hint)
				}
			})
		})
		ui.Row(c).FillWidth().Wrap().Gap(12).Children(func() {
			for i, model := range chartModels {
				ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Size(8, 8).Radius(2).Background(a.usageChartColor(i))
					ui.Text(c, a.usageChartModelName(model.ID)).FontSize(fontSize).TextColor(a.palette.Muted)
				})
			}
		})
	})
}

func (a *App) appUsageModels(c *ui.Context, models []appusage.Model) {
	a.usageChartCard(c, "Model usage", func() {
		if len(models) == 0 {
			ui.Text(c, tr(c, "No recorded usage in this period")).Padding(24, 0).TextColor(a.palette.Muted)
			return
		}
		var total int64
		for _, model := range models {
			total += model.Tokens
		}
		chartModels := usageChartModels(models)
		ui.Row(c).FillWidth().Wrap().Gap(24).AlignItems(ui.Center).Children(func() {
			ui.Column(c.Key("usage-model-donut")).Size(190, 190).AlignSelf(ui.Center).Center().
				Label(fmt.Sprintf("%s: %d Tokens", tr(c, "Model usage"), total)).
				Draw(func(p *ui.Painter, bounds ui.Rect) {
					var start float64 = -math.Pi / 2
					for i, model := range chartModels {
						angle := 2 * math.Pi * float64(model.Tokens) / float64(total)
						gap := float64(0)
						if len(chartModels) > 1 {
							gap = min(.025, angle*.2)
						}
						steps := max(2, int(angle*50))
						var path ui.Path
						for j := 0; j <= steps; j++ {
							theta := start + gap + (angle-2*gap)*float64(j)/float64(steps)
							x, y := bounds.X+bounds.W/2+84*float32(math.Cos(theta)),
								bounds.Y+bounds.H/2+84*float32(math.Sin(theta))
							if j == 0 {
								path.MoveTo(x, y)
							} else {
								path.LineTo(x, y)
							}
						}
						for j := steps; j >= 0; j-- {
							theta := start + gap + (angle-2*gap)*float64(j)/float64(steps)
							path.LineTo(bounds.X+bounds.W/2+60*float32(math.Cos(theta)),
								bounds.Y+bounds.H/2+60*float32(math.Sin(theta)))
						}
						p.FillPath(path.Close(), a.usageChartColor(i))
						start += angle
					}
				}).Children(func() {
				ui.Text(c, compactUsage(total)).FontSize(design.TextSize(c, design.TextXL)).FontWeight(500)
				ui.Text(c, "Tokens").TextColor(a.palette.Muted).FontSize(design.TextSize(c, design.TextSmall))
			})
			ui.Column(c.Key("usage-model-list")).Grow(1).MinWidth(180).Gap(12).Children(func() {
				for i, model := range chartModels {
					name := a.usageChartModelName(model.ID)
					hint := fmt.Sprintf("%s · %d Tokens · %.1f%%", name, model.Tokens, 100*float64(model.Tokens)/float64(total))
					ui.Row(c.Key(fmt.Sprintf("usage-model-%d", i))).FillWidth().Gap(8).AlignItems(ui.Center).Label(hint).Tooltip(hint).
						Children(func() {
							ui.Box(c).Size(9, 9).Radius(2).Background(a.usageChartColor(i))
							ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
								ui.Text(c, name).SingleLine()
								ui.Text(c, compactUsage(model.Tokens)+" Tokens").FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
							})
							ui.Text(c, fmt.Sprintf("%.1f%%", 100*float64(model.Tokens)/float64(total))).TextColor(a.palette.Muted)
						})
				}
			})
		})
	})
}
