//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/plugins"
	"gcode/internal/processutil"
	"gcode/internal/storage"
)

// The marketplace is a private local Git repository. All mutations pass through
// the visible native controls and real core; no provider or public market is used.
func TestDesktopLocalPluginManagement(t *testing.T) {
	binary, artifacts := testPaths(t)
	market := filepath.Join(artifacts, "market")
	project := filepath.Join(artifacts, "project")
	data := filepath.Join(artifacts, "data")
	for _, folder := range []string{filepath.Join(market, ".claude-plugin"), filepath.Join(market, "fixture", ".claude-plugin"), filepath.Join(market, "fixture", "skills", "fixture"), project} {
		if err := os.MkdirAll(folder, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	write := func(relative, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(market, relative), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(".claude-plugin/marketplace.json", `{"name":"native-market","owner":{"name":"Native test"},"plugins":[{"name":"fixture","source":"./fixture","description":"Native fixture skill"}]}`)
	write("fixture/.claude-plugin/plugin.json", `{"name":"fixture","version":"1.0.0","userConfig":{"token":{"type":"string","title":"Fixture token","sensitive":true}}}`)
	write("fixture/skills/fixture/SKILL.md", "---\nname: fixture\ndescription: Native fixture instructions\n---\nInspect the current fixture only.\n")
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", market}, args...)...)
		processutil.HideConsole(cmd)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("private marketplace git: %v: %s", err, output)
		}
	}
	git("init", "-b", "main")
	git("add", ".")
	git("-c", "user.name=GCode test", "-c", "user.email=rcode-test@example.invalid", "commit", "-m", "Fixture")
	store, err := storage.Open(storage.Options{Directory: data})
	if err != nil {
		t.Fatal(err)
	}
	core, err := engine.Open(engine.Options{Storage: store})
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: project}}); err != nil {
		core.Close()
		t.Fatal(err)
	}
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	readInstalled := func() ([]plugins.Installation, error) {
		payload, err := os.ReadFile(filepath.Join(data, "plugins", "registry.json"))
		if err != nil {
			return nil, err
		}
		var registry struct {
			Installations []plugins.Installation `json:"installations"`
		}
		err = json.Unmarshal(payload, &registry)
		return registry.Installations, err
	}
	app := startApp(t, binary, artifacts, "install", data, "--local")
	app.act("settings-nav-Plugins", "press", "")
	app.waitKey("plugin-browse-marketplace")
	pluginCapture(app, "empty-installed")
	app.act("plugin-browse-marketplace", "press", "")
	app.waitKey("plugin-source-manage")
	pluginCapture(app, "marketplace")
	app.act("plugin-source-add", "press", "")
	pluginCapture(app, "simple-add-marketplace")
	app.act("plugin-source-location", "setValue", filepath.Join(artifacts, "missing-market"))
	app.act("plugin-source-save", "press", "")
	app.waitKey("plugin-source-error")
	pluginCapture(app, "invalid-marketplace-draft-retained")
	sourceURL := (&url.URL{Scheme: "file", Path: "/" + strings.TrimPrefix(filepath.ToSlash(market), "/")}).String()
	app.act("plugin-source-location", "setValue", sourceURL)
	app.act("plugin-source-save", "press", "")
	app.waitKey("plugin-marketplace-native-market")
	app.act("plugin-source-manage", "press", "")
	app.waitKey("plugin-refresh-native-market")
	app.act("plugin-refresh-native-market", "press", "")
	app.await("private Git marketplace catalog", func() error {
		payload, err := os.ReadFile(filepath.Join(data, "plugins", "registry.json"))
		if err != nil {
			return err
		}
		var registry struct {
			Catalogs []plugins.Catalog `json:"catalogs"`
		}
		if err := json.Unmarshal(payload, &registry); err != nil {
			return err
		}
		if !slices.ContainsFunc(registry.Catalogs, func(c plugins.Catalog) bool {
			return c.Marketplace.Name == "native-market" && len(c.Marketplace.Plugins) == 1
		}) {
			return fmt.Errorf("private catalog not refreshed")
		}
		return nil
	})
	pluginCapture(app, "marketplace-sources")
	app.act("dialog-close", "press", "")
	pluginWaitNoKey(app, "plugin-refresh-native-market")
	app.act("plugin-marketplace-native-market", "press", "")
	pluginCapture(app, "marketplace-source-tab")
	app.act("plugin-install-fixture@native-market", "press", "")
	app.await("user plugin installed on the real host", func() error {
		installed, err := readInstalled()
		if err != nil {
			return err
		}
		if len(installed) != 1 || installed[0].ID != "fixture@native-market" {
			return fmt.Errorf("user installation not persisted: %+v", installed)
		}
		_, err = os.Stat(filepath.Join(installed[0].Path, "skills", "fixture", "SKILL.md"))
		return err
	})
	id := "fixture@native-market"
	app.act("settings-nav-Plugins", "press", "")
	app.waitKey("plugin-enabled-" + id)
	pluginCapture(app, "installed-user")
	app.act("plugin-configure-"+id, "press", "")
	app.act("value", "setValue", "discarded-fixture-secret")
	app.act("plugin-config-cancel", "press", "")
	pluginWaitNoKey(app, "plugin-config-save")
	if _, err := os.Stat(filepath.Join(data, "plugin-configuration.json")); !os.IsNotExist(err) {
		t.Fatalf("cancelled configuration wrote a file: %v", err)
	}
	app.act("plugin-configure-"+id, "press", "")
	app.act("value", "setValue", "saved-fixture-secret")
	app.act("plugin-config-save", "press", "")
	app.await("configuration saved only after explicit submit", func() error {
		payload, err := os.ReadFile(filepath.Join(data, "plugin-configuration.json"))
		if err != nil {
			return err
		}
		var values map[string]map[string]string
		if err := json.Unmarshal(payload, &values); err != nil {
			return err
		}
		if values[id]["token"] != "saved-fixture-secret" {
			return fmt.Errorf("expected scoped plugin configuration")
		}
		return nil
	})
	write("fixture/.claude-plugin/plugin.json", `{"name":"fixture","version":"1.1.0","userConfig":{"token":{"type":"string","title":"Fixture token","sensitive":true}}}`)
	git("add", ".")
	git("-c", "user.name=GCode test", "-c", "user.email=rcode-test@example.invalid", "commit", "-m", "Update fixture")
	app.act("plugin-update-"+id, "press", "")
	app.await("plugin update refreshes the private Git source", func() error {
		installed, err := readInstalled()
		if err != nil {
			return err
		}
		if len(installed) != 1 || installed[0].Version != "1.1.0" {
			return fmt.Errorf("plugin update not persisted: %+v", installed)
		}
		return nil
	})
	app.act("plugin-enabled-"+id, "press", "")
	app.waitChecked("plugin-enabled-"+id, false)
	app.await("plugin disabled on disk", func() error {
		installed, err := readInstalled()
		if err != nil {
			return err
		}
		if len(installed) != 1 || installed[0].Enabled {
			return fmt.Errorf("disable not persisted")
		}
		return nil
	})
	pluginCapture(app, "configured-disabled")
	app.stop()
	restarted := startApp(t, binary, artifacts, "restart", data, "--local", "--language", "zh-CN", "--appearance", "dark", "--width", "900", "--height", "600")
	restarted.act("settings-nav-Plugins", "press", "")
	restarted.waitChecked("plugin-enabled-"+id, false)
	pluginCapture(restarted, "restored-dark-narrow")
	restarted.act("plugin-configure-"+id, "press", "")
	restarted.act("plugin-config-remove-token", "press", "")
	restarted.act("plugin-config-save", "press", "")
	restarted.await("explicit plugin configuration removal persisted", func() error {
		payload, err := os.ReadFile(filepath.Join(data, "plugin-configuration.json"))
		if err != nil {
			return err
		}
		var values map[string]map[string]string
		if err := json.Unmarshal(payload, &values); err != nil {
			return err
		}
		if _, ok := values[id]["token"]; ok {
			return fmt.Errorf("removed token remains configured")
		}
		return nil
	})
	restarted.act("plugin-uninstall-"+id, "press", "")
	restarted.await("plugin removed from the real registry", func() error {
		installed, err := readInstalled()
		if err != nil {
			return err
		}
		if len(installed) != 0 {
			return fmt.Errorf("plugin still installed")
		}
		return nil
	})
	restarted.waitKey("plugin-browse-marketplace")
	pluginCapture(restarted, "uninstalled")
	restarted.act("plugin-browse-marketplace", "press", "")
	restarted.act("plugin-source-manage", "press", "")
	restarted.waitKey("plugin-remove-native-market")
	pluginCapture(restarted, "sources-dark-narrow")
	restarted.act("plugin-remove-native-market", "press", "")
	restarted.await("marketplace source removed from host", func() error {
		payload, err := os.ReadFile(filepath.Join(data, "plugins", "registry.json"))
		if err != nil {
			return err
		}
		var registry struct {
			Sources []plugins.MarketplaceSource `json:"sources"`
		}
		if err := json.Unmarshal(payload, &registry); err != nil {
			return err
		}
		if slices.ContainsFunc(registry.Sources, func(source plugins.MarketplaceSource) bool { return source.ID == "native-market" }) {
			return fmt.Errorf("marketplace still registered")
		}
		return nil
	})
	restarted.act("dialog-close", "press", "")
	restarted.act("plugin-source-add", "press", "")
	restarted.waitKey("plugin-source-choose-directory")
	pluginCapture(restarted, "simple-add-marketplace-dark-chinese")
}

