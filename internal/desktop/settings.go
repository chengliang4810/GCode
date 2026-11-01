package desktop

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/i18n"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type settingOption struct{ value, label string }
type settingLink struct{ title, section, description string }

func (a *App) sections() []string {
	s := []string{"General", "Appearance", "Personalization", "Notifications", "Shortcuts"}
	if a.state.Runtime != nil {
		s = append(s, "Model settings", "Plugins", "Subagents", "MCP servers", "Skills", "Commands", "Hooks")
	} else {
		s = append(s, "Providers")
	}
	s = append(s, "Devices")
	if a.state.Capabilities.Cloud && a.state.User.SignedIn {
		s = append(s, "Cloud", "Billing")
	}
	s = append(s, "Files")
	if runtime.GOOS == "darwin" && a.state.Capabilities.Appshots {
		s = append(s, "Appshots")
	}
	return append(s, "Archived sessions")
}

func settingsIcon(section string) string {
	switch section {
	case "General":
		return "settings"
	case "Appearance":
		return "tuning"
	case "Personalization":
		return "global"
	case "Notifications":
		return "bell"
	case "Shortcuts":
		return "keyboard"
	case "Providers", "Model settings":
		return "widget"
	case "Subagents":
		return "git-branch"
	case "MCP servers":
		return "remote-server"
	case "Skills":
		return "star"
	case "Plugins":
		return "widget"
	case "Commands":
		return "command"
	case "Hooks":
		return "action-play"
	case "Devices", "Appshots":
		return "monitor"
	case "Cloud":
		return "cloud"
	case "Billing":
		return "wallet"
	case "Files":
		return "folder"
	case "Archived sessions":
		return "archive-minimalistic"
	}
	return "settings"
}

func (a *App) settingsNavigation(c *ui.Context) {
	ui.Column(c).Grow(1).Padding(8, 8).Gap(4).Children(func() {
		controlNavButton(c.Key("back-to-conversations"), tr(c, "Back to conversations"), false).FillWidth().Children(func() {
			icon(c, "alt-arrow-left", 16)
			ui.Text(c, tr(c, "Back")).FontSize(design.TextSize(c, design.TextBase))
		}).OnClick(func() { a.clearShortcutRecording(); a.navigate("chat") })
		controlTextInput(c.Key("settings-search"), &a.settingsSearch).Placeholder(tr(c, "Search settings")).Label(tr(c, "Search settings")).Margin(2, 0, 8, 0).OnChange(func() {})
		ui.Scroll(c.Key("settings-navigation")).Grow(1).Gap(3).Children(func() {
			if strings.TrimSpace(a.settingsSearch) != "" {
				query := strings.ToLower(strings.TrimSpace(a.settingsSearch))
				found := false
				for _, hit := range a.localizedSettingsSearch(query) {
					found = true
					h := hit
					b := controlNavButton(c.Key("search-"+h.title), tr(c, h.title), false).FillWidth()
					b.Children(func() {
						icon(c, settingsIcon(h.section), 17)
						ui.Column(c).Grow(1).Gap(3).Children(func() {
							a.settingsMatchLabel(c, h.title, query)
							ui.Text(c, tr(c, h.section)).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
						})
					})
					b.OnClick(func() {
						a.clearShortcutRecording()
						a.selectSettingsSection(h.section)
						target := h.title
						if target == "Autosave delay" && !a.state.Settings.Files.Autosave {
							target = "Autosave"
						}
						if target == "Background effect" && a.state.Settings.Appearance.Background == "" {
							target = "New thread background"
						}
						a.expanded["settings-target:"+target] = true
					})
				}
				if !found {
					ui.Text(c, tr(c, "No settings found")).TextColor(a.palette.Muted).Padding(12, 10)
				}
				return
			}
			for _, name := range a.sections() {
				section := name
				if name == "Providers" || name == "Files" {
					ui.Spacer(c).Height(12)
				}
				b := controlNavButton(c.Key("settings-nav-"+section), tr(c, section), a.section == section).FillWidth()
				b.Children(func() {
					icon(c, settingsIcon(section), 18)
					ui.Text(c, tr(c, section)).FontSize(design.TextSize(c, design.TextBase)).FontWeight(500)
				})
				b.OnClick(func() { a.clearShortcutRecording(); a.selectSettingsSection(section) })
			}
		})
	})
}

func (a *App) settingLinks() []settingLink {
	links := []settingLink{
		{"Subagents", "Subagents", "agent profile prompt model reasoning 子智能体 提示词 模型 思考"},
		{"Model settings", "Model settings", "provider protocol API key models 模型 供应商"},
		{"MCP servers", "MCP servers", "tools stdio HTTP servers MCP 服务器"},
		{"Skills", "Skills", "SKILL.md instructions import 技能 导入"},
		{"Commands", "Commands", "slash prompt commands 命令 提示词"},
		{"Hooks", "Hooks", "events process shell hooks 钩子 事件"},
		{"Plugins", "Plugins", "marketplace install update skills commands MCP hooks 插件 市场 安装 更新"},
		{"Data directory", "General", "data storage path folder 数据 保存 目录"},
		{"Language", "General", "中文 Chinese English language 语言 国际化"},
		{"Send messages with", "General", "Enter keyboard composer"}, {"Compact mode", "General", "Collapse thinking and tools"}, {"Stop agent with Escape", "General", "Keyboard"}, {"Thread naming", "General", "Model"},
		{"Color scheme", "Appearance", "System light dark mode"}, {"Light theme", "Appearance", "Palette colors"}, {"Dark theme", "Appearance", "Palette colors"}, {"Reasoning theme", "Appearance", "思考强度 滑块 渐变 slider gradient swatch"}, {"Theme library", "Appearance", "Import add custom themes"}, {"Accent color", "Appearance", "Tint highlight swatch"}, {"Glass", "Appearance", "Frosted translucent opaque transparency blur vibrancy surface"}, {"Interface font", "Appearance", "Typography text typeface size menus conversations"}, {"Terminal font", "Appearance", "Monospace text typeface size"}, {"Code & diff font", "Appearance", "Editor text monospace typeface files size"}, {"Conversation width", "Appearance", "Transcript messages composer column maximum"}, {"New thread background", "Appearance", "Image picture wallpaper composer"}, {"Background effect", "Appearance", "Blur dim image"}, {"Reduce motion", "Appearance", "Animations accessibility"},
		{"Auto memory", "Personalization", "记忆 memory recall facts persist 保存 记住 事实 会话"}, {"Global instructions", "Personalization", "AGENTS.md 全局指令 instructions 编辑 editor global"},
		{"Match wallpaper colors", "Appearance", "Tint accent surface desktop"}, {"Pause animations in background", "Appearance", "focus unfocused inactive window battery"}, {"Render math", "Appearance", "Latex formulas equations markdown typeset"}, {"Math font", "Appearance", "Latex formulas typeface"}, {"Math size", "Appearance", "Latex formulas scale"},
		{"Session sounds", "Notifications", "Task completed input required"}, {"Desktop notifications", "Notifications", "Alerts banners system"}, {"Keyboard shortcuts", "Shortcuts", "Keybindings"}, {"Providers", "Providers", "Accounts models authentication"}, {"Devices", "Devices", "Remote connection"}, {"Cloud", "Cloud", "Sync storage"}, {"Billing", "Billing", "Plan credits usage invoices"}, {"Autosave", "Files", "Save files delay"}, {"Word wrap", "Files", "Editor"}, {"Show hidden and ignored files", "Files", "Explorer"}, {"Archived sessions", "Archived sessions", "Restore conversations"},
		{"Task completed", "Notifications", "Done finished audio sound"}, {"Input required", "Notifications", "Question waiting approval audio sound"}, {"Errors and disconnections", "Notifications", "Failure offline audio sound"}, {"Only when in the background", "Notifications", "Focus unfocused"}, {"Autosave delay", "Files", "Debounce wait"},
	}
	if a.checkForUpdates != nil {
		links = append(links, settingLink{"Software updates", "General", "Check for updates version release 软件 更新 版本"})
	}
	for _, shortcut := range a.state.Settings.Shortcuts {
		links = append(links, settingLink{shortcut.Label, "Shortcuts", shortcut.Group + " " + shortcut.Keys + " keyboard shortcut keybinding"})
	}
	if runtime.GOOS == "darwin" && a.state.Capabilities.Appshots {
		links = append(links,
			settingLink{"Capture Appshots", "Appshots", "Screenshot screen"},
			settingLink{"Capture sound", "Appshots", "Shutter audio"},
			settingLink{"Global shortcut", "Appshots", "Capture appshot screenshot preferred keybinding hotkey"},
			settingLink{"Destination", "Appshots", "Session composer where"},
			settingLink{"Window capture", "Appshots", "Screen recording permission"},
			settingLink{"Application text", "Appshots", "Accessibility semantic permission"},
		)
	}
	visible := make(map[string]bool)
	for _, name := range a.sections() {
		visible[name] = true
	}
	filtered := links[:0]
	for _, link := range links {
		if visible[link.section] {
			filtered = append(filtered, link)
		}
	}
	return filtered
}

