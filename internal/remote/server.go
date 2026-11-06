// Package remote adapts the Go core at actual device boundaries.
package remote

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"gcode/internal/contracts"
)

type Server struct {
	Core  contracts.Service
	Token string
}

func (s Server) Handler() (http.Handler, error) {
	if len(s.Token) < 24 {
		return nil, errors.New("remote API token must contain at least 24 characters")
	}
	mux := http.NewServeMux()
	s.resourceHandlers(mux)
	s.linkedFileHandlers(mux)
	s.terminalHandlers(mux)
	s.providerKeyHandlers(mux)
	s.providerLogoHandlers(mux)
	s.modelMetadataHandlers(mux)
	s.trajectoryHandlers(mux)
	s.applicationUsageHandlers(mux)
	mux.HandleFunc("GET /v1/snapshot", func(w http.ResponseWriter, r *http.Request) {
		sessionIDs, err := sessionScopeFromRequest(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		snapshot, err := s.Core.Snapshot(r.Context(), sessionIDs...)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		remoteSnapshot(&snapshot)
		writeJSON(w, snapshot)
	})
	mux.HandleFunc("POST /v1/commands", func(w http.ResponseWriter, r *http.Request) {
		var command contracts.Command
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&command); err != nil {
			http.Error(w, "invalid command", 400)
			return
		}
		if err := s.Core.Execute(r.Context(), command); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		writeJSON(w, struct {
			OK bool `json:"ok"`
		}{true})
	})
	mux.HandleFunc("POST /v1/directories", func(w http.ResponseWriter, r *http.Request) {
		service, ok := s.Core.(contracts.ProjectDirectoryService)
		if !ok {
			http.Error(w, "directory browsing unavailable", http.StatusNotImplemented)
			return
		}
		var request contracts.DirectoryRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&request); err != nil {
			http.Error(w, "invalid directory request", 400)
			return
		}
		result, err := service.ListDirectories(r.Context(), request)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		writeJSON(w, result)
	})
	mux.HandleFunc("POST /v1/directory-locations", func(w http.ResponseWriter, r *http.Request) {
		service, ok := s.Core.(contracts.ProjectDirectoryService)
		if !ok {
			http.Error(w, "directory browsing unavailable", http.StatusNotImplemented)
			return
		}
		var request contracts.DirectoryRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&request); err != nil {
			http.Error(w, "invalid location request", 400)
			return
		}
		result, err := service.ListDirectoryLocations(r.Context(), request)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		writeJSON(w, result)
	})
	mux.HandleFunc("GET /v1/events", func(w http.ResponseWriter, r *http.Request) {
		sessionIDs, err := sessionScopeFromRequest(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		stream, err := s.Core.Subscribe(r.Context(), sessionIDs...)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		controller := http.NewResponseController(w)
		heartbeat := time.NewTicker(15 * time.Second)
		defer heartbeat.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-heartbeat.C:
				_ = controller.SetWriteDeadline(time.Now().Add(15 * time.Second))
				if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
					return
				}
				if err := controller.Flush(); err != nil {
					return
				}
			case event, ok := <-stream:
				if !ok {
					return
				}
				if event.Snapshot != nil {
					snapshot := *event.Snapshot
					remoteSnapshot(&snapshot)
					event.Snapshot = &snapshot
				}
				data, err := json.Marshal(event)
				if err != nil {
					return
				}
				_ = controller.SetWriteDeadline(time.Now().Add(15 * time.Second))
				if _, err := fmt.Fprintf(w, "id: %d\ndata: %s\n\n", event.Sequence, data); err != nil {
					return
				}
				if err := controller.Flush(); err != nil {
					return
				}
			}
		}
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.Token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		mux.ServeHTTP(w, r)
	}), nil
}

func sessionScopeFromRequest(r *http.Request) ([]string, error) {
	values, present := r.URL.Query()["sessionId"]
	if !present {
		return nil, nil
	}
	if len(values) > maxSessionScopeIDs {
		return nil, fmt.Errorf("session scope contains too many IDs (maximum %d)", maxSessionScopeIDs)
	}
	for _, id := range values {
		if id == "" {
			return nil, errors.New("session scope contains an empty ID")
		}
	}
	return values, nil
}

func remoteSnapshot(snapshot *contracts.Snapshot) {
	if snapshot.Runtime != nil {
		info := *snapshot.Runtime
		info.Remote = true
		snapshot.Runtime = &info
	}
}
func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func Shutdown(ctx context.Context, server *http.Server) error { return server.Shutdown(ctx) }
