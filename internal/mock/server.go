// Package mock provides a deterministic in-process desktop backend.
package mock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/i18n"
)

var (
	ErrClosed   = errors.New("backend is closed")
	ErrNotFound = errors.New("item not found")
	ErrConflict = errors.New("file has changed since it was opened")
	ErrReadOnly = errors.New("file is read-only")
)

type Options struct {
	GitScenario         string
	IntegrationScenario string
	Scenario            string
	StatePath           string
	Platform            string
	Architecture        string
	Now                 func() time.Time
	TokenInterval       time.Duration
	Instant             bool
}

type activeRun struct {
	id     string
	cancel context.CancelFunc
}

// Server owns state. Every returned snapshot and every subscription delivery
// owns its data; callers may retain or edit a copy without racing the backend.
type Server struct {
	mu              sync.Mutex
	persistMu       sync.Mutex
	state           contracts.Snapshot
	opts            Options
	next            uint64
	sequence        uint64
	subNext         uint64
	subs            map[uint64]chan contracts.Event
	subSessions     map[uint64][]string
	runs            map[string]activeRun
	closed          bool
	done            chan struct{}
	wg              sync.WaitGroup
	diskRevision    uint64
	resources       map[string][]byte
	integrationJobs map[string]uint64
	gitJobs         map[string]uint64
	contextSessions map[string]contextSession
}

var _ contracts.Service = (*Server)(nil)
var _ contracts.ApplicationUsageService = (*Server)(nil)

func New(opts Options) (*Server, error) {
	if opts.Platform == "" {
		opts.Platform = runtime.GOOS
	}
	if opts.Architecture == "" {
		opts.Architecture = runtime.GOARCH
	}
	if opts.Now == nil {
		opts.Now = func() time.Time { return time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC) }
	}
	if opts.TokenInterval <= 0 {
		opts.TokenInterval = 30 * time.Millisecond
	}
	s := &Server{opts: opts, subs: make(map[uint64]chan contracts.Event), subSessions: make(map[uint64][]string), runs: make(map[string]activeRun), resources: make(map[string][]byte), done: make(chan struct{}), next: 1000}
	s.state = seed(opts)
	restored := false
	if opts.StatePath != "" {
		data, err := os.ReadFile(opts.StatePath)
		if err == nil {
			saved := persisted{Snapshot: contracts.Snapshot{Settings: DefaultSettings()}}
			if err = json.Unmarshal(data, &saved); err != nil {
				return nil, fmt.Errorf("read saved desktop state: %w", err)
			}
			if saved.Snapshot.SchemaVersion != contracts.SchemaVersion {
				return nil, fmt.Errorf("unsupported desktop state version %d", saved.Snapshot.SchemaVersion)
			}
			s.state = saved.Snapshot
			s.migrateComposerFooterGitProjects(data)
			s.contextSessions = saved.ContextSessions
			restored = true
			s.next = saved.Next
			if s.next < 1000 {
				s.next = 1000
			}
			s.diskRevision = s.state.Revision
			// A prior process cannot retain a running task or a live terminal lease.
			for i := range s.state.Sessions {
				if s.state.Sessions[i].Status == contracts.Working {
					s.state.Sessions[i].Status = contracts.Idle
					s.state.Sessions[i].LastOutcome = "interrupted"
				}
			}
			for id, ms := range s.state.Messages {
				for i := range ms {
					ms[i].Streaming = false
				}
				s.state.Messages[id] = ms
			}
			for i := range s.state.Runs {
				if s.state.Runs[i].Status == "running" {
					s.state.Runs[i].Status = "interrupted"
					s.state.Runs[i].FinishReason = "applicationClosed"
				}
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("read desktop state: %w", err)
		}
	}
	s.state.Capabilities.Voice = false
	s.ensureIntegrationsLocked()
	if !restored {
		s.seedIntegrationScenarioLocked(opts.IntegrationScenario)
	}
	s.state.Settings.General.Language = i18n.Normalize(s.state.Settings.General.Language)
	if restored {
		s.restoreIntegrationsLocked()
	}
	if status := s.state.ModelCatalog.State; status == "" || status == contracts.CatalogReady || status == contracts.CatalogEmpty || status == contracts.CatalogNoAgents {
		s.state.ModelCatalog.State = catalogStatus(s.state)
	}
	s.state.Capabilities.Appshots = opts.Platform == "darwin"
	if s.state.AppshotCapabilities.Platform != opts.Platform {
		s.state.AppshotCapabilities = appshotCapabilities(opts.Platform)
	}
	if s.state.Messages == nil {
		s.state.Messages = make(map[string][]contracts.Message)
	}
	for i := range s.state.Sessions {
		if s.state.Sessions[i].ReasoningLevel == "" {
			s.state.Sessions[i].ReasoningLevel = "High"
		}
	}
	for i := range s.state.Attachments {
		if s.state.Attachments[i].Status == "transferring" {
			s.state.Attachments[i].Status = "cancelled"
		}
	}
	if restored {
		for sid := range s.state.Messages {
			s.settleRunToolsLocked(sid)
		}
		reconcileAgentTurns(&s.state, opts)
	}
	s.ensureComposerFooterGitLocked(restored)
	s.ensureContextUsageLocked(restored)
	if opts.Scenario == "live" && s.sessionIndexLocked("chat-0") >= 0 && s.state.Sessions[s.sessionIndexLocked("chat-0")].Status == contracts.Working {
		s.mu.Lock()
		s.liveScenarioLocked()
		if active, ok := s.runs["chat-0"]; ok {
			s.beginHostContextLocked("chat-0", active.id)
		}
		s.mu.Unlock()
	}
	return s, nil
}