func (a *App) settingsView(c *ui.Context) {
	if a.section != "Shortcuts" && a.section != "Appshots" {
		a.clearShortcutRecording()
	}
	valid := false
	for _, section := range a.sections() {
		if strings.EqualFold(a.section, section) {
			a.section = section
			valid = true
			break
		}
	}
	if !valid {
		a.section = "General"
	}
	settingsPageViewport(c.Key("settings-page-" + a.section)).Children(func() {
		settingsPageContent(c).Children(func() {
			heading := settingsPageHeading(c, a.section)
			if a.expanded["settings-target:"+a.section] {
				heading.ScrollIntoView()
				delete(a.expanded, "settings-target:"+a.section)
			}
			if (a.section == "Providers" || a.section == "Devices" || a.section == "Cloud" || a.section == "Billing") && a.integrationUI.Error != "" {
				controlRowButton(c).FillWidth().Label(tr(c, "Dismiss settings error")).OnClick(func() { a.integrationUI.Error = "" }).Children(func() {
					ui.Text(c, a.integrationUI.Error).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Danger)
				})
			}
			switch a.section {
			case "General":
				a.softwareUpdateSettings(c)
				a.dataDirectorySettings(c)
				a.generalSettings(c)
			case "Appearance":
				a.appearanceSettings(c)
			case "Personalization":
				a.personalizationSettings(c)
			case "Notifications":
				a.notificationSettings(c)
			case "Shortcuts":
				a.shortcutSettings(c)
			case "Providers", "Model settings":
				a.providerSettings(c)
			case "Subagents":
				a.subagentSettings(c)
			case "MCP servers":
				a.mcpSettings(c)
			case "Skills":
				a.skillSettings(c)
			case "Commands":
				a.commandSettings(c)
			case "Hooks":
				a.hookSettings(c)
			case "Plugins":
				a.pluginSettings(c)
			case "Devices":
				a.deviceSettings(c)
			case "Cloud":
				a.cloudSettings(c)
			case "Billing":
				a.billingSettings(c)
			case "Files":
				a.fileSettings(c)
			case "Appshots":
				a.appshotSettings(c)
			case "Archived sessions":
				a.archivedSettings(c)
			}
		})
	})
	a.integrationDialogs(c)
	a.settingsImportDialog(c)
	a.settingsRemovalDialog(c)
}

func (a *App) settingsToggle(c *ui.Context, label string, value *bool, after ...func()) {
	a.controlSwitch(c.Key(label), value, tr(c, "Toggle ")+tr(c, label), false, func(bool) {
		for _, update := range after {
			update()
		}
		a.saveSettings()
	})
}

func (a *App) settingsChoice(c *ui.Context, label string, value *string, options []settingOption, after func()) {
	selected := *value
	display := selected
	for _, option := range options {
		if option.value == selected {
			display = option.label
			break
		}
	}
	sel := controlSelect(c.Key(label), value)
	searchable := label == "Interface font" || label == "Terminal font" || label == "Code & diff font"
	filter := ui.Local(sel.Trigger, "font-filter", func() string { return "" })
	if !sel.Open() {
		*filter = ""
	}
	sel.Trigger.Label(tr(c, "Choose ") + tr(c, label)).MinWidth(124).OnChange(func() {
		if after != nil {
			after()
		}
		a.saveSettings()
	}).Children(func() {
		ui.Text(c, a.settingOptionText(c, label, selected, display)).Grow(1).SingleLine()
		icon(c, "alt-arrow-down", 13)
	})
	if searchable {
		sel.Trigger.Width(220)
	}
	sel.Popup(func(panel ui.Element) {
		controlSelectPopup(c, panel).MinWidth(200).Children(func() {
			if searchable {
				panel.Width(220)
				controlTextInput(c.Key("font-search"), filter).Label(tr(c, "Search fonts")).Placeholder(tr(c, "Search fonts")).FillWidth().AutoFocus().Margin(3, 3, 6, 3)
			}
			ui.Scroll(c.Key("options-scroll")).MaxHeight(280).Children(func() {
				ui.Column(c).Gap(2).Children(func() {
					matches := 0
					for _, option := range options {
						if searchable && !strings.Contains(strings.ToLower(option.label), strings.ToLower(strings.TrimSpace(*filter))) {
							continue
						}
						matches++
						item := controlSelectItem(c, sel.Item(option.value)).Label(a.settingOptionText(c, label, option.value, option.label))
						if searchable {
							item.Label(tr(c, "Select ") + tr(c, label) + " " + a.settingOptionText(c, label, option.value, option.label))
						}
						item.Children(func() {
							ui.Text(c, a.settingOptionText(c, label, option.value, option.label)).Grow(1).SingleLine()
							if option.value == selected {
								icon(c, "check", 13)
							}
						})
					}
					if matches == 0 {
						ui.Text(c, tr(c, "No matching fonts")).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Faint).Padding(6, 8)
					}
				})
			})
		})
	})
}

