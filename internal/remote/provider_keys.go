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

var _ contracts.ProviderKeyService = (*Client)(nil)

func (s Server) providerKeyHandlers(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/provider-key", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		service, ok := s.Core.(contracts.ProviderKeyService)
		if !ok {
			http.Error(w, "provider keys unavailable", http.StatusNotImplemented)
			return
		}
		key, err := service.ReadProviderKey(r.Context(), r.URL.Query().Get("id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, key)
	})
}

func (c *Client) ReadProviderKey(ctx context.Context, id string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	response, err := c.request(ctx, http.MethodGet, "/v1/provider-key?id="+url.QueryEscape(id), nil)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	var key string
	err = json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&key)
	return key, err
}
