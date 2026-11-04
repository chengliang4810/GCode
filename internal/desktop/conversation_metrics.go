package desktop

import (
	"fmt"
	"strings"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

type modelMetrics struct {
	provider, model string
	totals          contracts.ModelRequestTotals
}

type conversationMetrics struct {
	totals             contracts.ModelRequestTotals
	models             []modelMetrics
	turns              int
	generationDuration time.Duration
	toolDuration       time.Duration
}

// A parent's accounting includes its delegated agents, but never side chats or
// forked conversations. Reply accounting follows only children spawned in it.
func (a *App) metricsFor(sessionID, turnID string) conversationMetrics {
	scope := map[string]bool{sessionID: true}
	if turnID != "" {
		for _, message := range a.state.Messages[sessionID] {
			if message.TurnID == turnID {
				for _, block := range message.Blocks {
					if block.Subagent != nil {
						scope[block.Subagent.SessionID] = true
					}
				}
			}
		}
	}
	for changed := true; changed; {
		changed = false
		for _, session := range a.state.Sessions {
			if scope[session.ID] || session.SubagentType == "" || !scope[session.ParentSessionID] {
				continue
			}
			if turnID != "" && session.ParentSessionID == sessionID {
				continue
			}
			scope[session.ID], changed = true, true
		}
	}
	stats := conversationMetrics{}
	models := make(map[string]int)
	recorded := make(map[string]bool)
	turns := make(map[string]bool)
	for _, request := range a.state.ModelRequests {
		if !scope[request.SessionID] || (turnID != "" && request.SessionID == sessionID && request.TurnID != turnID) {
			continue
		}
		stats.totals.Add(request)
		stats.generationDuration += max(0, request.DecodeDuration)
		if request.SessionID == sessionID && request.TurnID != "" {
			turns[request.TurnID] = true
		}
		recorded[request.SessionID+"\x00"+request.TurnID] = true
		key := request.ProviderID + "\x00" + request.ProviderName + "\x00" + request.ModelID
		index, exists := models[key]
		if !exists {
			index = len(stats.models)
			models[key] = index
			stats.models = append(stats.models, modelMetrics{provider: request.ProviderName, model: request.ModelID})
		}
		stats.models[index].totals.Add(request)
	}
	// Inherited fork text has no owned run. Missing records on an owned
	// model run make subsequent totals a lower bound, never a factual zero.
	for _, run := range a.state.Runs {
		if !scope[run.SessionID] || (turnID != "" && run.SessionID == sessionID && run.ID != turnID) ||
			recorded[run.SessionID+"\x00"+run.ID] {
			continue
		}
		for _, message := range a.state.Messages[run.SessionID] {
			if message.TurnID == run.ID && message.Role == "assistant" && message.ModelID != "" {
				stats.totals.Unrecorded = true
				break
			}
		}
	}
	stats.turns = len(turns)
	tools := make(map[string]contracts.ToolCall)
	for id := range scope {
		for _, message := range a.state.Messages[id] {
			if turnID != "" && id == sessionID && message.TurnID != turnID {
				continue
			}
			for _, block := range message.Blocks {
				if block.Tool != nil {
					tools[id+"\x00"+block.Tool.ID] = *block.Tool
				}
			}
		}
	}
	for _, tool := range tools {
		if tool.FinishedAt != nil && !tool.StartedAt.IsZero() {
			stats.toolDuration += max(0, tool.FinishedAt.Sub(tool.StartedAt))
		}
	}
	return stats
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
	if unknown {
		number = "≥" + number
	}
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
	if duration < time.Minute {
		return fmt.Sprintf("%.2fs", duration.Seconds())
	}
	return fmt.Sprintf("%dm %.1fs", int(duration/time.Minute), (duration % time.Minute).Seconds())
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
	group := ui.Row(c.Key(scope)).MinWidth(0).Gap(design.Metrics().Gap).AlignItems(ui.Center).
		Label(tr(c, "Conversation statistics"))
	state := ui.Local(group, "panel", func() metricPanelState { return metricPanelState{} })
	t := stats.totals
	tokens := a.metricTokens(c, t.Input+t.Output, t.UnknownUsage > 0 || t.Unrecorded)
	counts := fmt.Sprintf(tr(c, "%d rounds %d steps"), stats.turns, t.Requests)
	duration := metricDuration(stats.generationDuration, stats.generationDuration > 0)
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
		field("model", "Provider / model", stats.models[0].provider+" · "+stats.models[0].model)
	}
	a.metricsUsageFields(c, scope, t)
	if reply && len(stats.models) > 1 {
		for i, entry := range stats.models {
			ui.Box(c).FillWidth().Height(design.Controls().BorderWidth).Background(c.Theme().Border)
			modelScope := fmt.Sprintf("%s:model-%d", scope, i)
			controlMetricField(c.Key(modelScope), tr(c, "Provider / model"), entry.provider+" · "+entry.model)
			a.metricsUsageFields(c, modelScope, entry.totals)
		}
	}
	if t.UnknownUsage > 0 || t.Unrecorded {
		ui.Text(c, tr(c, "Only reported usage is counted; totals may be incomplete.")).
			FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
	}
}

func (a *App) metricsUsageFields(c *ui.Context, scope string, t contracts.ModelRequestTotals) {
	field := func(key, label, value string) {
		controlMetricField(c.Key(scope+":"+key), tr(c, label), value)
	}
	tokens := func(key, label string, count int64, unknown bool) {
		exact := footerCount(uint64(count))
		if unknown {
			if count == 0 {
				exact = "—"
			} else {
				exact = "≥" + exact
			}
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
	duration := metricDuration(stats.generationDuration, stats.generationDuration > 0)
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
	field("decode-time", "Decode time", duration)
	field("output-speed", "Output speed", metricSpeed(c, t))
}