// settingsEffortTheme chooses the device-local reasoning-slider preset. The
// trigger and every row carry the preset's three-stop swatch.
func (a *App) settingsEffortTheme(c *ui.Context) {
	value := &a.state.Settings.Appearance.EffortTheme
	current := contracts.NormalizeEffortTheme(*value)
	sel := controlSelect(c.Key("Reasoning theme"), value)
	sel.Trigger.Label(tr(c, "Choose ") + tr(c, "Reasoning theme")).MinWidth(148).OnChange(a.saveSettings).Children(func() {
		controlEffortSwatch(c, design.EffortPaletteFor(current, a.palette))
		ui.Text(c, tr(c, design.EffortThemeLabel(current))).Grow(1).SingleLine()
		icon(c, "alt-arrow-down", 13)
	})
	sel.Popup(func(panel ui.Element) {
		controlSelectPopup(c, panel).MinWidth(200).Children(func() {
			ui.Scroll(c.Key("effort-theme-options")).MaxHeight(280).Children(func() {
				ui.Column(c).Gap(2).Children(func() {
					for _, theme := range contracts.EffortThemeValues() {
						theme := theme
						label := tr(c, design.EffortThemeLabel(theme))
						item := controlSelectItem(c, sel.Item(theme)).Label(tr(c, "Select ") + tr(c, "Reasoning theme") + " " + label)
						item.Children(func() {
							controlEffortSwatch(c, design.EffortPaletteFor(theme, a.palette))
							ui.Text(c, label).Grow(1).SingleLine()
							if theme == current {
								icon(c, "check", 13)
							}
						})
					}
				})
			})
		})
	})
}

func stringOptions(values ...string) []settingOption {
	out := make([]settingOption, len(values))
	for i, v := range values {
		out[i] = settingOption{v, v}
	}
	return out
}

func (a *App) settingsRow(c *ui.Context, title, description string, control func(), leading ...func()) ui.Element {
	return a.settingsRowWithText(c, title, tr(c, title), tr(c, description), control, leading...)
}

func (a *App) settingsDataRow(c *ui.Context, title, description string, control func(), leading ...func()) ui.Element {
	return a.settingsRowWithText(c, title, title, description, control, leading...)
}

func (a *App) settingsRowWithText(c *ui.Context, key, title, description string, control func(), leading ...func()) ui.Element {
	row := ui.Row(c.Key("setting-row-"+key)).FillWidth().MinHeight(60).AlignItems(ui.Center).Gap(16).Padding(12, 16)
	started := ui.Local(row, "search-reveal", func() time.Time { return time.Time{} })
	if a.expanded["settings-target:"+key] {
		delete(a.expanded, "settings-target:"+key)
		row.ScrollIntoView()
		*started = c.Now()
	}
	if !started.IsZero() {
		elapsed := c.Now().Sub(*started)
		if elapsed < 1100*time.Millisecond {
			row.Background(a.palette.Accent.Alpha(.12))
			c.After(1100*time.Millisecond - elapsed)
		}
	}
	row.Children(func() {
		for _, draw := range leading {
			draw()
		}
		ui.Column(c).Grow(1).Gap(3).Children(func() {
			ui.Text(c, title).FontSize(design.TextSize(c, design.TextBase)).FontWeight(500).LineHeight(1.5)
			if description != "" {
				ui.Text(c, description).FontSize(design.TextSize(c, design.TextBase)).TextColor(c.Theme().TextMuted)
			}
		})
		control()
	})
	return row
}

func (a *App) settingsMatchLabel(c *ui.Context, title, query string) {
	title = tr(c, title)
	lower := strings.ToLower(title)
	start, end := -1, -1
	for _, word := range strings.Fields(query) {
		if at := strings.Index(lower, word); at >= 0 {
			start, end = at, at+len(word)
			break
		}
	}
	if start < 0 {
		ui.Text(c, tr(c, title)).FontWeight(600)
		return
	}
	ui.RichText(c, ui.Span{Text: title[:start]}, ui.Span{Text: title[start:end], Color: a.palette.Accent, Background: a.palette.Accent.Alpha(.24), Radius: 2}, ui.Span{Text: title[end:]}).FontWeight(600)
}

// Matches the original settings index policy: all words must match, title
// matches rank first, and edit-distance fallback is suppressed for real words.
func settingsSearchHits(links []settingLink, query string) []settingLink {
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return nil
	}
	misspelled := make([]bool, len(words))
	for i, word := range words {
		misspelled[i] = true
		for _, link := range links {
			if strings.Contains(strings.ToLower(link.title+" "+link.description+" "+link.section), word) {
				misspelled[i] = false
				break
			}
		}
	}
	type ranked struct {
		link  settingLink
		score int
	}
	var rankedHits []ranked
	for _, link := range links {
		score := 0
		matched := true
		for i, word := range words {
			best := 0
			for _, field := range []struct {
				text   string
				weight int
			}{{link.title, 100}, {link.description, 40}, {link.section, 20}} {
				text := strings.ToLower(field.text)
				if at := strings.Index(text, word); at >= 0 {
					weight := field.weight
					if at == 0 || text[at-1] == ' ' || text[at-1] == '&' {
						weight += weight / 2
					}
					best = max(best, weight)
				}
			}
			if best == 0 && misspelled[i] && (settingsTypo(strings.ToLower(link.title), word) || settingsTypo(strings.ToLower(link.section), word)) {
				best = 10
			}
			if best == 0 {
				matched = false
				break
			}
			score += best
		}
		if matched {
			if strings.ToLower(link.title) == strings.Join(words, " ") {
				score += 100
			}
			rankedHits = append(rankedHits, ranked{link, score})
		}
	}
	sort.SliceStable(rankedHits, func(i, j int) bool { return rankedHits[i].score > rankedHits[j].score })
	result := make([]settingLink, len(rankedHits))
	for i, hit := range rankedHits {
		result[i] = hit.link
	}
	return result
}

func settingsTypo(field, word string) bool {
	if len(word) < 4 {
		return false
	}
	budget := 1
	if len(word) >= 8 {
		budget = 2
	}
	for _, candidate := range strings.FieldsFunc(field, func(r rune) bool { return (r < 'a' || r > 'z') && (r < '0' || r > '9') }) {
		for _, length := range []int{len(candidate), len(word) - 1, len(word), len(word) + 1} {
			if length >= 0 && length <= len(candidate) && settingsDistance(word, candidate[:length], budget) {
				return true
			}
		}
	}
	return false
}

func settingsDistance(a, b string, budget int) bool {
	if len(a)-len(b) > budget || len(b)-len(a) > budget {
		return false
	}
	width := len(b) + 1
	rows := make([]int, 3*width)
	before, last, current := 0, width, 2*width
	for j := 0; j < width; j++ {
		rows[last+j] = j
	}
	for i := 1; i <= len(a); i++ {
		rows[current] = i
		rowMin := i
		for j := 1; j <= len(b); j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			cell := min(rows[last+j]+1, rows[current+j-1]+1, rows[last+j-1]+cost)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				cell = min(cell, rows[before+j-2]+1)
			}
			rows[current+j] = cell
			rowMin = min(rowMin, cell)
		}
		if rowMin > budget {
			return false
		}
		before, last, current = last, current, before
	}
	return rows[last+len(b)] <= budget
}

