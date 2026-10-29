package text

import (
	"bytes"
	"testing"

	"github.com/egoist/mygo/internal/scene"
)

// The probe supplies bitmap metadata independently of the requested AA mode.
// It deliberately is not DirectWrite, so public quarter requests must fall back.
type rasterModeProbe struct {
	stubEngine
	system         *System
	positionsAA    []bool
	glyphAA        []bool
	color, rgb     bool
	calledUnlocked bool
}

func newRasterModeProbe() (*System, *Font, *rasterModeProbe) {
	s := newSystem()
	p := &rasterModeProbe{system: s}
	s.eng = p
	return s, &Font{Size: 13, thin: true}, p
}

func (p *rasterModeProbe) checkLock() {
	if p.system.mu.TryLock() {
		p.system.mu.Unlock()
		p.calledUnlocked = true
	}
}

func (p *rasterModeProbe) positions(_ *Font, _ float32, subpixel bool) (int, bool) {
	p.checkLock()
	p.positionsAA = append(p.positionsAA, subpixel)
	return 4, false
}

func (p *rasterModeProbe) glyph(_ *Font, _ uint32, _, _ float32, _ Shade, subpixel bool) bitmap {
	p.checkLock()
	p.glyphAA = append(p.glyphAA, subpixel)
	pix := []byte{13}
	if p.color || p.rgb {
		pix = []byte{13, 29, 43, 255}
	}
	return bitmap{left: 3, top: -2, w: 1, h: 1, pix: pix, color: p.color, subpixel: p.rgb}
}

func TestRasterModeNativeKeysAndEngineRequests(t *testing.T) {
	for _, order := range []struct {
		name   string
		opaque [2]bool
	}{
		{"non-subpixel-first", [2]bool{false, true}},
		{"subpixel-first", [2]bool{true, false}},
	} {
		t.Run(order.name, func(t *testing.T) {
			s, f, probe := newRasterModeProbe()
			s.subpixel = true
			var images [2]GlyphImage
			for i, opaque := range order.opaque {
				images[i] = s.Glyph(f, 7, 2, .625, 0, opaque)
				if !images[i].OK {
					t.Fatal("probe bitmap was not rasterized")
				}
			}
			if len(s.glyphs) != 2 || len(s.places) != 2 {
				t.Fatalf("native AA requests aliased: glyphs=%d places=%d", len(s.glyphs), len(s.places))
			}
			for _, mode := range []uint8{0, 1} {
				if _, ok := s.glyphs[glyphKey{font: f, id: 7, scale: 512, subX: 2, mode: mode}]; !ok {
					t.Fatalf("missing native glyph mode %d", mode)
				}
				if _, ok := s.places[placeKey{font: f, scale: 512, mode: mode}]; !ok {
					t.Fatalf("missing native placement mode %d", mode)
				}
			}
			for i, opaque := range order.opaque {
				if got := s.Glyph(f, 7, 2, .625, 0, opaque); got != images[i] {
					t.Fatalf("native mode did not reuse its exact image: got=%+v want=%+v", got, images[i])
				}
			}
			if len(probe.positionsAA) != 2 || len(probe.glyphAA) != 2 {
				t.Fatalf("cache repeat reached engine: positions=%v glyphs=%v", probe.positionsAA, probe.glyphAA)
			}
			for i, want := range order.opaque {
				if probe.positionsAA[i] != want || probe.glyphAA[i] != want {
					t.Fatalf("engine AA request %d: positions=%v glyph=%v want=%v", i, probe.positionsAA[i], probe.glyphAA[i], want)
				}
			}
			if probe.calledUnlocked {
				t.Fatal("engine was called without the System lock")
			}
		})
	}
}

