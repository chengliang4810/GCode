package workbench

import (
	"strings"
	"sync"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// GPUI's detached tooltip starts with .SystemUIFont. Its Windows resolver
// reads the icon-title family once, independently of the owner's font.
var tabTooltipFontFamily = sync.OnceValue(func() string {
	proc := syscall.NewLazyDLL("user32.dll").NewProc("SystemParametersInfoW")
	return readTabTooltipFont(func(font *tabTooltipLogFont) bool {
		if proc.Find() != nil {
			return false
		}
		ok, _, _ := proc.Call(0x001F, unsafe.Sizeof(*font), uintptr(unsafe.Pointer(font)), 0)
		return ok != 0
	})
})

type tabTooltipLogFont struct {
	Height, Width, Escapement, Orientation, Weight int32
	Italic, Underline, StrikeOut, CharSet          byte
	OutPrecision, ClipPrecision, Quality, Pitch    byte
	FaceName                                       [32]uint16
}

func readTabTooltipFont(query func(*tabTooltipLogFont) bool) string {
	var font tabTooltipLogFont
	if !query(&font) {
		return "Segoe UI"
	}
	return strings.Trim(string(utf16.Decode(font.FaceName[:])), "\x00")
}