func (a *App) shortcutRecording() bool {
	for key, active := range a.expanded {
		if active && strings.HasPrefix(key, "shortcut:") {
			return true
		}
	}
	return false
}

func (a *App) clearShortcutRecording() {
	for key := range a.expanded {
		if strings.HasPrefix(key, "shortcut:") {
			delete(a.expanded, key)
		}
	}
}

func (a *App) registerSettingsShortcuts(c *ui.Context) {
	if a.shortcutRecording() {
		return
	}
	for _, binding := range a.state.Settings.Shortcuts {
		mods, key, ok := parseSettingsShortcut(binding.Keys)
		if !ok {
			continue
		}
		var action func()
		switch binding.ID {
		case "newSession":
			action = a.newConversation
		case "commandPalette":
			action = a.toggleCommandPalette
		case "settings":
			action = func() { a.navigate("settings") }
		case "toggleSidebar":
			action = func() { a.togglePanel(0) }
		case "files":
			action = func() { a.togglePanel(2) }
		case "terminal":
			action = a.toggleTerminal
		case "find":
			action = func() { a.expanded["conversation-find"] = !a.expanded["conversation-find"] }
		case "stop":
			if key == ui.KeyEscape && !a.state.Settings.General.StopWithEscape {
				continue
			}
			if key == ui.KeyEscape {
				focused := a.composer.Focused(c)
				for _, side := range a.sideComposers {
					focused = focused || side.Input.Focused(c)
				}
				if focused {
					continue
				}
			}
			action = a.stopRun
		}
		if action != nil {
			c.OnShortcut(mods, key, action)
		}
	}
}

func parseSettingsShortcut(value string) (ui.Modifiers, ui.Key, bool) {
	parts := strings.Split(value, "+")
	if len(parts) == 0 {
		return 0, ui.KeyUnknown, false
	}
	var mods ui.Modifiers
	for _, part := range parts[:len(parts)-1] {
		switch strings.ToLower(strings.TrimSpace(part)) {
		case "mod", "cmd", "command", "⌘":
			mods |= ui.Cmd
		case "ctrl", "control":
			mods |= ui.Ctrl
		case "alt", "option", "⌥":
			mods |= ui.Alt
		case "shift", "⇧":
			mods |= ui.Shift
		case "super", "meta":
			mods |= ui.Super
		default:
			return 0, ui.KeyUnknown, false
		}
	}
	name := strings.ToUpper(strings.TrimSpace(parts[len(parts)-1]))
	var key ui.Key
	if len(name) == 1 && name[0] >= 'A' && name[0] <= 'Z' {
		key = ui.KeyA + ui.Key(name[0]-'A')
	} else if len(name) == 1 && name[0] >= '0' && name[0] <= '9' {
		key = ui.Key0 + ui.Key(name[0]-'0')
	} else {
		key = map[string]ui.Key{"ENTER": ui.KeyEnter, "RETURN": ui.KeyEnter, "ESC": ui.KeyEscape, "ESCAPE": ui.KeyEscape, "SPACE": ui.KeySpace, "TAB": ui.KeyTab, "DELETE": ui.KeyDelete, "BACKSPACE": ui.KeyBackspace, "HOME": ui.KeyHome, "END": ui.KeyEnd, "PAGEUP": ui.KeyPageUp, "PAGEDOWN": ui.KeyPageDown, "LEFT": ui.KeyLeft, "RIGHT": ui.KeyRight, "UP": ui.KeyUp, "DOWN": ui.KeyDown, ",": ui.KeyComma, ".": ui.KeyPeriod, "/": ui.KeySlash, "`": ui.KeyBackquote, "-": ui.KeyMinus, "=": ui.KeyEqual, "F1": ui.KeyF1, "F2": ui.KeyF2, "F3": ui.KeyF3, "F4": ui.KeyF4, "F5": ui.KeyF5, "F6": ui.KeyF6, "F7": ui.KeyF7, "F8": ui.KeyF8, "F9": ui.KeyF9, "F10": ui.KeyF10, "F11": ui.KeyF11, "F12": ui.KeyF12}[name]
	}
	return mods, key, key != ui.KeyUnknown
}

func shortcutKeyName(key ui.Key) string {
	if key >= ui.KeyA && key <= ui.KeyZ {
		return string(rune('A' + key - ui.KeyA))
	}
	if key >= ui.Key0 && key <= ui.Key9 {
		return string(rune('0' + key - ui.Key0))
	}
	if key >= ui.KeyF1 && key <= ui.KeyF12 {
		return fmt.Sprintf("F%d", key-ui.KeyF1+1)
	}
	return map[ui.Key]string{ui.KeyEnter: "Enter", ui.KeySpace: "Space", ui.KeyTab: "Tab", ui.KeyDelete: "Delete", ui.KeyHome: "Home", ui.KeyEnd: "End", ui.KeyPageUp: "PageUp", ui.KeyPageDown: "PageDown", ui.KeyLeft: "Left", ui.KeyRight: "Right", ui.KeyUp: "Up", ui.KeyDown: "Down", ui.KeyComma: ",", ui.KeyPeriod: ".", ui.KeySlash: "/", ui.KeyBackquote: "`", ui.KeyMinus: "-", ui.KeyEqual: "="}[key]
}

func recordedShortcut(event ui.InputEvent) string {
	key := event.Key
	if event.PhysicalKey != ui.KeyUnknown {
		key = event.PhysicalKey
	}
	name := shortcutKeyName(key)
	if name == "" {
		return ""
	}
	var parts []string
	if event.Mods&ui.Cmd != 0 {
		parts = append(parts, "Mod")
	}
	if runtime.GOOS == "darwin" && event.Mods&ui.Ctrl != 0 {
		parts = append(parts, "Ctrl")
	}
	if runtime.GOOS != "darwin" && event.Mods&ui.Super != 0 {
		parts = append(parts, "Super")
	}
	if event.Mods&ui.Alt != 0 {
		parts = append(parts, "Alt")
	}
	if event.Mods&ui.Shift != 0 {
		parts = append(parts, "Shift")
	}
	return strings.Join(append(parts, name), "+")
}

func (a *App) generalSettings(c *ui.Context) {
	g := &a.state.Settings.General
	a.settingsCard(c, func() {
		a.settingsRow(c, "Language", "", func() {
			a.settingsChoice(c, "Language", &g.Language, []settingOption{{"zh-CN", "简体中文"}, {"en", "English"}}, nil)
		})
	})
	a.settingsCard(c, func() {
		a.settingsRow(c, "Send messages with", "", func() {
			modifier := "Ctrl + Enter"
			if runtime.GOOS == "darwin" {
				modifier = "Cmd + Enter"
			}
			a.settingsChoice(c, "Send messages with", &g.SendBehavior, []settingOption{{"enter", "Enter"}, {"modEnter", modifier}}, nil)
		})
		line(c)
		a.settingsRow(c, "While a response is running", activeTurnSendDescription(g.ActiveTurnSendBehavior), func() {
			a.settingsChoice(c, "While a response is running", &g.ActiveTurnSendBehavior,
				[]settingOption{{"steer", "Steer the running response"}, {"queue", "Queue for the next turn"}}, nil)
		})
		line(c)
		a.settingsRow(c, "Compact mode", "Collapse thinking and tools.", func() { a.settingsToggle(c, "Compact mode", &g.CompactMode) })
		line(c)
		a.settingsRow(c, "Stop agent with Escape", "When no dialog or menu is open.", func() { a.settingsToggle(c, "Stop agent with Escape", &g.StopWithEscape) })
	})
	a.settingsCard(c, func() {
		description := "Uses an additional model request for each new conversation."
		if g.ThreadNamingModel == "first-message" {
			description = "Use the first message without an additional model request."
		}
		a.settingsRow(c, "Thread naming", description, func() {
			if g.ThreadNamingModel != "first-message" {
				textButton(c, tr(c, "Reset")).Label(tr(c, "Reset thread naming")).OnClick(func() { g.ThreadNamingModel = "first-message"; a.saveSettings() })
			}
			options := []settingOption{{"first-message", "First message"}}
			for _, model := range a.state.Models {
				options = append(options, settingOption{model.ID, model.Label})
			}
			a.settingsChoice(c, "Thread naming model", &g.ThreadNamingModel, options, nil)
		})
	})
}

