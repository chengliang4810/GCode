package engine

import (
	"errors"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/reorder"
)

func (e *Engine) reorderProvidersLocked(command contracts.Command) error {
	input := command.Reorder
	if command.Kind == "provider.reorder" {
		providers, err := reorder.Move(e.providers, input.SourceID, input.TargetID, input.After,
			func(provider model.Provider) string { return provider.ID })
		if err != nil {
			return err
		}
		e.providers = providers
	} else {
		index := -1
		for i, provider := range e.providers {
			if provider.ID == input.ProviderID {
				index = i
				break
			}
		}
		if index < 0 {
			return errors.New("provider no longer exists")
		}
		models, err := reorder.Move(e.providers[index].Models, input.SourceID, input.TargetID, input.After,
			func(config model.Model) string { return config.ID })
		if err != nil {
			return err
		}
		// The transaction's backup retains the original model slice on save failure.
		e.providers[index].Models = models
	}
	e.refreshProvidersLocked()
	return nil
}
