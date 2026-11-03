package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/model"
	"github.com/egoist/mygo/ui"
)

type trajectoryViewState struct {
	data       contracts.ModelTrajectory
	loading    bool
	dirty      bool
	loaded     bool
	error      string
	search     string
	expanded   map[string]bool
	all        *bool
	searchText []string
	copies     []string
	toolText   []string
}

func (a *App) openModelTrajectory(sessionID string) {
	for _, session := range a.state.Sessions {
		if session.ID != sessionID {
			continue
		}
		if a.trajectoryViews == nil {
			a.trajectoryViews = make(map[string]*trajectoryViewState)
		}
		if a.trajectoryViews[sessionID] == nil {
			a.trajectoryViews[sessionID] = &trajectoryViewState{expanded: make(map[string]bool)}
		}
		a.workspace.OpenTrajectory(sessionID, session.Title)
		a.refreshModelTrajectory(sessionID)
		return
	}
}

func (a *App) refreshModelTrajectory(sessionID string) {
	state := a.trajectoryViews[sessionID]
	if state == nil {
		return
	}
	if state.loading {
		state.dirty = true
		return
	}
	service, ok := a.service.(contracts.ModelTrajectoryService)
	if !ok {
		state.loaded, state.error = true, a.t("This host does not provide call trajectories.")
		return
	}
	state.loading, state.dirty = true, false
	a.backgroundTask(func() func() {
		ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
		defer cancel()
		data, err := service.ReadModelTrajectory(ctx, sessionID)
		copies, searchable, toolText := make([]string, len(data.Records)), make([]string, len(data.Records)), make([]string, len(data.Records))
		for i, record := range data.Records {
			text, marshalErr := json.MarshalIndent(record, "", "  ")
			if marshalErr != nil && err == nil {
				err = marshalErr
			}
			copies[i], searchable[i] = string(text), strings.ToLower(string(text))
			tools, toolErr := json.MarshalIndent(record.Tools, "", "  ")
			if toolErr != nil && err == nil {
				err = toolErr
			}
			toolText[i] = string(tools)
		}
		return func() {
			state.loading, state.loaded = false, true
			state.error = ""
			if err != nil {
				state.error = err.Error()
			}
			if err == nil || len(data.Records) > 0 {
				state.data, state.copies, state.searchText = data, copies, searchable
				state.toolText = toolText
			}
			if state.dirty {
				a.refreshModelTrajectory(sessionID)
			}
		}
	})
}

func (a *App) modelTrajectoryView(c *ui.Context, sessionID string) {
	state := a.trajectoryViews[sessionID]
	if state == nil {
		return
	}
	p := a.palette
	ui.Column(c.Key("trajectory-pane:" + sessionID)).Label(tr(c, "Call trajectory")).Fill().Gap(8).Padding(12).Children(func() {
		ui.Row(c).FillWidth().Gap(4).AlignItems(ui.Center).Children(func() {
			ui.Text(c, tr(c, "Call trajectory")).Grow(1).FontWeight(600)
			expandLabel := "Expand all"
			if state.all != nil && *state.all {
				expandLabel = "Collapse all"
			}
			iconButton(c.Key("trajectory-expand-all"), "expand-arrows", tr(c, expandLabel)).OnClick(func() {
				expand := state.all == nil || !*state.all
				state.all = &expand
				state.expanded = make(map[string]bool)
			})
			iconButton(c.Key("trajectory-refresh"), "refresh", tr(c, "Refresh")).Disabled(state.loading).
				OnClick(func() { a.refreshModelTrajectory(sessionID) })
		})
		controlTextInput(c.Key("trajectory-search"), &state.search).FillWidth().
			Placeholder(tr(c, "Search call trajectory")).Label(tr(c, "Search call trajectory"))
		if state.error != "" {
			ui.Text(c.Key("trajectory-error"), state.error).FillWidth().TextColor(p.Danger).Selectable()
		}
		if state.loading && !state.loaded {
			ui.Text(c, tr(c, "Loading call trajectory…")).TextColor(p.Muted)
			return
		}
		if len(state.data.Records) == 0 {
			ui.Text(c.Key("trajectory-empty"), tr(c, "No recorded model calls")).Padding(16, 0).TextColor(p.Muted)
			return
		}
		query := strings.ToLower(strings.TrimSpace(state.search))
		matches := 0
		for i := range state.data.Records {
			if query == "" || strings.Contains(state.searchText[i], query) {
				matches++
			}
		}
		ui.Text(c.Key("trajectory-summary"), fmt.Sprintf(tr(c, "%d model calls"), matches)).
			FontSize(design.TextSize(c, design.TextSmall)).TextColor(p.Muted)
		if state.data.Truncated {
			ui.Text(c, tr(c, "Some older or oversized trace content was omitted.")).FillWidth().
				FontSize(design.TextSize(c, design.TextSmall)).TextColor(p.Warning)
		}
		ui.Scroll(c.Key("trajectory-timeline:" + sessionID)).Grow(1).FillWidth().Gap(10).Children(func() {
			for i, record := range state.data.Records {
				if query != "" && !strings.Contains(state.searchText[i], query) {
					continue
				}
				key := fmt.Sprintf("%s-%d", record.ID, record.Attempt)
				ui.Column(c.Key("trajectory-call:" + key)).FillWidth().Padding(10).Gap(8).Radius(8).Background(p.Card).Children(func() {
					defaultOpen := query != "" || i == len(state.data.Records)-1
					a.trajectoryDisclosure(c, state, key, fmt.Sprintf("%02d · %s · %s", i+1, record.ModelID, a.trajectorySourceLabel(record.Source)), defaultOpen, func() {
						status := tr(c, "Running")
						if record.CompletedAt != nil {
							status = tr(c, "Completed")
						}
						if record.Error != "" {
							status = tr(c, "Failed")
						}
						metadata := []string{record.StartedAt.Local().Format("01-02 15:04:05"), record.Provider, status}
						if record.CompletedAt != nil {
							metadata = append(metadata, fmt.Sprintf("%d ms", record.DurationMS))
						}
						if record.HTTPStatus > 0 {
							metadata = append(metadata, fmt.Sprintf("HTTP %d", record.HTTPStatus))
						}
						if record.Attempt > 1 {
							metadata = append(metadata, fmt.Sprintf(tr(c, "Attempt %d"), record.Attempt))
						}
						ui.Text(c, strings.Join(metadata, " · ")).
							FillWidth().FontSize(design.TextSize(c, design.TextSmall)).TextColor(p.Muted)
						ui.Text(c, string(record.Protocol)+" · "+record.TurnID).FillWidth().
							FontSize(design.TextSize(c, design.TextSmall)).TextColor(p.Muted).Selectable()
						a.trajectoryDisclosure(c, state, key+":request", tr(c, "Request"), true, func() {
							if record.System != "" {
								a.trajectoryText(c, state, key+":system", "System prompt", record.System, false)
							}
							for j, message := range record.Messages {
								label := fmt.Sprintf("%s %02d", a.trajectoryRoleLabel(message.Role), j+1)
								messageKey := fmt.Sprintf("%s:message:%d", key, j)
								text := trajectoryMessageText(message)
								if message.Reasoning != "" {
									text = message.Reasoning + "\n" + text
								}
								a.trajectoryText(c, state, messageKey, label, text,
									query != "" || j == len(record.Messages)-1)
							}
							if len(record.Tools) > 0 {
								a.trajectoryText(c, state, key+":tools", "Tool definitions", state.toolText[i], false)
							}
						})
						if response := record.Response; response != nil {
							usage := response.Usage
							if usage.Known {
								ui.Text(c.Key("trajectory-usage:"+key), fmt.Sprintf(tr(c, "Input %d · Output %d · Cached %d"),
									usage.Input, usage.Output, usage.Cached)).FillWidth().FontSize(design.TextSize(c, design.TextSmall))
							} else {
								ui.Text(c, tr(c, "Usage unavailable or incomplete")).TextColor(p.Muted)
							}
							a.trajectoryDisclosure(c, state, key+":response", tr(c, "Response")+" · "+string(response.Stop), true, func() {
								if response.Message.Reasoning != "" {
									a.trajectoryText(c, state, key+":reasoning", "Reasoning", response.Message.Reasoning, false)
								}
								a.trajectoryText(c, state, key+":output", "Output", trajectoryMessageText(response.Message), true)
							})
						}
						if record.Error != "" {
							ui.Text(c.Key("trajectory-call-error:"+key), record.Error).FillWidth().TextColor(p.Danger).Selectable()
						}
						services := c.Services()
						ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
							iconButton(c.Key("trajectory-copy:"+key), "copy", tr(c, "Copy call")).OnClick(func() {
								services.WriteClipboard(state.copies[i])
							})
						})
					})
				})
			}
			if matches == 0 {
				ui.Text(c, tr(c, "No matching calls")).TextColor(p.Muted)
			}
		})
	})
}

