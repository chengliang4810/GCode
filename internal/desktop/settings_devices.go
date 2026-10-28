package desktop

import (
	"fmt"
	"time"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func (a *App) deviceSettings(c *ui.Context) {
	copy := "Devices in this workspace."
	scope := a.state.Integrations.WorkspaceScope
	switch scope {
	case "local":
		copy = "Devices in this local workspace."
	case "synced":
		copy = "Devices synced to this workspace."
	}
	a.integrationSubtitle(c, copy)
	for _, current := range []bool{true, false} {
		if !current && scope == "local" {
			continue
		}
		devices := []contracts.Device{}
		for _, d := range a.state.Devices {
			if d.Current == current {
				devices = append(devices, d)
			}
		}
		if current && len(devices) == 0 {
			continue
		}
		label := "Other devices"
		if current {
			label = "This device"
		}
		sectionTitle(c, label)
		a.settingsCard(c, func() {
			if len(devices) == 0 {
				a.settingsRow(c, "Sign in on another device to see it here.", "", func() {})
			}
			for i, d := range devices {
				if i > 0 {
					line(c)
				}
				a.deviceRow(c, d)
			}
		})
	}
}

func (a *App) deviceRow(c *ui.Context, d contracts.Device) {
	meta := devicePlatform(d.Platform)
	presence := "Offline"
	switch d.Status {
	case "asleep":
		presence = "Asleep"
	case "connected":
		presence = "Online"
	}
	if d.Version != "" {
		meta += " · v" + d.Version
	}
	if !d.Current && d.Platform == "cloud" {
		meta += " · " + presence
	} else if !d.Current && presence == "Online" {
		meta += " · Online"
	} else if !d.Current {
		meta += " · Last seen " + deviceLastSeen(d.LastSeen, a.state.Integrations.ObservedAt)
	}
	services := c.Services()
	a.settingsRow(c, d.Name, meta, func() {
		label := "Copy ID"
		if a.integrationUI.CopiedDevice == d.ID && c.Now().Before(a.integrationUI.CopiedUntil) {
			label = "Copied"
			c.After(a.integrationUI.CopiedUntil.Sub(c.Now()))
		}
		textButton(c, label).Label("Copy device ID " + d.ID).Tooltip(d.ID).OnClick(func() {
			services.WriteClipboard(d.ID)
			a.integrationUI.CopiedDevice = d.ID
			a.integrationUI.CopiedUntil = c.Now().Add(1800 * time.Millisecond)
		})
		textButton(c, "Rename").Label("Rename " + d.Name).Background(a.palette.Text.Alpha(.05)).OnClick(func() {
			a.integrationUI.RenameID = d.ID
			a.integrationUI.RenameText = d.Name
			a.integrationUI.RenameOpen = true
		})
	})
}

func devicePlatform(value string) string {
	switch value {
	case "darwin", "macos":
		return "macOS"
	case "windows":
		return "Windows"
	case "linux":
		return "Linux"
	case "cloud":
		return "Cloud"
	case "web":
		return "Web"
	}
	return value
}
func deviceLastSeen(at, now time.Time) string {
	if at.IsZero() {
		return "never seen"
	}
	seconds := int64(now.Sub(at).Seconds())
	switch {
	case seconds < 60:
		return "just now"
	case seconds < 3600:
		return fmt.Sprintf("%dm ago", seconds/60)
	case seconds < 86400:
		return fmt.Sprintf("%dh ago", seconds/3600)
	default:
		return fmt.Sprintf("%dd ago", seconds/86400)
	}
}
