package engine

import (
	"context"
	"errors"
	"strings"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
)

const modelCheckTimeout = 30 * time.Second

type modelCheckRun struct {
	providerID string
	cancel     context.CancelFunc
}

// startModelCheckLocked accepts a user-requested test, then lets the engine own
// its lifetime. No session, Agent, tool, workspace or conversation is created.
func (e *Engine) startModelCheckLocked(modelID string) error {
	var provider model.Provider
	var selected model.Model
	found := false
	for _, candidate := range e.providers {
		for _, candidateModel := range candidate.Models {
			if candidate.ID+"/"+candidateModel.ID == modelID {
				provider = model.CloneProvider(candidate)
				selected = candidateModel
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		return errors.New("configured model not found")
	}
	if e.modelChecks[modelID] != nil {
		return errors.New("this model is already being tested")
	}
	if provider.Disabled {
		return errors.New("enable the provider before testing its model")
	}
	if selected.Disabled {
		return errors.New("enable the model before testing it")
	}
	// Bound the diagnostic response independently of the normal execution limit.
	// Bind still validates the frozen provider and its matching selected model.
	for i := range provider.Models {
		if provider.Models[i].ID == selected.ID {
			if selected.MaxOutput == 0 || selected.MaxOutput > 64 {
				provider.Models[i].MaxOutput = 64
			}
			selected = provider.Models[i]
			break
		}
	}
	gateway, err := e.client.Bind(provider, selected)
	if err != nil {
		return err
	}
	effort := ""
	if len(selected.ReasoningLevels) > 0 {
		effort = selected.ReasoningLevels[0]
		for _, level := range selected.ReasoningLevels {
			if level == "none" {
				effort = level
				break
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), modelCheckTimeout)
	run := &modelCheckRun{providerID: provider.ID, cancel: cancel}
	e.modelChecks[modelID] = run
	e.modelCheckResults[modelID] = contracts.ModelCheck{
		ProviderID: provider.ID, ModelID: modelID, State: "testing", StartedAt: time.Now(),
	}
	e.refreshProvidersLocked()
	e.publishLocked(contracts.Event{Kind: "providers.changed"})
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		defer cancel()
		result, callErr := gateway.Stream(ctx, model.Request{
			History: []model.Message{{Role: "user", Text: "Reply only OK."}},
			Effort:  effort,
		}, func(model.Delta) error { return nil })
		e.mu.Lock()
		defer e.mu.Unlock()
		// Configuration replacement, deletion and shutdown invalidate this request.
		if e.closed || e.modelChecks[modelID] != run {
			return
		}
		delete(e.modelChecks, modelID)
		check := e.modelCheckResults[modelID]
		check.State = "ready"
		finished := time.Now()
		check.FinishedAt = &finished
		if result.Usage.Known || result.Usage.Input != 0 || result.Usage.Output != 0 || result.Usage.Cached != 0 {
			usage := result.Usage
			check.Usage = &usage
		}
		if callErr == nil && (len(result.Message.Calls) > 0 || strings.TrimSpace(result.Message.Text) == "") {
			callErr = errors.New("model test returned no text response")
		}
		if callErr != nil {
			check.State = "error"
			check.Error = modelCheckError(callErr)
		}
		e.modelCheckResults[modelID] = check
		e.refreshProvidersLocked()
		e.publishLocked(contracts.Event{Kind: "providers.changed"})
	}()
	return nil
}

// The model boundary redacts credentials before constructing ProviderError.
// Keep its original provider diagnostics in the model test as in conversations.
func modelCheckError(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "Model test timed out."
	case errors.Is(err, context.Canceled):
		return "Model test was canceled."
	}
	var providerErr *model.ProviderError
	if errors.As(err, &providerErr) {
		return providerErr.Error()
	}
	return "Model returned an invalid or incomplete response."
}

func (e *Engine) invalidateModelChecksLocked(providerID string) {
	for modelID, run := range e.modelChecks {
		if run.providerID == providerID {
			run.cancel()
			delete(e.modelChecks, modelID)
		}
	}
	for modelID, check := range e.modelCheckResults {
		if check.ProviderID == providerID {
			delete(e.modelCheckResults, modelID)
		}
	}
}

func (e *Engine) providerModelChecksLocked(providerID string) []contracts.ModelCheck {
	for _, provider := range e.providers {
		if provider.ID != providerID {
			continue
		}
		checks := make([]contracts.ModelCheck, 0, len(provider.Models))
		for _, configured := range provider.Models {
			if check, ok := e.modelCheckResults[provider.ID+"/"+configured.ID]; ok {
				checks = append(checks, check)
			}
		}
		return contracts.CloneModelChecks(checks)
	}
	return nil
}
