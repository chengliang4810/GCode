//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/providerlogo"
	"gcode/internal/storage"
)

// Native controls and the real local core. Only the public website is a local
// HTTP fixture; no model endpoint is requested and no user profile is touched.
func TestDesktopLocalProviderLogo(t *testing.T) {
	shadowSVG, err := os.ReadFile("../providerlogo/testdata/shadow.svg")
	if err != nil {
		t.Fatal(err)
	}
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			binary, artifacts := testPaths(t)
			var requests atomic.Int32
			var broken atomic.Bool
			var svgMode atomic.Bool
			release := make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			website := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
					t.Error("website request carried credentials")
				}
				if broken.Load() {
					http.NotFound(w, r)
					return
				}
				switch r.URL.Path {
				case "/":
					if svgMode.Load() {
						fmt.Fprint(w, `<link rel="icon" type="image/svg+xml" href="/logo.svg">`)
					} else {
						fmt.Fprint(w, `<link rel="icon" href="/logo.png">`)
					}
				case "/logo.svg":
					w.Header().Set("Content-Type", "image/svg+xml")
					_, _ = w.Write(shadowSVG)
				case "/logo.png":
					select {
					case <-release:
					case <-r.Context().Done():
						return
					}
					img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
					for y := range 32 {
						for x := range 32 {
							img.Set(x, y, color.NRGBA{R: 35, G: 160, B: 220, A: 255})
						}
					}
					_ = png.Encode(w, img)
				default:
					t.Errorf("unexpected API request: %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(website.Close)
			t.Cleanup(unblock)
			data := filepath.Join(artifacts, "data")
			store, err := storage.Open(storage.Options{Directory: data})
			if err != nil {
				t.Fatal(err)
			}
			core, err := openExecutionFixture(t, engine.Options{Storage: store})
			if err != nil {
				t.Fatal(err)
			}
			provider := model.Provider{ID: "logo", Name: "Logo fixture", Protocol: model.Chat, BaseURL: website.URL + "/v1",
				APIKey: "private-logo-test-key", Models: []model.Model{{ID: "model", ReasoningLevels: []string{"low", "high"}}}}
			if err := core.Execute(context.Background(), contracts.Command{Kind: "provider.save",
				Provider: &contracts.ProviderInput{Provider: provider}}); err != nil {
				t.Fatal(err)
			}
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}
			readLogo := func() (string, error) {
				raw, err := os.ReadFile(filepath.Join(data, "engine.json"))
				if err != nil {
					return "", err
				}
				var disk struct {
					Providers []model.Provider `json:"providers"`
				}
				if err := json.Unmarshal(raw, &disk); err != nil || len(disk.Providers) != 1 {
					return "", fmt.Errorf("provider configuration unavailable: %v", err)
				}
				if disk.Providers[0].APIKey != "private-logo-test-key" {
					return "", fmt.Errorf("logo editing changed the API key")
				}
				return disk.Providers[0].Logo, nil
			}
			app := startApp(t, binary, artifacts, "choose", data, "--local", "--appearance", appearance,
				"--width", "900", "--height", "800")
			app.act("settings-nav-Model settings", "press", "")
			app.act("provider-logo", "press", "")
			app.await("brand choices contain only artwork and retain accessible names", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				for _, brand := range providerlogo.Brands() {
					choice, err := tree.findKey("provider-logo:" + brand.ID)
					if err != nil || choice.Label != brand.Name {
						return fmt.Errorf("missing accessible brand %s: %v", brand.ID, err)
					}
					for _, node := range tree.Nodes {
						if node.Role == "text" && node.Label == brand.Name {
							return fmt.Errorf("brand %s still has a visible text label", brand.ID)
						}
					}
				}
				if activityTreeContains(tree, "Choose a built-in brand logo or fetch") {
					return fmt.Errorf("removed introductory text remains")
				}
				return nil
			})
			app.capture("builtin-logo-picker")
			app.act("provider-logo:deepseek-mark", "press", "")
			app.await("built-in logo saved without website requests", func() error {
				logo, err := readLogo()
				if err != nil || logo != "deepseek-mark" || requests.Load() != 0 {
					return fmt.Errorf("built-in logo=%q requests=%d: %v", logo, requests.Load(), err)
				}
				return nil
			})
			app.act("provider-logo", "press", "")
			app.act("provider-logo-fetch", "press", "")
			app.waitKey("provider-logo-loading")
			app.await("website request is in flight", func() error {
				if requests.Load() < 2 {
					return fmt.Errorf("website request has not reached the icon endpoint")
				}
				return nil
			})
			app.capture("website-logo-loading")
			app.act("dialog-close", "press", "")
			app.await("cancel closes the logo dialog before releasing the website", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				if _, err := tree.findKey("provider-logo-cancel"); err == nil {
					return fmt.Errorf("logo dialog remains open")
				}
				return nil
			})
			if logo, err := readLogo(); err != nil || logo != "deepseek-mark" {
				t.Fatal("canceled request replaced the built-in logo")
			}
			unblock()
			app.act("provider-logo", "press", "")
			app.act("provider-logo-fetch", "press", "")
			var savedLogo string
			app.await("website logo saved", func() error {
				logo, err := readLogo()
				if err != nil || !strings.HasPrefix(logo, "data:image/png;base64,") {
					return fmt.Errorf("website logo not persisted: %v", err)
				}
				savedLogo = logo
				return nil
			})
			if err := app.client.call("idle", map[string]int{"timeout": 2000}, nil); err != nil {
				t.Fatal(err)
			}
			app.capture("website-logo-saved")
			svgMode.Store(true)
			app.act("provider-logo", "press", "")
			app.act("provider-logo-fetch", "press", "")
			app.await("SVG logo saved with filters removed", func() error {
				logo, err := readLogo()
				if err != nil || !strings.HasPrefix(logo, "data:image/svg+xml;base64,") {
					return fmt.Errorf("SVG logo not persisted: %v", err)
				}
				content, _, err := providerlogo.ImageData(logo)
				if err != nil || providerlogo.Validate(logo) != nil || strings.Contains(string(content), "feDropShadow") ||
					strings.Contains(string(content), `filter="`) || !strings.Contains(string(content), "linearGradient") {
					return fmt.Errorf("SVG logo was not normalized correctly: %v", err)
				}
				savedLogo = logo
				return nil
			})
			if err := app.client.call("idle", map[string]int{"timeout": 2000}, nil); err != nil {
				t.Fatal(err)
			}
			app.capture("website-svg-logo-saved")
			broken.Store(true)
			app.act("provider-logo", "press", "")
			app.act("provider-logo-fetch", "press", "")
			app.waitKey("provider-logo-error")
			if logo, err := readLogo(); err != nil || logo != savedLogo {
				t.Fatal("failed fetch replaced the saved logo")
			}
			app.capture("website-logo-error")
			app.act("provider-logo-cancel", "press", "")
			app.stop()
			beforeRestart := requests.Load()
			app = startApp(t, binary, artifacts, "restart", data, "--local", "--appearance", appearance,
				"--width", "900", "--height", "800")
			app.act("settings-nav-Model settings", "press", "")
			app.waitKey("provider-logo")
			if logo, err := readLogo(); err != nil || logo != savedLogo || requests.Load() != beforeRestart {
				t.Fatal("restart lost the logo or fetched it again")
			}
			app.capture("website-logo-restarted")
			app.act("back-to-conversations", "press", "")
			app.waitKey("reasoning-trigger")
			if err := app.client.call("idle", map[string]int{"timeout": 2000}, nil); err != nil {
				t.Fatal(err)
			}
			app.capture("selected-model-website-logo")
		})
	}
}
