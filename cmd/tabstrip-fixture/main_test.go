//go:build uifixture

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gcode/internal/workbench"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestTabStripFixtureFlagsAndMatrix(t *testing.T) {
	o, err := parseOptions([]string{"--out-dir", filepath.Join(t.TempDir(), "fresh")}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	all := scenes(o)
	if len(all) != 36 {
		t.Fatalf("matrix count: %d", len(all))
	}
	seen := map[string]bool{}
	for _, scene := range all {
		if err := scene.Validate(); err != nil {
			t.Fatal(err)
		}
		name := sceneName(scene)
		if seen[name] {
			t.Fatal("duplicate output name")
		}
		seen[name] = true
	}
	if !seen["tabstrip-light-w220-n6-scrollmiddle"] || !seen["tabstrip-dark-w400-n0-scrollend"] {
		t.Fatal("matrix omitted source scenarios")
	}
	o, err = parseOptions([]string{"--out-dir", filepath.Join(t.TempDir(), "fresh"), "--matrix=false", "--appearance=light", "--host-width=220", "--count=2", "--scroll-fraction=.5"}, &bytes.Buffer{})
	if err != nil || len(scenes(o)) != 1 || o.scene != (workbench.TabStripFixtureScene{Appearance: "light", HostWidth: 220, Count: 2, ScrollFraction: .5}) {
		t.Fatalf("single scene flags: %+v %v", o, err)
	}
	for _, args := range [][]string{{}, {"--appearance=system"}, {"--host-width=300"}, {"--count=1"}, {"--scroll-fraction=NaN"}, {"--scroll-fraction=+Inf"}, {"--scroll-fraction=-.1"}, {"--capture-delay=-1s"}, {"--capture-mode=unknown"}, {"--capture-mode=wgc", "--capture-bridge=relative.dll"}, {"--expected-scale=0"}, {"--expected-scale=1.5"}, {"--expected-scale=NaN"}, {"extra"}} {
		base := []string{"--out-dir", filepath.Join(t.TempDir(), "fresh")}
		if len(args) == 0 {
			base = nil
		}
		if _, err := parseOptions(append(base, args...), &bytes.Buffer{}); err == nil {
			t.Fatalf("invalid CLI accepted: %v", args)
		}
	}
}

