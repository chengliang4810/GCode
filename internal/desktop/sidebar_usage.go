package desktop

import (
	"fmt"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

type conversationUsage struct {
	Input, Output, Cached int64
}

func summarizeConversationUsage(messages map[string][]contracts.Message) conversationUsage {
	var usage conversationUsage
	turns := make(map[string]conversationUsage)
	for _, messages := range messages {
		sessionTurns := make(map[string]conversationUsage)
		for _, message := range messages {
			metered := conversationUsage{Input: int64(max(0, message.Usage.InputTokens)),
				Output: int64(max(0, message.Usage.OutputTokens)), Cached: int64(max(0, message.Usage.CachedTokens))}
			if message.TurnID == "" {
				usage.add(metered)
			} else {
				turn := sessionTurns[message.TurnID]
				turn.add(metered)
				sessionTurns[message.TurnID] = turn
			}
		}
		for id, turn := range sessionTurns {
			// Forks retain the original execution ID in their copied history.
			// Keep one total per execution, including all model steps in it.
			seen := turns[id]
			turns[id] = conversationUsage{Input: max(seen.Input, turn.Input),
				Output: max(seen.Output, turn.Output), Cached: max(seen.Cached, turn.Cached)}
		}
	}
	for _, turn := range turns {
		usage.add(turn)
	}
	return usage
}

func (usage *conversationUsage) add(other conversationUsage) {
	usage.Input += other.Input
	usage.Output += other.Output
	usage.Cached += other.Cached
}

func (a *App) sidebarUsageDialog(c *ui.Context) {
	if !a.sidebarUI.usageOpen {
		return
	}
	usage := summarizeConversationUsage(a.state.Messages)
	a.settingsDialog(c, "sidebar-usage-dialog", "Usage statistics", &a.sidebarUI.usageOpen, 420, func() {
		ui.Text(c, tr(c, "Recorded conversation usage")).TextColor(a.palette.Muted)
		for _, row := range []struct {
			key, label string
			tokens     int64
		}{
			{"usage-input", "Input tokens", usage.Input},
			{"usage-output", "Output tokens", usage.Output},
			{"usage-cached", "Cached input tokens", usage.Cached},
			{"usage-total", "Total tokens", usage.Input + usage.Output},
		} {
			ui.Row(c).FillWidth().Gap(16).Children(func() {
				ui.Text(c, tr(c, row.label)).Grow(1)
				ui.Text(c.Key(row.key), fmt.Sprint(row.tokens))
			})
		}
	})
}
