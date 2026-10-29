package workbench

import (
	"syscall"
	"testing"
	"unicode/utf16"
	"unsafe"
)

func TestTabTooltipWindowsFontMatchesActualIconTitleFamily(t *testing.T) {
	// Independent native query: test the family used by actual card measurement,
	// rather than treating the cached helper's value as a source constant.
	var font tabTooltipLogFont
	proc := syscall.NewLazyDLL("user32.dll").NewProc("SystemParametersInfoW")
	ok, _, err := proc.Call(0x001F, unsafe.Sizeof(font), uintptr(unsafe.Pointer(&font)), 0)
	if ok == 0 {
		t.Fatalf("independent SPI_GETICONTITLELOGFONT failed: %v", err)
	}
	if got, want := tabTooltipFontFamily(), syscall.UTF16ToString(font.FaceName[:]); got != want {
		t.Fatalf("cached tooltip family%q, actual icon-title family%q", got, want)
	}
	if unsafe.Sizeof(font) != 92 {
		t.Fatalf("LOGFONTW layout is %d bytes, want92", unsafe.Sizeof(font))
	}
}

func TestTabTooltipWindowsFontQueryFailureAndFamilyOnly(t *testing.T) {
	if got := readTabTooltipFont(func(*tabTooltipLogFont) bool { return false }); got != "Segoe UI" {
		t.Fatalf("failed SPI fallback%q", got)
	}
	got := readTabTooltipFont(func(font *tabTooltipLogFont) bool {
		font.Height, font.Weight, font.Italic = -33, 900, 1
		copy(font.FaceName[:], utf16.Encode([]rune("An actual family")))
		return true
	})
	if got != "An actual family" {
		t.Fatalf("family-only query returned%q", got)
	}
}
