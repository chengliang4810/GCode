//go:build uifixture

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image/png"
	"io"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"time"

	"gcode/internal/design"
	"gcode/internal/workbench"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

var fixtureSourceStamp = "unstamped"

type options struct {
	out, mode, bridge, bridgeSHA string
	styleCase                    string
	matrix                       bool
	scene                        workbench.TabStripFixtureScene
	delay                        time.Duration
	hoverDwell                   time.Duration
	expectedScale                float64
}

func parseOptions(args []string, stderr io.Writer) (options, error) {
	o := options{}
	fs := flag.NewFlagSet("tabstrip-fixture", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&o.out, "out-dir", "", "Fresh, nonexistent output directory")
	fs.BoolVar(&o.matrix, "matrix", true, "Capture all36 controlled scenes")
	fs.StringVar(&o.styleCase, "style-case", "", "Separate two-theme supplement: inactive-hover, dirty-file, file-hover or file-hover-edge")
	fs.StringVar(&o.scene.Appearance, "appearance", "dark", "Single-scene appearance: dark or light")
	fs.IntVar(&o.scene.HostWidth, "host-width", 400, "Single-scene host width:400 or220 DIPs")
	fs.IntVar(&o.scene.Count, "count", 6, "Single-scene session-tab count:0,2 or6")
	fraction := fs.Float64("scroll-fraction", 0, "Single-scene positive scroll fraction from0 to1")
	fs.StringVar(&o.mode, "capture-mode", "client", "client or wgc")
	fs.StringVar(&o.bridge, "capture-bridge", "", "Absolute fixture-only WGC bridge DLL path")
	fs.DurationVar(&o.delay, "capture-delay", 350*time.Millisecond, "Minimum presentation delay before checking committed readiness")
	fs.DurationVar(&o.hoverDwell, "hover-dwell", 0, "Monotonic dwell after observed production hover settles; hover supplements only, at most10s")
	fs.Float64Var(&o.expectedScale, "expected-scale", 2, "Required native scale:1 or2")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	if fs.NArg() != 0 {
		return o, errors.New("positional arguments are not accepted")
	}
	if o.out == "" {
		return o, errors.New("--out-dir is required")
	}
	if math.IsNaN(*fraction) || math.IsInf(*fraction, 0) || *fraction < 0 || *fraction > 1 {
		return o, errors.New("scroll fraction must be finite and between zero and one")
	}
	o.scene.ScrollFraction = float32(*fraction)
	if err := o.scene.Validate(); err != nil {
		return o, err
	}
	if o.styleCase != "" && o.styleCase != "inactive-hover" && o.styleCase != "dirty-file" && o.styleCase != "file-hover" && o.styleCase != "file-hover-edge" {
		return o, errors.New("style case must be inactive-hover, dirty-file, file-hover or file-hover-edge")
	}
	if o.delay < 0 {
		return o, errors.New("capture delay cannot be negative")
	}
	if o.hoverDwell < 0 || o.hoverDwell > 10*time.Second {
		return o, errors.New("hover dwell must be between zero and10s")
	}
	if o.hoverDwell > 0 && o.styleCase != "inactive-hover" && o.styleCase != "file-hover" && o.styleCase != "file-hover-edge" {
		return o, errors.New("positive hover dwell requires inactive-hover, file-hover or file-hover-edge")
	}
	if o.expectedScale != 1 && o.expectedScale != 2 {
		return o, errors.New("expected scale must be 1 or 2")
	}
	switch o.mode {
	case "client":
		if o.bridge != "" {
			return o, errors.New("--capture-bridge requires --capture-mode=wgc")
		}
	case "wgc":
		if !filepath.IsAbs(o.bridge) {
			return o, errors.New("WGC mode requires an absolute --capture-bridge DLL path")
		}
		data, err := os.ReadFile(o.bridge)
		if err != nil {
			return o, fmt.Errorf("read capture bridge: %w", err)
		}
		o.bridgeSHA = digest(data)
	default:
		return o, errors.New("capture mode must be client or wgc")
	}
	var err error
	o.out, err = filepath.Abs(o.out)
	return o, err
}

