package mock

import (
	"slices"

	"gcode/internal/contracts"
)

func (s *Server) normalizeSessionCapabilitiesLocked(session *contracts.Session) {
	providerID := ""
	for _, model := range s.state.Models {
		if model.ID == session.ModelID {
			providerID = model.ProviderID
			break
		}
	}
	for _, provider := range s.state.Providers {
		if provider.ID != providerID {
			continue
		}
		if len(provider.AccessModes) > 0 && !slices.Contains(provider.AccessModes, session.AccessMode) {
			session.AccessMode = provider.AccessModes[0]
			if slices.Contains(provider.AccessModes, contracts.AccessFullAccess) {
				session.AccessMode = contracts.AccessFullAccess
			}
		}
		if !provider.SupportsPlan {
			session.Plan = false
		}
		return
	}
}