func (s *Server) Bootstrap(ctx context.Context) (contracts.Snapshot, error) { return s.Snapshot(ctx) }
func (s *Server) Snapshot(ctx context.Context, sessionIDs ...string) (contracts.Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return contracts.Snapshot{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return contracts.Snapshot{}, ErrClosed
	}
	return cloneSnapshot(contracts.SessionProjection(s.state, uniqueSessionIDs(sessionIDs))), nil
}

// ReadApplicationUsage returns compact usage metadata without exposing
// transcript bodies. Resident messages are projected while the backend lock is
// held, so callers never need a full scoped snapshot to build usage reports.
func (s *Server) ReadApplicationUsage(ctx context.Context) (contracts.ApplicationUsage, error) {
	if err := ctx.Err(); err != nil {
		return contracts.ApplicationUsage{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return contracts.ApplicationUsage{}, ErrClosed
	}
	usage := contracts.ApplicationUsage{Records: make(map[string][]contracts.UsageRecord, len(s.state.Messages))}
	for sessionID, messages := range s.state.Messages {
		usage.Records[sessionID] = contracts.UsageRecordsFromMessages(messages)
	}
	usage.Runs = make([]contracts.Run, 0, len(s.state.Runs))
	for _, run := range s.state.Runs {
		usage.Runs = append(usage.Runs, contracts.Run{
			ID:         run.ID,
			SessionID:  run.SessionID,
			Status:     run.Status,
			DurationMS: run.DurationMS,
		})
	}
	return usage, nil
}

// Subscribe sends the current scoped state immediately. A slow reader receives
// the newest scoped snapshot instead of an unbounded backlog of obsolete deltas.
func (s *Server) Subscribe(ctx context.Context, sessionIDs ...string) (<-chan contracts.Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrClosed
	}
	s.subNext++
	id := s.subNext
	ch := make(chan contracts.Event, 1)
	s.subs[id] = ch
	sessionIDs = uniqueSessionIDs(sessionIDs)
	s.subSessions[id] = slices.Clone(sessionIDs)
	snap := cloneSnapshot(contracts.SessionProjection(s.state, sessionIDs))
	ch <- contracts.Event{SchemaVersion: contracts.SchemaVersion, EventID: fmt.Sprintf("snapshot-%d", id), Sequence: s.sequence, Revision: snap.Revision, Kind: "snapshot", Snapshot: &snap}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		select {
		case <-ctx.Done():
		case <-s.done:
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if existing, ok := s.subs[id]; ok {
			delete(s.subs, id)
			delete(s.subSessions, id)
			close(existing)
		}
	}()
	return ch, nil
}

func (s *Server) idLocked(prefix string) string {
	s.next++
	return fmt.Sprintf("%s-%d", prefix, s.next)
}

