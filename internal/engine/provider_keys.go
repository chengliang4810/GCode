package engine

import (
	"context"
	"errors"

	"gcode/internal/contracts"
)

var _ contracts.ProviderKeyService = (*Engine)(nil)

// RevealProviderKey returns the stored credential for one explicit reveal. The
// caller owns the plaintext result and must not persist or publish it.
func (e *Engine) RevealProviderKey(ctx context.Context, id string) (string, error) {
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