func (a *App) appearanceSettings(c *ui.Context) {
	p := &a.state.Settings.Appearance
	sectionTitle(c, tr(c, "Color scheme"))
	choices := ui.Row(c.Key("color-scheme")).FillWidth().Gap(16)
	if a.expanded["settings-target:Color scheme"] {
		choices.ScrollIntoView()
		delete(a.expanded, "settings-target:Color scheme")
	}
	choices.Children(func() {
		for _, choice := range []struct{ value, label, icon string }{{"system", "System", "monitor"}, {"light", "Light", "sun"}, {"dark", "Dark", "moon"}} {
			choice := choice
			selected := strings.EqualFold(p.Mode, choice.value)
			border := a.palette.Border
			labelColor := a.palette.Muted
			if selected {
				border = a.palette.Accent
				labelColor = a.palette.Accent
			}
			controlPreviewButton(c.Key("appearance-mode-"+choice.value), tr(c, "Appearance ")+tr(c, choice.label)).Grow(1).MinWidth(0).OnClick(func() { p.Mode = choice.value; p.ThemeID = ""; a.saveSettings() }).Children(func() {
				ui.Column(c).FillWidth().Gap(8).Children(func() {
					preview := ui.Box(c).FillWidth().Height(148).Radius(6).Border(1, border).Clip()
					light := design.ApplyAccent(design.Palette(p.LightThemeID, false), p.Accent)
					dark := design.ApplyAccent(design.Palette(p.DarkThemeID, true), p.Accent)
					preview.Draw(func(painter *ui.Painter, r ui.Rect) {
						switch choice.value {
						case "light":
							drawAppearanceMiniature(painter, r, light)
						case "dark":
							drawAppearanceMiniature(painter, r, dark)
						default:
							half := r
							half.W /= 2
							drawAppearanceMiniature(painter, half, light)
							half.X += half.W
							drawAppearanceMiniature(painter, half, dark)
						}
					})
					ui.Row(c).AlignSelf(ui.Center).Gap(6).TextColor(labelColor).Children(func() {
						icon(c, choice.icon, 16)
						ui.Text(c, tr(c, choice.label)).FontSize(design.TextSize(c, design.TextBase))
					})
				})
			})
		}
	})
	a.settingsCard(c, func() {
		for index, isDark := range []bool{false, true} {
			dark := isDark
			title := "Light theme"
			value := &p.LightThemeID
			if dark {
				title = "Dark theme"
				value = &p.DarkThemeID
			}
			if index > 0 {
				line(c)
			}
			a.settingsRow(c, title, "", func() {
				var options []settingOption
				for _, theme := range design.Themes() {
					if (theme.Appearance == "dark") == dark {
						options = append(options, settingOption{theme.ID, theme.Name})
					}
				}
				a.settingsChoice(c, title, value, options, func() { p.ThemeID = "" })
			})
		}
		line(c)
		a.settingsRow(c, "Reasoning theme", "", func() { a.settingsEffortTheme(c) })
		line(c)
		description := "Theme default"
		if p.Accent != "" && p.Accent != "theme" {
			description = strings.ToUpper(p.Accent[:1]) + p.Accent[1:]
		}
		a.settingsRow(c, "Accent color", description, func() {
			ui.Row(c).Gap(8).Children(func() {
				for _, selection := range []string{"theme", "rcode", "orange", "amber", "green", "cyan", "blue", "pink"} {
					selection := selection
					tokens := design.ApplyAccent(a.palette, selection)
					b := controlAccentSwatch(c.Key("accent-"+selection), tr(c, "Accent ")+tr(c, selection), tokens, p.Accent == selection, selection == "theme")
					b.OnClick(func() { p.Accent = selection; a.saveSettings() })
				}
			})
		})
	})
	sectionTitle(c, tr(c, "Material and background"))
	a.settingsCard(c, func() {
		a.settingsRow(c, "Glass", "", func() {
			a.settingsChoice(c, "Glass", &p.Surface, []settingOption{{"theme", "Theme default"}, {"Frosted", "Frosted"}, {"Opaque", "Opaque"}}, nil)
		})
		line(c)
		description := tr(c, "Use wallpaper colors for accents, highlights, and subtle surface tints.")
		if p.WallpaperColors && a.wallpaper.err != "" {
			description = a.wallpaper.err
		}
		a.settingsRow(c, "Match wallpaper colors", description, func() { a.settingsToggle(c, "Match wallpaper colors", &p.WallpaperColors) })
		line(c)
		a.backgroundSettings(c)
	})
	sectionTitle(c, tr(c, "Motion"))
	a.settingsCard(c, func() {
		description := "Following the system, which currently allows motion."
		switch p.MotionPreference {
		case "on":
			description = "Animations skip straight to their final state."
		case "off":
			description = "Animations play even if the system asks for less motion."
		}
		a.settingsRow(c, "Reduce motion", description, func() {
			a.settingsChoice(c, "Reduce motion", &p.MotionPreference, []settingOption{{"system", "System"}, {"on", "On"}, {"off", "Off"}}, func() { p.ReduceMotion = p.MotionPreference == "on" })
		})
		line(c)
		a.settingsRow(c, "Pause animations in background", "Hold animations still while GCode isn't the focused window.", func() { a.settingsToggle(c, "Pause animations in background", &p.PauseInBackground) })
	})
	a.themeLibrary(c)
	sectionTitle(c, tr(c, "Fonts and layout"))
	a.settingsCard(c, func() {
		a.settingsRow(c, "Interface font", "Menus and conversations", func() {
			a.settingsChoice(c, "Interface font", &p.Font, settingsFontOptions(false), nil)
			a.settingsSizeChoice(c, "Interface font size", &p.FontSize, []float64{12, 13, 14, 15, 16, 18, 20})
		})
		line(c)
		a.settingsRow(c, "Terminal font", "Terminal output · monospace only", func() {
			a.settingsChoice(c, "Terminal font", &p.TerminalFont, settingsFontOptions(true), nil)
			a.settingsSizeChoice(c, "Terminal font size", &p.TerminalFontSize, fontSizeValues())
		})
		line(c)
		a.settingsRow(c, "Code & diff font", "Code, diffs, and files", func() {
			a.settingsChoice(c, "Code & diff font", &p.CodeFont, settingsFontOptions(false), nil)
			a.settingsSizeChoice(c, "Code font size", &p.CodeFontSize, fontSizeValues())
		})
		line(c)
		a.settingsRow(c, "Conversation width", "Maximum width for messages and the composer.", func() {
			controlStepSlider(c.Key("conversation-width"), &p.TranscriptWidth, 560, 1200, 16).Width(150).Label(tr(c, "Conversation width")).OnChange(a.saveSettings)
			textButton(c, tr(c, "Reset")).OnClick(func() { p.TranscriptWidth = 736; a.saveSettings() })
		})
		line(c)
		a.settingsRow(c, "Render math", "Typeset LaTeX in messages. Off shows the source as code.", func() { a.settingsToggle(c, "Render math", &p.RenderMath) })
		line(c)
		a.settingsRow(c, "Math font", mathFontDescription(p.MathFont), func() {
			a.settingsChoice(c, "Math font", &p.MathFont, stringOptions("Fira Math", "STIX Two", "Computer Modern", "Libertinus", "Palatino", "Euler"), nil)
		})
		line(c)
		a.settingsRow(c, "Math size", "Relative to the text around it.", func() { a.settingsSizeChoice(c, "Math size", &p.MathScale, []float64{90, 100, 110, 120, 135, 150}) })
	})
}