func (s *Server) publishLocked(kind, sessionID, runID, requestID, text string) {
	s.observeHostContextLocked(kind, sessionID, runID)
	if sessionID != "" && runID != "" {
		s.updateRunSummaryLocked(sessionID, runID)
	}
	s.state.Revision++
	s.deliverLocked(kind, sessionID, runID, requestID, text)
}

func (s *Server) deliverLocked(kind, sessionID, runID, requestID, text string) {
	s.sequence++
	for id, ch := range s.subs {
		snap := cloneSnapshot(contracts.SessionProjection(s.state, s.subSessions[id]))
		ev := contracts.Event{SchemaVersion: contracts.SchemaVersion, EventID: fmt.Sprintf("event-%d", s.sequence), Sequence: s.sequence, Revision: s.state.Revision, Kind: kind, SessionID: sessionID, RunID: runID, RequestID: requestID, Text: text, Snapshot: &snap}
		select {
		case ch <- ev:
		default:
			select {
			case <-ch:
			default:
			}
			ch <- ev
		}
	}
}

func clone[T any](v T) T {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	var out T
	if err = json.Unmarshal(data, &out); err != nil {
		panic(err)
	}
	return out
}

func uniqueSessionIDs(ids []string) []string {
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != "" && !slices.Contains(result, id) {
			result = append(result, id)
		}
	}
	return result
}

type persisted struct {
	Snapshot        contracts.Snapshot        `json:"snapshot"`
	Next            uint64                    `json:"next"`
	ContextSessions map[string]contextSession `json:"contextSessions,omitempty"`
}

func (s *Server) persist() error {
	if s.opts.StatePath == "" {
		return nil
	}
	s.mu.Lock()
	p := s.withContextSessionsLocked(persisted{Snapshot: cloneSnapshot(s.state), Next: s.next})
	s.mu.Unlock()
	return s.savePersisted(p)
}

func (s *Server) savePersisted(p persisted) error {
	if s.opts.StatePath == "" {
		return nil
	}
	s.persistMu.Lock()
	defer s.persistMu.Unlock()
	if p.Snapshot.Revision <= s.diskRevision {
		return nil
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.opts.StatePath)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".desktop-state-*.json")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(name, s.opts.StatePath); err != nil {
		return err
	}
	s.diskRevision = p.Snapshot.Revision
	return nil
}

func (s *Server) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	for i := range s.state.Attachments {
		if s.state.Attachments[i].Status == "transferring" {
			s.state.Attachments[i].Status = "cancelled"
		}
	}
	for _, r := range s.runs {
		r.cancel()
	}
	for sid := range s.state.Messages {
		s.settleRunToolsLocked(sid)
	}
	for ri := range s.state.Runs {
		if s.state.Runs[ri].Status == "running" {
			s.state.Runs[ri].Status = "interrupted"
			s.state.Runs[ri].FinishReason = "applicationClosed"
			end := s.finishRunTimingLocked(s.state.Runs[ri].SessionID, s.state.Runs[ri].ID)
			s.state.Runs[ri].FinishedAt = &end
		}
	}
	for i := range s.state.Sessions {
		if s.state.Sessions[i].Status == contracts.Working {
			s.state.Sessions[i].Status = contracts.Idle
			s.state.Sessions[i].LastOutcome = "interrupted"
		}
	}
	for key, ms := range s.state.Messages {
		for i := range ms {
			ms[i].Streaming = false
		}
		s.state.Messages[key] = ms
	}
	s.state.Revision++
	close(s.done)
	s.mu.Unlock()
	s.wg.Wait()
	return s.persist()
}

