package desktop

import (
	"strconv"
	"time"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

var islandMascots = []settingOption{
	{"pululu", "Pululu"}, {"cindy", "Cindy"}, {"blackcat", "Black cat"}, {"tarara", "Tarara"},
	{"boli", "Boli"}, {"whitesnow", "White snow"}, {"annie", "Annie"}, {"chaku", "Chaku"}, {"muffin", "Muffin"}, {"erika", "Erika"},
}
var islandSounds = []settingOption{
	{"", "Follow system"}, {"none", "Silent"}, {"startup-chime", "Startup chime"}, {"ring-chime", "Ring chime"},
	{"item-found", "Item found"}, {"gem-collect", "Gem collect"}, {"item-fanfare", "Item fanfare"},
	{"victory-fanfare", "Victory fanfare"}, {"error-buzz", "Error buzz"}, {"secret-chime", "Secret chime"},
}

func (a *App) islandSettings(c *ui.Context) {
	prefs := &a.state.Settings.Island
	ui.Column(c).FillWidth().Gap(20).Children(func() {
		ui.Column(c).FillWidth().Padding(16).Gap(16).Radius(10).Border(1, c.Theme().Border).Children(func() {
			islandSettingHeader(c, "Show task capsule", "Show running tasks, input requests and unread results at the top of the screen.", func() {
				a.controlSwitch(c.Key("island-enabled"), &a.state.Settings.AgentIsland, tr(c, "Enable Dynamic Island"), false, a.setIslandEnabled)
			})
			islandSettingDivider(c)
			islandSettingHeader(c, "Subtle reminders", "Keep the capsule compact when a task needs input, completes or fails. Click it to view the task list.", func() {
				a.controlSwitch(c.Key("island-subtle"), &prefs.SubtleReminders, tr(c, "Toggle ")+tr(c, "Subtle reminders"), false, func(bool) {
					a.island.CollapseAt = time.Time{}
					a.island.Expanded = false
					a.saveSettings()
					a.syncAgentIsland()
				})
			})
			islandSettingDivider(c)
			ui.Text(c, tr(c, "Display position")).Bold()
			ui.Text(c, tr(c, "Choose a display, or show a capsule on every display.")).Wrap().TextColor(c.Theme().TextMuted)
			options := []settingOption{{"", "Primary display"}, {"all", "All displays"}}
			for _, display := range a.island.Displays {
				options = append(options, settingOption{strconv.FormatInt(display.ID, 10), display.Label})
			}
			a.settingsChoice(c, "Display position", &prefs.DisplayTarget, options, a.syncAgentIsland)
		})
		ui.Column(c).FillWidth().Padding(16).Gap(12).Radius(10).Border(1, c.Theme().Border).Children(func() {
			islandSettingHeader(c, "Character", "The character shown in the capsule and expanded task panel.", func() {
				a.islandMascot(c, 32)
				a.settingsChoice(c, "Character", &prefs.Mascot, append([]settingOption{{"", "Default (Pululu)"}}, islandMascots...), a.syncAgentIsland)
			})
		})
		ui.Column(c).FillWidth().Padding(16).Gap(16).Radius(10).Border(1, c.Theme().Border).Children(func() {
			islandSettingHeader(c, "Event sounds", "Play the selected sound for each task event. Follow system uses the notification sound preference.", func() {
				a.controlSwitch(c.Key("island-sound-enabled"), &prefs.SoundEnabled, tr(c, "Toggle ")+tr(c, "Event sounds"), false, func(enabled bool) {
					if enabled && prefs.StartSound == "" && prefs.AttentionSound == "" && prefs.CompleteSound == "" && prefs.ErrorSound == "" && prefs.SelectSound == "" {
						prefs.StartSound = "startup-chime"
						prefs.AttentionSound = "secret-chime"
						prefs.CompleteSound = "gem-collect"
						prefs.ErrorSound = "error-buzz"
					}
					a.saveSettings()
				})
			})
			islandSettingDivider(c)
			for _, event := range []struct {
				key, label string
				value      *string
				kind       notificationKind
			}{
				{"start", "Task started", &prefs.StartSound, notificationStarted},
				{"attention", "Input required", &prefs.AttentionSound, notificationQuestion},
				{"complete", "Task completed", &prefs.CompleteSound, notificationCompleted},
				{"error", "Task failed", &prefs.ErrorSound, notificationError},
				{"select", "Click action", &prefs.SelectSound, notificationSelected},
			} {
				event := event
				ui.Row(c.Key("island-sound-" + event.key)).FillWidth().Gap(12).AlignItems(ui.Center).Wrap().Children(func() {
					ui.Text(c, tr(c, event.label)).Grow(1)
					a.settingsChoice(c, "Island sound "+event.key, event.value, islandSounds, nil)
					iconButton(c.Key("island-preview-"+event.key), "volume-loud", tr(c, "Preview sound")).Disabled(*event.value == "none").OnClick(func() { a.previewIslandSound(event.kind, *event.value) })
				})
			}
			textButton(c.Key("island-sound-reset"), tr(c, "Reset event sounds")).OnClick(func() {
				prefs.SoundEnabled = true
				prefs.StartSound = "startup-chime"
				prefs.AttentionSound = "secret-chime"
				prefs.CompleteSound = "gem-collect"
				prefs.ErrorSound = "error-buzz"
				prefs.SelectSound = ""
				a.saveSettings()
			})
		})
	})
}

func islandSettingHeader(c *ui.Context, title, description string, controls func()) {
	ui.Column(c).FillWidth().Gap(8).Children(func() {
		ui.Row(c).FillWidth().Gap(12).Wrap().AlignItems(ui.Center).Children(func() {
			ui.Text(c, tr(c, title)).Bold().Grow(1)
			controls()
		})
		ui.Text(c, tr(c, description)).Wrap().TextColor(c.Theme().TextMuted)
	})
}
func islandSettingDivider(c *ui.Context) {
	ui.Box(c).FillWidth().Height(1).Background(c.Theme().Border)
}

func islandSoundChoice(prefs contracts.AgentIslandSettings, kind notificationKind) string {
	switch kind {
	case notificationStarted:
		return prefs.StartSound
	case notificationQuestion:
		return prefs.AttentionSound
	case notificationCompleted:
		return prefs.CompleteSound
	case notificationError:
		return prefs.ErrorSound
	case notificationSelected:
		return prefs.SelectSound
	}
	return ""
}

func (a *App) previewIslandSound(kind notificationKind, choice string) {
	if choice == "none" || a.notifications == nil || a.notifications.queue == nil {
		return
	}
	select {
	case a.notifications.queue <- desktopNotice{Kind: kind, Sound: true, SoundChoice: choice}:
	default:
	}
}

func (a *App) playIslandClick() {
	prefs := a.state.Settings
	if !prefs.AgentIsland || !prefs.Island.SoundEnabled {
		return
	}
	choice := prefs.Island.SelectSound
	if choice == "" && !prefs.Notifications.Sound {
		return
	}
	a.previewIslandSound(notificationSelected, choice)
}