func TestRasterModeUnsupportedPoliciesNormalizeBeforeCaches(t *testing.T) {
	if GlyphPlacementNative != 0 || GlyphPlacementWindowsClearTypeQuarter != 1 {
		t.Fatal("public placement identities changed; they are not the private raster modes")
	}
	for _, policy := range []struct {
		name  string
		value uint8
	}{
		{"unknown-2-first", 2},
		{"unknown-255-first", 255},
		{"quarter-on-other-backend-first", GlyphPlacementWindowsClearTypeQuarter},
	} {
		for _, aa := range []struct {
			name     string
			subpixel bool
			mode     uint8
		}{
			{"non-subpixel", false, 0},
			{"subpixel", true, 1},
		} {
			t.Run(policy.name+"/"+aa.name, func(t *testing.T) {
				s, f, probe := newRasterModeProbe()
				s.subpixel = aa.subpixel
				first := RasterGlyph(s, f, 7, 2, .625, 0, true, policy.value)
				if !first.OK {
					t.Fatal("fallback did not rasterize the probe bitmap")
				}
				if got := s.Glyph(f, 7, 2, .625, 0, true); got != first {
					t.Fatalf("unsupported request did not reuse exact native image: got=%+v want=%+v", got, first)
				}
				if len(s.glyphs) != 1 || len(s.places) != 1 || len(probe.positionsAA) != 1 || len(probe.glyphAA) != 1 {
					t.Fatalf("unsupported request made another identity: glyphs=%d places=%d positions=%v raster=%v", len(s.glyphs), len(s.places), probe.positionsAA, probe.glyphAA)
				}
				for key := range s.glyphs {
					if key.mode != aa.mode {
						t.Fatalf("unsupported public policy %d became glyph mode %d, want %d", policy.value, key.mode, aa.mode)
					}
				}
				for key := range s.places {
					if key.mode != aa.mode {
						t.Fatalf("unsupported public policy %d became placement mode %d, want %d", policy.value, key.mode, aa.mode)
					}
				}
				if probe.positionsAA[0] != aa.subpixel || probe.glyphAA[0] != aa.subpixel || probe.calledUnlocked {
					t.Fatalf("fallback changed engine AA/locking: positions=%v glyphs=%v unlocked=%v", probe.positionsAA, probe.glyphAA, probe.calledUnlocked)
				}
			})
		}
	}
}

func TestRasterModeBitmapFlagsComeFromEngine(t *testing.T) {
	for _, tc := range []struct {
		name               string
		opaque, color, rgb bool
	}{
		{"subpixel-request-grayscale-bitmap", true, false, false},
		{"subpixel-request-color-bitmap", true, true, false},
		{"non-subpixel-request-rgb-bitmap", false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, f, probe := newRasterModeProbe()
			s.subpixel = true
			probe.color, probe.rgb = tc.color, tc.rgb
			g := s.Glyph(f, 7, 2, .625, 0, tc.opaque)
			if !g.OK || g.Colored != tc.color || g.Subpixel != tc.rgb || g.Thin != !tc.color {
				t.Fatalf("engine bitmap metadata was replaced by request mode: got=%+v color=%v subpixel=%v", g, tc.color, tc.rgb)
			}
			atlas, want := s.MaskAtlas, []byte{13}
			if tc.color || tc.rgb {
				atlas, want = s.ColorAtlas, []byte{13, 29, 43, 255}
			}
			at := (int(g.Y)*atlas.W + int(g.X)) * atlas.BPP
			if at+len(want) > len(atlas.Pix) || !bytes.Equal(atlas.Pix[at:at+len(want)], want) {
				t.Fatal("bitmap was not copied to the engine-selected atlas")
			}
			if len(probe.glyphAA) != 1 || probe.glyphAA[0] != tc.opaque || probe.calledUnlocked {
				t.Fatalf("request AA/locking changed: glyphs=%v unlocked=%v", probe.glyphAA, probe.calledUnlocked)
			}
		})
	}
}

// This calls the existing locked raster boundary directly. It checks the
// normalized quarter key without making a fake backend eligible for quarter.
func rasterModeProbeAt(s *System, f *Font, subpixel bool, policy uint8) GlyphImage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.glyphAt(f, 7, 2, 0, subpixel, 2, 4, 0, policy)
}

