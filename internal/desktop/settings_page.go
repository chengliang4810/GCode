package desktop

import (
	"fmt"
	"strings"
	"time"

	"gcode/internal/appmodel"
	"github.com/egoist/mygo/ui"
)

func (a *App) settingsPage(c *ui.Context, p palette, settings appmodel.Settings, draft SettingsDraft, errorText string, visualSample bool) {
	bar := c.TitleBar()
	left := bar.Left + 12
	if left < 20 {
		left = 20
	}
	right := bar.Right + 20
	if right < 20 {
		right = 20
	}
	height := float32(62)
	if bar.Height > height {
		height = bar.Height
	}
	ui.Column(c).Fill().Background(p.background).Children(func() {
		ui.Row(c).Height(height).Padding(0, right, 0, left).Gap(12).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(p.border).Children(func() {
			if iconButton(c, p, "chevron-left", "返回工作台").Clicked() {
				a.closeSettings()
			}
			ui.Column(c).Gap(2).Children(func() {
				ui.Text(c, "设置").FontSize(20).FontWeight(650)
				ui.Text(c, "模型服务、发送偏好和外观").FontSize(12).TextColor(p.muted)
			})
			ui.Spacer(c)
			if errorText != "" {
				ui.Text(c, errorText).FontSize(12).TextColor(p.danger).MaxWidth(300)
			}
		})
		ui.Scroll(c.Key("settings-scroll")).Grow(1).Padding(24, 28, 40, 28).Children(func() {
			ui.Column(c).WidthPercent(100).MaxWidth(980).AlignSelf(ui.Center).Children(func() {
				if visualSample {
					ui.Text(c, "示例数据：当前页面使用内置演示数据").FontSize(10.5).TextColor(p.warning).SingleLine()
					ui.Box(c).Height(10)
				}
				a.appearanceSettings(c, p, draft)
				a.sendSettings(c, p, draft)
				a.providerSettings(c, p, settings, draft)
			})
		})
		a.settingsFooter(c, p, draft)
	})
}

func (a *App) appearanceSettings(c *ui.Context, p palette, draft SettingsDraft) {
	settingsSection(c, p, "外观", "跟随系统主题，或固定为浅色/深色。", func() {
		ui.Row(c).Height(38).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "主题").FontSize(13.5).FontWeight(600).Width(130)
			index := themeIndex(draft.Theme)
			control := ui.Segmented(c.Key("theme-mode"), &index, "跟随系统", "浅色", "深色").Label("主题")
			if control.Changed() {
				a.updateSettingsDraft(func(value *SettingsDraft) { value.Theme = themeFromIndex(index) })
			}
		})
	})
}

func (a *App) sendSettings(c *ui.Context, p palette, draft SettingsDraft) {
	settingsSection(c, p, "发送方式", "运行中的会话可以为单条消息选择另一种方式。", func() {
		ui.Row(c).Height(38).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "默认发送").FontSize(13.5).FontWeight(600).Width(130)
			index := modeIndex(draft.SendMode)
			control := ui.Segmented(c.Key("default-send-mode"), &index, "纠正", "队列").Label("默认发送方式")
			if control.Changed() {
				a.updateSettingsDraft(func(value *SettingsDraft) { value.SendMode = modeFromIndex(index) })
			}
		})
	})
}

func (a *App) providerSettings(c *ui.Context, p palette, settings appmodel.Settings, draft SettingsDraft) {
	settingsSection(c, p, "模型服务", "密钥保存在本机。留空可保留已保存的密钥。", func() {
		ui.Row(c).Gap(14).AlignItems(ui.Start).Children(func() {
			ui.Column(c).Width(236).Gap(6).Children(func() {
				if len(settings.Providers) == 0 {
					emptyPanel(c, p, "key", "还没有模型服务", "添加服务后可在输入区选择模型。")
				}
				for _, provider := range settings.Providers {
					a.providerRow(c, p, provider, provider.ID == draft.ProviderID)
				}
				if compactButton(c, p, "添加服务").Width(116).Clicked() {
					a.addProviderDraft()
				}
			})
			ui.Column(c).Grow(1).MinWidth(0).Background(p.surface).Border(1, p.border).Radius(10).Padding(16).Gap(11).Children(func() {
				a.providerEditor(c, p, draft)
			})
		})
	})
}

func (a *App) settingsFooter(c *ui.Context, p palette, draft SettingsDraft) {
	ui.Row(c).Height(54).Padding(0, 28).BorderWidth(1, 0, 0, 0).BorderColor(p.border).Children(func() {
		ui.Row(c).WidthPercent(100).MaxWidth(980).AlignSelf(ui.Center).Gap(10).AlignItems(ui.Center).Children(func() {
			if primaryButton(c, p, "check", "保存设置").Width(110).Clicked() {
				a.saveSettings()
			}
			if draft.Provider.ID != "" {
				if compactButton(c, p, "保存服务").Width(100).Clicked() {
					a.saveProvider()
				}
				if iconButton(c, p, "trash", "删除当前服务").Clicked() {
					a.removeProvider(draft.Provider.ID)
				}
			}
		})
	})
}