func (s *Server) Execute(ctx context.Context, c contracts.Command) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := c.Validate(); err != nil {
		return err
	}
	c = c.Clone()
	if contracts.IsComposerGitCommand(c.Kind) {
		return s.composerGitCommand(ctx, c)
	}
	switch c.Kind {
	case "background.install", "background.remove", "background.adjust":
		return s.updateBackground(ctx, c)
	case "message.send":
		req := contracts.SendRequest{SessionID: c.SessionID, ProjectID: c.ProjectID, Text: c.Text}
		if c.Send != nil {
			req = *c.Send
		}
		if req.ProjectID == "" {
			req.ProjectID = c.ProjectID
		}
		_, err := s.Send(ctx, req)
		return err
	case "message.stop":
		return s.Stop(ctx, c.SessionID)
	case "message.retry":
		_, err := s.Retry(ctx, c.SessionID)
		return err
	case "session.create":
		_, err := s.CreateSession(ctx, c.ProjectID)
		return err
	case "session.update":
		if c.Session == nil {
			return errors.New("session update is required")
		}
		return s.UpdateSession(ctx, *c.Session)
	case "session.archive":
		return s.UpdateSession(ctx, contracts.SessionUpdate{ID: c.ID, Archived: &c.Enabled})
	case "group.create":
		return s.GroupCreate(ctx, c.Text)
	case "group.rename":
		return s.GroupRename(ctx, c.ID, c.Text)
	case "group.delete":
		return s.GroupDelete(ctx, c.ID)
	case "session.pin":
		return s.UpdateSession(ctx, contracts.SessionUpdate{ID: c.ID, Pinned: &c.Enabled})
	case "session.rename":
		return s.UpdateSession(ctx, contracts.SessionUpdate{ID: c.ID, Title: &c.Text})
	case "project.create":
		if c.ProjectCreate != nil {
			_, err := s.CreateProjectOnDevice(ctx, *c.ProjectCreate)
			return err
		}
		_, err := s.CreateProject(ctx, c.Name, c.Path)
		return err
	case "settings.update":
		if c.Settings == nil {
			return errors.New("settings payload is required")
		}
		return s.UpdateSettings(ctx, *c.Settings)
	case "file.save":
		_, err := s.SaveFile(ctx, c.ID, c.Text, c.Version)
		return err
	case "request.answer":
		return s.answer(ctx, c.ID, c.Text, c.QuestionAnswers)
	case "terminal.input":
		_, err := s.TerminalInput(ctx, c.ID, c.Text)
		return err
	default:
		return s.mutate(ctx, c)
	}
}

func (s *Server) mutate(ctx context.Context, c contracts.Command) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrClosed
	}
	err := s.applyLocked(c)
	if err == nil {
		s.publishLocked(c.Kind, c.SessionID, "", "", "")
	}
	s.mu.Unlock()
	if err != nil {
		return err
	}
	return s.persist()
}

