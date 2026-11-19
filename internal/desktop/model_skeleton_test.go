package desktop

import (
	"hash/fnv"
	"image"
	"testing"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

// The picker's loading branch is what a model catalog still being read looks
// like: ghost rows that breathe in lockstep, and a still frame when motion is
// reduced. The engine reports its own configured models synchronously, so the
// backend is the only place this state comes from; the branch is kept honest
// here.
func TestModelCatalogSkeletonBreathesInLockstep(t *testing.T) {
	for _, reduced := range []bool{false, true} {
		for _, appearance := range []string{"light", "dark"} {
			a := testApp(t, "default")
			// The picker only stands in for the list while no model is
			// configured: the list itself comes from the local provider
			// configuration and never waits for the remote catalog.
			a.state.Models = nil
			a.state.ModelCatalog.State = contracts.CatalogLoading
			a.state.Settings.Appearance.Surface = "opaque"
			a.state.Settings.Appearance.Mode = appearance
			a.state.Settings.Appearance.ThemeID = "rcode-" + appearance
			a.state.Settings.Appearance.ReduceMotion = reduced
			a = modelSnapshotApp(t, a.state)
			a.modelMenu, a.effortMenu = true, false
			tt := ui.NewTester(a.View, 1320, 880)
			tt.Advance(1200 * 1000 * 1000)
			if _, ok := tt.Access(a.t("Loading models")); !ok {
				t.Fatalf("reduced=%v %s: the loading label is missing", reduced, appearance)
			}
			writeModelSurfaceEvidence(t, "model-skeleton-"+appearance+"-reduced-"+boolName(reduced), tt, map[string]any{
				"scope": "model catalog loading state", "appearance": appearance, "reducedMotion": reduced,
			})
			// The breathing itself: two frames half a cycle apart must differ
			// when motion is on and be identical when it is reduced.
			before := frameDigest(tt.Image())
			tt.Advance(600 * 1000 * 1000)
			tt.Frame()
			after := frameDigest(tt.Image())
			if reduced && before != after {
				t.Fatalf("%s: a still skeleton changed between frames", appearance)
			}
			if !reduced && before == after {
				t.Fatalf("%s: the skeleton did not breathe", appearance)
			}
		}
	}
}

func boolName(value bool) string {
	if value {
		return "on"
	}
	return "off"
}

// frameDigest is a cheap fingerprint of a rendered frame.
func frameDigest(img *image.RGBA) uint64 {
	hash := fnv.New64a()
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			offset := img.PixOffset(x, y)
			hash.Write(img.Pix[offset : offset+4])
		}
	}
	return hash.Sum64()
}
