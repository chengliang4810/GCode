package desktop

import (
	"encoding/json"
	"strconv"
	"strings"

	"gcode/internal/appmodel"
	"github.com/egoist/mygo/ui"
)

func (a *App) conversation(c *ui.Context, p palette, data appmodel.Snapshot, activeID, input string, sendMode appmodel.SendMode, errorText string, rightOpen, narrow bool) {
	session, ok := activeSession(data.Sessions, activeID)
	ui.Column(c).Fill().Background(p.surface).MinWidth(0).Children(func() {
		a.conversationHeader(c, p, data, session, ok, rightOpen, narrow)
		if !ok {
			emptyConversation(c, p, "选择一个会话开始编码", "从左侧选择会话，或创建一个新的本地项目会话。")
			return
		}
		messages := session.Messages
		messageCount := len(messages)
		permissionCount := 0
		if len(session.Permissions) > 0 {
			permissionCount = 1
		}
		questionCount := 0
		if len(session.Questions) > 0 {
			questionCount = 1
		}
		rowCount := messageCount + permissionCount + questionCount
		list := a.conversationListState(session.ID)
		list.FollowEnd = true
		list.Key = func(i int) any {
			if i < messageCount {
				return conversationMessageKey(messages[i], i)
			}
			if i < messageCount+permissionCount {
				return "permission:" + session.Permissions[0].ID
			}
			return "question:" + session.Questions[0].ID
		}
		list.Label = func(i int) string {
			if i < messageCount {
				return conversationMessageLabel(messages[i])
			}
			if i < messageCount+permissionCount {
				return "需要你的授权"
			}
			return "Agent 需要你的选择"
		}
		ui.List(c.Key("message-list-"+session.ID), list, rowCount, func(i int) {
			switch {
			case i < messageCount:
				a.renderMessage(c, p, session.ID, conversationMessageKey(messages[i], i), messages[i])
			case i < messageCount+permissionCount:
				a.permissionCard(c, p, session.Permissions[0])
			default:
				a.questionCard(c, p, session.Questions[0])
			}
		}).Grow(1).WidthPercent(100).MaxWidth(conversationColumnMaxWidth).AlignSelf(ui.Center).
			Justify(ui.End).Padding(20, 24, 16, 24).Gap(16).Children(func() {
			if rowCount == 0 {
				emptyConversation(c, p, "开始一轮编码任务", "描述你要检查、修改或验证的代码，Agent 会在这里展示过程。")
			}
		})
		if len(session.Queue) > 0 {
			queuePanel(c, p, session.Queue, session.QueuePaused, a.resumeQueue, a.withdraw)
		}
		if errorText != "" {
			errorRow(c, p, errorText, a.clearError)
		}
		a.composer(c, p, data, session, input, sendMode)
	})
}

func (a *App) conversationListState(sessionID string) *ui.ListState {
	a.mu.Lock()
	defer a.mu.Unlock()
	if list := a.conversationLists[sessionID]; list != nil {
		return list
	}
	list := &ui.ListState{FollowEnd: true}
	a.conversationLists[sessionID] = list
	return list
}

func conversationMessageKey(message appmodel.Message, index int) string {
	if id := strings.TrimSpace(message.ID); id != "" {
		return "message:" + id
	}
	return "message:index:" + strconv.Itoa(index)
}

func conversationMessageLabel(message appmodel.Message) string {
	text := strings.TrimSpace(message.Text)
	if text == "" {
		if len(message.Tools) > 0 {
			return message.Tools[0].Name
		}
		return message.Role
	}
	return text
}

