//go:build uifixture

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"time"

	"gcode/internal/design"
	"gcode/internal/desktop"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type footerFixtureCapture struct {
	Name        string                          `json:"name"`
	File        string                          `json:"file"`
	SHA256      string                          `json:"sha256"`
	PixelWidth  int                             `json:"pixelWidth"`
	PixelHeight int                             `json:"pixelHeight"`
	State       desktop.FooterUsageFixtureState `json:"state"`
}

func footerFixtureScenes() []string {
	result := []string{"known", "unknown", "capacity", "zero", "over", "warning", "danger"}
	for _, base := range []string{"known", "capacity", "zero", "over", "warning", "danger"} {
		result = append(result, base+"-context")
	}
	return append(result, "known-accounts")
}

func runFooterUsageFixture(root string, delay time.Duration, settings captureSettings) error {
	if err := os.MkdirAll(root, 0755); err != nil {
		return err
	}
	fixture, err := desktop.NewFooterUsageFixture("dark", "known")
	if err != nil {
		return err
	}
	defer fixture.Close()
	mygo.App.SetName("Zeron footer usage fixture")
	mygo.App.SetPath(mygo.PathUserData, filepath.Join(root, "native-profile"))
	mygo.Theme.SetSource(settings.theme("dark"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	mygo.App.WhenReady(func() {
		window := mygo.NewWindow(mygo.WindowOptions{Title: settings.title(), X: 40, Y: 40, Width: 840, Height: 560, MinWidth: 840, MinHeight: 560, UseContentSize: true, Content: ui.View(fixture.View)})
		fixture.UseNativeWindow(window)
		if settings.mode == "wgc" {
			window.SetTitle("")
		}
		window.OnClosed(cancel)
		window.Show()
		handle := window.NativeHandle()
		go func() {
			done <- captureFooterUsageScenes(ctx, window, handle, fixture, root, delay, settings)
			mygo.App.Quit()
		}()
	})
	if err := mygo.App.Run(); err != nil {
		return err
	}
	return <-done
}

func captureFooterUsageScenes(ctx context.Context, window *mygo.Window, handle uintptr, fixture *desktop.FooterUsageFixture, root string, delay time.Duration, settings captureSettings) error {
	var captures []footerFixtureCapture
	for _, appearance := range []string{"dark", "light"} {
		for _, scene := range footerFixtureScenes() {
			changed := make(chan error, 1)
			window.Update(func() {
				changed <- fixture.SetScene(appearance, scene)
				mygo.Theme.SetSource(settings.theme(appearance))
			})
			select {
			case err := <-changed:
				if err != nil {
					return err
				}
			case <-ctx.Done():
				return ctx.Err()
			}
			if err := footerFixturePause(ctx, delay); err != nil {
				return err
			}
			state, err := settledFooterState(ctx, window, fixture, appearance, scene)
			if err != nil {
				return err
			}
			name := "footer-" + scene + "-" + appearance
			if err := writeJSON(filepath.Join(root, name+".ready.json"), state); err != nil {
				return err
			}
			file := name + ".png"
			var data []byte
			if settings.mode == "wgc" {
				data, err = captureNativeWindow(ctx, handle, settings.bridgePath, filepath.Join(root, file))
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
			captures = append(captures, footerFixtureCapture{Name: name, File: file, SHA256: hex.EncodeToString(digest[:]), PixelWidth: config.Width, PixelHeight: config.Height, State: state})
			fmt.Println(filepath.Join(root, file))
		}
	}
	return writeJSON(filepath.Join(root, "evidence.json"), struct {
		SchemaVersion       int                    `json:"schemaVersion"`
		ReferenceCommit     string                 `json:"referenceCommit"`
		Status              string                 `json:"status"`
		CaptureMode         string                 `json:"captureMode"`
		CaptureBridgeSHA256 string                 `json:"captureBridgeSHA256,omitempty"`
		Framework           mygo.SourcePatch       `json:"framework"`
		InputAcceptance     string                 `json:"inputAcceptance"`
		Captures            []footerFixtureCapture `json:"captures"`
	}{1, design.SourceCommit, "implementation production footer captures; reference comparison pending", settings.mode, settings.bridgeSHA256, mygo.FrameworkPatch(), "Scene DTOs control popup state; native input is not established by this capture harness", captures})
}

func footerFixturePause(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func settledFooterState(ctx context.Context, window *mygo.Window, fixture *desktop.FooterUsageFixture, appearance, scene string) (desktop.FooterUsageFixtureState, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for {
		ready := make(chan desktop.FooterUsageFixtureState, 1)
		window.Update(func() { ready <- fixture.State() })
		select {
		case state := <-ready:
			if state.LayoutError != "" {
				return state, fmt.Errorf("footer geometry: %s", state.LayoutError)
			}
			if state.Appearance == appearance && state.Scene == scene && state.Width == 840 && state.Height == 560 && state.GeometryStable {
				return state, nil
			}
		case <-ctx.Done():
			return desktop.FooterUsageFixtureState{}, ctx.Err()
		}
		if err := footerFixturePause(ctx, 16*time.Millisecond); err != nil {
			return desktop.FooterUsageFixtureState{}, err
		}
	}
}