func TestTabStripFixtureCaptureProfileAndFreshPaths(t *testing.T) {
	bridge := filepath.Join(t.TempDir(), "test-only.dll")
	if err := os.WriteFile(bridge, []byte("not loaded in this test"), 0600); err != nil {
		t.Fatal(err)
	}
	o, err := parseOptions([]string{"--out-dir", filepath.Join(t.TempDir(), "fresh"), "--capture-mode=wgc", "--capture-bridge", bridge}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if o.title() != "" || o.theme("dark") != mygo.ThemeSystem || o.theme("light") != mygo.ThemeSystem || o.bridgeSHA == "" {
		t.Fatal("WGC changed default native frame profile")
	}
	root := filepath.Join(t.TempDir(), "fresh")
	if err := freshOutput(root); err != nil {
		t.Fatal(err)
	}
	if err := freshOutput(root); err == nil {
		t.Fatal("existing empty directory accepted")
	}
	file := filepath.Join(root, "proof.txt")
	if err := writeFresh(file, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := writeFresh(file, []byte("replacement")); err == nil {
		t.Fatal("existing evidence overwritten")
	}
	data, _ := os.ReadFile(file)
	if string(data) != "first" {
		t.Fatal("old evidence changed")
	}
	if err := freshOutput(filepath.Join(file, "child")); err == nil {
		t.Fatal("file ancestor accepted")
	}
	link := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, link); err == nil {
		if err := freshOutput(filepath.Join(link, "fresh")); err == nil {
			t.Fatal("symlink ancestor accepted")
		}
	}
}

func TestTabStripFixtureMetadataKeepsOriginalTransport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scene.png.capture.json")
	original := []byte(`{"schemaVersion":1,"pngSHA256":"original-hash","pixelWidth":1682,"clientOrigin":[1,31]}`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	state := workbench.TabStripFixtureState{Scene: workbench.TabStripFixtureScene{Appearance: "dark", HostWidth: 400, Count: 0}, ParentSessionID: "parent"}
	before := windowMetrics{PID: 1, HWND: 2, DPI: 192, ActualScale: 2, ForegroundHWND: 2, NativeWindowActive: true}
	after := before
	after.PointerMessagesSent = true
	data, err := attachFixtureMetadata(path, state, before, after, nil, "wgc")
	if err != nil {
		t.Fatal(err)
	}
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal(data, &metadata); err != nil {
		t.Fatal(err)
	}
	var origin []int
	if err := json.Unmarshal(metadata["clientOrigin"], &origin); err != nil {
		t.Fatal(err)
	}
	if string(metadata["pngSHA256"]) != `"original-hash"` || len(origin) != 2 || origin[0] != 1 || origin[1] != 31 || metadata["fixtureState"] == nil || metadata["fixtureNative"] == nil || metadata["fixtureNativeAfter"] == nil {
		t.Fatal("transport metadata was replaced instead of extended")
	}
	var savedBefore, savedAfter windowMetrics
	if err := json.Unmarshal(metadata["fixtureNative"], &savedBefore); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(metadata["fixtureNativeAfter"], &savedAfter); err != nil {
		t.Fatal(err)
	}
	if savedBefore != before || savedAfter != after {
		t.Fatal("distinct before/after measurements were not retained")
	}
	record, err := json.Marshal(captureRecord{Native: before, NativeAfter: after})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(record, &fields); err != nil || fields["nativeAfter"] == nil {
		t.Fatal("manifest record omitted nativeAfter")
	}
	saved, err := os.ReadFile(path + ".transport-original.json")
	if err != nil || !bytes.Equal(saved, original) {
		t.Fatal("original transport bytes not retained")
	}
	if _, err := attachFixtureMetadata(path, state, before, after, nil, "wgc"); err == nil {
		t.Fatal("already enriched metadata accepted")
	}
	clientPath := filepath.Join(t.TempDir(), "client.capture.json")
	if _, err := attachFixtureMetadata(clientPath, state, before, after, nil, "client"); err != nil {
		t.Fatal(err)
	}
	if _, err := attachFixtureMetadata(clientPath, state, before, after, nil, "client"); err == nil {
		t.Fatal("client metadata was overwritten")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if pause(cancelled, time.Hour) == nil {
		t.Fatal("cancelled wait did not stop")
	}
}

func TestTabStripFixtureCapturePostcheck(t *testing.T) {
	fixture, err := workbench.NewTabStripFixture(workbench.TabStripFixtureScene{Appearance: "dark", HostWidth: 400, Count: 6})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fixture.Close)
	tt := ui.NewTester(fixture.View, 840, 560)
	tt.Move(10, 10)
	tt.Advance(time.Second)
	expected := fixture.State()
	if !expected.Ready() {
		t.Fatal("postcheck fixture did not settle through production layout")
	}
	before := windowMetrics{PID: 1, HWND: 2, DPI: 192, ActualScale: 2, PointerTargetHWND: 3, ForegroundHWND: 2, NativeWindowActive: true, PointerMessagesSent: true}
	for _, test := range []struct {
		name   string
		change func(*workbench.TabStripFixtureState, *windowMetrics)
	}{
		{"unchanged", func(*workbench.TabStripFixtureState, *windowMetrics) {}},
		{"foreground changed", func(_ *workbench.TabStripFixtureState, n *windowMetrics) {
			n.ForegroundHWND = 4
			n.NativeWindowActive = false
		}},
		{"active flag inconsistent", func(_ *workbench.TabStripFixtureState, n *windowMetrics) { n.NativeWindowActive = false }},
		{"different window", func(_ *workbench.TabStripFixtureState, n *windowMetrics) { n.HWND = 4; n.ForegroundHWND = 4 }},
		{"scale changed", func(_ *workbench.TabStripFixtureState, n *windowMetrics) { n.DPI = 96; n.ActualScale = 1 }},
		{"pointer changed", func(s *workbench.TabStripFixtureState, _ *windowMetrics) { s.Pointer.LastMoveX = 20 }},
		{"hover changed", func(s *workbench.TabStripFixtureState, _ *windowMetrics) { s.Pointer.AnyControlHovered = true }},
		{"geometry changed", func(s *workbench.TabStripFixtureState, _ *windowMetrics) { s.Geometry.Host.Width-- }},
	} {
		t.Run(test.name, func(t *testing.T) {
			current, after := expected, before
			test.change(&current, &after)
			err := validateCapturedState(current, expected, before, after)
			if (err == nil) != (test.name == "unchanged") {
				t.Fatalf("postcheck accepted changed state or rejected unchanged state: %v", err)
			}
		})
	}
}

func TestTabStripStyleSupplementFlagsAndPostcheck(t *testing.T) {
	for _, style := range []string{"inactive-hover", "dirty-file", "file-hover", "file-hover-edge"} {
		o, err := parseOptions([]string{"--out-dir", filepath.Join(t.TempDir(), "fresh"), "--style-case", style}, &bytes.Buffer{})
		if err != nil {
			t.Fatal(err)
		}
		all := scenes(o)
		if len(all) != 2 || all[0].Appearance != "dark" || all[1].Appearance != "light" || all[0].HostWidth != 400 || all[0].Count != 2 || all[0].ScrollFraction != 0 || captureName(all[0], style) != "tabstrip-dark-w400-n2-scrollstart-style-"+style {
			t.Fatal("supplement changed controlled scene names/data")
		}
		f, err := workbench.NewTabStripFixture(all[0])
		if err != nil {
			t.Fatal(err)
		}
		if err := f.SetStyleCase(style); err != nil {
			t.Fatal(err)
		}
		tt := ui.NewTester(f.View, 840, 560)
		tt.Advance(time.Second)
		x, y, valid := workbench.TabStripFixturePointerTarget(f.State())
		if !valid {
			t.Fatal("missing actual pointer target")
		}
		tt.Move(x, y)
		tt.Advance(time.Second)
		expected := f.State()
		native := windowMetrics{PID: 1, HWND: 2, DPI: 192, ActualScale: 2, PointerTargetHWND: 3, ForegroundHWND: 2, NativeWindowActive: true, PointerMessagesSent: true}
		if err := validateCapturedState(expected, expected, native, native); err != nil {
			t.Fatal(err)
		}
		changed := f.State()
		changed.StyleCase = ""
		if validateCapturedState(changed, expected, native, native) == nil {
			t.Fatal("postcheck accepted changed supplement")
		}
		changed = f.State()
		changed.StyleEvidence.HoveredTabID = "other"
		if validateCapturedState(changed, expected, native, native) == nil {
			t.Fatal("postcheck accepted changed production style state")
		}
		f.Close()
	}
	if _, err := parseOptions([]string{"--out-dir", filepath.Join(t.TempDir(), "fresh"), "--style-case=unknown"}, &bytes.Buffer{}); err == nil {
		t.Fatal("unknown style accepted")
	}
}
