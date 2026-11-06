package remote

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"gcode/internal/contracts"
)

var _ contracts.ApplicationUsageService = (*Client)(nil)

func (s Server) applicationUsageHandlers(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/application-usage", func(w http.ResponseWriter, r *http.Request) {
		service, ok := s.Core.(contracts.ApplicationUsageService)
		if !ok {
			http.Error(w, "application usage unavailable", http.StatusNotImplemented)
			return
		}
		result, err := service.ReadApplicationUsage(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, result)
	})
}

func (c *Client) ReadApplicationUsage(ctx context.Context) (contracts.ApplicationUsage, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	response, err := c.request(ctx, http.MethodGet, "/v1/application-usage", nil)
	if err != nil {
		return contracts.ApplicationUsage{}, err
	}
	defer response.Body.Close()
	var result contracts.ApplicationUsage
	err = json.NewDecoder(io.LimitReader(response.Body, 64<<20)).Decode(&result)
	return result, err
}
