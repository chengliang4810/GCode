// Package terminalhost owns real shell processes without a window or MyGo dependency.
package terminalhost

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"gcode/internal/processutil"
	"gcode/internal/terminalhost/pty"
)

const HistoryLimit = 8 << 20

type Session struct {
	mu            sync.Mutex
	process       *pty.PTY
	history       []byte
	historyStart  int
	streams       map[*Stream]struct{}
	done          chan struct{}
	readDone      chan struct{}
	closeOnce     sync.Once
	code          int
	waitErr       error
	columns, rows int
}

func Start(directory string) (*Session, error) {
	return StartWithEnvironment(directory, nil)
}

// StartWithEnvironment applies overrides only to the owned process.
func StartWithEnvironment(directory string, overrides map[string]string) (*Session, error) {
	program, args, err := shell()
	if err != nil {
		return nil, err
	}
	env := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.EqualFold(key, "TERM") && !strings.EqualFold(key, "COLORTERM") {
			env = append(env, entry)
		}
	}
	env = processutil.Environment(env, map[string]string{"TERM": "xterm-256color", "COLORTERM": "truecolor"}, overrides)
	p, err := pty.Start(pty.Options{Path: program, Args: args, Dir: directory, Env: env, Cols: 80, Rows: 24})
	if err != nil {
		return nil, err
	}
	s := &Session{process: p, streams: map[*Stream]struct{}{}, done: make(chan struct{}), readDone: make(chan struct{}), columns: 80, rows: 24}
	go s.read()
	go s.wait()
	return s, nil
}

func shell() (string, []string, error) {
	if runtime.GOOS == "windows" {
		for _, name := range []string{"pwsh.exe", "powershell.exe"} {
			if path, err := exec.LookPath(name); err == nil {
				return path, []string{path, "-NoLogo"}, nil
			}
		}
		return "", nil, errors.New("PowerShell was not found")
	}
	name := os.Getenv("SHELL")
	if name == "" {
		name = "/bin/sh"
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return "", nil, err
	}
	return path, []string{path, "-i"}, nil
}

func (s *Session) read() {
	defer close(s.readDone)
	buf := make([]byte, 32<<10)
	for {
		n, err := s.process.Read(buf)
		if n > 0 {
			s.mu.Lock()
			s.broadcastLocked(buf[:n])
			s.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

func (s *Session) broadcastLocked(data []byte) {
	s.appendHistoryLocked(data)
	for stream := range s.streams {
		if !stream.push(data) {
			delete(s.streams, stream)
		}
	}
}

func (s *Session) appendHistoryLocked(data []byte) {
	if len(data) >= HistoryLimit {
		s.history = append(s.history[:0], data[len(data)-HistoryLimit:]...)
		s.historyStart = 0
		return
	}
	if len(s.history) < HistoryLimit {
		n := min(len(data), HistoryLimit-len(s.history))
		s.history = append(s.history, data[:n]...)
		data = data[n:]
	}
	if len(data) > 0 {
		n := copy(s.history[s.historyStart:], data)
		copy(s.history, data[n:])
		s.historyStart = (s.historyStart + len(data)) % len(s.history)
	}
}

func (s *Session) historyLocked() []byte {
	if s.historyStart == 0 {
		return bytes.Clone(s.history)
	}
	data := make([]byte, len(s.history))
	n := copy(data, s.history[s.historyStart:])
	copy(data[n:], s.history[:s.historyStart])
	return data
}

func (s *Session) wait() {
	code, err := s.process.Wait()
	if runtime.GOOS != "windows" {
		select {
		case <-s.readDone:
		case <-time.After(500 * time.Millisecond):
		}
	}
	_ = s.process.Close()
	<-s.readDone
	s.mu.Lock()
	s.code, s.waitErr = code, err
	for stream := range s.streams {
		stream.end(io.EOF)
	}
	close(s.done)
	s.mu.Unlock()
}

func (s *Session) Done() <-chan struct{} { return s.done }
func (s *Session) Result() (int, error) {
	<-s.done
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.code, s.waitErr
}
func (s *Session) History() []byte { s.mu.Lock(); defer s.mu.Unlock(); return s.historyLocked() }

func (s *Session) Attach(ctx context.Context) (*Stream, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.streams) >= 16 {
		return nil, errors.New("terminal has too many attached views")
	}
	stream := &Stream{session: s, initial: bytes.NewReader(s.historyLocked()), done: make(chan struct{})}
	stream.ready = sync.NewCond(&stream.mu)
	s.streams[stream] = struct{}{}
	select {
	case <-s.done:
		stream.end(io.EOF)
	default:
	}
	go func() {
		select {
		case <-ctx.Done():
			_ = stream.Close()
		case <-stream.done:
		}
	}()
	return stream, nil
}

func (s *Session) Write(data []byte) (int, error) {
	select {
	case <-s.done:
		return 0, io.ErrClosedPipe
	default:
	}
	return s.process.Write(data)
}

func (s *Session) Resize(columns, rows int) error {
	if columns < 1 || columns > 32767 || rows < 1 || rows > 32767 {
		return errors.New("terminal dimensions must be between 1 and 32767")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.columns == columns && s.rows == rows {
		return nil
	}
	if err := s.process.Resize(columns, rows, 0, 0); err != nil {
		return err
	}
	s.columns, s.rows = columns, rows
	return nil
}

func (s *Session) Clear() error {
	if err := s.process.ClearScreen(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = nil
	s.historyStart = 0
	s.broadcastLocked([]byte("\x1b[3J\x1b[2J\x1b[H"))
	return nil
}

func (s *Session) Close() error {
	s.closeOnce.Do(func() {
		_ = s.process.Close()
		select {
		case <-s.done:
		case <-time.After(time.Second):
			_ = s.process.Kill()
			<-s.done
		}
	})
	return nil
}

// Stream carries raw VT bytes. It does not echo, line-buffer or interpret input.
type Stream struct {
	session *Session
	mu      sync.Mutex
	ready   *sync.Cond
	initial *bytes.Reader
	buffer  bytes.Buffer
	err     error
	done    chan struct{}
}

func (s *Stream) push(data []byte) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return false
	}
	if s.buffer.Len()+len(data) > 4<<20 {
		s.endLocked(errors.New("terminal view fell behind; reconnect to resume"))
		return false
	}
	s.buffer.Write(data)
	s.ready.Signal()
	return true
}
func (s *Stream) end(err error) { s.mu.Lock(); defer s.mu.Unlock(); s.endLocked(err) }
func (s *Stream) endLocked(err error) {
	if s.err == nil {
		s.err = err
		close(s.done)
		s.ready.Broadcast()
	}
}
func (s *Stream) Read(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.initial.Len() > 0 {
		return s.initial.Read(data)
	}
	for s.buffer.Len() == 0 && s.err == nil {
		s.ready.Wait()
	}
	if s.buffer.Len() > 0 {
		return s.buffer.Read(data)
	}
	return 0, s.err
}
func (s *Stream) Write(data []byte) (int, error) {
	s.mu.Lock()
	err := s.err
	s.mu.Unlock()
	if err != nil {
		return 0, io.ErrClosedPipe
	}
	return s.session.Write(data)
}
func (s *Stream) Resize(columns, rows int) error { return s.session.Resize(columns, rows) }
func (s *Stream) Close() error {
	s.end(io.EOF)
	s.session.mu.Lock()
	delete(s.session.streams, s)
	s.session.mu.Unlock()
	return nil
}