func mathFontDescription(font string) string {
	switch font {
	case "STIX Two":
		return "A Times-like serif face with the TeX look."
	case "Computer Modern":
		return "A serif face with the classic LaTeX look."
	case "Libertinus":
		return "A warm serif book face."
	case "Palatino":
		return "A Palatino serif face, from TeX Gyre Pagella."
	case "Euler":
		return "Zapf's upright, hand-drawn math."
	default:
		return "A sans face that sits beside the interface font."
	}
}

func drawAppearanceMiniature(p *ui.Painter, r ui.Rect, t design.Tokens) {
	p.Fill(r, t.Shell, 6)
	for i, fraction := range []float32{.7, 1, .85, 1} {
		tone := t.Text.Alpha(.22)
		if i == 0 {
			tone = t.Text.Alpha(.34)
		}
		p.Fill(ui.Rect{X: r.X + 8, Y: r.Y + 14 + float32(i)*12, W: 28 * fraction, H: 5}, tone, 3)
	}
	content := ui.Rect{X: r.X + 44, Y: r.Y + 8, W: max(0, r.W-52), H: r.H - 16}
	p.Fill(content, t.Background, 6)
	p.Stroke(content, t.Border, 6, 1)
	for i, fraction := range []float32{.62, .88, .76, .52} {
		tone := t.Text.Alpha(.22)
		if i == 0 {
			tone = t.Text.Alpha(.34)
		}
		p.Fill(ui.Rect{X: content.X + 10, Y: content.Y + 10 + float32(i)*12, W: max(0, content.W-20) * fraction, H: 5}, tone, 3)
	}
}

func settingsFontOptions(mono bool) []settingOption {
	var options []settingOption
	if !mono {
		options = append(options, settingOption{"system", "System UI"})
	}
	for _, family := range ui.FontFamilies() {
		if !mono || family.Monospaced {
			options = append(options, settingOption{family.Name, family.Name})
		}
	}
	return options
}

func fontSizeValues() []float64 {
	var values []float64
	for size := float64(8); size <= 32; size += .5 {
		values = append(values, size)
	}
	return values
}
func (a *App) settingsSizeChoice(c *ui.Context, label string, value *float64, values []float64) {
	selected := *value
	unit := " px"
	if label == "Math size" {
		unit = "%"
	}
	sel := controlSelect(c.Key(label), value)
	sel.Trigger.Label(tr(c, "Choose ") + tr(c, label)).OnChange(a.saveSettings).Children(func() { ui.Text(c, fmt.Sprintf(tr(c, "%g%s"), selected, unit)); icon(c, "alt-arrow-down", 13) })
	sel.Popup(func(panel ui.Element) {
		controlSelectPopup(c, panel).Children(func() {
			ui.Scroll(c).MaxHeight(280).Children(func() {
				for _, size := range values {
					item := controlSelectItem(c, sel.Item(size))
					item.Children(func() { ui.Text(c, fmt.Sprintf(tr(c, "%g%s"), size, unit)) })
				}
			})
		})
	})
}

type themeDialogState struct {
	Open       bool
	Path, Mode string
}

