package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"gcode/internal/attachmentio"
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
		contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || contentType != "application/octet-stream" {
			http.Error(w, "upload raw attachment bytes", http.StatusUnsupportedMediaType)
			return
		}
		if len(r.URL.RawQuery) > 8192 {
			http.Error(w, "invalid attachment metadata", http.StatusBadRequest)
			return
		}
		query := r.URL.Query()
		name := query.Get("name")
		if name == "" || len(name) > 1024 ||
			strings.ContainsRune(name, 0) || len(query.Get("sessionId")) > 128 || len(query.Get("mime")) > 256 {
			http.Error(w, "invalid attachment metadata", http.StatusBadRequest)
			return
		}
		if r.ContentLength > contracts.MaxAttachmentBytes {
			http.Error(w, "attachment exceeds 24 MiB", http.StatusRequestEntityTooLarge)
			return
		}
		data, err := attachmentio.Read(http.MaxBytesReader(w, r.Body, contracts.MaxAttachmentBytes), r.ContentLength, contracts.MaxAttachmentBytes)
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.Is(err, attachmentio.ErrLimit) || errors.As(err, &tooLarge) {
				http.Error(w, "attachment exceeds 24 MiB", http.StatusRequestEntityTooLarge)
			} else {
				http.Error(w, "incomplete attachment upload", http.StatusBadRequest)
			}
			return
		}
		if len(data) == 0 {
			http.Error(w, "empty attachment upload", http.StatusBadRequest)
			return
		}
		request := contracts.AttachmentImportRequest{SessionID: query.Get("sessionId"),
			Clipboard: &contracts.ClipboardAttachment{Name: name, MIME: query.Get("mime"), Data: data}}
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
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	files, err := attachmentio.Plan(request)
	if err != nil {
		return nil, err
	}
	var result []contracts.Attachment
	upload := func(name, mime string, size int64, body io.Reader) error {
		query := url.Values{"name": {name}, "sessionId": {request.SessionID}, "mime": {mime}}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+"/v1/attachments?"+query.Encode(), body)
		if err != nil {
			return err
		}
		req.ContentLength = size
		req.Header.Set("Content-Type", "application/octet-stream")
		response, err := c.send(req)
		if err != nil {
			return err
		}
		var items []contracts.Attachment
		err = json.NewDecoder(response.Body).Decode(&items)
		_ = response.Body.Close()
		if err != nil {
			return err
		}
		result = append(result, items...)
		return nil
	}
	remaining := int64(contracts.MaxAttachmentBatchBytes)
	if request.Clipboard != nil {
		remaining -= int64(len(request.Clipboard.Data))
	}
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		f, size, err := attachmentio.Open(file, remaining)
		if err != nil {
			return nil, err
		}
		err = upload(file.Name, "", size, io.LimitReader(f, size))
		_ = f.Close()
		if err != nil {
			return nil, err
		}
		remaining -= size
	}
	if input := request.Clipboard; input != nil {
		if err := upload(input.Name, input.MIME, int64(len(input.Data)), bytes.NewReader(input.Data)); err != nil {
			return nil, err
		}
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
