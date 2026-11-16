package remote

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"gcode/internal/contracts"
)

func (s Server) instructionHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/v1/instructions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		service, ok := s.Core.(contracts.GlobalInstructionsService)
		if !ok {
			http.Error(w, "global instructions unavailable", http.StatusNotImplemented)
			return
		}
		switch r.Method {
		case http.MethodGet:
			text, err := service.ReadGlobalInstructions(r.Context())
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			writeJSON(w, text)
		case http.MethodPut:
			var text string
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 6*contracts.MaxGlobalInstructionsBytes+2)).Decode(&text); err != nil {
				http.Error(w, "invalid global instructions", http.StatusBadRequest)
				return
			}
			if err := service.WriteGlobalInstructions(r.Context(), text); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.Header().Set("Allow", "GET, PUT")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

func (c *Client) ReadGlobalInstructions(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	response, err := c.request(ctx, http.MethodGet, "/v1/instructions", nil)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	var text string
	err = json.NewDecoder(io.LimitReader(response.Body, 6*contracts.MaxGlobalInstructionsBytes+2)).Decode(&text)
	return text, err
}

func (c *Client) WriteGlobalInstructions(ctx context.Context, text string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	response, err := c.request(ctx, http.MethodPut, "/v1/instructions", text)
	if err != nil {
		return err
	}
	return response.Body.Close()
}
