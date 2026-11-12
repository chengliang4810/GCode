package desktop

import (
	"fmt"
	"strings"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/i18n"
	"github.com/egoist/mygo/ui"
)

type modelMetrics = contracts.ModelMetrics

type conversationMetrics struct {
	totals             contracts.ModelRequestTotals
	models             []modelMetrics
	turns              int
	generationDuration time.Duration
	wallDuration       time.Duration
	wallKnown          bool
	toolDuration       time.Duration
	running            bool
}

// The core owns and persists all aggregates. Rendering reads a typed projection;
// only an active wall clock depends on the current time.
func (a *App) metricsFor(sessionID, turnID string) conversationMetrics {
	saved := a.state.Metrics[sessionID].Total
	if turnID != "" {
		saved = a.state.Metrics[sessionID].Turns[turnID]
	}
	now := time.Time{}
	if !saved.RunningSince.IsZero() {
		now = a.metricsNow()
	}
	elapsed, known := saved.Elapsed(now)
	return conversationMetrics{
		totals: saved.Totals, models: saved.Models, turns: saved.Turns,
		generationDuration: saved.GenerationDuration, wallDuration: elapsed,
		wallKnown: known, toolDuration: saved.ToolDuration, running: !saved.RunningSince.IsZero(),
	}
}

func (a *App) metricsNow() time.Time {
	if a.now != nil {
		return a.now()
	}
	return time.Now()
}

func metricsAvailable(stats conversationMetrics) bool {
	t := stats.totals
	_, cache := t.CacheHitFraction()
	return t.Requests > t.UnknownUsage || t.Input+t.Output > 0 || stats.generationDuration > 0 || cache
}

func (a *App) metricNumber(n int64) string {
	if a.state.Settings.General.Language == "zh-CN" {
		for _, unit := range []struct {
			divisor int64
			suffix  string
		}{{100_000_000, "亿"}, {10_000, "万"}, {1_000, "千"}} {
			if n >= unit.divisor {
				return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(n)/float64(unit.divisor)), ".0") + unit.suffix
			}
		}
	}
	return compactUsage(n)
}

func (a *App) metricTokens(c *ui.Context, n int64, unknown bool) string {
	if unknown && n == 0 {
		return "—"
	}
	number := a.metricNumber(n)
	return fmt.Sprintf(tr(c, "%s Token"), number)
}

func metricCache(t contracts.ModelRequestTotals) string {
	if value, known := t.CacheHitFraction(); known {
		return fmt.Sprintf("%.1f%%", value*100)
	}
	return "—"
}

func metricSpeed(c *ui.Context, t contracts.ModelRequestTotals) string {
	if value, known := t.TokensPerSecond(); known {
		return fmt.Sprintf(tr(c, "%s Token/s"), strings.TrimSuffix(fmt.Sprintf("%.1f", value), ".0"))
	}
	return "—"
}

func metricDuration(duration time.Duration, known bool) string {
	if !known {
		return "—"
	}
	return i18n.Duration(i18n.English, duration.Milliseconds())
}

type metricPanelState struct {
	hoverPanelState
	timing bool
}

func (a *App) conversationMetricsView(c *ui.Context, sessionID string) {
	if sessionID != "" {
		stats := a.metricsFor(sessionID, "")
		if metricsAvailable(stats) {
			a.metricsGroup(c, "conversation-metrics:"+sessionID, stats, false)
		}
	}
}

func (a *App) replyMetricsView(c *ui.Context, message contracts.Message) bool {
	if message.TurnID == "" {
		return false
	}
	stats := a.metricsFor(message.SessionID, message.TurnID)
	if stats.totals.Requests == 0 {
		return false
	}
	return a.metricsGroup(c, "reply-metrics:"+message.ID, stats, true)
}

