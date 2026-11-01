package desktop

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// The composer pill is a frosted surface: with a window material under it,
// the pill keeps the reference's light tint over the glass, so the shell
// shows through; without one, it lies flat on the page.
func TestComposerPillFollowsTheFrost(t *testing.T) {
	a := testApp(t, "default")
	a.state.Settings.Appearance.Mode = "dark"
	a.state.Settings.Appearance.Surface = "theme"
	tt := ui.NewTester(a.View, 1320, 880)
	tt.SetVibrancy(true)
	tt.Frame()
	box, ok := tt.Find("Message")
	if !ok {
		t.Fatal("composer missing")
	}
	sample := func() [3]uint32 {
		img := tt.Image()
		x, y := int(box.X+box.W/2), int(box.Y+4)
		r, g, b, _ := img.At(x, y).RGBA()
		return [3]uint32{r >> 8, g >> 8, b >> 8}
	}
	frosted := sample()
	tt.SetVibrancy(false)
	tt.Frame()
	opaque := sample()
	if frosted == opaque {
		t.Fatalf("the frosted pill paints like the opaque one: %v", frosted)
	}
	// The dark frosted tint is a near-black at the reference's 15% scene
	// coverage, over the shell glass: the pill stays with the panel.
	if frosted[0] > 20 || frosted[1] > 20 || frosted[2] > 20 {
		t.Fatalf("frosted pill=%v, want a tint over the glass", frosted)
	}
	// Without a material the input tint lies flat on the page.
	if opaque[0] != 39 || opaque[1] != 39 || opaque[2] != 42 {
		t.Fatalf("opaque pill=%v, want the flattened input tint", opaque)
	}
	captureFrostEvidence(t, tt, "composer-opaque")
	tt.SetVibrancy(true)
	tt.Frame()
	captureFrostEvidence(t, tt, "composer-frosted")
}

// captureFrostEvidence writes the frame under RCODE_FROST_EVIDENCE, the
// desktop audit convention (see layout_test.go).
func captureFrostEvidence(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	dir := os.Getenv("RCODE_FROST_EVIDENCE")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	encodeErr := png.Encode(file, tt.Image())
	closeErr := file.Close()
	if encodeErr != nil {
		t.Fatal(encodeErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
}
