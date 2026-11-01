package remote

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	"gcode/internal/contracts"
)

func (s Server) terminalHandlers(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/terminals/{id}/stream", func(w http.ResponseWriter, r *http.Request) {
		service, ok := s.Core.(contracts.TerminalService)
		if !ok {
			http.Error(w, "terminal streaming unavailable", http.StatusNotImplemented)
			return
		}
		stream, err := service.OpenTerminal(r.Context(), r.PathValue("id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer stream.Close()
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Cache-Control", "no-store")
		controller := http.NewResponseController(w)
		if err = controller.Flush(); err != nil {
			return
		}
		data := make([]byte, 32<<10)
		for {
			n, readErr := stream.Read(data)
			if n > 0 {
				_ = controller.SetWriteDeadline(time.Now().Add(30 * time.Second))
				if _, err = w.Write(data[:n]); err != nil {
					return
				}
				if err = controller.Flush(); err != nil {
					return
				}
			}
			if readErr != nil {
				return
			}
		}
	})
	mux.HandleFunc("POST /v1/terminals/{id}/input", func(w http.ResponseWriter, r *http.Request) {
		service, ok := s.Core.(contracts.TerminalInputService)
		if !ok {
			http.Error(w, "terminal input unavailable", http.StatusNotImplemented)
			return
		}
		data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
		if err != nil {
			http.Error(w, "terminal input exceeds 64 KiB", http.StatusBadRequest)
			return
		}
		_, err = service.WriteTerminal(r.Context(), r.PathValue("id"), data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func (c *Client) OpenTerminal(ctx context.Context, id string) (contracts.TerminalStream, error) {
	streamCtx, cancel := context.WithCancel(ctx)
	response, err := c.request(streamCtx, http.MethodGet, "/v1/terminals/"+url.PathEscape(id)+"/stream", nil)
	if err != nil {
		cancel()
		return nil, err
	}
	stream := &terminalStream{client: c, id: id, reader: response.Body, cancel: cancel, ctx: streamCtx}
	go func() {
		select {
		case <-c.done:
			stream.Close()
		case <-streamCtx.Done():
		}
	}()
	return stream, nil
}

type terminalStream struct {
	client *Client
	id     string
	reader io.ReadCloser
	ctx    context.Context
	cancel context.CancelFunc
}

func (s *terminalStream) Read(data []byte) (int, error) { return s.reader.Read(data) }
func (s *terminalStream) Write(data []byte) (int, error) {
	written := 0
	for len(data) > 0 {
		n, err := s.writeChunk(data[:min(len(data), 64<<10)])
		written += n
		if err != nil {
			return written, err
		}
		data = data[n:]
	}
	return written, nil
}

func (s *terminalStream) writeChunk(data []byte) (int, error) {
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.client.url+"/v1/terminals/"+url.PathEscape(s.id)+"/input", bytes.NewReader(data))
	if err != nil {
		return 0, err
	}
	request.Header.Set("Authorization", "Bearer "+s.client.token)
	request.Header.Set("Content-Type", "application/octet-stream")
	response, err := s.client.http.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return 0, errors.New("remote terminal input failed")
	}
	return len(data), nil
}
func (s *terminalStream) Resize(columns, rows int) error {
	return s.client.Execute(s.ctx, contracts.Command{Kind: "terminal.resize", ID: s.id, TerminalSize: &contracts.TerminalSize{Columns: columns, Rows: rows}})
}
func (s *terminalStream) Close() error { s.cancel(); return s.reader.Close() }
