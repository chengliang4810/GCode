package desktop

import (
	"fmt"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func TestPluginSettingsCardsAdaptAndTruncate(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(fmt.Sprintf("dark=%t", dark), func(t *testing.T) {
			app, _ := pluginUIFixture(t)
			app.section = "Marketplace"
			app.palette = design.Palette("rcode-light", false)
			if dark {
				app.palette = design.Palette("rcode-dark", true)
			}
			description := strings.Repeat("A long plugin description that must end with an ellipsis on the card. ", 12)
			app.state.Plugins.Catalog = nil
			for i := range 6 {
				app.state.Plugins.Catalog = append(app.state.Plugins.Catalog, contracts.PluginCatalogItem{
					ID: fmt.Sprintf("card-%d@official", i), Name: fmt.Sprintf("Card %d", i), Marketplace: "official", Description: description,
				})
			}
			tt := pluginUITester(app)
			for _, test := range []struct{ width, columns int }{{1320, 3}, {900, 2}, {620, 1}, {1320, 3}} {
				tt.SetSize(test.width, 1100)
				first, ok := tt.Find("Plugin details Card 0")
				if !ok {
					t.Fatal("first card missing")
				}
				for i := range 6 {
					card, ok := tt.Find(fmt.Sprintf("Plugin details Card %d", i))
					if !ok || card.X < 0 || card.X+card.W > float32(test.width)+1 {
						t.Fatalf("width %d: card %d escapes shared viewport: %+v", test.width, i, card)
					}
					if (i < test.columns) != (card.Y == first.Y) {
						t.Fatalf("width %d: expected %d columns; card %d is at %+v, first %+v", test.width, test.columns, i, card, first)
					}
				}
				summary, ok := tt.Find(description)
				name, _ := tt.Find("Card 0")
				if !ok || summary.H > name.H+1 || summary.W > first.W || summary.X+summary.W > first.X+first.W {
					t.Fatalf("width %d: description is not constrained to a single card line: %+v", test.width, summary)
				}
			}
		})
	}
}

func TestPluginSettingsCardDetailsUsesCurrentProjection(t *testing.T) {
	app, service := pluginUIFixture(t)
	app.section = "Marketplace"
	item := &app.state.Plugins.Catalog[0]
	item.Description = strings.Repeat("Full description including text beyond the card width. ", 20)
	item.Version, item.Author, item.Category = "1.2.3", "Fixture author", "Development"
	item.Homepage = "https://example.invalid/plugin-info"
	tt := pluginUITester(app)
	if _, ok := tt.Find("Install fixture"); ok || tt.HasText("Fixture dependency issue") {
		t.Fatal("card exposes actions or detailed issues outside its dialog")
	}
	if err := tt.Click("Plugin details fixture"); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{item.Description, item.Version, item.Author, item.Category, item.Homepage, "skills", "Fixture dependency issue"} {
		if !tt.HasText(text) {
			t.Fatalf("plugin details omitted %q", text)
		}
	}
	full, _ := tt.Find("Description: " + item.Description)
	if full.H < 100 {
		t.Fatalf("detail description remained truncated: %+v", full)
	}
	app.state.Plugins.Catalog[0].Description = "Updated catalog description"
	tt.Frame()
	if !tt.HasText("Updated catalog description") {
		t.Fatal("open details did not follow the current host projection")
	}
	if err := tt.Click("Close dialog"); err != nil {
		t.Fatal(err)
	}
	if app.capabilityUI.plugins.detailsOpen || app.capabilityUI.plugins.detailsID != "" {
		t.Fatal("close button retained plugin details state")
	}
	if err := tt.Click("Plugin details fixture"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if app.capabilityUI.plugins.detailsOpen || app.capabilityUI.plugins.detailsID != "" {
		t.Fatal("Escape retained plugin details state")
	}
	if len(service.commands) != 0 {
		t.Fatal("viewing details dispatched a business mutation")
	}
	if err := tt.Click("Plugin details fixture"); err != nil {
		t.Fatal(err)
	}
	app.selectSettingsSection("Plugins")
	tt.Frame()
	if app.capabilityUI.plugins.detailsOpen || app.capabilityUI.plugins.detailsID != "" {
		t.Fatal("navigation retained the details dialog")
	}
	app.selectSettingsSection("Marketplace")
	tt.Frame()
	if err := tt.Click("Plugin details fixture"); err != nil {
		t.Fatal(err)
	}
	app.state.Plugins.Catalog = nil
	tt.Frame()
	if app.capabilityUI.plugins.detailsOpen || app.capabilityUI.plugins.detailsID != "" {
		t.Fatal("removed catalog entry left stale details open")
	}
}
