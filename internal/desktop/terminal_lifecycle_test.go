package desktop

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

type terminalLifecycleSurface struct {
	closed    chan struct{}
	closeOnce sync.Once
	closeCall atomic.Int32
}

func newTerminalLifecycleSurface() *terminalLifecycleSurface {
	return &terminalLifecycleSurface{closed: make(chan struct{})}
}

func (t *terminalLifecycleSurface) View(*ui.Context) ui.Element { return ui.Element{} }

func (t *terminalLifecycleSurface) Close() error {
	t.closeCall.Add(1)
	t.closeOnce.Do(func() { close(t.closed) })
	return nil
}

type terminalOpenCall struct {
	ctx         context.Context
	cwd         string
	term        *terminalLifecycleSurface
	release     chan struct{}
	releaseOnce sync.Once
}

func (c *terminalOpenCall) finish() {
	c.releaseOnce.Do(func() { close(c.release) })
}

type terminalLifecycleFactory struct {
	opens chan *terminalOpenCall
}

func newTerminalLifecycleFactory() *terminalLifecycleFactory {
	return &terminalLifecycleFactory{opens: make(chan *terminalOpenCall, 8)}
}

func (f *terminalLifecycleFactory) Open(ctx context.Context, cwd string) (TerminalSurface, error) {
	call := &terminalOpenCall{
		ctx:     ctx,
		cwd:     cwd,
		term:    newTerminalLifecycleSurface(),
		release: make(chan struct{}),
	}
	f.opens <- call
	<-call.release
	return call.term, nil
}

func waitForTerminalOpen(t *testing.T, factory *terminalLifecycleFactory) *terminalOpenCall {
	t.Helper()
	select {
	case call := <-factory.opens:
		return call
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for terminal factory Open")
		return nil
	}
}

func waitForTerminalClosed(t *testing.T, term *terminalLifecycleSurface) {
	t.Helper()
	select {
	case <-term.closed:
		if got := term.closeCall.Load(); got != 1 {
			t.Fatalf("terminal Close call count = %d, want 1", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for terminal Close")
	}
}

type queuedTerminalDispatch struct {
	mu        sync.Mutex
	callbacks []func()
	notice    chan struct{}
}

func newQueuedTerminalDispatch() *queuedTerminalDispatch {
	return &queuedTerminalDispatch{notice: make(chan struct{}, 1)}
}

func (d *queuedTerminalDispatch) dispatch(fn func()) {
	d.mu.Lock()
	d.callbacks = append(d.callbacks, fn)
	d.mu.Unlock()
	select {
	case d.notice <- struct{}{}:
	default:
	}
}

func (d *queuedTerminalDispatch) waitFor(t *testing.T, count int) []func() {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		d.mu.Lock()
		callbacks := append([]func(){}, d.callbacks...)
		d.mu.Unlock()
		if len(callbacks) >= count {
			return callbacks
		}
		select {
		case <-d.notice:
		case <-deadline.C:
			t.Fatalf("timed out waiting for %d terminal dispatch callbacks; got %d", count, len(callbacks))
			return nil
		}
	}
}

func TestEnsureTerminalClosesLateOpenAfterCloseWithDroppedDispatch(t *testing.T) {
	factory := newTerminalLifecycleFactory()
	app := Open(nil, OpenOptions{
		TerminalFactory: factory,
		Dispatch:        func(func()) {},
	})

	app.ensureTerminal("/late")
	open := waitForTerminalOpen(t, factory)
	if err := app.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	select {
	case <-open.ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("terminal Open context was not canceled after app Close")
	}

	open.finish()
	waitForTerminalClosed(t, open.term)
}

func TestEnsureTerminalClosesQueuedOpenWhenDispatchIsDiscarded(t *testing.T) {
	factory := newTerminalLifecycleFactory()
	dispatch := newQueuedTerminalDispatch()
	app := Open(nil, OpenOptions{
		TerminalFactory: factory,
		Dispatch:        dispatch.dispatch,
	})

	app.ensureTerminal("/queued")
	open := waitForTerminalOpen(t, factory)
	open.finish()
	dispatch.waitFor(t, 1)
	if err := app.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	waitForTerminalClosed(t, open.term)
}

func TestEnsureTerminalDoesNotOpenAfterClose(t *testing.T) {
	factory := newTerminalLifecycleFactory()
	app := Open(nil, OpenOptions{TerminalFactory: factory})
	if err := app.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	app.ensureTerminal("/after-close")
	select {
	case open := <-factory.opens:
		open.finish()
		t.Fatalf("terminal factory opened after app Close for %q", open.cwd)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestEnsureTerminalDoesNotLetStaleDirectoryOpenReplaceCurrentTerminal(t *testing.T) {
	factory := newTerminalLifecycleFactory()
	dispatch := newQueuedTerminalDispatch()
	app := Open(nil, OpenOptions{
		TerminalFactory: factory,
		Dispatch:        dispatch.dispatch,
	})

	app.ensureTerminal("/old")
	oldOpen := waitForTerminalOpen(t, factory)
	oldOpen.finish()
	dispatch.waitFor(t, 1)
	app.ensureTerminal("/new")
	newOpen := waitForTerminalOpen(t, factory)

	newOpen.finish()
	callbacks := dispatch.waitFor(t, 2)
	waitForTerminalClosed(t, oldOpen.term)

	callbacks[1]()
	waitForDesktopState(t, "new terminal installation", func() bool {
		app.mu.RLock()
		defer app.mu.RUnlock()
		return app.terminal == newOpen.term && !app.terminalLoading
	})
	callbacks[0]()
	waitForDesktopState(t, "new terminal after stale callback", func() bool {
		app.mu.RLock()
		defer app.mu.RUnlock()
		return app.terminal == newOpen.term && !app.terminalLoading
	})

	if err := app.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	waitForTerminalClosed(t, newOpen.term)
}
