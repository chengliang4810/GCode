package remote

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"gcode/internal/contracts"
	"gcode/internal/providerlogo"
)

var _ contracts.ProviderLogoService = (*Client)(nil)

func (s Server) providerLogoHandlers(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/provider-logo", func(w http.ResponseWriter, r *http.Request) {
		service, ok := s.Core.(contracts.ProviderLogoService)
		if !ok {
			http.Error(w, "website logo unavailable", http.StatusNotImplemented)
			return
		}
		var input struct {
			BaseURL string `json:"baseUrl"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input); err != nil {
			http.Error(w, "invalid logo request", http.StatusBadRequest)
			return
		}
		logo, err := service.FetchProviderLogo(r.Context(), input.BaseURL)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, struct {
			Logo string `json:"logo"`
		}{logo})
	})
}

func (c *Client) FetchProviderLogo(ctx context.Context, baseURL string) (string, error) {
	response, err := c.request(ctx, http.MethodPost, "/v1/provider-logo", struct {
		BaseURL string `json:"baseUrl"`
	}{baseURL})
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	var result struct {
		Logo string `json:"logo"`
	}
	err = json.NewDecoder(io.LimitReader(response.Body, 512<<10)).Decode(&result)
	if err == nil {
		err = providerlogo.Validate(result.Logo)
	}
	return result.Logo, err
}