func scenes(o options) []workbench.TabStripFixtureScene {
	if o.styleCase != "" {
		return []workbench.TabStripFixtureScene{{Appearance: "dark", HostWidth: 400, Count: 2}, {Appearance: "light", HostWidth: 400, Count: 2}}
	}
	if !o.matrix {
		return []workbench.TabStripFixtureScene{o.scene}
	}
	var all []workbench.TabStripFixtureScene
	for _, appearance := range []string{"dark", "light"} {
		for _, width := range []int{400, 220} {
			for _, count := range []int{0, 2, 6} {
				for _, fraction := range []float32{0, .5, 1} {
					all = append(all, workbench.TabStripFixtureScene{Appearance: appearance, HostWidth: width, Count: count, ScrollFraction: fraction})
				}
			}
		}
	}
	return all
}

func sceneName(s workbench.TabStripFixtureScene) string {
	position := fmt.Sprintf("%g", s.ScrollFraction)
	switch s.ScrollFraction {
	case 0:
		position = "start"
	case .5:
		position = "middle"
	case 1:
		position = "end"
	}
	return fmt.Sprintf("tabstrip-%s-w%d-n%d-scroll%s", s.Appearance, s.HostWidth, s.Count, position)
}

func captureName(s workbench.TabStripFixtureScene, style string) string {
	name := sceneName(s)
	if style != "" {
		name += "-style-" + style
	}
	return name
}

