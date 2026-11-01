package mock

import (
	"context"
	"reflect"
	"testing"

	"gcode/internal/contracts"
)

func TestCatalogRetryRefreshesMetadataWithoutChangingProviderPreferences(t *testing.T) {
	s := newTestServer(t, Options{})
	ctx := context.Background()
	s.mu.Lock()
	s.state.ModelCatalog = contracts.ModelCatalogState{State: contracts.CatalogError, Error: "Catalog request failed"}
	s.state.Models = nil
	s.state.Providers[0].Enabled = false
	providers := cloneSnapshot(s.state).Providers
	s.mu.Unlock()
	if err := s.Execute(ctx, contracts.Command{Kind: "catalog.retry"}); err != nil {
		t.Fatal(err)
	}
	after, _ := s.Snapshot(ctx)
	if after.ModelCatalog.State != contracts.CatalogReady || after.ModelCatalog.Error != "" || len(after.Models) == 0 {
		t.Fatal("retry did not recover a missing catalog")
	}
	if !reflect.DeepEqual(providers, after.Providers) {
		t.Fatal("catalog retry changed provider connection or enabled state")
	}
}

func TestCatalogEmptyAndNoAgentStatesStayDistinct(t *testing.T) {
	for _, tc := range []struct {
		name      string
		providers []contracts.Provider
		want      contracts.ModelCatalogStatus
	}{{"no agents", nil, contracts.CatalogNoAgents}, {"all disabled", []contracts.Provider{{ID: "claude", Enabled: false}}, contracts.CatalogNoAgents}, {"empty directory", []contracts.Provider{{ID: "custom-provider", Name: "Custom", Enabled: true}}, contracts.CatalogEmpty}} {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer(t, Options{})
			ctx := context.Background()
			s.mu.Lock()
			s.state.Providers = tc.providers
			s.state.Models = nil
			s.state.ModelCatalog = contracts.ModelCatalogState{State: contracts.CatalogLoading}
			s.mu.Unlock()
			if err := s.Execute(ctx, contracts.Command{Kind: "catalog.retry"}); err != nil {
				t.Fatal(err)
			}
			snapshot, _ := s.Snapshot(ctx)
			if snapshot.ModelCatalog.State != tc.want || len(snapshot.Models) != 0 {
				t.Fatalf("catalog state=%s, models=%d", snapshot.ModelCatalog.State, len(snapshot.Models))
			}
		})
	}
}

func TestSidebarDefaultsFollowReferenceShowFlags(t *testing.T) {
	settings := DefaultSettings()
	if settings.SidebarGrouping != "projects" || !settings.ShowIcons {
		t.Fatal("sidebar defaults do not preserve the current shared display controls")
	}
}
