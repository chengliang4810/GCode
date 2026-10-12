package desktop

import (
	"fmt"
	"strings"
	"time"

	"gcode/internal/appmodel"
	"github.com/egoist/mygo/ui"
)

func iconButton(c *ui.Context, p palette, name, label string) ui.Element {
	b := ui.ButtonBase(c).Size(32, 32).Radius(8).Center().Label(label).Tooltip(label).TextColor(p.text)
	if b.Hovered() {
		b = b.Background(p.surfaceAlt)
	}
	return b.Children(func() {
		ui.Icon(c, icon(name)).FontSize(17)
	})
}

func accentIconButton(c *ui.Context, p palette, name, label string) ui.Element {
	b := ui.ButtonBase(c).Size(32, 32).Radius(8).Center().Label(label).Tooltip(label).
		Background(p.accent).TextColor(ui.RGB(255, 255, 255))
	if b.Hovered() {
		b = b.Background(p.accent.Mix(ui.RGB(0, 0, 0), 0.08))
	}
	return b.Children(func() {
		ui.Icon(c, icon(name)).FontSize(17)
	})
}

func compactButton(c *ui.Context, p palette, label string) ui.Element {
	return compactAction(c, p, "", label)
}

func compactAction(c *ui.Context, p palette, name, label string) ui.Element {
	b := ui.ButtonBase(c).Height(32).Padding(0, 10).Radius(8).Gap(7).Center().Label(label).TextColor(p.text)
	if b.Hovered() {
		b = b.Background(p.surfaceAlt)
	}
	return b.Children(func() {
		if name != "" {
			ui.Icon(c, icon(name)).FontSize(15)
		}
		ui.Text(c, label).FontSize(13.5).SingleLine()
	})
}

func primaryButton(c *ui.Context, p palette, name, label string) ui.Element {
	b := ui.ButtonBase(c).Height(34).Padding(0, 12).Radius(8).Gap(7).Center().Label(label).
		Background(p.accent).TextColor(ui.RGB(255, 255, 255))
	if b.Hovered() {
		b = b.Background(p.accent.Mix(ui.RGB(0, 0, 0), 0.08))
	}
	return b.Children(func() {
		if name != "" {
			ui.Icon(c, icon(name)).FontSize(16)
		}
		ui.Text(c, label).FontSize(13.5).FontWeight(600).SingleLine()
	})
}

func circleAction(c *ui.Context, p palette, name, label string, color ui.Color) ui.Element {
	b := ui.ButtonBase(c).Size(32, 32).Radius(16).Center().Label(label).Tooltip(label).
		Background(color).TextColor(ui.RGB(255, 255, 255))
	if b.Hovered() {
		b = b.Background(color.Mix(ui.RGB(0, 0, 0), 0.1))
	}
	return b.Children(func() {
		ui.Icon(c, icon(name)).FontSize(16)
	})
}

func neutralCircleAction(c *ui.Context, p palette, name, label string) ui.Element {
	b := ui.ButtonBase(c).Size(32, 32).Radius(16).Center().Label(label).Tooltip(label).
		Background(p.surfaceRaised).Border(1, p.border).TextColor(p.text)
	if b.Hovered() {
		b = b.Background(p.surfaceAlt)
	}
	return b.Children(func() {
		ui.Icon(c, icon(name)).FontSize(15)
	})
}

func sectionLabel(c *ui.Context, p palette, text string) ui.Element {
	return ui.Text(c, text).FontSize(11).FontWeight(600).LetterSpacing(0.6).
		TextColor(p.muted).SingleLine()
}

func statusBadge(c *ui.Context, p palette, state appmodel.RunState) ui.Element {
	label := stateLabel(state)
	tone := p.statusColor(stateTone(state))
	return ui.Row(c).Height(26).Padding(0, 9).Gap(6).Radius(13).AlignItems(ui.Center).
		Background(tone.Alpha(0.14)).Children(func() {
		ui.Box(c).Size(7, 7).Radius(4).Background(tone)
		ui.Text(c, label).FontSize(12).FontWeight(600).TextColor(tone).SingleLine()
	})
}

func stateLabel(state appmodel.RunState) string {
	switch state {
	case appmodel.Running:
		return "运行中"
	case appmodel.Cancelling:
		return "正在停止"
	case appmodel.WaitingPermission:
		return "等待授权"
	case appmodel.WaitingQuestion:
		return "等待回答"
	case appmodel.Failed:
		return "失败"
	case appmodel.Interrupted:
		return "已中断"
	default:
		return "空闲"
	}
}

func stateTone(state appmodel.RunState) appState {
	switch state {
	case appmodel.Running, appmodel.Cancelling:
		return stateRunning
	case appmodel.WaitingPermission, appmodel.WaitingQuestion:
		return stateWaiting
	case appmodel.Failed:
		return stateFailed
	case appmodel.Idle:
		return stateDone
	default:
		return stateIdle
	}
}

func messageTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Local().Format("15:04")
}

func relativeTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	age := time.Since(value)
	if age < 0 {
		age = 0
	}
	switch {
	case age < time.Minute:
		return "刚刚"
	case age < time.Hour:
		return fmt.Sprintf("%d 分钟前", int(age/time.Minute))
	case age < 24*time.Hour:
		return fmt.Sprintf("%d 小时前", int(age/time.Hour))
	case age < 7*24*time.Hour:
		return fmt.Sprintf("%d 天前", int(age/(24*time.Hour)))
	default:
		return value.Local().Format("1月2日")
	}
}

func providerLabel(settings appmodel.Settings, id string) string {
	for _, provider := range settings.Providers {
		if provider.ID == id {
			if provider.Name != "" {
				return provider.Name
			}
			return provider.Model
		}
	}
	if id == "" {
		return "未配置模型"
	}
	return id
}

// sessionModelLabel deliberately prefers ActiveRun.Model. ProviderID is the
// configuration for the next task and may change while an existing run is
// still using its captured RunConfig.
func sessionModelLabel(settings appmodel.Settings, session appmodel.Session) string {
	if session.ActiveRun != nil && strings.TrimSpace(session.ActiveRun.Model) != "" {
		return session.ActiveRun.Model
	}
	for _, provider := range settings.Providers {
		if provider.ID == session.ProviderID {
			if strings.TrimSpace(provider.Model) != "" {
				return provider.Model
			}
			if strings.TrimSpace(provider.Name) != "" {
				return provider.Name
			}
		}
	}
	return providerLabel(settings, session.ProviderID)
}

func truncateMiddle(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len([]rune(value)) <= max {
		return value
	}
	runes := []rune(value)
	left := max / 2
	right := max - left - 1
	return string(runes[:left]) + "…" + string(runes[len(runes)-right:])
}

func queuedCount(session appmodel.Session) string {
	if len(session.Queue) == 0 {
		return ""
	}
	return fmt.Sprintf("%d", len(session.Queue))
}
