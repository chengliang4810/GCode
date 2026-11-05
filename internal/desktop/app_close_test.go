package desktop

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gcode/internal/contracts"
)

type blockingCloseService struct {
	contracts.Service
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *blockingCloseService) Close() error {
	s.once.Do(func() { close(s.started) })
	<-s.release
	return s.Service.Close()
}

func TestWindowCloseStartsCleanupAndExplicitCloseJoins(t *testing.T) {
	a := testApp(t, "default")
	var starts atomic.Int32
	a.onCloseStart = func() { starts.Add(1) }
	service := &blockingCloseService{Service: a.service, started: make(chan struct{}), release: make(chan struct{})}
	a.service = service
	a.startClose()
	a.startClose()
	// This is the callback attached to OnClosed; backend work cannot hold it.
	if !a.closed {
		t.Fatal("window close did not cancel UI work")
	}
	select {
	case <-service.started:
		if starts.Load() != 1 {
			t.Fatal("shutdown deadline was not started exactly once before backend cleanup")
		}
	case <-time.After(time.Second):
		close(service.release)
		t.Fatal("backend cleanup did not start")
	}
	joined := make(chan struct{})
	go func() { a.Close(); close(joined) }()
	select {
	case <-joined:
		close(service.release)
		t.Fatal("explicit Close returned before backend persistence")
	default:
	}
	close(service.release)
	select {
	case <-joined:
	case <-time.After(time.Second):
		t.Fatal("Close did not join completed cleanup")
	}
}
