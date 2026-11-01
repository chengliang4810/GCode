package remote

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"gcode/internal/contracts"
)

var _ contracts.LinkedFileService = (*Client)(nil)

func (s Server) linkedFileHandlers(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/linked-files/read", func(w http.ResponseWriter, r *http.Request) {
		service, ok := s.Core.(contracts.LinkedFileService)
		if !ok {
			http.Error(w, "linked files unavailable", http.StatusNotImplemented)
			return
		}
		var request contracts.LinkedFileRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			http.Error(w, "invalid linked file request", http.StatusBadRequest)
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			http.Error(w, "invalid linked file request", http.StatusBadRequest)
			return
		}
		result, err := service.ReadLinkedFile(r.Context(), request)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, result)
	})
}

func (c *Client) ReadLinkedFile(ctx context.Context, request contracts.LinkedFileRequest) (contracts.LinkedFileResult, error) {
	response, err := c.request(ctx, http.MethodPost, "/v1/linked-files/read", request)
	if err != nil {
		return contracts.LinkedFileResult{}, err
	}
	defer response.Body.Close()
	var result contracts.LinkedFileResult
	err = json.NewDecoder(io.LimitReader(response.Body, 128<<20)).Decode(&result)
	return result, err
}
