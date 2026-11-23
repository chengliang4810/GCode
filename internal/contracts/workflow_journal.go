package contracts

import (
	"context"
	"encoding/json"
	"time"
)

// WorkflowJournalRecord is one fact a run wrote down, in the order it was
// appended. The interface reads these; it never rewrites them.
type WorkflowJournalRecord struct {
	Sequence uint64          `json:"sequence"`
	Kind     string          `json:"kind"`
	At       time.Time       `json:"at"`
	RunID    string          `json:"runId,omitempty"`
	Data     json.RawMessage `json:"data,omitempty"`
}

// WorkflowJournal is one run's own event log.
type WorkflowJournal struct {
	Records []WorkflowJournalRecord `json:"records"`
	// Damaged counts lines the reader could not parse. One broken line must not
	// hide the facts around it.
	Damaged int `json:"damaged,omitempty"`
}

// WorkflowJournalService reads the event log of a workflow run. It is a
// capability a host may or may not offer: a host without it simply shows no
// steps for the run, instead of pretending the run had none.
type WorkflowJournalService interface {
	WorkflowJournal(ctx context.Context, runID string) (WorkflowJournal, error)
}
