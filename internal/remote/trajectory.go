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

func (s Server) trajectoryHandlers(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/model-trajectory", func(w http.ResponseWriter, r *http.Request) {
		service, ok := s.Core.(contracts.ModelTrajectoryService)
		if !ok {
			http.Error(w, "model trajectory unavailable", http.StatusNotImplemented)
			return
		}
		result, err := service.ReadModelTrajectory(r.Context(), r.URL.Query().Get("sessionId"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, result)
	})
}

func (c *Client) ReadModelTrajectory(ctx context.Context, sessionID string) (contracts.ModelTrajectory, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	response, err := c.request(ctx, http.MethodGet, "/v1/model-trajectory?"+url.Values{"sessionId": {sessionID}}.Encode(), nil)
	if err != nil {
		return contracts.ModelTrajectory{}, err
	}
	defer response.Body.Close()
	var result contracts.ModelTrajectory
	err = json.NewDecoder(io.LimitReader(response.Body, 48<<20)).Decode(&result)
	return result, err
}
