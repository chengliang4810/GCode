package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"context"
	"gcode/internal/assets"
	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/desktop"
	"gcode/internal/mock"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func main() {
	scenario := flag.String("scenario", "", "Deterministic data scenario (default, empty, loading, live, transcript, questions, permissions, showcase, offline, error, stress)")
	route := flag.String("route", "chat", "Initial route: chat, chat/<id>, settings/<section>, pull-requests")
	stateDir := flag.String("data-dir", "", "Independent state directory; scenarios default to memory")
	appearance := flag.String("appearance", "", "Appearance override: light, dark, system")
	themeID := flag.String("theme", "", "Exact built-in theme variant ID")
	dump := flag.String("dump-scene", "", "Write the initial versioned contract fixture as JSON and exit")
	capture := flag.String("capture", "", "Write a PNG capture")
	headless := flag.Bool("headless", false, "Render via MyGo Tester without a native window")
	exitCapture := flag.Bool("exit-after-capture", false, "Exit after native capture")
	captureDelay := flag.Duration("capture-delay", 2*time.Second, "Wait before a native capture")
	width := flag.Int("width", 1320, "Window client width in logical pixels")
	height := flag.Int("height", 880, "Window client height in logical pixels")
	scale := flag.Float64("scale", 1, "Scale for headless captures")
	flag.Parse()
	if !validScenario(*scenario) {
		log.Fatalf("unknown scenario %q", *scenario)
	}
	if *width < 900 || *height < 600 || *scale <= 0 || math.IsNaN(*scale) || math.IsInf(*scale, 0) {
		log.Fatal("window minimum is 900x600; scale must be positive")
	}
	if err := assets.RegisterFonts(); err != nil {
		log.Fatal(err)
	}
	statePath := ""
	if *stateDir != "" {
		statePath = filepath.Join(*stateDir, "state.json")
	} else if *scenario == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			log.Fatal(err)
		}
		statePath = filepath.Join(dir, "gcode-zeron-ui", "state.json")
	}
	opts := mock.Options{Scenario: *scenario, StatePath: statePath, Platform: runtime.GOOS, Architecture: runtime.GOARCH}
	if *scenario != "" {
		opts.Now = func() time.Time { return time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC) }
	}
	backend, err := mock.New(opts)
	if err != nil {
		log.Fatal(err)
	}
	if *appearance != "" || *themeID != "" {
		s, err := backend.Snapshot(context.Background())
		if err != nil {
			log.Fatal(err)
		}
		if *appearance != "" {
			s.Settings.Appearance.Mode = *appearance
			s.Settings.Appearance.ThemeID = ""
		}
		if *themeID != "" {
			found := false
			for _, variant := range design.Themes() {
				if variant.ID == *themeID {
					found = true
					s.Settings.Appearance.Mode = variant.Appearance
					s.Settings.Appearance.ThemeID = variant.ID
					break
				}
			}
			if !found {
				log.Fatalf("unknown theme variant %q", *themeID)
			}
		}
		if err := backend.Execute(context.Background(), contracts.Command{Kind: "settings.update", Settings: &s.Settings}); err != nil {
			log.Fatal(err)
		}
	}
	if *dump != "" {
		if *scenario == "" {
			log.Fatal("--dump-scene requires an explicit deterministic --scenario")
		}
		snapshot, err := backend.Snapshot(context.Background())
		if err != nil {
			log.Fatal(err)
		}
		data, err := json.MarshalIndent(struct {
			SourceCommit string             `json:"sourceCommit"`
			Scenario     string             `json:"scenario"`
			Snapshot     contracts.Snapshot `json:"snapshot"`
		}{design.SourceCommit, *scenario, snapshot}, "", "  ")
		if err != nil {
			log.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(*dump), 0755); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(*dump, data, 0644); err != nil {
			log.Fatal(err)
		}
		_ = backend.Close()
		fmt.Println(*dump)
		return
	}
	app, err := desktop.NewWithOptions(backend, desktop.Options{Now: opts.Now})
	if err != nil {
		log.Fatal(err)
	}
	defer app.Close()
	if *headless {
		app.OpenRoute(*route)
		tester := ui.NewTester(app.View, *width, *height)
		tester.SetScale(float32(*scale))
		tester.Advance(time.Second)
		if *capture == "" {
			log.Fatal("--headless requires --capture")
		}
		if err := os.MkdirAll(filepath.Dir(*capture), 0755); err != nil {
			log.Fatal(err)
		}
		f, err := os.Create(*capture)
		if err != nil {
			log.Fatal(err)
		}
		if err := png.Encode(f, tester.Image()); err != nil {
			_ = f.Close()
			log.Fatal(err)
		}
		if err := f.Close(); err != nil {
			log.Fatal(err)
		}
		fmt.Println(*capture)
		return
	}
	if err := configureNativeDirectories(statePath, *scenario); err != nil {
		log.Fatal(err)
	}
	mygo.App.SetName("Zeron")
	mygo.Theme.SetSource(mygo.ThemeSource(app.Snapshot().Settings.Appearance.Mode))
	mygo.App.WhenReady(func() {
		mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
			{Role: mygo.RoleAppMenu}, {Label: "File", Submenu: []*mygo.MenuItem{
				{Label: "New session", Accelerator: "CmdOrCtrl+N", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { app.NewConversation() }},
				{Label: "Settings", Accelerator: "CmdOrCtrl+,", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { app.OpenRoute("settings") }},
				{Role: mygo.RoleClose},
			}},
			{Role: mygo.RoleEditMenu}, {Role: mygo.RoleWindowMenu},
		}))
		stateKey := "gcode-zeron-ui-main"
		if *scenario != "" {
			stateKey = ""
		}
		window := mygo.NewWindow(mygo.WindowOptions{Title: "Zeron", Width: *width, Height: *height, MinWidth: 900, MinHeight: 600, UseContentSize: true,
			TitleBarStyle: mygo.TitleBarHidden, TitleBarHeight: 38, TrafficLightPosition: &mygo.Point{X: 14, Y: 14}, Vibrancy: mygo.VibrancySidebar,
			Content: ui.View(app.View), StateKey: stateKey})
		if err := app.Attach(window); err != nil {
			log.Print(err)
			window.Close()
			return
		}
		window.Update(func() { app.OpenRoute(*route) })
		if *capture != "" {
			go func() {
				time.Sleep(*captureDelay)
				data, err := window.CapturePage()
				if err == nil {
					err = os.MkdirAll(filepath.Dir(*capture), 0755)
				}
				if err == nil {
					err = os.WriteFile(*capture, data, 0644)
				}
				if err != nil {
					log.Print(err)
				} else {
					fmt.Println(*capture)
				}
				if *exitCapture {
					mygo.App.Quit()
				}
			}()
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
