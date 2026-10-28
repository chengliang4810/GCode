package mock

import "gcode/internal/contracts"

func catalogStatus(snapshot contracts.Snapshot) contracts.ModelCatalogStatus {
	enabled := false
	for _, provider := range snapshot.Providers {
		if !provider.Enabled {
			continue
		}
		enabled = true
		for _, model := range snapshot.Models {
			if model.ProviderID == provider.ID {
				return contracts.CatalogReady
			}
		}
	}
	if !enabled {
		return contracts.CatalogNoAgents
	}
	return contracts.CatalogEmpty
}
