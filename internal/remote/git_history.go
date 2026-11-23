package remote

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"gcode/internal/contracts"
)

func (s Server) gitHistoryHandlers(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/git-history", func(w http.ResponseWriter, r *http.Request) {
		service, ok := s.Core.(contracts.GitHistoryService)
		if !ok {
			http.Error(w, "git history unavailable", http.StatusNotImplemented)
			return
		}
		var request contracts.GitHistoryRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			http.Error(w, "invalid git history request", http.StatusBadRequest)
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			http.Error(w, "invalid git history request", http.StatusBadRequest)
			return
		}
		result, err := service.ListGitHistory(r.Context(), request)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, result)
	})
}

func (c *Client) ListGitHistory(ctx context.Context, request contracts.GitHistoryRequest) (contracts.GitHistoryResult, error) {
	response, err := c.request(ctx, http.MethodPost, "/v1/git-history", request)
	if err != nil {
		return contracts.GitHistoryResult{}, err
	}
	defer response.Body.Close()
	var result contracts.GitHistoryResult
	err = json.NewDecoder(io.LimitReader(response.Body, 16<<20)).Decode(&result)
	return result, err
}