func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func main() {
	o, err := parseOptions(os.Args[1:], os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if err := run(o); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func freshOutput(root string) error {
	if !filepath.IsAbs(root) || root == filepath.VolumeName(root)+string(filepath.Separator) {
		return errors.New("output must name a new absolute child directory")
	}
	for entry := root; ; entry = filepath.Dir(entry) {
		info, err := os.Lstat(entry)
		if err == nil {
			if entry == root {
				return errors.New("output directory must not already exist, even when empty")
			}
			if info.Mode()&os.ModeSymlink != 0 || isReparse(info) {
				return fmt.Errorf("output ancestor is a link or reparse point: %s", entry)
			}
			if !info.IsDir() {
				return fmt.Errorf("output ancestor is not a directory: %s", entry)
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		if parent := filepath.Dir(entry); parent == entry {
			break
		}
	}
	if err := os.MkdirAll(filepath.Dir(root), 0755); err != nil {
		return err
	}
	return os.Mkdir(root, 0755)
}

func (o options) title() string {
	if o.mode == "wgc" {
		return ""
	}
	return "Tab strip fixture"
}
func (o options) theme(appearance string) mygo.ThemeSource {
	if o.mode == "wgc" {
		return mygo.ThemeSystem
	}
	return mygo.ThemeSource(appearance)
}

func run(o options) error {
	if err := freshOutput(o.out); err != nil {
		return err
	}
	for name, path := range map[mygo.PathName]string{mygo.PathUserData: "profile", mygo.PathCache: "cache", mygo.PathLogs: "logs", mygo.PathTemp: "temp"} {
		dir := filepath.Join(o.out, path)
		if err := os.Mkdir(dir, 0755); err != nil {
			return err
		}
		mygo.App.SetPath(name, dir)
		if name == mygo.PathTemp {
			if err := os.Setenv("TMP", dir); err != nil {
				return err
			}
			if err := os.Setenv("TEMP", dir); err != nil {
				return err
			}
		}
	}
	all := scenes(o)
	fixture, err := workbench.NewTabStripFixture(all[0])
	if err != nil {
		return err
	}
	defer fixture.Close()
	if err := fixture.SetStyleCase(o.styleCase); err != nil {
		return err
	}
	mygo.App.SetName("RCode tab strip fixture")
	mygo.Theme.SetSource(o.theme(all[0].Appearance))
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	done := make(chan error, 1)
	mygo.App.WhenReady(func() {
		window := mygo.NewWindow(mygo.WindowOptions{Title: o.title(), X: 40, Y: 40, Width: 840, Height: 560, UseContentSize: true, Content: ui.View(fixture.View)})
		fixture.UseNativeWindow(window)
		if o.mode == "wgc" {
			window.SetTitle("")
		}
		window.OnClosed(cancel)
		window.Show()
		hwnd := window.NativeHandle()
		go func() { done <- captureScenes(ctx, window, hwnd, fixture, all, o); mygo.App.Quit() }()
	})
	if err := mygo.App.Run(); err != nil {
		return err
	}
	return <-done
}

type windowMetrics struct {
	PID                   int            `json:"pid"`
	HWND                  uint64         `json:"hwnd"`
	DPI                   uint32         `json:"windowDpi,omitempty"`
	ActualScale           float64        `json:"actualScale"`
	WindowBounds          mygo.Rectangle `json:"windowBounds"`
	ClientBounds          mygo.Rectangle `json:"clientBounds"`
	RequestedWindowOrigin [2]int         `json:"requestedWindowOriginDIP"`
	PointerTargetHWND     uint64         `json:"pointerTargetHWND"`
	ForegroundHWND        uint64         `json:"foregroundHWND"`
	NativeWindowActive    bool           `json:"nativeWindowActive"`
	PointerMessagesSent   bool           `json:"pointerMessagesSent"`
}

type captureRecord struct {
	Name           string                         `json:"name"`
	File           string                         `json:"file"`
	SHA256         string                         `json:"sha256"`
	PixelWidth     int                            `json:"pixelWidth"`
	PixelHeight    int                            `json:"pixelHeight"`
	MetadataFile   string                         `json:"captureMetadataFile"`
	MetadataSHA256 string                         `json:"captureMetadataSHA256"`
	Native         windowMetrics                  `json:"native"`
	NativeAfter    windowMetrics                  `json:"nativeAfter"`
	State          workbench.TabStripFixtureState `json:"state"`
	HoverDwell     *hoverDwellEvidence            `json:"hoverDwell,omitempty"`
}

func captureScenes(ctx context.Context, window *mygo.Window, hwnd uintptr, fixture *workbench.TabStripFixture, all []workbench.TabStripFixtureScene, o options) error {
	var captures []captureRecord
	for _, scene := range all {
		changed := make(chan error, 1)
		window.Update(func() {
			if err := fixture.SetScene(scene); err != nil {
				changed <- err
				return
			}
			mygo.Theme.SetSource(o.theme(scene.Appearance))
			window.SetContentSize(840, 560)
			changed <- nil
		})
		select {
		case err := <-changed:
			if err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
		if err := pause(ctx, o.delay); err != nil {
			return err
		}
		state, native, err := settledState(ctx, window, hwnd, fixture, scene, o.expectedScale)
		if err != nil {
			diagnosticErr := writeJSONFresh(filepath.Join(o.out, captureName(scene, o.styleCase)+".settle-error.json"), struct {
				Error      string                         `json:"error"`
				LastState  workbench.TabStripFixtureState `json:"lastObservedState"`
				LastNative windowMetrics                  `json:"lastObservedNative"`
			}{err.Error(), state, native})
			return errors.Join(err, diagnosticErr)
		}
		name := captureName(scene, o.styleCase)
		var dwell *hoverDwellEvidence
		if o.hoverDwell > 0 {
			expected := resultSnapshot{state, native}
			actual, evidence, dwellErr := captureHoverDwell(ctx, window, fixture, expected, o.hoverDwell)
			if dwellErr != nil {
				diagnosticErr := writeJSONFresh(filepath.Join(o.out, name+".hover-dwell-error.json"), struct {
					Error      string              `json:"error"`
					Expected   resultSnapshot      `json:"expected"`
					Actual     resultSnapshot      `json:"actual"`
					HoverDwell *hoverDwellEvidence `json:"hoverDwell,omitempty"`
				}{dwellErr.Error(), expected, actual, evidence})
				return errors.Join(dwellErr, diagnosticErr)
			}
			state, native, dwell = actual.State, actual.Native, evidence
		}
		if err := writeJSONFresh(filepath.Join(o.out, name+".ready.json"), state); err != nil {
			return err
		}
		file := name + ".png"
		var data []byte
		if o.mode == "wgc" {
			data, err = captureNativeWindow(ctx, hwnd, o.bridge, filepath.Join(o.out, file))
		} else {
			data, err = window.CapturePage()
		}
		if err != nil {
			return err
		}
		image, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return err
		}
		postcheck, err := confirmCapturedState(ctx, window, fixture, state, native)
		if err != nil {
			diagnosticErr := writeJSONFresh(filepath.Join(o.out, name+".postcheck-error.json"), struct {
				Error    string         `json:"error"`
				Expected resultSnapshot `json:"expected"`
				Actual   resultSnapshot `json:"actual"`
			}{err.Error(), resultSnapshot{state, native}, postcheck})
			return errors.Join(err, diagnosticErr)
		}
		nativeAfter := postcheck.Native
		if o.mode == "client" {
			if err := writeFresh(filepath.Join(o.out, file), data); err != nil {
				return err
			}
		}
		metadataPath := filepath.Join(o.out, file+".capture.json")
		metadata, err := attachFixtureMetadata(metadataPath, state, native, nativeAfter, dwell, o.mode)
		if err != nil {
			return err
		}
		captures = append(captures, captureRecord{Name: name, File: file, SHA256: digest(data), PixelWidth: image.Width, PixelHeight: image.Height, MetadataFile: file + ".capture.json", MetadataSHA256: digest(metadata), Native: native, NativeAfter: nativeAfter, State: state, HoverDwell: dwell})
		fmt.Println(filepath.Join(o.out, file))
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	binary, err := os.ReadFile(exe)
	if err != nil {
		return err
	}
	return writeJSONFresh(filepath.Join(o.out, "evidence.json"), struct {
		SchemaVersion   int              `json:"schemaVersion"`
		ReferenceCommit string           `json:"referenceCommit"`
		Status          string           `json:"status"`
		Renderer        string           `json:"renderer"`
		CaptureMode     string           `json:"captureMode"`
		ExpectedScale   float64          `json:"expectedScale"`
		BridgeSHA256    string           `json:"captureBridgeSHA256,omitempty"`
		Framework       mygo.SourcePatch `json:"framework"`
		SourceStamp     string           `json:"sourceStamp"`
		BinarySHA256    string           `json:"binarySHA256"`
		Captures        []captureRecord  `json:"captures"`
	}{1, design.SourceCommit, "implementation-native-tabstrip-fixture; reference comparison pending", "production workbench.State.TabStrip", o.mode, o.expectedScale, o.bridgeSHA, mygo.FrameworkPatch(), fixtureSourceStamp, digest(binary), captures})
}

func confirmCapturedState(ctx context.Context, window *mygo.Window, fixture *workbench.TabStripFixture, expected workbench.TabStripFixtureState, before windowMetrics) (resultSnapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	type result struct {
		snapshot resultSnapshot
		err      error
	}
	ch := make(chan result, 1)
	window.Update(func() {
		native, err := measureNativeWindow(window, before.ActualScale, before.PointerMessagesSent)
		ch <- result{resultSnapshot{fixture.State(), native}, err}
	})
	select {
	case r := <-ch:
		if r.err != nil {
			return r.snapshot, r.err
		}
		return r.snapshot, validateCapturedState(r.snapshot.State, expected, before, r.snapshot.Native)
	case <-ctx.Done():
		return resultSnapshot{}, ctx.Err()
	}
}

func validateCapturedState(current, expected workbench.TabStripFixtureState, before, after windowMetrics) error {
	if !current.Ready() || current.Scene != expected.Scene || current.StyleCase != expected.StyleCase || !reflect.DeepEqual(current.StyleEvidence, expected.StyleEvidence) || current.PositiveX != expected.PositiveX || current.MaxX != expected.MaxX || !reflect.DeepEqual(current.Geometry, expected.Geometry) || !workbench.TabStripFixturePointerReady(current) || !after.NativeWindowActive || after.ForegroundHWND != after.HWND || after.HWND == 0 || after.HWND != before.HWND || after.PID != before.PID || after.DPI != before.DPI || after.ActualScale != before.ActualScale || after.PointerTargetHWND != before.PointerTargetHWND {
		return errors.New("fixture state, native window or hover changed during capture; PNG retained without success manifest")
	}
	return nil
}

// measureNativeWindow is called only inside Window.Update on the UI thread.
func measureNativeWindow(window *mygo.Window, expectedScale float64, messagesSent bool) (windowMetrics, error) {
	hwnd := window.NativeHandle()
	dpi, err := nativeWindowDPI(hwnd)
	pointerTarget, targetErr := nativePointerTarget(hwnd)
	if err == nil {
		err = targetErr
	}
	scale := mygo.Screen.DisplayMatching(window.Bounds()).ScaleFactor
	if dpi > 0 {
		scale = float64(dpi) / 96
	}
	if err == nil && (scale <= 0 || math.IsNaN(scale) || math.IsInf(scale, 0)) {
		err = errors.New("native actual scale is unavailable")
	}
	if err == nil && scale != expectedScale {
		err = fmt.Errorf("native scale %g does not match expected scale %g", scale, expectedScale)
	}
	foreground := nativeForegroundWindow()
	return windowMetrics{os.Getpid(), uint64(hwnd), dpi, scale, window.Bounds(), window.ContentBounds(), [2]int{40, 40}, uint64(pointerTarget), uint64(foreground), foreground == hwnd, messagesSent}, err
}

func settledState(ctx context.Context, window *mygo.Window, hwnd uintptr, fixture *workbench.TabStripFixture, scene workbench.TabStripFixtureScene, expectedScale float64) (workbench.TabStripFixtureState, windowMetrics, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	type result struct {
		state  workbench.TabStripFixtureState
		native windowMetrics
		err    error
	}
	var last result
	samples := 0
	pointerInjected := false
	fail := func(err error) (workbench.TabStripFixtureState, windowMetrics, error) {
		diagnostic, _ := json.Marshal(struct {
			Samples int            `json:"pollSamples"`
			Last    resultSnapshot `json:"lastObserved"`
		}{samples, resultSnapshot{last.state, last.native}})
		return last.state, last.native, fmt.Errorf("fixture scene did not settle: %s: %w", diagnostic, err)
	}
	for {
		ch := make(chan result, 1)
		messagesSent := pointerInjected
		window.Update(func() {
			native, err := measureNativeWindow(window, expectedScale, messagesSent)
			ch <- result{fixture.State(), native, err}
		})
		select {
		case r := <-ch:
			last = r
			samples++
			if r.err != nil {
				return fail(r.err)
			}
			if r.state.LayoutError != "" {
				return fail(errors.New(r.state.LayoutError))
			}
			geometryReady := r.state.Scene == scene && r.state.NativeWindow && r.state.Ready() && r.native.NativeWindowActive
			if geometryReady && !pointerInjected {
				// Wait for committed widget handlers before posting input. The
				// first scene can otherwise receive both messages before its
				// initial view exists, which loses the fixture observations.
				sent := make(chan error, 1)
				x, y, valid := workbench.TabStripFixturePointerTarget(r.state)
				if !valid {
					return fail(errors.New("style pointer target is not laid out"))
				}
				window.Update(func() { sent <- syntheticFixturePointerAt(hwnd, x, y) })
				select {
				case err := <-sent:
					if err != nil {
						return fail(err)
					}
				case <-ctx.Done():
					return fail(ctx.Err())
				}
				pointerInjected = true
			} else if geometryReady && pointerInjected && workbench.TabStripFixturePointerReady(r.state) {
				return r.state, r.native, nil
			}
		case <-ctx.Done():
			return fail(ctx.Err())
		}
		if err := pause(ctx, 16*time.Millisecond); err != nil {
			return fail(err)
		}
	}
}

type resultSnapshot struct {
	State  workbench.TabStripFixtureState `json:"state"`
	Native windowMetrics                  `json:"native"`
}

func pause(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func writeFresh(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	closeErr := f.Close()
	return errors.Join(err, closeErr)
}
func writeJSONFresh(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeFresh(path, append(data, '\n'))
}

func attachFixtureMetadata(path string, state workbench.TabStripFixtureState, native, nativeAfter windowMetrics, dwell *hoverDwellEvidence, mode string) ([]byte, error) {
	metadata := map[string]json.RawMessage{}
	if mode == "wgc" {
		original, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(original, &metadata); err != nil {
			return nil, err
		}
		for _, name := range []string{"fixtureState", "fixtureNative", "fixtureNativeAfter", "fixtureHoverDwell"} {
			if _, ok := metadata[name]; ok {
				return nil, errors.New("transport metadata already contains fixture metadata")
			}
		}
		if err := writeFresh(path+".transport-original.json", original); err != nil {
			return nil, err
		}
	}
	var err error
	metadata["fixtureState"], err = json.Marshal(state)
	if err != nil {
		return nil, err
	}
	metadata["fixtureNative"], err = json.Marshal(native)
	if err != nil {
		return nil, err
	}
	metadata["fixtureNativeAfter"], err = json.Marshal(nativeAfter)
	if err != nil {
		return nil, err
	}
	if dwell != nil {
		metadata["fixtureHoverDwell"], err = json.Marshal(dwell)
		if err != nil {
			return nil, err
		}
	}
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')
	if mode == "wgc" {
		err = os.WriteFile(path, data, 0644)
	} else {
		err = writeFresh(path, data)
	}
	return data, err
}
