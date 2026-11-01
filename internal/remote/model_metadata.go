package remote

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/modelcatalog"
)

var _ contracts.ModelMetadataService = (*Client)(nil)

func (s Server) modelMetadataHandlers(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/model-metadata", func(w http.ResponseWriter, r *http.Request) {
		service, ok := s.Core.(contracts.ModelMetadataService)
		if !ok {
			http.Error(w, "model metadata unavailable", http.StatusNotImplemented)
			return
		}
		result, err := service.LookupModelMetadata(r.Context(), modelcatalog.Query{
			ID: r.URL.Query().Get("id"), BaseURL: r.URL.Query().Get("baseUrl"),
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, result)
	})
}

func (c *Client) LookupModelMetadata(ctx context.Context, q modelcatalog.Query) (modelcatalog.Metadata, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	query := url.Values{"id": {q.ID}, "baseUrl": {q.BaseURL}}
	response, err := c.request(ctx, http.MethodGet, "/v1/model-metadata?"+query.Encode(), nil)
	if err != nil {
		return modelcatalog.Metadata{}, err
	}
	defer response.Body.Close()
	var result modelcatalog.Metadata
	err = json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&result)
	return result, err
}
