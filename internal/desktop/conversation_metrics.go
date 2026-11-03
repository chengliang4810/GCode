package desktop

import (
	"fmt"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

type modelMetrics struct {
	provider, model string
	totals          contracts.ModelRequestTotals
}

type conversationMetrics struct {
	totals contracts.ModelRequestTotals
	models []modelMetrics
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
	for _, request := range a.state.ModelRequests {
		if !scope[request.SessionID] || (turnID != "" && request.SessionID == sessionID && request.TurnID != turnID) {
			continue
		}
		stats.totals.Add(request)
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
	return stats
}

func metricsReading(totals contracts.ModelRequestTotals) string {
	tokens := "—"
	if totals.Requests > 0 && (totals.UnknownUsage == 0 || totals.Input+totals.Output > 0) {
		tokens = footerCount(uint64(totals.Input + totals.Output))
		if totals.UnknownUsage > 0 || totals.Unrecorded {
			tokens = "≥" + tokens
		}
	}
	speed := "—"
	if tps, known := totals.TokensPerSecond(); known {
		speed = fmt.Sprintf("%.1f", tps)
	}
	return tokens + " tokens · " + speed + " tokens/s"
}

func metricsDetails(c *ui.Context, totals contracts.ModelRequestTotals) string {
	cached := "—"
	if totals.CacheRequests > 0 {
		cached = footerCount(uint64(totals.CacheReadTokens))
	}
	details := fmt.Sprintf(tr(c, "Input: %s · Output: %s · Cached: %s"),
		footerCount(uint64(totals.Input)), footerCount(uint64(totals.Output)), cached)
	details += "\n" + tr(c, "Speed = reported output tokens / decode time; excludes first-token wait and tools.")
	details += "\n" + fmt.Sprintf(tr(c, "Speed available for %d/%d requests."), totals.DecodeRequests, totals.Requests)
	details += "\n" + tr(c, "Cache hit = cached input / input from requests reporting cache usage.")
	details += "\n" + fmt.Sprintf(tr(c, "Cache data available for %d/%d requests."), totals.CacheRequests, totals.Requests)
	if totals.UnknownUsage > 0 {
		details += "\n" + fmt.Sprintf(tr(c, "%d requests did not report complete usage; totals are a lower bound."), totals.UnknownUsage)
	}
	if totals.Unrecorded {
		details += "\n" + tr(c, "Some replies have no request records; totals are a lower bound.")
	}
	if totals.Requests == 0 {
		details = tr(c, "Usage and speed not recorded.")
	}
	return details
}

func metricsDisplay(c *ui.Context, totals contracts.ModelRequestTotals) string {
	cache := "—"
	if fraction, known := totals.CacheHitFraction(); known {
		cache = fmt.Sprintf("%.1f%%", fraction*100)
	}
	return metricsReading(totals) + " · " + fmt.Sprintf(tr(c, "%s cache"), cache)
}

func (a *App) conversationMetricsView(c *ui.Context, sessionID string) {
	if sessionID == "" {
		return
	}
	stats := a.metricsFor(sessionID, "")
	details := metricsDisplay(c, stats.totals) + "\n" + tr(c, "Conversation total") + "\n" + metricsDetails(c, stats.totals)
	ui.Text(c.Key("conversation-metrics:"+sessionID), metricsDisplay(c, stats.totals)).
		FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted).
		SingleLine().Label(details).Tooltip(details)
}

func (a *App) replyMetricsView(c *ui.Context, message contracts.Message) {
	stats := a.metricsFor(message.SessionID, message.TurnID)
	// Empty TurnID is not a request association (for example, a host notice).
	if message.TurnID == "" {
		stats = conversationMetrics{}
	}
	names := make([]string, 0, len(stats.models))
	details := []string{metricsDetails(c, stats.totals)}
	for _, entry := range stats.models {
		name := entry.provider + " / " + entry.model
		names = append(names, name)
		details = append(details, name+"\n"+metricsDisplay(c, entry.totals)+"\n"+metricsDetails(c, entry.totals))
	}
	reading := metricsDisplay(c, stats.totals)
	if len(names) > 0 {
		reading += " · " + strings.Join(names, " · ")
	}
	full := reading + "\n" + strings.Join(details, "\n\n")
	ui.Text(c.Key("reply-metrics:"+message.ID), reading).
		FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Faint).
		SingleLine().MinWidth(0).Label(full).Tooltip(full)
}
