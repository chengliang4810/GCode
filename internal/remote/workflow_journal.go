package remote

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"

	"gcode/internal/contracts"
)

var _ contracts.WorkflowJournalService = (*Client)(nil)

// WorkflowJournal serves a run's own event log to a connected client. The log
// belongs to the host, so a client reads it instead of keeping its own copy.
func (s Server) workflowJournalHandlers(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/workflow-journal", func(w http.ResponseWriter, r *http.Request) {
		service, ok := s.Core.(contracts.WorkflowJournalService)
		if !ok {
			http.Error(w, "workflow journal unavailable", http.StatusNotImplemented)
			return
		}
		result, err := service.WorkflowJournal(r.Context(), r.URL.Query().Get("runId"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, result)
	})
}

func (c *Client) WorkflowJournal(ctx context.Context, runID string) (contracts.WorkflowJournal, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	response, err := c.request(ctx, http.MethodGet, "/v1/workflow-journal?runId="+url.QueryEscape(runID), nil)
	if err != nil {
		return contracts.WorkflowJournal{}, err
	}
	defer response.Body.Close()
	var result contracts.WorkflowJournal
	err = json.NewDecoder(io.LimitReader(response.Body, 32<<20)).Decode(&result)
	return result, err
}
