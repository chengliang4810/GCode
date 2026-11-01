package mock

import (
	"errors"
	"slices"
	"sort"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/reorder"
)

func (s *Server) reorderProvidersLocked(command contracts.Command) error {
	next := contracts.CloneSnapshot(s.state)
	input := command.Reorder
	if command.Kind == "provider.reorder" {
		providers, err := reorder.Move(next.Providers, input.SourceID, input.TargetID, input.After,
			func(provider contracts.Provider) string { return provider.ID })
		if err != nil {
			return err
		}
		next.Providers = providers
		ranks := make(map[string]int, len(providers))
		for index, provider := range providers {
			ranks[provider.ID] = index
		}
		sort.SliceStable(next.CustomProviders, func(i, j int) bool {
			return ranks[next.CustomProviders[i].Provider.ID] < ranks[next.CustomProviders[j].Provider.ID]
		})
		sort.SliceStable(next.Models, func(i, j int) bool {
			return ranks[next.Models[i].ProviderID] < ranks[next.Models[j].ProviderID]
		})
		sort.SliceStable(next.Integrations.Providers, func(i, j int) bool {
			return ranks[next.Integrations.Providers[i].ProviderID] < ranks[next.Integrations.Providers[j].ProviderID]
		})
	} else {
		index := slices.IndexFunc(next.Providers, func(provider contracts.Provider) bool {
			return provider.ID == input.ProviderID
		})
		if index < 0 {
			return errors.New("provider no longer exists")
		}
		localID := func(id string) string { return strings.TrimPrefix(id, input.ProviderID+"/") }
		order := slices.Clone(next.Providers[index].Models)
		configIndex := slices.IndexFunc(next.CustomProviders, func(config contracts.ProviderConfiguration) bool {
			return config.Provider.ID == input.ProviderID
		})
		if configIndex >= 0 {
			config := &next.CustomProviders[configIndex].Provider
			models, err := reorder.Move(config.Models, input.SourceID, input.TargetID, input.After,
				func(config model.Model) string { return config.ID })
			if err != nil {
				return err
			}
			config.Models = models
			order = make([]string, 0, len(models))
			for _, config := range models {
				order = append(order, config.ID)
			}
		} else {
			var err error
			order, err = reorder.Move(order, input.SourceID, input.TargetID, input.After, localID)
			if err != nil {
				return err
			}
		}
		ranks := make(map[string]int, len(order))
		for rank, id := range order {
			ranks[localID(id)] = rank
		}
		sort.SliceStable(next.Providers[index].Models, func(i, j int) bool {
			models := next.Providers[index].Models
			return ranks[localID(models[i])] < ranks[localID(models[j])]
		})
		models := []contracts.Model{}
		indices := []int{}
		for i, config := range next.Models {
			if config.ProviderID == input.ProviderID {
				models = append(models, config)
				indices = append(indices, i)
			}
		}
		sort.SliceStable(models, func(i, j int) bool {
			return ranks[localID(models[i].ID)] < ranks[localID(models[j].ID)]
		})
		for i, index := range indices {
			next.Models[index] = models[i]
		}
	}
	s.state = next
	return nil
}

// reorderGroupsLocked moves one user group before or after another and
// renumbers the retained order densely.
func (s *Server) reorderGroupsLocked(command contracts.Command) error {
	next := contracts.CloneSnapshot(s.state)
	groups, err := reorder.Move(next.SessionGroups, command.Reorder.SourceID, command.Reorder.TargetID, command.Reorder.After,
		func(group contracts.SessionGroup) string { return group.ID })
	if err != nil {
		return err
	}
	for i := range groups {
		groups[i].Order = i
	}
	next.SessionGroups = groups
	s.state = next
	return nil
}