func (a *App) trajectoryDisclosure(c *ui.Context, state *trajectoryViewState, key, label string, initial bool, draw func()) {
	open, chosen := state.expanded[key]
	if !chosen {
		open = initial
		if state.all != nil && strings.TrimSpace(state.search) == "" {
			open = *state.all
		}
	}
	button := controlNavButton(c.Key("trajectory-toggle:"+key), label, false).FillWidth().Children(func() {
		name := "alt-arrow-right"
		if open {
			name = "alt-arrow-down"
		}
		icon(c, name, 12)
		ui.Text(c, label).Grow(1).MinWidth(0).SingleLine()
	})
	button.OnClick(func() { state.expanded[key] = !open })
	if open {
		draw()
	}
}

func (a *App) trajectoryText(c *ui.Context, state *trajectoryViewState, key, title, text string, initial bool) {
	if text == "" {
		return
	}
	if query := strings.ToLower(strings.TrimSpace(state.search)); query != "" {
		initial = strings.Contains(strings.ToLower(text), query)
	}
	a.trajectoryDisclosure(c, state, key, tr(c, title), initial, func() {
		ui.Text(c.Key("trajectory-content:"+key), text).FillWidth().Selectable().
			Font(a.state.Settings.Appearance.CodeFont).FontSize(design.TextSize(c, design.TextSmall)).
			Padding(6, 8).Background(a.palette.CodeWash).Radius(4)
	})
}

func (a *App) trajectoryRoleLabel(role string) string {
	for _, item := range []struct{ value, label string }{{"system", "System prompt"}, {"user", "User"}, {"assistant", "Assistant"}, {"tool", "Tool"}} {
		if role == item.value {
			return a.t(item.label)
		}
	}
	return role
}

func (a *App) trajectorySourceLabel(source string) string {
	for _, item := range []struct{ value, label string }{{"main", "Main conversation"}, {"subagent", "Subagent"}, {"compact", "Context compaction"}, {"naming", "Conversation naming"}} {
		if source == item.value {
			return a.t(item.label)
		}
	}
	return source
}

func trajectoryMessageText(message model.Message) string {
	text := message.Text
	for _, call := range message.Calls {
		text += "\n" + call.Name + " [" + call.ID + "]\n" + call.Arguments
	}
	if message.CallID != "" {
		text = "[" + message.CallID + "]\n" + text
	}
	for _, image := range message.Images {
		text += fmt.Sprintf("\n[%s %dx%d]", image.MIME, image.Width, image.Height)
	}
	return strings.TrimSpace(text)
}