func (a *App) metricsGroup(c *ui.Context, scope string, stats conversationMetrics, reply bool) bool {
	if stats.running {
		c.After(time.Second)
	}
	group := ui.Row(c.Key(scope)).MinWidth(0).Gap(design.Metrics().Gap).AlignItems(ui.Center).
		Label(tr(c, "Conversation statistics"))
	state := ui.Local(group, "panel", func() metricPanelState { return metricPanelState{} })
	t := stats.totals
	tokens := a.metricTokens(c, t.Input+t.Output, t.UnknownUsage > 0 || t.Unrecorded)
	counts := fmt.Sprintf(tr(c, "%d rounds %d steps"), stats.turns, t.Requests)
	duration := metricDuration(stats.wallDuration, stats.wallKnown)
	timeReading := counts + " · " + tr(c, "Time taken") + " " + duration
	usageReading := tokens
	if _, known := t.CacheHitFraction(); known {
		usageReading += " · " + fmt.Sprintf(tr(c, "Cache hit %s"), metricCache(t))
	}
	hovered := false
	group.Children(func() {
		button := func(timing bool) {
			reading, label, glyph, kind := usageReading, tr(c, "Conversation usage"), "metrics-database", "usage"
			if reply {
				label = tr(c, "Turn usage")
				reading = tr(c, "Usage") + " " + tokens
			}
			if timing {
				reading, label, glyph, kind = timeReading, tr(c, "Time taken"), "clock-circle", "time"
				if reply {
					reading = tr(c, "Time taken") + " " + duration
				}
			}
			selected := state.open && state.timing == timing
			trigger := controlMetricButton(c.Key(scope+":"+kind), glyph, label+": "+reading, reading, selected)
			pointer := trigger.Hovered()
			if pointer {
				hovered = true
				if (!state.suppressed || state.timing != timing) && (!state.open || state.timing != timing) {
					state.open, state.timing, state.keyboard = true, timing, false
					c.Invalidate()
				}
			}
			trigger.OnClick(func() {
				state.open, state.timing, state.suppressed, state.keyboard = true, timing, false, !pointer
			})
			selected = state.open && state.timing == timing
			if selected {
				width, height := c.Size()
				width = min(design.Metrics().PanelWidth, max(0, width-16))
				panel := a.footerPopover(c, trigger, &state.open, label, width, reply, func() {
					ui.Scroll(c.Key(scope + ":" + kind + "-panel")).FillWidth().
						MaxHeight(min(design.Metrics().PanelMaxHeight, max(0, height-32))).
						Label(label).Children(func() {
						controlMetricPanel(c, label).Children(func() {
							if timing {
								a.metricsTimePanel(c, scope, stats)
							} else {
								a.metricsUsagePanel(c, scope, stats, reply)
							}
						})
					})
				})
				hovered = panel.Hovered() || hovered
				if !state.open {
					state.suppressed, state.keyboard = true, false
				}
			}
		}
		if reply {
			button(false)
			button(true)
		} else {
			button(true)
			button(false)
		}
	})
	state.finish(c, hovered)
	return state.open
}

func (a *App) metricsUsagePanel(c *ui.Context, scope string, stats conversationMetrics, reply bool) {
	t := stats.totals
	title := tr(c, "Conversation usage")
	if reply {
		title = tr(c, "Turn usage")
	}
	controlMetricHeading(c, "metrics-database", title, a.metricTokens(c, t.Input+t.Output, t.UnknownUsage > 0 || t.Unrecorded))
	field := func(key, label, value string) {
		controlMetricField(c.Key(scope+":"+key), tr(c, label), value)
	}
	if reply && len(stats.models) == 1 {
		field("model", "Provider / model", stats.models[0].Provider+" · "+stats.models[0].ModelID)
	}
	a.metricsUsageFields(c, scope, t)
	if reply && len(stats.models) > 1 {
		for i, entry := range stats.models {
			ui.Box(c).FillWidth().Height(design.Controls().BorderWidth).Background(c.Theme().Border)
			modelScope := fmt.Sprintf("%s:model-%d", scope, i)
			controlMetricField(c.Key(modelScope), tr(c, "Provider / model"), entry.Provider+" · "+entry.ModelID)
			a.metricsUsageFields(c, modelScope, entry.Totals)
		}
	}
}

func (a *App) metricsUsageFields(c *ui.Context, scope string, t contracts.ModelRequestTotals) {
	field := func(key, label, value string) {
		controlMetricField(c.Key(scope+":"+key), tr(c, label), value)
	}
	tokens := func(key, label string, count int64, unknown bool) {
		exact := footerCount(uint64(count))
		if unknown && count == 0 {
			exact = "—"
		}
		controlMetricField(c.Key(scope+":"+key), tr(c, label), a.metricTokens(c, count, unknown)).Tooltip(exact)
	}
	tokens("input", "Input tokens", t.Input, t.UnknownUsage > 0 || t.Unrecorded)
	tokens("cached", "Cache read", t.CacheReadTokens, t.CacheRequests == 0)
	tokens("output", "Output tokens", t.Output, t.UnknownUsage > 0 || t.Unrecorded)
	field("cache-hit", "Cache hit rate", metricCache(t))
}

func (a *App) metricsTimePanel(c *ui.Context, scope string, stats conversationMetrics) {
	t := stats.totals
	duration := metricDuration(stats.wallDuration, stats.wallKnown)
	controlMetricHeading(c, "clock-circle", tr(c, "Time taken"), duration)
	field := func(key, label, value string) {
		controlMetricField(c.Key(scope+":"+key), tr(c, label), value)
	}
	field("rounds", "Rounds / steps", fmt.Sprintf(tr(c, "%d rounds %d steps"), stats.turns, t.Requests))
	field("model-time", "Model request time", metricDuration(t.RequestDuration, t.TimedRequests > 0))
	field("tool-time", "Tool time", metricDuration(stats.toolDuration, true))
	latency := time.Duration(0)
	if t.FirstTokenRequests > 0 {
		latency = t.FirstTokenLatency / time.Duration(t.FirstTokenRequests)
	}
	field("first-token", "Average first-token wait", metricDuration(latency, t.FirstTokenRequests > 0))
	field("decode-time", "Decode time", metricDuration(stats.generationDuration, stats.generationDuration > 0))
	field("output-speed", "Output speed", metricSpeed(c, t))
}