func (a *App) themeLibrary(c *ui.Context) {
	owner := ui.Column(c.Key("theme-library")).FillWidth().Radius(10).Background(a.palette.Card)
	state := ui.Local(owner, "import", func() themeDialogState { return themeDialogState{Mode: "imported"} })
	owner.Children(func() {
		a.settingsRow(c, "Theme library", "", func() {
			textButton(c, tr(c, "Add theme")).OnClick(func() { state.Open = true; state.Path = ""; state.Mode = "imported" })
		})
		for _, kind := range []string{"imported", "linked", "editable"} {
			var entries []contracts.ThemeLibraryEntry
			for _, entry := range a.state.ThemeLibrary {
				if entry.SourceKind == kind {
					entries = append(entries, entry)
				}
			}
			if len(entries) == 0 {
				continue
			}
			line(c)
			label := "Imported"
			if kind != "imported" {
				label = "Linked"
			}
			ui.Text(c, tr(c, label)).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted).Margin(12, 16, 4, 16)
			for _, entry := range entries {
				entry := entry
				ui.Row(c.Key("theme-entry-"+entry.ID)).Padding(10, 16).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
						ui.Text(c, entry.Name).FontSize(design.TextSize(c, design.TextBase)).FontWeight(600).SingleLine()
						ui.Text(c, entry.Path).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted).SingleLine()
						if entry.Warning != "" {
							ui.Text(c, entry.Warning).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Warning)
						}
					})
					if kind != "imported" {
						textButton(c, tr(c, "Reload")).Label(tr(c, "Reload theme ") + entry.ID).OnClick(func() { a.dispatch(contracts.Command{Kind: "theme.reload", ID: entry.ID}) })
					}
					textButton(c, tr(c, "Duplicate as editable")).Label(tr(c, "Duplicate theme ") + entry.ID).OnClick(func() { a.dispatch(contracts.Command{Kind: "theme.duplicate", ID: entry.ID}) })
					textButton(c, tr(c, "Remove")).Label(tr(c, "Remove theme ") + entry.ID).OnClick(func() { a.dispatch(contracts.Command{Kind: "theme.remove", ID: entry.ID}) })
				})
			}
		}
	})
	ui.DialogBase(c.Key("add-theme-dialog"), &state.Open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.Hex("#000000").Alpha(.55))
		panel.Width(640).MaxWidth(760).Padding(20).Radius(12).Gap(16)
		a.floatingSurface(c, panel, 12)
		ui.Row(c).Gap(16).AlignItems(ui.Start).Children(func() {
			ui.Column(c).Grow(1).Gap(4).Children(func() {
				ui.Text(c, tr(c, "Add a theme")).FontSize(design.TextSize(c, design.TextTitle)).FontWeight(600)
				ui.Text(c, tr(c, "Import a local theme into your library or keep it linked to its source.")).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted)
			})
			iconButton(c, "close", tr(c, "Close theme import")).OnClick(func() { state.Open = false })
		})
		ui.Row(c).Gap(10).Children(func() {
			for _, mode := range []struct{ id, title, description string }{{"imported", "Import a copy", "Keep a self-contained snapshot."}, {"linked", "Link to file", "Reload changes from the file on disk."}} {
				mode := mode
				b := controlTileButton(c.Key(mode.id), state.Mode == mode.id).Grow(1).Label(tr(c, mode.title))
				b.Children(func() {
					ui.Column(c).Gap(4).Children(func() {
						ui.Text(c, tr(c, mode.title)).FontWeight(600)
						ui.Text(c, tr(c, mode.description)).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
					})
				})
				b.OnClick(func() { state.Mode = mode.id })
			}
		})
		ui.Row(c).Gap(8).Children(func() {
			controlTextInput(c.Key("theme-source-path"), &state.Path).Placeholder(tr(c, "Theme file, package.json, or extension folder")).Label(tr(c, "Theme source path")).Grow(1).AutoFocus()
			textButton(c, tr(c, "Browse")).OnClick(func() {
				if a.win == nil {
					return
				}
				paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: "Choose Theme Source"})
				if err != nil {
					a.notification(err.Error())
				} else if len(paths) > 0 {
					state.Path = paths[0]
				}
			})
		})
		preview := a.state.ThemeImport
		if preview != nil && preview.Path == state.Path {
			for _, variant := range preview.Variants {
				variant := variant
				selected := variant.Selected
				controlCheckbox(c.Key("theme-import-"+variant.ID), &selected, variant.Name).OnClick(func() { a.dispatch(contracts.Command{Kind: "theme.select", ID: variant.ID, Enabled: selected}) })
			}
			for _, warning := range preview.Warnings {
				ui.Text(c, warning).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Warning)
			}
			if preview.Error != "" {
				ui.Text(c, preview.Error).TextColor(a.palette.Danger).FontSize(design.TextSize(c, design.TextBase))
			}
		}
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			textButton(c, tr(c, "Cancel")).Label(tr(c, "Cancel theme import")).OnClick(func() { state.Open = false })
			label := "Analyze theme"
			ready := false
			if preview != nil && preview.Path == state.Path && len(preview.Variants) > 0 {
				label = "Import selected"
				ready = preview.Ready
			}
			primaryButton(c, tr(c, label)).Disabled(label == "Import selected" && !ready).OnClick(func() {
				if label == "Analyze theme" {
					a.dispatch(contracts.Command{Kind: "theme.analyze", Path: state.Path, Name: state.Mode})
				} else {
					a.dispatch(contracts.Command{Kind: "theme.import", Name: state.Mode})
					state.Open = false
				}
			})
		})
	})
}
func (a *App) notificationSettings(c *ui.Context) {
	n := &a.state.Settings.Notifications
	sectionTitle(c, tr(c, "Desktop"))
	a.settingsCard(c, func() {
		a.settingsRow(c, "Desktop notifications", "", func() { a.settingsToggle(c, "Desktop notifications", &n.Enabled) })
		line(c)
		a.settingsRow(c, "Only when in the background", "", func() {
			on := !n.WhenFocused
			a.settingsToggle(c, "Only when in the background", &on, func() { n.WhenFocused = !on })
		}).Disabled(!n.Enabled).Opacity(notificationRowOpacity(n.Enabled))
	})
	sectionTitle(c, tr(c, "Sounds"))
	a.settingsCard(c, func() {
		a.settingsRow(c, "Session sounds", "", func() { a.settingsToggle(c, "Session sounds", &n.Sound) })
		line(c)
		a.settingsRow(c, "Task completed", "", func() { a.settingsToggle(c, "Task completed", &n.Completion) }).Disabled(!n.Sound).Opacity(notificationRowOpacity(n.Sound))
		line(c)
		a.settingsRow(c, "Input required", "", func() { a.settingsToggle(c, "Input required", &n.Questions) }).Disabled(!n.Sound).Opacity(notificationRowOpacity(n.Sound))
		line(c)
		a.settingsRow(c, "Errors and disconnections", "", func() { a.settingsToggle(c, "Errors and disconnections", &n.Errors) }).Disabled(!n.Sound).Opacity(notificationRowOpacity(n.Sound))
	})
}

func notificationRowOpacity(enabled bool) float32 {
	if enabled {
		return 1
	}
	return .55
}

func (a *App) shortcutSettings(c *ui.Context) {
	last := ""
	for i := range a.state.Settings.Shortcuts {
		shortcut := &a.state.Settings.Shortcuts[i]
		if shortcut.Group != last {
			sectionTitle(c, shortcut.Group)
			last = shortcut.Group
		}
		s := shortcut
		a.settingsCard(c, func() {
			a.settingsRow(c, s.Label, "", func() { a.recordShortcut(c, s.ID, s.Label, &s.Keys) })
		})
	}
}

func (a *App) recordShortcut(c *ui.Context, id, label string, binding *string) {
	if a.expanded["shortcut:"+id] {
		ui.Box(c.Key("shortcut-recorder-"+id)).Width(178).MinHeight(30).Padding(7, 10).Radius(8).Background(a.palette.Raised).Focusable().AutoFocus().Label(tr(c, "Record shortcut for ") + tr(c, label)).HandleInput(func(event ui.InputEvent) bool {
			if event.Kind != ui.InputKeyDown || event.Repeat {
				return false
			}
			if event.Key == ui.KeyEscape {
				a.expanded["shortcut:"+id] = false
				return true
			}
			if event.Key == ui.KeyBackspace && event.Mods == 0 {
				*binding = ""
				a.expanded["shortcut:"+id] = false
				a.saveSettings()
				return true
			}
			keys := recordedShortcut(event)
			if keys == "" {
				return true
			}
			mods, key, _ := parseSettingsShortcut(keys)
			for _, other := range a.state.Settings.Shortcuts {
				otherMods, otherKey, valid := parseSettingsShortcut(other.Keys)
				if other.ID != id && valid && otherMods == mods && otherKey == key {
					a.notification("That shortcut is already in use.")
					return true
				}
			}
			if id != "captureAppshot" {
				otherMods, otherKey, valid := parseSettingsShortcut(a.state.Settings.Appshots.Shortcut)
				if a.state.Capabilities.Appshots && valid && otherMods == mods && otherKey == key {
					a.notification("That shortcut is already in use.")
					return true
				}
			}
			*binding = keys
			a.expanded["shortcut:"+id] = false
			a.saveSettings()
			return true
		}).Children(func() {
			ui.Text(c, tr(c, "Press a shortcut…")).TextColor(a.palette.Muted).FontSize(design.TextSize(c, design.TextBase))
		})
		textButton(c, tr(c, "Cancel")).OnClick(func() { a.expanded["shortcut:"+id] = false })
		return
	}
	text := strings.ReplaceAll(*binding, "Mod", map[bool]string{true: "⌘", false: "Ctrl"}[runtime.GOOS == "darwin"])
	if text == "" {
		text = "Add shortcut"
	}
	textButton(c, tr(c, text)).Label(tr(c, "Edit shortcut for ") + tr(c, label)).OnClick(func() {
		for key := range a.expanded {
			if strings.HasPrefix(key, "shortcut:") {
				a.expanded[key] = false
			}
		}
		a.expanded["shortcut:"+id] = true
	})
}
func (a *App) fileSettings(c *ui.Context) {
	f := &a.state.Settings.Files
	a.settingsCard(c, func() {
		a.settingsRow(c, "Autosave", "", func() { a.settingsToggle(c, "Autosave", &f.Autosave) })
		if f.Autosave {
			line(c)
			a.settingsRow(c, "Autosave delay", "", func() {
				sel := controlSelect(c.Key("autosave-delay"), &f.AutosaveDelay)
				sel.Trigger.Label(tr(c, "Choose Autosave delay")).OnChange(a.saveSettings).Children(func() {
					ui.Text(c, autosaveDelayLabel(f.AutosaveDelay, a.state.Settings.General.Language))
					icon(c, "alt-arrow-down", 13)
				})
				sel.Popup(func(panel ui.Element) {
					controlSelectPopup(c, panel).Children(func() {
						for _, delay := range []int{300, 600, 900, 1500, 3000} {
							item := controlSelectItem(c, sel.Item(delay))
							item.Children(func() { ui.Text(c, autosaveDelayLabel(delay, a.state.Settings.General.Language)) })
						}
					})
				})
			})
		}
		line(c)
		a.settingsRow(c, "Word wrap", "", func() { a.settingsToggle(c, "Word wrap", &f.WordWrap) })
		line(c)
		a.settingsRow(c, "Show hidden and ignored files", "", func() { a.settingsToggle(c, "Show hidden and ignored files", &f.ShowHidden) })
	})
}

