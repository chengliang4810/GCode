package engine

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
)

const modelFetchTimeout = 30 * time.Second

type modelFetchRun struct {
	cancel context.CancelFunc
}

// startModelFetchLocked accepts a user-requested provider model listing, then
// lets the engine own its lifetime. It reads the configured provider's own
// models endpoint; nothing is added to the configuration automatically.
func (e *Engine) startModelFetchLocked(providerID string) error {
	var provider model.Provider
	found := false
	for _, candidate := range e.providers {
		if candidate.ID == providerID {
			provider = model.CloneProvider(candidate)
			found = true
			break
		}
	}
	if !found {
		return errors.New("configured provider not found")
	}
	if e.modelFetchRuns[providerID] != nil {
		return errors.New("this provider is already being fetched")
	}
	if provider.Disabled {
		return errors.New("enable the provider before listing its models")
	}
	ctx, cancel := context.WithTimeout(context.Background(), modelFetchTimeout)
	run := &modelFetchRun{cancel: cancel}
	e.modelFetchRuns[providerID] = run
	e.modelFetchResults[providerID] = contracts.ModelFetch{
		ProviderID: providerID, State: "fetching", StartedAt: time.Now(),
	}
	e.refreshProvidersLocked()
	e.publishLocked(contracts.Event{Kind: "providers.changed"})
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		defer cancel()
		models, callErr := e.client.ListModels(ctx, provider)
		e.mu.Lock()
		defer e.mu.Unlock()
		// Configuration replacement, deletion and shutdown invalidate this request.
		if e.closed || e.modelFetchRuns[providerID] != run {
			return
		}
		delete(e.modelFetchRuns, providerID)
		fetch := e.modelFetchResults[providerID]
		finished := time.Now()
		fetch.FinishedAt = &finished
		if callErr != nil {
			fetch.State = "error"
			fetch.Error = modelFetchError(callErr)
		} else {
			fetch.State = "ready"
			fetch.Models = models
			if len(models) == 0 {
				fetch.Error = "The provider returned no models."
			}
		}
		e.modelFetchResults[providerID] = fetch
		e.refreshProvidersLocked()
		e.publishLocked(contracts.Event{Kind: "providers.changed"})
	}()
	return nil
}

// Provider error payloads are untrusted and may echo request headers. Preserve
// only host-generated categories and numeric HTTP status; never publish them.
func modelFetchError(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "Model list request timed out."
	case errors.Is(err, context.Canceled):
		return "Model list request was canceled."
	case err.Error() == "provider connection failed":
		return "Could not connect to the provider endpoint."
	}
	var status int
	if _, scanErr := fmt.Sscanf(err.Error(), "model list request failed (HTTP %d)", &status); scanErr == nil &&
		status >= 100 && status <= 599 &&
		err.Error() == "model list request failed (HTTP "+strconv.Itoa(status)+")" {
		return "Model list request failed (HTTP " + strconv.Itoa(status) + ")."
	}
	return "The provider returned an invalid model list."
}

func (e *Engine) invalidateModelFetchesLocked(providerID string) {
	if run := e.modelFetchRuns[providerID]; run != nil {
		run.cancel()
		delete(e.modelFetchRuns, providerID)
	}
	delete(e.modelFetchResults, providerID)
}

func (e *Engine) providerModelFetchLocked(providerID string) *contracts.ModelFetch {
	fetch, ok := e.modelFetchResults[providerID]
	if !ok {
		return nil
	}
	return contracts.CloneModelFetch(&fetch)
}