func (a *App) conversationHeader(c *ui.Context, p palette, data appmodel.Snapshot, session appmodel.Session, ok bool, rightOpen, narrow bool) {
	bar := c.TitleBar()
	right := bar.Right + 12
	if right < 12 {
		right = 12
	}
	height := desktopHeaderHeight(c)
	ui.Row(c).Height(height).Padding(0, right, 0, 24).Gap(12).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(p.border).Children(func() {
		ui.Row(c).Gap(10).MinWidth(0).Grow(1).AlignItems(ui.Center).Children(func() {
			title := "没有选择会话"
			if ok {
				title = session.Title
				if title == "" {
					title = "未命名会话"
				}
				if project := projectName(data.Projects, session.ProjectID); project != "" {
					ui.Text(c, project).FontSize(13).TextColor(p.muted).MaxWidth(120).SingleLine().Tooltip(session.Cwd)
					ui.Text(c, "/").FontSize(13).TextColor(p.muted)
				}
			}
			ui.Text(c, title).FontSize(14.5).FontWeight(550).Grow(1).MinWidth(0).SingleLine().Tooltip(title)
		})
		if iconButton(c, p, "panel-right", map[bool]string{true: "收起资源面板", false: "展开资源面板"}[rightOpen]).Size(30, 30).Radius(7).Clicked() {
			a.toggleRightPanel(rightOpen, narrow)
		}
	})
}

func (a *App) renderMessage(c *ui.Context, p palette, sessionID, rowKey string, message appmodel.Message) {
	user := strings.EqualFold(message.Role, "user")
	cacheKey := sessionID + ":" + rowKey
	if message.Text == "" {
		// The waiting state skips the Markdown renderer. An empty source
		// still resets this message's prefix before text arrives again.
		a.mu.Lock()
		if cached, exists := a.markdownCache[cacheKey]; exists && cached.text != "" {
			a.markdownCache[cacheKey] = cachedMessageRender{generation: cached.generation + 1}
		}
		a.mu.Unlock()
	}
	// ui.List already keys the row through ListState. Avoid a second key based
	// only on Message.ID: older records without IDs would otherwise all share
	// the same element identity.
	row := ui.Row(c).WidthPercent(100).Gap(10).AlignItems(ui.Start)
	if user {
		row = row.Justify(ui.End)
	}
	row.Children(func() {
		body := ui.Column(c).MaxWidth(700).MinWidth(0).Gap(8).Padding(3, 0)
		if user {
			body = ui.Column(c).MaxWidth(580).MinWidth(0).Gap(7).Padding(10, 14).Radius(16).Background(p.surfaceAlt)
		}
		body.Children(func() {
			if message.Text == "" && len(message.Tools) == 0 {
				ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
					ui.Spinner(c).FontSize(14)
					ui.Text(c, "正在生成…").FontSize(13).TextColor(p.muted)
				})
			} else {
				a.renderTextBlocks(c, p, cacheKey, message.Text)
			}
			for _, tool := range message.Tools {
				a.renderTool(c, p, tool, message.Interrupted)
			}
			ui.Row(c).Height(20).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, messageTime(message.CreatedAt)).FontSize(10.5).TextColor(p.muted)
				if message.Interrupted && !user {
					ui.Text(c, "已中断").FontSize(10.5).TextColor(p.muted)
				}
				ui.Spacer(c)
				copyButton := iconButton(c, p, "copy", "复制消息").Size(24, 24).Radius(6)
				if copyButton.Clicked() {
					c.WriteClipboard(message.Text)
				}
			})
		})
	})
}