func autosaveDelayLabel(delay int, language ...string) string {
	lang := "en"
	if len(language) > 0 {
		lang = language[0]
	}
	if delay >= 1000 {
		return i18n.In(lang, "%g s", float64(delay)/1000)
	}
	return i18n.In(lang, "%d ms", delay)
}

func (a *App) appshotSettings(c *ui.Context) {
	s := &a.state.Settings.Appshots
	capabilities := a.state.AppshotCapabilities
	ui.Text(c, tr(c, "Set up once, one permission at a time. Screen Recording captures the window; Accessibility optionally adds off-screen application text.")).FontSize(design.TextSize(c, design.TextBase)).LineHeight(1.5).TextColor(a.palette.Muted).Margin(0, 8, 8, 8)
	a.settingsCard(c, func() {
		a.settingsRow(c, "Capture Appshots", "Captures are staged for review and never sent automatically.", func() { a.settingsToggle(c, "Capture Appshots", &s.Enabled) })
		line(c)
		a.settingsRow(c, "Capture sound", "Play a sound when an Appshot is ready.", func() { a.settingsToggle(c, "Capture sound", &s.Sound) })
		line(c)
		description := "The shortcut works while another application has focus."
		if capabilities.GlobalShortcut != "ready" {
			description = "This shortcut is unavailable. Choose a different key combination."
		}
		a.settingsRow(c, "Global shortcut", description, func() {
			a.appshotCapabilityBadge(c, capabilities.GlobalShortcut)
			a.recordShortcut(c, "captureAppshot", "Global shortcut", &s.Shortcut)
		})
		line(c)
		description = "Use the open session, or the new-session composer when no session is open."
		switch s.Destination {
		case "lastSession":
			description = "Use the open session, or return to the last session used for an Appshot."
		case "newSession":
			description = "Stage captures in a new-session composer, keeping existing drafts intact."
		}
		a.settingsRow(c, "Destination", description, func() {
			a.settingsChoice(c, "Appshot destination", &s.Destination, []settingOption{{"automatic", "Automatic"}, {"lastSession", "Last session"}, {"newSession", "New session"}}, nil)
		})
		line(c)
		a.settingsRow(c, "Window capture", "Screen Recording lets GCode capture the frontmost window. macOS may request one restart.", func() {
			a.appshotCapabilityBadge(c, capabilities.WindowCapture)
			if s.Enabled && capabilities.WindowCapture == "permissionRequired" {
				textButton(c, tr(c, "Allow window capture")).OnClick(func() { a.dispatch(contracts.Command{Kind: "appshots.permission", ID: "windowCapture"}) })
			}
		})
		line(c)
		a.settingsRow(c, "Application text", "Accessibility adds visible and off-screen application text. Screenshots work without it.", func() {
			a.appshotCapabilityBadge(c, capabilities.ApplicationText)
			if s.Enabled && capabilities.ApplicationText == "permissionRequired" {
				textButton(c, tr(c, "Enable text capture")).OnClick(func() { a.dispatch(contracts.Command{Kind: "appshots.permission", ID: "applicationText"}) })
			}
		})
	})
	ui.Row(c).FillWidth().Padding(12, 8, 0, 8).AlignItems(ui.Center).Gap(12).Children(func() {
		ui.Text(c, tr(c, "Changed a permission? Check again after returning to GCode.")).Grow(1).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted)
		textButton(c, tr(c, "Check again")).Label(tr(c, "Check permissions again")).OnClick(func() { a.dispatch(contracts.Command{Kind: "appshots.permissions.check"}) })
	})
}

func (a *App) appshotCapabilityBadge(c *ui.Context, status string) {
	text := "Unavailable"
	switch status {
	case "ready":
		text = "Ready"
	case "permissionRequired":
		text = "Required"
	case "checking":
		text = "Checking"
	case "setupRequired":
		text = "Set up"
	case "userSelection":
		text = "Select window"
	}
	ui.Text(c, tr(c, text)).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted).Background(a.palette.Raised).Radius(4).Padding(3, 6)
}

// archivedSettings is the only surface that lists archived conversations.
// The sidebar drops them; here they can be opened, restored and deleted.
func (a *App) archivedSettings(c *ui.Context) {
	count := 0
	for _, session := range a.state.Sessions {
		if !session.Archived {
			continue
		}
		count++
		s := session
		a.settingsCard(c, func() {
			a.settingsRowWithText(c, "archived-session:"+s.ID, s.Title, a.projectNameFor(s.ProjectID), func() {
				textButton(c.Key("archived-view-"+s.ID), tr(c, "View")).OnClick(func() { a.navigate("chat/" + s.ID) })
				textButton(c.Key("archived-restore-"+s.ID), tr(c, "Restore")).OnClick(func() {
					a.dispatch(contracts.Command{Kind: "session.archive", ID: s.ID, Enabled: false})
				})
				dangerButton(c.Key("archived-delete-"+s.ID), tr(c, "Delete")).OnClick(func() {
					a.sidebarUI.deleteID = s.ID
					a.sidebarUI.deleteOpen = true
				})
			})
		})
	}
	if count == 0 {
		ui.Text(c, tr(c, "No archived sessions")).TextColor(a.palette.Muted).Padding(16, 8)
	}
}