func pluginActLabel(app *nativeApp, label string) {
	app.t.Helper()
	var node accessNode
	app.await("actionable label "+label, func() error {
		tree, err := app.client.tree()
		if err != nil {
			return err
		}
		for _, candidate := range tree.Nodes {
			if candidate.Label == label && slices.Contains(candidate.Actions, "press") {
				node = candidate
				return nil
			}
		}
		return fmt.Errorf("missing actionable label %q", label)
	})
	if err := app.client.call("act", map[string]any{"id": node.ID, "action": "press"}, nil); err != nil {
		app.t.Fatal(err)
	}
}

func pluginWaitNoKey(app *nativeApp, key string) {
	app.t.Helper()
	app.await("closed plugin editor "+key, func() error {
		tree, err := app.client.tree()
		if err != nil {
			return err
		}
		for _, node := range tree.Nodes {
			var value string
			if json.Unmarshal(node.Key, &value) == nil && value == key {
				return fmt.Errorf("editor %q is still present", key)
			}
		}
		return nil
	})
}

func pluginCapture(app *nativeApp, name string) {
	app.t.Helper()
	// Business state is awaited before this helper. idle only lets the current
	// switch animation and requested paint finish before collecting pixels.
	if err := app.client.call("idle", map[string]int{"timeout": 2000}, nil); err != nil {
		app.t.Fatal(err)
	}
	app.capture(name)
}