func (a *App) renderTool(c *ui.Context, p palette, tool appmodel.ToolCall, interrupted bool) {
	state := tool.State
	// Preserve the diagnostic and raw arguments in the expandable detail, but
	// present an expected cancellation as an interruption rather than a failure.
	if interrupted && (strings.EqualFold(state, "error") || strings.EqualFold(state, "failed")) &&
		(strings.HasSuffix(strings.TrimSpace(tool.Output), "context canceled") || strings.HasSuffix(strings.TrimSpace(tool.Error), "context canceled")) {
		state = "interrupted"
	}
	tone := p.muted
	if strings.EqualFold(state, "running") || strings.EqualFold(state, "pending") {
		tone = p.accent
	} else if state != "interrupted" && (strings.EqualFold(state, "error") || tool.Error != "") {
		tone = p.danger
	} else if strings.EqualFold(state, "done") || strings.EqualFold(state, "success") || strings.EqualFold(state, "completed") || strings.EqualFold(state, "ok") {
		tone = p.success
	}
	key := toolIdentity(tool)
	forcedOpen := state != "interrupted" && (strings.EqualFold(state, "error") || strings.EqualFold(state, "failed") || tool.Error != "")
	open := a.toolExpandedValue(key, forcedOpen)
	parts := ui.CollapsibleBase(c.Key("tool-"+key), &open)
	trigger := parts.Trigger.Height(28).WidthPercent(100).Padding(0, 8).Radius(8).Gap(7).AlignItems(ui.Center).
		Label("工具 " + tool.Name).TextColor(p.text)
	if forcedOpen {
		trigger = trigger.Background(p.danger.Alpha(0.1))
	} else if trigger.Hovered() {
		trigger = trigger.Background(p.surfaceAlt)
	}
	trigger.Children(func() {
		chevron := "chevron-right"
		if open {
			chevron = "chevron-down"
		}
		ui.Icon(c, icon(chevron)).FontSize(13).TextColor(p.muted)
		ui.Icon(c, icon("code")).FontSize(15).TextColor(tone)
		name := tool.Name
		if name == "" {
			name = "工具调用"
		}
		ui.Text(c, name).FontSize(12.5).FontWeight(600).SingleLine()
		if summary := toolArgumentsSummary(tool); summary != "" {
			ui.Text(c, summary).Grow(1).Font("SFMono-Regular, Menlo, Consolas, monospace").FontSize(11).TextColor(p.muted).SingleLine()
		} else {
			ui.Spacer(c)
		}
		ui.Text(c, stateLabelFromTool(state)).FontSize(11).TextColor(tone).SingleLine()
		if tool.Arguments != "" {
			copyButton := iconButton(c, p, "copy", "复制原始参数").Size(24, 24).Radius(6)
			if copyButton.Clicked() {
				c.WriteClipboard(tool.Arguments)
			}
		}
	})
	if trigger.Changed() {
		a.rememberToolExpanded(key, open)
	}
	if panel := parts.Panel(func() {
		ui.Column(c).Gap(6).Padding(8, 10, 10, 29).Background(p.surfaceAlt).Radius(8).Children(func() {
			if tool.Arguments != "" {
				ui.Text(c, tool.Arguments).Font("SFMono-Regular, Menlo, Consolas, monospace").FontSize(11.5).LineHeight(1.35).TextColor(p.muted).Selectable()
			}
			if tool.Output != "" {
				ui.Text(c, tool.Output).Font("SFMono-Regular, Menlo, Consolas, monospace").FontSize(11.5).LineHeight(1.35).TextColor(p.text).Selectable()
			}
			if tool.Error != "" {
				ui.Text(c, tool.Error).FontSize(12).LineHeight(1.4).TextColor(p.danger).Selectable()
			}
		})
	}); panel.Valid() {
		panel.WidthPercent(100)
	}
}

func toolIdentity(tool appmodel.ToolCall) string {
	if id := strings.TrimSpace(tool.ID); id != "" {
		return id
	}
	name := strings.TrimSpace(tool.Name)
	if name == "" {
		name = "tool"
	}
	return name + ":" + strconv.Itoa(len(tool.Arguments)) + ":" + strconv.Itoa(len(tool.Output))
}

func (a *App) toolExpandedValue(key string, forced bool) bool {
	if forced {
		return true
	}
	a.mu.RLock()
	open := a.toolExpanded[key]
	a.mu.RUnlock()
	return open
}

func (a *App) rememberToolExpanded(key string, open bool) {
	a.mu.Lock()
	if a.toolExpanded == nil {
		a.toolExpanded = make(map[string]bool)
	}
	a.toolExpanded[key] = open
	a.mu.Unlock()
}

func stateLabelFromTool(state string) string {
	switch strings.ToLower(state) {
	case "running", "pending":
		return "执行中"
	case "error", "failed":
		return "失败"
	case "interrupted", "canceled", "cancelled":
		return "已中断"
	case "done", "success", "completed", "ok":
		return "完成"
	default:
		return "等待"
	}
}

func toolArgumentsSummary(tool appmodel.ToolCall) string {
	raw := strings.TrimSpace(tool.Arguments)
	if raw == "" {
		return ""
	}
	var values map[string]any
	if err := json.Unmarshal([]byte(raw), &values); err == nil {
		for _, key := range []string{"cmd", "command", "script"} {
			if command, ok := values[key].(string); ok && strings.TrimSpace(command) != "" {
				return truncateMiddle(readableCommand(command), 96)
			}
		}
		for _, key := range []string{"path", "file", "file_path", "filePath", "directory", "dir", "cwd", "workdir"} {
			if path, ok := values[key].(string); ok && strings.TrimSpace(path) != "" {
				return truncateMiddle(path, 96)
			}
		}
		return "查看详情"
	}
	return truncateMiddle(raw, 96)
}

