package remote

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"gcode/internal/contracts"
)

func (s Server) resourceHandlers(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/resources", func(w http.ResponseWriter, r *http.Request) {
		service, ok := s.Core.(contracts.ResourceService)
		if !ok {
			http.Error(w, "resources unavailable", http.StatusNotImplemented)
			return
		}
		data, err := service.ReadResource(r.Context(), r.URL.Query().Get("ref"))
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(data)
	})
	mux.HandleFunc("POST /v1/attachments", func(w http.ResponseWriter, r *http.Request) {
		service, ok := s.Core.(contracts.AttachmentImporter)
		if !ok {
			http.Error(w, "attachments unavailable", http.StatusNotImplemented)
			return
		}
		var request contracts.AttachmentImportRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 140<<20)).Decode(&request); err != nil || len(request.Paths) > 0 {
			http.Error(w, "upload attachment bytes, not host paths", 400)
			return
		}
		items, err := service.ImportAttachments(r.Context(), request)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		writeJSON(w, items)
	})
	mux.HandleFunc("POST /v1/search", func(w http.ResponseWriter, r *http.Request) {
		service, ok := s.Core.(contracts.WorkspaceSearchService)
		if !ok {
			http.Error(w, "search unavailable", http.StatusNotImplemented)
			return
		}
		var request contracts.WorkspaceSearchRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&request); err != nil {
			http.Error(w, "invalid search", 400)
			return
		}
		result, err := service.SearchWorkspace(r.Context(), request)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		writeJSON(w, result)
	})
}
func (c *Client) ReadResource(ctx context.Context, ref string) ([]byte, error) {
	response, err := c.request(ctx, http.MethodGet, "/v1/resources?ref="+url.QueryEscape(ref), nil)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, contracts.MaxAttachmentBytes+1))
	if len(data) > contracts.MaxAttachmentBytes {
		return nil, errors.New("remote resource exceeds limit")
	}
	return data, err
}
func (c *Client) ImportAttachments(ctx context.Context, request contracts.AttachmentImportRequest) ([]contracts.Attachment, error) {
	var inputs []contracts.ClipboardAttachment
	var total int
	for _, path := range request.Paths {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(f, contracts.MaxAttachmentBytes+1))
		_ = f.Close()
		if err != nil {
			return nil, err
		}
		if len(data) > contracts.MaxAttachmentBytes {
			return nil, errors.New("attachment exceeds 24 MiB")
		}
		total += len(data)
		inputs = append(inputs, contracts.ClipboardAttachment{Name: filepath.Base(path), Data: data})
	}
	if request.Clipboard != nil {
		inputs = append(inputs, *request.Clipboard)
		total += len(request.Clipboard.Data)
	}
	if len(inputs) == 0 || len(inputs) > contracts.MaxAttachmentCount || total > contracts.MaxAttachmentBatchBytes {
		return nil, errors.New("invalid attachment batch")
	}
	var result []contracts.Attachment
	for _, input := range inputs {
		response, err := c.request(ctx, http.MethodPost, "/v1/attachments", contracts.AttachmentImportRequest{SessionID: request.SessionID, Clipboard: &input})
		if err != nil {
			return nil, err
		}
		var items []contracts.Attachment
		err = json.NewDecoder(response.Body).Decode(&items)
		_ = response.Body.Close()
		if err != nil {
			return nil, err
		}
		result = append(result, items...)
	}
	return result, nil
}
func (c *Client) SearchWorkspace(ctx context.Context, request contracts.WorkspaceSearchRequest) (contracts.WorkspaceSearchResult, error) {
	response, err := c.request(ctx, http.MethodPost, "/v1/search", request)
	if err != nil {
		return contracts.WorkspaceSearchResult{}, err
	}
	defer response.Body.Close()
	var result contracts.WorkspaceSearchResult
	err = json.NewDecoder(response.Body).Decode(&result)
	return result, err
}
