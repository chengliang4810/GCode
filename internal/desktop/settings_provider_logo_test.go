package desktop

import (
	"context"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"github.com/egoist/mygo/ui"
)

func TestProviderLogoSelectFetchAndFailurePreserveConfiguration(t *testing.T) {
	app, core := runtimeSettingsTestApp(t)
	app.route, app.section = "settings", "Model settings"
	var broken atomic.Bool
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if broken.Load() || r.URL.Path != "/favicon.png" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("API key sent to website")
		}
		_ = png.Encode(w, image.NewRGBA(image.Rect(0, 0, 32, 32)))
	}))
	defer site.Close()
	app.dispatch(contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: model.Provider{
		ID: "logos", Name: "Logo fixture", Protocol: model.Chat, BaseURL: site.URL + "/v1", APIKey: "private-api-key",
		Models: []model.Model{{ID: "model"}},
	}}})
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(app.palette.UI())
		ui.Column(c).Fill().Children(func() { app.customProviderSettings(c) })
	}, 1000, 800)
	if err := tt.Click("Choose provider logo"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("OpenAI"); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := core.Snapshot(context.Background())
	if snapshot.CustomProviders[0].Provider.Logo != "openai-mark" || snapshot.Providers[0].Logo != "openai-mark" {
		t.Fatal("built-in logo was not saved and projected into the model picker")
	}
	if err := tt.Click("Choose provider logo"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Fetch from website"); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = core.Snapshot(context.Background())
	logo := snapshot.CustomProviders[0].Provider.Logo
	if !strings.HasPrefix(logo, "data:image/png;base64,") || !snapshot.CustomProviders[0].HasAPIKey || snapshot.CustomProviders[0].Provider.APIKey != "" {
		t.Fatal("website logo was not saved or credential ownership changed")
	}
	broken.Store(true)
	if err := tt.Click("Choose provider logo"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Fetch from website"); err != nil {
		t.Fatal(err)
	}
	if app.modelsUI.logoError == "" || !app.modelsUI.logoOpen {
		t.Fatal("website failure did not remain actionable in the dialog")
	}
	snapshot, _ = core.Snapshot(context.Background())
	if snapshot.CustomProviders[0].Provider.Logo != logo {
		t.Fatal("failed website lookup replaced the existing logo")
	}
	key, err := core.ReadProviderKey(context.Background(), "logos")
	if err != nil || key != "private-api-key" {
		t.Fatal("logo editing changed the API key")
	}
	if err := tt.Click("Use default icon"); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = core.Snapshot(context.Background())
	if snapshot.CustomProviders[0].Provider.Logo != "" {
		t.Fatal("default icon did not clear the saved logo")
	}
}
