package engine

import (
	"context"
	"errors"
	"slices"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/workflow"
)

var _ contracts.WorkflowJournalService = (*Engine)(nil)

// WorkflowJournal reads one run's own event log. The log is the run's durable
// fact history, so the interface can show what happened step by step without
// the engine handing out anything mutable.
//
// The run must be one this engine knows: a journal path is built from the run
// identity, and an unknown identity must not become a path.
func (e *Engine) WorkflowJournal(ctx context.Context, runID string) (contracts.WorkflowJournal, error) {
	if err := ctx.Err(); err != nil {
		return contracts.WorkflowJournal{}, err
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return contracts.WorkflowJournal{}, errors.New("a workflow run ID is required")
	}
	e.mu.Lock()
	known := slices.ContainsFunc(e.state.WorkflowRuns, func(run contracts.WorkflowRun) bool { return run.ID == runID })
	e.mu.Unlock()
	if !known {
		return contracts.WorkflowJournal{}, errors.New("workflow run not found")
	}
	records, damaged, err := workflow.ReadJournal(workflow.JournalPath(e.storage.ConfigDirectory(), runID))
	if err != nil {
		return contracts.WorkflowJournal{}, err
	}
	journal := contracts.WorkflowJournal{
		Damaged: damaged,
		Records: make([]contracts.WorkflowJournalRecord, 0, len(records)),
	}
	for _, record := range records {
		journal.Records = append(journal.Records, contracts.WorkflowJournalRecord{
			Sequence: record.Sequence, Kind: record.Kind, At: record.At, RunID: record.RunID, Data: record.Data,
		})
	}
	return journal, nil
}