func readableCommand(command string) string {
	var readable strings.Builder
	for _, char := range command {
		switch {
		case char == '\n' || char == '\t' || char >= 0x20:
			readable.WriteRune(char)
		case char <= 0x1f:
			readable.WriteRune(0x2400 + char)
		}
	}
	return readable.String()
}

func (a *App) permissionCard(c *ui.Context, p palette, permission appmodel.Permission) {
	ui.Column(c).Background(p.warning.Alpha(0.1)).Border(1, p.warning.Alpha(0.3)).Radius(9).Padding(11, 12).Gap(8).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon("shield")).FontSize(17).TextColor(p.warning)
			ui.Column(c).Gap(2).Grow(1).Children(func() {
				ui.Text(c, "需要你的授权").FontSize(13.5).FontWeight(650)
				ui.Text(c, permission.Tool).FontSize(11.5).TextColor(p.muted).SingleLine()
			})
		})
		if permission.Arguments != "" {
			ui.Scroll(c.Key("permission-arguments-" + permission.ID)).MaxHeight(180).WidthPercent(100).Children(func() {
				ui.Text(c, permissionArgumentsText(permission.Arguments)).Font("SFMono-Regular, Menlo, Consolas, monospace").FontSize(12).LineHeight(1.4).TextColor(p.text).Selectable()
			})
		}
		ui.Row(c).Gap(8).Children(func() {
			for _, option := range permission.Options {
				button := compactButton(c, p, option.Label)
				if button.Clicked() {
					a.permission(permission, option.ID)
				}
			}
		})
	})
}

func permissionArgumentsText(raw string) string {
	var values map[string]any
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return raw
	}
	for _, key := range []string{"cmd", "command", "script"} {
		if command, ok := values[key].(string); ok && strings.TrimSpace(command) != "" {
			text := readableCommand(command)
			for _, directoryKey := range []string{"workdir", "cwd"} {
				if directory, ok := values[directoryKey].(string); ok && strings.TrimSpace(directory) != "" {
					return text + "\n工作目录：" + directory
				}
			}
			return text
		}
	}
	return raw
}

func (a *App) questionCard(c *ui.Context, p palette, request appmodel.AskRequest) {
	ui.Column(c).Background(p.accentSoft).Border(1, p.accent.Alpha(0.3)).Radius(9).Padding(11, 12).Gap(8).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon("message")).FontSize(17).TextColor(p.accent)
			ui.Text(c, "Agent 需要你的选择").FontSize(13.5).FontWeight(650)
		})
		for _, question := range request.Questions {
			ui.Text(c, question.Text).FontSize(13.5).LineHeight(1.4)
			ui.Row(c).Gap(8).Children(func() {
				for _, option := range question.Options {
					button := compactButton(c, p, option.Label)
					if button.Clicked() {
						a.ask(request, option.Label)
					}
				}
			})
		}
	})
}

func queuePanel(c *ui.Context, p palette, queue []appmodel.QueuedMessage, paused bool, resume func(), withdraw func(string)) {
	ui.Column(c).Background(p.surfaceAlt).BorderWidth(1, 0, 0, 0).BorderColor(p.border).Padding(8, 20, 6, 20).Gap(5).Children(func() {
		ui.Row(c).Height(22).AlignItems(ui.Center).Gap(7).Children(func() {
			ui.Icon(c, icon("clock")).FontSize(14).TextColor(p.warning)
			ui.Text(c, "待执行队列").FontSize(12.5).FontWeight(600)
			ui.Text(c, stringCount(len(queue))).FontSize(11.5).TextColor(p.muted)
			ui.Spacer(c)
			if paused {
				if compactButton(c, p, "继续").Width(62).Clicked() {
					resume()
				}
			}
		})
		for _, item := range queue {
			ui.Row(c.Key("queue-" + item.ID)).Height(28).Gap(7).AlignItems(ui.Center).Children(func() {
				ui.Text(c, item.Text).FontSize(12.5).SingleLine().Grow(1)
				ui.Text(c, sendModeLabel(item.Mode)).FontSize(11).TextColor(p.muted).SingleLine()
				remove := iconButton(c, p, "x", "撤回排队消息").Size(24, 24).Radius(6)
				if remove.Clicked() {
					withdraw(item.ID)
				}
			})
		}
	})
}