func TestRasterModeThreeGlyphIdentities(t *testing.T) {
	requests := []struct {
		subpixel bool
		policy   uint8
		mode     uint8
	}{
		{false, GlyphPlacementNative, 0},
		{true, GlyphPlacementNative, 1},
		{true, GlyphPlacementWindowsClearTypeQuarter, 2},
	}
	s, f, probe := newRasterModeProbe()
	var images [3]GlyphImage
	var entries [3]*atlasEntry
	for i, request := range requests {
		images[i] = rasterModeProbeAt(s, f, request.subpixel, request.policy)
		if !images[i].OK || images[i].Colored || images[i].Subpixel {
			t.Fatalf("mode %d changed the grayscale probe bitmap: %+v", request.mode, images[i])
		}
		entries[i] = s.glyphs[glyphKey{font: f, id: 7, scale: 512, subX: 2, mode: request.mode}]
		if entries[i] == nil {
			t.Fatalf("public policy %d and AA=%v did not normalize to glyph mode %d", request.policy, request.subpixel, request.mode)
		}
	}
	if len(s.glyphs) != 3 || entries[0] == entries[1] || entries[0] == entries[2] || entries[1] == entries[2] {
		t.Fatal("the three normalized raster identities alias")
	}
	for i := len(requests) - 1; i >= 0; i-- {
		request := requests[i]
		if got := rasterModeProbeAt(s, f, request.subpixel, request.policy); got != images[i] {
			t.Fatalf("mode %d did not reuse its exact image: got=%+v want=%+v", request.mode, got, images[i])
		}
	}
	if len(probe.glyphAA) != 3 || probe.glyphAA[0] || !probe.glyphAA[1] || !probe.glyphAA[2] || probe.calledUnlocked {
		t.Fatalf("mode-to-engine AA/locking changed: glyphs=%v unlocked=%v", probe.glyphAA, probe.calledUnlocked)
	}
}

func TestRasterModeFullAtlasDoesNotCacheFailedGlyph(t *testing.T) {
	for _, request := range []struct {
		name     string
		subpixel bool
		policy   uint8
		mode     uint8
	}{
		{"native-non-subpixel", false, GlyphPlacementNative, 0},
		{"native-subpixel", true, GlyphPlacementNative, 1},
		{"quarter-subpixel", true, GlyphPlacementWindowsClearTypeQuarter, 2},
	} {
		t.Run(request.name, func(t *testing.T) {
			s, f, probe := newRasterModeProbe()
			s.MaskAtlas = scene.NewAtlas(1, 4, 4)
			x, y, ok := s.MaskAtlas.Alloc(3, 3)
			if !ok {
				t.Fatal("could not fill the control atlas")
			}
			s.MaskAtlas.Put(x, y, 3, 3, bytes.Repeat([]byte{9}, 9), 3)
			if g := rasterModeProbeAt(s, f, request.subpixel, request.policy); g.OK || !s.Full() || len(s.glyphs) != 0 {
				t.Fatalf("failed mode %d was cached or not reported full: image=%+v full=%v glyphs=%d", request.mode, g, s.Full(), len(s.glyphs))
			}
			s.MakeRoom()
			if s.Full() {
				t.Fatal("MakeRoom did not clear the full indication")
			}
			g := rasterModeProbeAt(s, f, request.subpixel, request.policy)
			if !g.OK || len(s.glyphs) != 1 || s.glyphs[glyphKey{font: f, id: 7, scale: 512, subX: 2, mode: request.mode}] == nil {
				t.Fatalf("mode %d did not rasterize/cache after MakeRoom: %+v", request.mode, g)
			}
			if got := rasterModeProbeAt(s, f, request.subpixel, request.policy); got != g || len(probe.glyphAA) != 2 || probe.calledUnlocked {
				t.Fatalf("post-MakeRoom cache/locking changed: got=%+v want=%+v raster=%v unlocked=%v", got, g, probe.glyphAA, probe.calledUnlocked)
			}
		})
	}
}
