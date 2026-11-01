package engine

import (
	"context"
	"errors"

	"gcode/internal/contracts"
)

var _ contracts.ProviderKeyService = (*Engine)(nil)

func (e *Engine) ReadProviderKey(ctx context.Context, id string) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.checkLocked(ctx); err != nil {
		return "", err
	}
	for _, provider := range e.providers {
		if provider.ID == id {
			return provider.APIKey, nil
		}
	}
	return "", errors.New("provider not found")
}
