package engine

import (
	"context"

	"gcode/internal/contracts"
	"gcode/internal/providerlogo"
)

var _ contracts.ProviderLogoService = (*Engine)(nil)

func (e *Engine) FetchProviderLogo(ctx context.Context, baseURL string) (string, error) {
	e.mu.Lock()
	err := e.checkLocked(ctx)
	e.mu.Unlock()
	if err != nil {
		return "", err
	}
	return providerlogo.Fetch(ctx, baseURL)
}