func (a *App) providerRow(c *ui.Context, p palette, provider appmodel.Provider, selected bool) {
	button := ui.ButtonBase(c.Key("provider-"+provider.ID)).Height(52).Padding(7, 9).Radius(8).Gap(8).AlignItems(ui.Center).
		Label("模型服务 " + provider.Name).TextColor(p.text)
	if selected {
		button = button.Background(p.accentSoft)
	} else if button.Hovered() {
		button = button.Background(p.surfaceAlt)
	}
	button.Children(func() {
		ui.Row(c).Size(30, 30).Radius(8).Center().Background(p.surfaceAlt).Children(func() {
			ui.Icon(c, icon("key")).FontSize(16).TextColor(p.accent)
		})
		ui.Column(c).Gap(2).MinWidth(0).Grow(1).Children(func() {
			name := provider.Name
			if name == "" {
				name = "未命名服务"
			}
			ui.Text(c, name).FontSize(13).FontWeight(600).SingleLine()
			ui.Text(c, provider.Model).FontSize(11).TextColor(p.muted).SingleLine()
		})
	})
	if button.Clicked() {
		a.selectProvider(provider.ID)
	}
}

func (a *App) providerEditor(c *ui.Context, p palette, draft SettingsDraft) {
	if draft.Provider.ID == "" {
		emptyPanel(c, p, "key", "选择模型服务", "在左侧选择已有服务，或先添加一个新的服务。")
		return
	}
	name := draft.Provider.Name
	ui.Text(c, "服务名称").FontSize(11.5).TextColor(p.muted)
	nameInput := ui.TextInput(c.Key("provider-name"), &name).Placeholder("例如：OpenAI 主账号").Label("服务名称").Height(34)
	if nameInput.Changed() {
		a.updateSettingsDraft(func(value *SettingsDraft) { value.Provider.Name = name })
	}
	baseURL := draft.Provider.BaseURL
	format := string(draft.Provider.Format)
	if format == "" {
		format = string(appmodel.ChatCompletions)
	}
	ui.Row(c).Gap(10).Children(func() {
		ui.Column(c).Grow(1).Gap(4).Children(func() {
			ui.Text(c, "Base URL").FontSize(11.5).TextColor(p.muted)
			baseInput := ui.TextInput(c.Key("provider-url"), &baseURL).Placeholder("https://api.openai.com").Label("Base URL").Height(34)
			if baseInput.Changed() {
				a.updateSettingsDraft(func(value *SettingsDraft) { value.Provider.BaseURL = baseURL })
			}
		})
		ui.Column(c).Width(180).Gap(4).Children(func() {
			ui.Text(c, "API 类型").FontSize(11.5).TextColor(p.muted)
			formats := []string{string(appmodel.Responses), string(appmodel.ChatCompletions), string(appmodel.Anthropic), string(appmodel.Gemini)}
			selector := ui.Select(c.Key("provider-format"), &format, formats).Height(34).
				Background(p.surfaceAlt).Border(1, p.border).Radius(8).Label("API 类型")
			if selector.Changed() {
				a.updateSettingsDraft(func(value *SettingsDraft) { value.Provider.Format = appmodel.APIFormat(format) })
			}
		})
	})
	model := draft.Provider.Model
	ui.Text(c, "模型").FontSize(11.5).TextColor(p.muted)
	modelInput := ui.TextInput(c.Key("provider-model"), &model).Placeholder("模型名称").Label("模型").Height(34)
	if modelInput.Changed() {
		a.updateSettingsDraft(func(value *SettingsDraft) { value.Provider.Model = model })
	}
	apiKey := draft.APIKey
	ui.Text(c, "API Key").FontSize(11.5).TextColor(p.muted)
	keyInput := ui.TextInput(c.Key("provider-key"), &apiKey).Placeholder("输入新的 API Key，留空可保留已保存的密钥").Label("API Key").Password().Height(34)
	if keyInput.Changed() {
		a.updateSettingsDraft(func(value *SettingsDraft) { value.APIKey = apiKey })
	}
	ui.Text(c, "已配置：").FontSize(11.5).TextColor(p.muted)
	if draft.Provider.HasAPIKey {
		ui.Text(c, "本机凭据存储中已有密钥").FontSize(11.5).TextColor(p.success)
	} else {
		ui.Text(c, "尚未配置密钥").FontSize(11.5).TextColor(p.warning)
	}
}

func settingsSection(c *ui.Context, p palette, title, detail string, body func()) {
	ui.Column(c).Gap(10).Margin(0, 0, 24, 0).Children(func() {
		ui.Text(c, title).FontSize(16).FontWeight(650)
		ui.Text(c, detail).FontSize(12.5).TextColor(p.muted).LineHeight(1.4)
		body()
	})
}

func themeIndex(theme string) int {
	switch strings.ToLower(theme) {
	case "light":
		return 1
	case "dark":
		return 2
	default:
		return 0
	}
}

func themeFromIndex(index int) string {
	switch index {
	case 1:
		return "light"
	case 2:
		return "dark"
	default:
		return "system"
	}
}

func (a *App) selectProvider(id string) {
	a.mu.Lock()
	for _, provider := range a.data.Settings.Providers {
		if provider.ID == id {
			a.settingsDraft.ProviderID = id
			a.settingsDraft.Provider = provider
			a.settingsDraft.APIKey = ""
			break
		}
	}
	a.settingsDirty = true
	a.mu.Unlock()
}

func (a *App) addProviderDraft() {
	provider := appmodel.Provider{
		ID:      fmt.Sprintf("provider-%d", time.Now().UnixNano()),
		Name:    "新模型服务",
		Format:  appmodel.ChatCompletions,
		BaseURL: "https://api.openai.com",
		Model:   "gpt-4.1-mini",
	}
	a.mu.Lock()
	a.data.Settings.Providers = append(a.data.Settings.Providers, provider)
	a.settingsDraft.ProviderID = provider.ID
	a.settingsDraft.Provider = provider
	a.settingsDraft.APIKey = ""
	a.settingsDirty = true
	a.mu.Unlock()
}
