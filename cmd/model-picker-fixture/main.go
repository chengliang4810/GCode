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
	"log"
	"os"
	"path/filepath"
	"time"

	"gcode/internal/design"
	"gcode/internal/desktop"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type scene struct {
	appearance, page string
	width, height    int
}

func (s scene) name() string {
	width := "wide"
	if s.width == 360 {
		width = "narrow"
	}
	page := "panel"
	if s.page == "list" {
		page = "model-list"
	}
	return "compact-" + page + "-" + s.appearance + "-" + width
}

func (s scene) ready(state desktop.CompactPickerFixtureState) bool {
	return state.PopoverOpen && state.Appearance == s.appearance && state.Page == s.page &&
		state.Width == float32(s.width) && state.Height == float32(s.height) &&
		state.Geometry.Ready() && state.GeometryStable && state.LayoutError == ""
}

type capture struct {
	Name         string                            `json:"name"`
	File         string                            `json:"file"`
	SHA256       string                            `json:"sha256"`
	PixelWidth   int                               `json:"pixelWidth"`
	PixelHeight  int                               `json:"pixelHeight"`
	MetadataFile string                            `json:"captureMetadataFile,omitempty"`
	State        desktop.CompactPickerFixtureState `json:"state"`
}

type captureSettings struct {
	mode, bridgePath, bridgeSHA256 string
}

func newCaptureSettings(mode, bridgePath string) (captureSettings, error) {
	settings := captureSettings{mode: mode, bridgePath: bridgePath}
	switch mode {
	case "client":
		if bridgePath != "" {
			return settings, errors.New("--capture-bridge requires --capture-mode=wgc")
		}
	case "wgc":
		if !filepath.IsAbs(bridgePath) {
			return settings, errors.New("WGC mode requires an absolute --capture-bridge DLL path")
		}
		data, err := os.ReadFile(bridgePath)
		if err != nil {
			return settings, fmt.Errorf("read WGC capture bridge: %w", err)
		}
		digest := sha256.Sum256(data)
		settings.bridgeSHA256 = hex.EncodeToString(digest[:])
	default:
		return settings, errors.New("--capture-mode must be client or wgc")
	}
	return settings, nil
}

func (s captureSettings) theme(appearance string) mygo.ThemeSource {
	if s.mode == "wgc" {
		return mygo.ThemeSystem
	}
	return mygo.ThemeSource(appearance)
}

func (s captureSettings) title() string {
	if s.mode == "wgc" {
		return ""
	}
	return "Compact picker fixture"
}

