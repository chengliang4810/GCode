package mock

import "gcode/internal/contracts"

func appshotCapabilities(platform string) contracts.AppshotCapabilities {
	c := contracts.AppshotCapabilities{Platform: platform, GlobalShortcut: "unavailable", WindowCapture: "unavailable", ApplicationText: "unavailable"}
	if platform == "darwin" {
		c.GlobalShortcut = "ready"
		c.WindowCapture = "permissionRequired"
		c.ApplicationText = "permissionRequired"
	}
	return c
}
