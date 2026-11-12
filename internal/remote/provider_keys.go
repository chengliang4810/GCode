package remote

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"gcode/internal/contracts"
)

var _ contracts.ProviderKeyService = (*Client)(nil)

type providerKeyReveal struct {
	ID string `json:"id"`
}

// providerKeyHandlers exposes one explicit reveal instead of a readable field:
// the request carries the provider ID in a body, so neither the credential nor
// the request lands in access-log query strings.
func (s Server) providerKeyHandlers(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/provider-key/reveal", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		service, ok := s.Core.(contracts.ProviderKeyService)
		if !ok {
			http.Error(w, "provider keys unavailable", http.StatusNotImplemented)
			return
		}
		var request providerKeyReveal
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&request); err != nil || request.ID == "" {
			http.Error(w, "invalid reveal request", http.StatusBadRequest)
			return
		}
		key, err := service.RevealProviderKey(r.Context(), request.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// A reveal is auditable: record the target and caller, never the value.
		log.Printf("provider credential revealed: provider=%s remote=%s", request.ID, r.RemoteAddr)
		writeJSON(w, key)
	})
}

func (c *Client) RevealProviderKey(ctx context.Context, id string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	response, err := c.request(ctx, http.MethodPost, "/v1/provider-key/reveal", providerKeyReveal{ID: id})
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	var key string
	err = json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&key)
	return key, err
}