func (s *Server) applyLocked(c contracts.Command) error {
	if contracts.IsIntegrationCommand(c.Kind) {
		return s.integrationCommandLocked(c)
	}
	switch c.Kind {
	case "provider.reorder", "provider.models.reorder":
		return s.reorderProvidersLocked(c)
	case "group.reorder":
		return s.reorderGroupsLocked(c)
	case "session.reorder":
		return s.reorderSessionLocked(c)
	case "catalog.retry":
		s.state.ModelCatalog.Error = ""
		if catalogStatus(s.state) == contracts.CatalogNoAgents {
			s.state.ModelCatalog.State = contracts.CatalogNoAgents
			return nil
		}
		if len(s.state.Models) == 0 {
			catalog := seed(s.opts).Models
			for _, model := range catalog {
				for _, provider := range s.state.Providers {
					if model.ProviderID == provider.ID {
						s.state.Models = append(s.state.Models, model)
						break
					}
				}
			}
		}
		s.state.ModelCatalog.State = catalogStatus(s.state)
		return nil
	case "appshots.permissions.check":
		return nil
	case "appshots.permission":
		if s.opts.Platform != "darwin" {
			return errors.New("window capture is unavailable on this platform")
		}
		switch c.ID {
		case "windowCapture":
			s.state.AppshotCapabilities.WindowCapture = "ready"
		case "applicationText":
			s.state.AppshotCapabilities.ApplicationText = "ready"
		default:
			return errors.New("unknown Appshots permission")
		}
		return nil
	case "session.fork", "session.sidechat":
		return s.branchSessionLocked(c)
	case "theme.analyze", "theme.select", "theme.import", "theme.remove", "theme.reload", "theme.duplicate":
		return s.themeMutationLocked(c)
	case "file.create", "file.rename", "file.delete":
		return s.fileMutationLocked(c)
	case "session.delete":
		for i, v := range s.state.Sessions {
			if v.ID == c.ID {
				if r, ok := s.runs[v.ID]; ok {
					r.cancel()
					delete(s.runs, v.ID)
				}
				s.state.Sessions = append(s.state.Sessions[:i], s.state.Sessions[i+1:]...)
				delete(s.state.Messages, c.ID)
				s.deleteContextUsageLocked(c.ID)
				s.removeQueueLocked(c.ID)
				for ri := range s.state.Runs {
					if s.state.Runs[ri].SessionID == c.ID && (s.state.Runs[ri].Status == "running" || s.state.Runs[ri].Status == "awaitingInput") {
						s.state.Runs[ri].Status = "interrupted"
						s.state.Runs[ri].FinishReason = "sessionDeleted"
						end := s.finishRunTimingLocked(c.ID, s.state.Runs[ri].ID)
						s.state.Runs[ri].FinishedAt = &end
					}
				}
				return nil
			}
		}
		return ErrNotFound
	case "queue.remove", "queue.update":
		for i, v := range s.state.Queue {
			if v.ID == c.ID {
				if c.Kind == "queue.remove" {
					s.state.Queue = append(s.state.Queue[:i], s.state.Queue[i+1:]...)
				} else {
					if strings.TrimSpace(c.Text) == "" {
						return errors.New("message is empty")
					}
					s.state.Queue[i].Text = c.Text
				}
				return nil
			}
		}
		return ErrNotFound
	case "provider.toggle", "provider.connect":
		for i, v := range s.state.Providers {
			if v.ID == c.ID {
				s.state.Providers[i].Enabled = c.Enabled
				if c.Kind == "provider.connect" {
					s.state.Providers[i].Enabled = true
					s.state.Providers[i].Status = "connected"
					s.state.Providers[i].Account = s.state.User.Email
				}
				s.state.ModelCatalog = contracts.ModelCatalogState{State: catalogStatus(s.state)}
				return nil
			}
		}
		return ErrNotFound
	case "device.toggle":
		for i, v := range s.state.Devices {
			if v.ID == c.ID {
				if c.Enabled {
					s.state.Devices[i].Status = "connected"
					s.state.Devices[i].LastSeen = s.opts.Now().UTC()
				} else {
					s.state.Devices[i].Status = "offline"
				}
				return nil
			}
		}
		return ErrNotFound
	case "terminal.create":
		cwd := "~"
		if c.ProjectID != "" {
			found := false
			for _, p := range s.state.Projects {
				if p.ID == c.ProjectID {
					cwd = p.Path
					found = true
					break
				}
			}
			if !found {
				return ErrNotFound
			}
		}
		id := s.idLocked("terminal")
		s.state.Terminals = append(s.state.Terminals, contracts.Terminal{ID: id, ProjectID: c.ProjectID, Title: "Terminal", CWD: cwd, Columns: 80, Rows: 24, Status: "idle", Output: "\x1b[38;2;139;132;255m❯\x1b[0m "})
		return nil
	case "terminal.close":
		for i, v := range s.state.Terminals {
			if v.ID == c.ID {
				s.state.Terminals = append(s.state.Terminals[:i], s.state.Terminals[i+1:]...)
				return nil
			}
		}
		return ErrNotFound
	case "terminal.resize":
		rows, err := strconv.Atoi(c.Name)
		if c.TerminalSize != nil {
			c.Version = uint64(max(0, c.TerminalSize.Columns))
			rows = c.TerminalSize.Rows
			err = nil
		}
		if err != nil || rows < 1 || c.Version < 1 || c.Version > 1000 || rows > 1000 {
			return errors.New("invalid terminal size")
		}
		for i, v := range s.state.Terminals {
			if v.ID == c.ID {
				s.state.Terminals[i].Columns = int(c.Version)
				s.state.Terminals[i].Rows = rows
				return nil
			}
		}
		return ErrNotFound
	case "update.check":
		s.state.Update.Status = "upToDate"
		return nil
	case "cloud.toggle":
		kind := "cloud.delete"
		if c.Enabled {
			kind = "cloud.enable"
		}
		return s.integrationCommandLocked(contracts.Command{Kind: kind})
	case "attachment.add":
		if c.Attachment == nil {
			return errors.New("attachment is required")
		}
		s.state.Appshots = append(s.state.Appshots, clone(*c.Attachment))
		return nil
	default:
		return fmt.Errorf("unsupported command %q", c.Kind)
	}
}