func sendModeLabel(mode appmodel.SendMode) string {
	if mode == appmodel.Queue {
		return "队列"
	}
	return "纠正"
}

func errorRow(c *ui.Context, p palette, text string, clear func()) {
	ui.Row(c).Padding(8, 20).Gap(8).AlignItems(ui.Center).Background(p.danger.Alpha(0.1)).Children(func() {
		ui.Icon(c, icon("x")).FontSize(15).TextColor(p.danger)
		ui.Text(c, text).FontSize(12.5).TextColor(p.danger).Grow(1)
		if iconButton(c, p, "x", "关闭错误").Size(24, 24).Radius(6).Clicked() {
			clear()
		}
	})
}

func (a *App) composer(c *ui.Context, p palette, data appmodel.Snapshot, session appmodel.Session, input string, sendMode appmodel.SendMode) {
	draft := input
	ui.Column(c).WidthPercent(100).MaxWidth(conversationColumnMaxWidth).AlignSelf(ui.Center).
		Padding(8, 24, 12, 24).Gap(8).Children(func() {
		ui.Column(c).Background(p.surfaceAlt).Border(1, p.border).Radius(18).Padding(10, 12, 10, 12).Gap(7).Children(func() {
			field := ui.TextArea(c.Key("composer"), &draft).Height(78).Placeholder("输入任务，例如：检查最近的错误处理并运行测试").Label("任务输入").
				Background(ui.Transparent).BorderWidth(0, 0, 0, 0).Padding(2, 2)
			if field.Changed() {
				a.mu.Lock()
				a.input = draft
				a.mu.Unlock()
			}
			if field.Submitted() {
				a.send(draft, sendMode)
			}
			ui.Row(c).Height(34).AlignItems(ui.Center).Gap(8).Children(func() {
				choices := providerChoices(data.Settings)
				if len(choices) > 0 {
					provider := session.ProviderID
					selectBox := composerProviderSelect(c.Key("composer-provider-"+session.ID), p, &provider, choices)
					if selectBox.Changed() {
						a.setProvider(session.ID, provider)
					}
				}
				ui.Spacer(c)
				if session.State == appmodel.Running || session.State == appmodel.Cancelling {
					stop := neutralCircleAction(c, p, "stop", "停止")
					if stop.Clicked() {
						a.stop()
					}
				}
				send := circleAction(c, p, "arrow-up", "发送", p.accent).Disabled(strings.TrimSpace(draft) == "")
				if send.Clicked() {
					a.send(draft, sendMode)
				}
			})
		})
		ui.Row(c).Height(28).Padding(0, 5).Gap(12).AlignItems(ui.Center).Children(func() {
			index := modeIndex(sendMode)
			parts := ui.SegmentedBase(c.Key("send-mode"), &index, 2)
			mode := parts.Track.Label("发送方式").Height(26).Gap(2).Children(func() {
				for i, label := range []string{"纠正", "队列"} {
					segment := parts.Segment(i).Height(26).Padding(0, 8).Radius(7).Label(label).TextColor(p.muted)
					if i == index {
						segment = segment.Background(p.selection).TextColor(p.text)
					} else if segment.Hovered() {
						segment = segment.Background(p.surfaceAlt).TextColor(p.text)
					}
					segment.Children(func() { ui.Text(c, label).FontSize(11.5).SingleLine() })
				}
			})
			if mode.Changed() {
				selected := modeFromIndex(index)
				a.mu.Lock()
				a.sendMode = selected
				a.sendModeDirty = selected != a.data.Settings.DefaultSendMode
				a.mu.Unlock()
			}
			fullAccess := session.FullAccess
			access := ui.Checkbox(c.Key("composer-full-access-"+session.ID), &fullAccess, "完全访问").
				Height(28).Gap(6).FontSize(11.5).TextColor(p.muted).Label("权限模式")
			if access.Changed() {
				a.mu.Lock()
				for i := range a.data.Sessions {
					if a.data.Sessions[i].ID == session.ID {
						a.data.Sessions[i].FullAccess = fullAccess
					}
				}
				a.mu.Unlock()
				a.setFullAccess(session.ID, fullAccess)
			}
			ui.Spacer(c)
			if branch := strings.TrimSpace(session.Branch); branch != "" {
				ui.Icon(c, icon("git")).FontSize(12).TextColor(p.muted)
				ui.Text(c, branch).FontSize(10.5).TextColor(p.muted).MinWidth(0).SingleLine().Tooltip(branch)
			}
		})
	})
}

