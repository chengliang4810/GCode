//go:build !windows

package workbench

// The pinned native text stack resolves this alias on non-Windows systems.
// Its actual macOS face has not yet been accepted on a native machine.
func tabTooltipFontFamily() string { return "system-ui" }
