package engine

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/terminalhost"
)

var _ contracts.TerminalService = (*Engine)(nil)

func (e *Engine) createTerminalLocked(command contracts.Command) error {
	if len(e.terminals) >= 32 {
		return errors.New("too many terminals; close unused terminal tabs")
	}
	directory := e.projectPath(command.ProjectID)
	projectID := command.ProjectID
	rootID := e.storage.ActiveRootID()
	if projectID != "" && !slices.ContainsFunc(e.state.Projects, func(p contracts.Project) bool { return p.ID == projectID }) {
		return errors.New("project not found")
	}
	if command.SessionID != "" {
		index := e.sessionIndex(command.SessionID)
		if index < 0 {
			return errors.New("session not found")
		}
		if err := e.ensureSessionDataLocked(command.SessionID); err != nil {
			return err
		}
		session := e.state.Sessions[index]
		if command.ProjectID != "" && command.ProjectID != session.ProjectID {
			return errors.New("terminal project does not match the session")
		}
		directory = session.CWD
		projectID = session.ProjectID
		rootID = session.StorageRootID
	}
	if _, err := e.storage.Resolve(rootID); err != nil {
		return err
	}
	terminalID := id("terminal")
	ownerID := command.SessionID
	if ownerID == "" {
		ownerID = terminalID
	}
	environment, err := e.processEnvironment(rootID, ownerID)
	if err != nil {
		return err
	}
	host, err := terminalhost.StartWithEnvironment(directory, environment)
	if err != nil {
		return err
	}
	meta := contracts.Terminal{ID: terminalID, StorageRootID: rootID, SessionID: command.SessionID, ProjectID: projectID, Title: "Terminal", CWD: directory, Columns: 80, Rows: 24, Status: "running"}
	e.terminals[meta.ID] = host
	e.state.Terminals = append(e.state.Terminals, meta)
	if err = e.persistLocked(command.SessionID); err != nil {
		delete(e.terminals, meta.ID)
		_ = host.Close()
		return err
	}
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		code, waitErr := host.Result()
		e.mu.Lock()
		defer e.mu.Unlock()
		if e.terminals[meta.ID] != host {
			return
		}
		for i := range e.state.Terminals {
			current := &e.state.Terminals[i]
			if current.ID != meta.ID {
				continue
			}
			current.Status = "exited"
			current.ExitCode = code
			current.Output = terminalHistoryText(host.History())
			if waitErr != nil {
				current.Status = "failed"
				current.Output += "\r\n" + waitErr.Error()
			}
		}
		if err := e.persistLocked(meta.SessionID); err != nil {
			for i := range e.state.Terminals {
				if e.state.Terminals[i].ID == meta.ID {
					e.state.Terminals[i].Status = "failed"
				}
			}
		}
		e.publishLocked(contracts.Event{Kind: "terminal.exited"})
	}()
	e.publishLocked(contracts.Event{Kind: "terminal.created"})
	return nil
}

func (e *Engine) terminalCommandLocked(command contracts.Command) error {
	host := e.terminals[command.ID]
	if host == nil {
		return errors.New("terminal process is not running")
	}
	switch command.Kind {
	case "terminal.input":
		if len(command.Text) > 64<<10 {
			return errors.New("terminal input exceeds 64 KiB")
		}
		_, err := host.Write([]byte(command.Text))
		return err
	case "terminal.resize":
		if command.TerminalSize == nil {
			return errors.New("terminal size is required")
		}
		size := command.TerminalSize
		if err := host.Resize(size.Columns, size.Rows); err != nil {
			return err
		}
		for i := range e.state.Terminals {
			if e.state.Terminals[i].ID == command.ID {
				e.state.Terminals[i].Columns, e.state.Terminals[i].Rows = size.Columns, size.Rows
			}
		}
	case "terminal.clear":
		return host.Clear()
	default:
		return fmt.Errorf("unknown terminal command %s", command.Kind)
	}
	return nil
}

func (e *Engine) closeTerminalLocked(id string) error {
	index := slices.IndexFunc(e.state.Terminals, func(t contracts.Terminal) bool { return t.ID == id })
	if index < 0 {
		return errors.New("terminal not found")
	}
	removed := e.state.Terminals[index]
	sessionID := removed.SessionID
	e.state.Terminals = slices.Delete(e.state.Terminals, index, index+1)
	if err := e.persistLocked(sessionID); err != nil {
		return err
	}
	if removed.SessionID == "" && removed.OutputRef != "" && !e.terminalRefInUse(removed.StorageRootID, removed.OutputRef) {
		e.removeTerminalOutput(removed.StorageRootID, removed.OutputRef)
	}
	host := e.terminals[id]
	delete(e.terminals, id)
	if host != nil {
		_ = host.Close()
	}
	e.publishLocked(contracts.Event{Kind: "terminal.closed"})
	return nil
}

func (e *Engine) OpenTerminal(ctx context.Context, id string) (contracts.TerminalStream, error) {
	e.mu.Lock()
	if err := e.checkLocked(ctx); err != nil {
		e.mu.Unlock()
		return nil, err
	}
	host := e.terminals[id]
	if host == nil {
		e.mu.Unlock()
		return nil, errors.New("terminal process is not running; create a new terminal")
	}
	stream, err := host.Attach(ctx)
	e.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return &engineTerminalStream{Stream: stream, engine: e, id: id}, nil
}

func (e *Engine) WriteTerminal(ctx context.Context, id string, data []byte) (int, error) {
	e.mu.Lock()
	if err := e.checkLocked(ctx); err != nil {
		e.mu.Unlock()
		return 0, err
	}
	host := e.terminals[id]
	e.mu.Unlock()
	if host == nil {
		return 0, errors.New("terminal process is not running")
	}
	return host.Write(data)
}

type engineTerminalStream struct {
	*terminalhost.Stream
	engine *Engine
	id     string
}

func (s *engineTerminalStream) Resize(columns, rows int) error {
	e := s.engine
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return errors.New("engine is closed")
	}
	host := e.terminals[s.id]
	if host == nil {
		return errors.New("terminal is closed")
	}
	if err := host.Resize(columns, rows); err != nil {
		return err
	}
	for i := range e.state.Terminals {
		meta := &e.state.Terminals[i]
		if meta.ID == s.id {
			if meta.Columns == columns && meta.Rows == rows {
				return nil
			}
			meta.Columns, meta.Rows = columns, rows
			e.publishLocked(contracts.Event{Kind: "terminal.resized"})
			break
		}
	}
	return nil
}

func terminalHistoryText(history []byte) string {
	return strings.ToValidUTF8(string(history), "\uFFFD")
}
