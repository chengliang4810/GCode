package engine

import (
	"context"

	"gcode/internal/contracts"
	"gcode/internal/modelcatalog"
)

// settleModelCatalogLocked derives the host-visible catalog state from the
// remote refresh lifecycle and the locally configured models.
//
// The model list itself comes from the local provider configuration and never
// waits for the network, so loading only describes the remote catalog data.
// A failed refresh keeps the last usable state: the error is reported only
// while no refresh has ever succeeded in this process.
//
// It reports whether the visible state changed.
func (e *Engine) settleModelCatalogLocked() bool {
	before := e.state.ModelCatalog
	next := contracts.ModelCatalogState{State: contracts.CatalogReady}
	status := e.catalogStatus()
	switch {
	case e.catalogPending || status.Refreshing || (!status.Synced && e.catalogDue()):
		next.State = contracts.CatalogLoading
	case status.Err != nil && !status.Synced:
		next = contracts.ModelCatalogState{State: contracts.CatalogError, Error: status.Err.Error()}
	case len(e.state.Models) == 0:
		next.State = contracts.CatalogEmpty
	}
	e.state.ModelCatalog = next
	return next != before
}

// catalogStatus reports the remote refresh lifecycle of the cached catalog.
func (e *Engine) catalogStatus() modelcatalog.RefreshStatus {
	if e.catalog == nil {
		return modelcatalog.RefreshStatus{}
	}
	return e.catalog.Status()
}

// catalogDue reports whether the stored check time is due. A catalog that
// never refreshed in this process is due, so its first remote check reports
// loading instead of a settled state.
func (e *Engine) catalogDue() bool {
	return e.catalog != nil && e.catalog.NeedsRefresh()
}

// watchModelCatalog republishes the catalog state for every refresh edge the
// cache reports, so hosts see loading start and settle without polling.
func (e *Engine) watchModelCatalog(ctx context.Context) {
	for {
		// Read the change channel before the status: an edge that lands in
		// between closes the channel and wakes the next iteration.
		changes := e.catalog.Changes()
		e.mu.Lock()
		if e.settleModelCatalogLocked() {
			e.publishLocked(contracts.Event{Kind: "state.changed"})
		}
		e.mu.Unlock()
		select {
		case <-changes:
		case <-ctx.Done():
			return
		}
	}
}

// startModelCatalogRefreshLocked reports loading at once and refreshes the
// remote catalog off the engine lock, so the command never blocks on the
// network and the visible state settles when the refresh finishes.
func (e *Engine) startModelCatalogRefreshLocked() {
	if e.catalog == nil || e.catalogPending {
		return
	}
	e.catalogPending = true
	e.settleModelCatalogLocked()
	ctx := e.background
	if ctx == nil {
		ctx = context.Background()
	}
	e.catalogRefresh.Add(1)
	go func() {
		defer e.catalogRefresh.Done()
		_ = e.catalog.Refresh(ctx)
		e.mu.Lock()
		e.catalogPending = false
		if e.settleModelCatalogLocked() {
			e.publishLocked(contracts.Event{Kind: "state.changed"})
		}
		e.mu.Unlock()
	}()
}