func composerProviderSelect(c *ui.Context, p palette, selected *string, choices []providerChoice) ui.Element {
	parts := ui.SelectBase(c, selected)
	label := providerOptionLabel(choices, *selected)
	trigger := parts.Trigger.Width(170).Height(30).Padding(0, 6).Gap(7).Radius(8).
		AlignItems(ui.Center).Label("下一任务模型").Tooltip("下一任务模型：" + label).TextColor(p.muted)
	if trigger.Hovered() {
		trigger = trigger.Background(p.surfaceRaised).TextColor(p.text)
	}
	trigger.Children(func() {
		ui.Icon(c, icon("bot")).FontSize(15)
		ui.Text(c, label).FontSize(12.5).MinWidth(0).Grow(1).SingleLine()
		ui.Icon(c, icon("chevron-down")).FontSize(12)
	})
	parts.Popup(func(panel ui.Element) {
		panel.MinWidth(220).Padding(4).Radius(10).Background(p.surfaceRaised).Border(1, p.border).
			Shadow(0, 6, 20, 0, ui.RGBA(0, 0, 0, 0.2))
		ui.Scroll(c).MaxHeight(260).Gap(2).Children(func() {
			for _, choice := range choices {
				item := parts.Item(choice.id).Height(32).Padding(0, 9).Radius(7).AlignItems(ui.Center).TextColor(p.text)
				if item.Highlighted() || choice.id == *selected {
					item = item.Background(p.selection)
				}
				item.Children(func() { ui.Text(c, choice.label).FontSize(12.5).SingleLine() })
			}
		})
	})
	return trigger
}

func modeIndex(mode appmodel.SendMode) int {
	if mode == appmodel.Queue {
		return 1
	}
	return 0
}

func modeFromIndex(index int) appmodel.SendMode {
	if index == 1 {
		return appmodel.Queue
	}
	return appmodel.Correction
}

type providerChoice struct {
	id    string
	label string
}

func providerChoices(settings appmodel.Settings) []providerChoice {
	choices := make([]providerChoice, 0, len(settings.Providers))
	counts := make(map[string]int, len(settings.Providers))
	for _, provider := range settings.Providers {
		label := strings.TrimSpace(provider.Name)
		if label == "" {
			label = strings.TrimSpace(provider.Model)
		}
		if label == "" {
			label = "未命名服务"
		}
		counts[label]++
		choices = append(choices, providerChoice{id: provider.ID, label: label})
	}
	for index := range choices {
		if counts[choices[index].label] <= 1 {
			continue
		}
		for _, provider := range settings.Providers {
			if provider.ID != choices[index].id {
				continue
			}
			suffix := strings.TrimSpace(provider.Model)
			if suffix == "" {
				suffix = provider.ID
			}
			choices[index].label += " · " + suffix
			break
		}
	}
	return choices
}

func providerOptionLabel(choices []providerChoice, id string) string {
	for _, choice := range choices {
		if choice.id == id {
			return choice.label
		}
	}
	return id
}

func emptyConversation(c *ui.Context, p palette, title, detail string) {
	ui.Column(c).Grow(1).Center().Gap(9).Padding(24).Children(func() {
		ui.Row(c).Size(44, 44).Radius(14).Center().Background(p.accentSoft).Children(func() {
			ui.Icon(c, icon("bot")).FontSize(23).TextColor(p.accent)
		})
		ui.Text(c, title).FontSize(18).FontWeight(650).TextAlign(ui.Center)
		ui.Text(c, detail).FontSize(13.5).LineHeight(1.45).TextColor(p.muted).TextAlign(ui.Center).MaxWidth(430)
	})
}
