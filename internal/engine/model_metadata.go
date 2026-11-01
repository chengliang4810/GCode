package engine

import (
	"context"

	"gcode/internal/contracts"
	"gcode/internal/modelcatalog"
)

var _ contracts.ModelMetadataService = (*Engine)(nil)

func (e *Engine) LookupModelMetadata(ctx context.Context, query modelcatalog.Query) (modelcatalog.Metadata, error) {
	e.mu.Lock()
	err := e.checkLocked(ctx)
	e.mu.Unlock()
	if err != nil {
		return modelcatalog.Metadata{}, err
	}
	return e.catalog.Lookup(ctx, query)
}
