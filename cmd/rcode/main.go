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
	"runtime/debug"
	"strings"
	"time"

	"context"
	"gcode/internal/appconfig"
	"gcode/internal/applog"
	"gcode/internal/assets"
	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/desktop"
	"gcode/internal/engine"
	"gcode/internal/i18n"
	"gcode/internal/mock"
	"gcode/internal/remote"
	"gcode/internal/storage"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/plugins/updater/native"
	"github.com/egoist/mygo/ui"
)

// enforceSingleInstance reports whether this launch claims the one instance
// of the app on this machine. Packaged installs do; development builds (go
// run, plain go build) stay multi-open, and the environment forces either
// side when testing. Deterministic scenarios and headless captures are
// tooling and never claim the lock.
func enforceSingleInstance(scenario string, headless bool) bool {
	if scenario != "" || headless {
		return false
	}
	if os.Getenv("RCODE_ALLOW_MULTIPLE_INSTANCE") == "1" {
		return false
	}
	return mygo.App.IsPackaged() || os.Getenv("RCODE_ENFORCE_SINGLE_INSTANCE") == "1"
}

func main() {
	scenario := flag.String("scenario", "", "Deterministic data scenario (default, empty, loading, live, transcript, questions, permissions, showcase, offline, error, stress)")
	route := flag.String("route", "chat", "Initial route: chat, chat/<id>, settings/<section>, trajectory/<id>")
	stateDir := flag.String("data-dir", "", "Fixed configuration/profile directory; runtime root is selected in settings; scenarios default to memory")
	connect := flag.String("connect", "", "Connect to a GCode headless host by HTTP(S) URL")
	tokenFile := flag.String("token-file", "", "File containing the remote host API token")
	local := flag.Bool("local", false, "Use the local Go engine and disable the saved remote connection")
	language := flag.String("language", "", "Interface language override: zh-CN, en")
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
	mygo.App.SetName("GCode")
	if enforceSingleInstance(*scenario, *headless) && !mygo.App.RequestSingleInstanceLock() {
		// The running instance received this launch's command line and
		// focuses its window; this launch has nothing left to do.
		return
	}
	if err := assets.RegisterFonts(); err != nil {
		log.Fatal(err)
	}
	statePath := ""
	if *stateDir != "" {
		statePath = filepath.Join(*stateDir, "state.json")
	}
	opts := mock.Options{Scenario: *scenario, StatePath: statePath, Platform: runtime.GOOS, Architecture: runtime.GOARCH}
	if *scenario != "" {
		opts.Now = func() time.Time { return time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC) }
	}
	var backend contracts.Service
	var connections contracts.ConnectionSettingsService
	var err error
	var store *storage.Store
	var stopLogging func()
	var stopClientMaintenance func()
	if *scenario != "" {
		if *connect != "" {
			log.Fatal("--connect cannot be combined with a deterministic scenario")
		}
		backend, err = mock.New(opts)
	} else {
		store, err = storage.Open(storage.Options{Directory: *stateDir})
		if err == nil {
			defer store.Close()
			stopLogging, err = applog.StartRuntime(store, os.Stderr)
			if err != nil {
				log.Fatal(err)
			}
			defer stopLogging()
			statePath = filepath.Join(store.ConfigDirectory(), "engine.json")
			var manager *appconfig.Connections
			manager, err = appconfig.OpenConnections(store)
			if err == nil {
				connections = manager
				if *local && *connect != "" {
					log.Fatal("--local and --connect are mutually exclusive")
				}
				if *connect != "" {
					var token []byte
					token, err = os.ReadFile(*tokenFile)
					if err == nil {
						err = manager.ConfigureConnection(context.Background(), contracts.ConnectionInput{URL: *connect, Token: strings.TrimSpace(string(token)), Enabled: true})
					}
				} else if *local {
					cfg := manager.Credentials()
					err = manager.ConfigureConnection(context.Background(), contracts.ConnectionInput{URL: cfg.URL, Enabled: false, PreserveToken: true})
				}
				if err == nil {
					cfg := manager.Credentials()
					if cfg.Enabled {
						stopClientMaintenance, err = startClientRuntimeMaintenance(store)
						if err == nil {
							defer stopClientMaintenance()
							manager.Activate(cfg.URL)
							backend, err = remote.NewClient(cfg.URL, cfg.Token)
						}
					} else {
						backend, err = engine.Open(engine.Options{Storage: store})
					}
				}
			}
		}
	}
	if err != nil {
		log.Fatal(err)
	}
	if *language != "" && *language != "zh-CN" && *language != "en" {
		log.Fatal("language must be zh-CN or en")
	}
	if *appearance != "" || *themeID != "" || *language != "" {
		s, err := backend.Snapshot(context.Background())
		if err != nil {
			log.Fatal(err)
		}
		if *language != "" {
			s.Settings.General.Language = *language
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
	desktopOptions := desktop.Options{Now: opts.Now, Connections: connections}
	if *scenario == "" && !*headless {
		mygo.Use(native.Plugin)
		desktopOptions.CheckForUpdates = updater.CheckForUpdates
	}
	app, err := desktop.NewWithOptions(backend, desktopOptions)
	if err != nil {
		log.Fatal(err)
	}
	defer app.Close()
	if *headless {
		app.OpenRoute(*route)
		tester := ui.NewTester(app.View, *width, *height)
		tester.SetLocale(i18n.Native(app.Snapshot().Settings.General.Language))
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
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	configRoot, runtimeRoot := filepath.Join(home, ".gcode"), filepath.Join(home, ".gcode")
	if store != nil {
		configRoot, runtimeRoot = store.ConfigDirectory(), store.Directory()
	} else if *stateDir != "" {
		configRoot, runtimeRoot = *stateDir, *stateDir
	}
	if err := configureNativeDirectories(configRoot, runtimeRoot, *scenario); err != nil {
		log.Fatal(err)
	}
	mygo.App.SetLanguages("zh-Hans", "en")
	mygo.App.SetLocale(i18n.Native(app.Snapshot().Settings.General.Language))
	mygo.Theme.SetSource(mygo.ThemeSource(app.Snapshot().Settings.Appearance.Mode))
	// MyGo starts the replacement process after OnQuit, before Run returns.
	// Release the local profile lock before that process opens the same data.
	mygo.App.OnQuit(func() {
		app.Close()
		if stopClientMaintenance != nil {
			stopClientMaintenance()
		}
		if stopLogging != nil {
			stopLogging()
		}
		if store != nil {
			_ = store.Close()
		}
	})
	mygo.App.WhenReady(func() {
		// Startup allocates window buffers, catalogs and the first transcript
		// in bursts; once the first frames settle, hand the free spans back to
		// the OS instead of keeping them resident until memory pressure.
		go func() {
			time.Sleep(20 * time.Second)
			debug.FreeOSMemory()
		}()
		updateMenu := func() {
			appMenu := &mygo.MenuItem{Role: mygo.RoleAppMenu}
			if desktopOptions.CheckForUpdates != nil {
				appMenu = &mygo.MenuItem{Label: "GCode", Submenu: []*mygo.MenuItem{
					{Role: mygo.RoleAbout}, updater.MenuItem(), mygo.Separator(),
					{Role: mygo.RoleServices}, mygo.Separator(),
					{Role: mygo.RoleHide}, {Role: mygo.RoleHideOthers}, {Role: mygo.RoleUnhide},
					mygo.Separator(), {Role: mygo.RoleQuit},
				}}
			}
			mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
				appMenu, {Label: i18n.In(mygo.App.Locale(), "File"), Submenu: []*mygo.MenuItem{
					{Label: i18n.In(mygo.App.Locale(), "New session"), Accelerator: "CmdOrCtrl+N", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { app.NewConversation() }},
					{Label: i18n.In(mygo.App.Locale(), "Settings"), Accelerator: "CmdOrCtrl+,", Click: func(_ *mygo.MenuItem, _ *mygo.Window) { app.OpenRoute("settings") }},
					{Role: mygo.RoleClose},
				}},
				{Role: mygo.RoleEditMenu}, {Role: mygo.RoleWindowMenu},
			}))
		}
		updateMenu()
		mygo.App.OnLocaleChanged(func(string) { updateMenu() })
		stateKey := "gcode-main"
		if *scenario != "" {
			stateKey = ""
		}
		window := mygo.NewWindow(mygo.WindowOptions{Title: "GCode", Width: *width, Height: *height, MinWidth: 900, MinHeight: 600, UseContentSize: true,
			TitleBarStyle: mygo.TitleBarHidden, TitleBarHeight: 38, HideWindowButtons: desktop.HideNativeWindowButtons, TrafficLightPosition: &mygo.Point{X: 14, Y: 14}, Vibrancy: app.NativeVibrancy(),
			Content: ui.View(app.View), StateKey: stateKey})
		if err := app.Attach(window); err != nil {
			log.Print(err)
			window.Close()
			return
		}
		if enforceSingleInstance(*scenario, *headless) {
			offSecond := mygo.App.OnSecondInstance(func([]string, string) { window.Focus() })
			window.OnClosed(offSecond)
		}
		if desktopOptions.CheckForUpdates != nil {
			updateAvailability := func() { app.SetUpdateAvailable(updater.AvailableVersion() != "") }
			off := updater.OnChange(updateAvailability)
			window.OnClosed(off)
			updateAvailability()
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