func main() {
	log.SetFlags(0)
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	output := flag.String("out-dir", "", "Fresh output directory for native content PNGs and metadata")
	matrix := flag.Bool("matrix", true, "Capture dark/light x 840/360 client width x panel/list at height 560")
	appearance := flag.String("appearance", "dark", "Single-scene appearance: dark or light")
	page := flag.String("page", "panel", "Single-scene page: panel or list")
	width := flag.Int("width", 840, "Single-scene client width in DIPs")
	height := flag.Int("height", 560, "Single-scene client height in DIPs")
	delay := flag.Duration("capture-delay", 260*time.Millisecond, "Wait for native layout before each content capture")
	captureMode := flag.String("capture-mode", "client", "Capture client content or the complete native window with wgc")
	captureBridge := flag.String("capture-bridge", "", "Absolute fixture-only WGC bridge DLL path; required in wgc mode")
	footerUsage := flag.Bool("footer-usage", false, "Capture the production footer usage ring/card matrix at 840x560")
	flag.Parse()
	if *output == "" || *width < 240 || *height < 280 || *delay < 0 {
		return errors.New("--out-dir is required; minimum client size is 240x280; capture delay must be nonnegative")
	}
	if flag.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if *appearance != "dark" && *appearance != "light" || *page != "panel" && *page != "list" {
		return errors.New("appearance must be dark/light and page must be panel/list")
	}
	captureSettings, err := newCaptureSettings(*captureMode, *captureBridge)
	if err != nil {
		return err
	}
	root, err := filepath.Abs(*output)
	if err != nil {
		return err
	}
	if entries, err := os.ReadDir(root); err == nil && len(entries) != 0 {
		return fmt.Errorf("output directory must be empty: %s", root)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if *footerUsage {
		return runFooterUsageFixture(root, *delay, captureSettings)
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		return err
	}
	scenes := []scene{{*appearance, *page, *width, *height}}
	if *matrix {
		scenes = nil
		for _, mode := range []string{"dark", "light"} {
			for _, width := range []int{840, 360} {
				for _, page := range []string{"panel", "list"} {
					scenes = append(scenes, scene{mode, page, width, 560})
				}
			}
		}
	}
	fixture, err := desktop.NewCompactPickerFixture(scenes[0].appearance, scenes[0].page)
	if err != nil {
		return err
	}
	defer fixture.Close()
	mygo.App.SetName("Zeron compact picker fixture")
	mygo.App.SetPath(mygo.PathUserData, filepath.Join(root, "native-profile"))
	mygo.Theme.SetSource(captureSettings.theme(scenes[0].appearance))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	mygo.App.WhenReady(func() {
		window := mygo.NewWindow(mygo.WindowOptions{
			Title: captureSettings.title(), Width: scenes[0].width, Height: scenes[0].height,
			MinWidth: 240, MinHeight: 280, UseContentSize: true, Content: ui.View(fixture.View),
		})
		fixture.UseNativeWindow(window)
		if captureSettings.mode == "wgc" {
			// NewWindow defaults an empty option title to the app name.
			// SetTitle deliberately keeps the reference's blank native title.
			window.SetTitle("")
		}
		window.OnClosed(cancel)
		window.Show()
		nativeHandle := window.NativeHandle()
		go func() {
			done <- captureScenes(ctx, window, nativeHandle, fixture, root, scenes, *delay, captureSettings)
			mygo.App.Quit()
		}()
	})
	if err := mygo.App.Run(); err != nil {
		return err
	}
	return <-done
}

func captureScenes(ctx context.Context, window *mygo.Window, nativeHandle uintptr, fixture *desktop.CompactPickerFixture, root string, scenes []scene, delay time.Duration, settings captureSettings) error {
	var captures []capture
	for _, scene := range scenes {
		changed := make(chan error, 1)
		window.Update(func() {
			if err := fixture.SetScene(scene.appearance, scene.page); err != nil {
				changed <- err
				return
			}
			mygo.Theme.SetSource(settings.theme(scene.appearance))
			window.SetContentSize(scene.width, scene.height)
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
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		}
		state, err := settledState(ctx, window, fixture, scene)
		if err != nil {
			return err
		}
		if err := writeJSON(filepath.Join(root, scene.name()+".ready.json"), state); err != nil {
			return err
		}
		file := scene.name() + ".png"
		var data []byte
		if settings.mode == "wgc" {
			data, err = captureNativeWindow(ctx, nativeHandle, settings.bridgePath, filepath.Join(root, file))
		} else {
			data, err = window.CapturePage()
		}
		if err != nil {
			return err
		}
		config, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return err
		}
		if settings.mode == "client" {
			if err := os.WriteFile(filepath.Join(root, file), data, 0644); err != nil {
				return err
			}
		}
		digest := sha256.Sum256(data)
		metadataFile := ""
		if settings.mode == "wgc" {
			metadataFile = file + ".capture.json"
		}
		captures = append(captures, capture{
			Name: scene.name(), File: file, SHA256: hex.EncodeToString(digest[:]),
			PixelWidth: config.Width, PixelHeight: config.Height, MetadataFile: metadataFile, State: state,
		})
		fmt.Println(filepath.Join(root, file))
	}
	return writeJSON(filepath.Join(root, "evidence.json"), struct {
		SchemaVersion       int              `json:"schemaVersion"`
		ReferenceCommit     string           `json:"referenceCommit"`
		ReferenceFixture    string           `json:"referenceFixture"`
		Status              string           `json:"status"`
		Renderer            string           `json:"renderer"`
		ReferenceComparison string           `json:"referenceComparison"`
		WindowFrame         string           `json:"windowFrame"`
		GeometrySpace       string           `json:"geometrySpace"`
		CaptureMode         string           `json:"captureMode"`
		CaptureBridgeSHA256 string           `json:"captureBridgeSHA256,omitempty"`
		Framework           mygo.SourcePatch `json:"framework"`
		Captures            []capture        `json:"captures"`
	}{
		SchemaVersion: 2, ReferenceCommit: design.SourceCommit,
		ReferenceFixture: "crates/ui/examples/compact-picker-fixture.rs + pickers.rs::fixture_compact_catalog",
		Status:           "implementation-native-fixture-captures", Renderer: captureRenderer(settings.mode),
		ReferenceComparison: "pending", WindowFrame: captureFrame(settings.mode),
		GeometrySpace: "committed layout boxes in window client DIPs",
		CaptureMode:   settings.mode, CaptureBridgeSHA256: settings.bridgeSHA256,
		Framework: mygo.FrameworkPatch(), Captures: captures,
	})
}

func captureRenderer(mode string) string {
	if mode == "wgc" {
		return "fixture-only in-process Windows Graphics Capture bridge; complete native window"
	}
	return "MyGo Window.CapturePage native content"
}

func captureFrame(mode string) string {
	if mode == "wgc" {
		return "default native frame, blank title, system OS appearance; unmodified full-window PNG"
	}
	return "default native frame; PNGs capture client content only"
}

func settledState(ctx context.Context, window *mygo.Window, fixture *desktop.CompactPickerFixture, scene scene) (desktop.CompactPickerFixtureState, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var state desktop.CompactPickerFixtureState
	for {
		ready := make(chan desktop.CompactPickerFixtureState, 1)
		window.Update(func() { ready <- fixture.State() })
		select {
		case state = <-ready:
		case <-ctx.Done():
			return state, fmt.Errorf("fixture scene did not settle: %+v: %w", state, ctx.Err())
		}
		if state.LayoutError != "" {
			return state, fmt.Errorf("fixture scene did not settle: %s", state.LayoutError)
		}
		if scene.ready(state) {
			return state, nil
		}
		timer := time.NewTimer(16 * time.Millisecond)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return state, fmt.Errorf("fixture scene did not settle: %+v: %w", state, ctx.Err())
		}
	}
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}
