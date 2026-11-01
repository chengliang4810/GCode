package workbench

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"gcode/internal/contracts"
)

type scriptTestPage struct {
	mu      sync.Mutex
	calls   []string
	entered chan string
	release chan struct{}
}

func (p *scriptTestPage) EvalContext(ctx context.Context, code string) (any, error) {
	p.mu.Lock()
	p.calls = append(p.calls, code)
	p.mu.Unlock()
	if code == "block" {
		p.entered <- code
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-p.release:
		}
	}
	return code, nil
}

func TestScriptQueuePreservesLabelsBeforeMarkerRemoval(t *testing.T) {
	page := &scriptTestPage{entered: make(chan string, 1), release: make(chan struct{})}
	queue := newScriptQueue(context.Background(), page)
	t.Cleanup(queue.close)
	if err := queue.post("block", ""); err != nil {
		t.Fatal(err)
	}
	select {
	case <-page.entered:
	case <-time.After(time.Second):
		t.Fatal("script worker did not start")
	}
	for _, task := range [][2]string{{"label", ""}, {"focus-old", "focus"}, {"keep", "keep"}, {"focus-latest", "focus"}} {
		if err := queue.post(task[0], task[1]); err != nil {
			t.Fatal(err)
		}
	}
	close(page.release)
	if _, err := queue.EvalContext(context.Background(), "barrier"); err != nil {
		t.Fatal(err)
	}
	page.mu.Lock()
	defer page.mu.Unlock()
	if want := []string{"block", "label", "keep", "focus-latest", "barrier"}; !reflect.DeepEqual(page.calls, want) {
		t.Fatalf("script order = %v; want %v", page.calls, want)
	}
}

func TestScriptQueueCloseCancelsActiveAndPendingWork(t *testing.T) {
	page := &scriptTestPage{entered: make(chan string, 1), release: make(chan struct{})}
	queue := newScriptQueue(context.Background(), page)
	result := make(chan error, 1)
	go func() { _, err := queue.EvalContext(context.Background(), "block"); result <- err }()
	select {
	case <-page.entered:
	case <-time.After(time.Second):
		t.Fatal("script worker did not start")
	}
	if err := queue.post("must not execute", ""); err != nil {
		t.Fatal(err)
	}
	queue.close()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("active script result = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("active script caller did not unblock")
	}
	select {
	case <-queue.done:
	case <-time.After(time.Second):
		t.Fatal("script worker leaked after close")
	}
	if err := queue.post("after close", ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("post after close = %v", err)
	}
	page.mu.Lock()
	defer page.mu.Unlock()
	if !reflect.DeepEqual(page.calls, []string{"block"}) {
		t.Fatalf("closed queue ran pending work: %v", page.calls)
	}
}

func TestSwitchingWorkspaceStopsHiddenPicker(t *testing.T) {
	s, _, _ := testWorkspace(t)
	page := &scriptTestPage{}
	queue := newScriptQueue(s.ctx, page)
	ctx, cancel := context.WithCancel(s.ctx)
	b := &browser{data: contracts.BrowserTab{ID: "test-browser"}, scripts: queue, annotation: &annotationSession{cancel: cancel, nonce: 17, page: queue}}
	s.browsers[b.data.ID] = b
	s.choose("browser", b.data.ID, "Browser")
	s.OpenSurface("history")
	if b.annotation != nil || ctx.Err() != context.Canceled {
		t.Fatal("hidden browser picker remained active")
	}
	if _, err := queue.EvalContext(context.Background(), "barrier"); err != nil {
		t.Fatal(err)
	}
	page.mu.Lock()
	defer page.mu.Unlock()
	if len(page.calls) != 2 || !strings.Contains(page.calls[0], "a.take(17)") || !strings.Contains(page.calls[0], "a.stop()") {
		t.Fatalf("picker stop script missing: %v", page.calls)
	}
}

func TestAnnotationResetIsAnOrderedBarrier(t *testing.T) {
	s, _, _ := testWorkspace(t)
	page := &scriptTestPage{entered: make(chan string, 1), release: make(chan struct{})}
	queue := newScriptQueue(s.ctx, page)
	b := &browser{data: contracts.BrowserTab{ID: "test-browser"}, scripts: queue}
	s.browsers[b.data.ID] = b
	if err := queue.post("block", ""); err != nil {
		t.Fatal(err)
	}
	select {
	case <-page.entered:
	case <-time.After(time.Second):
		t.Fatal("script worker did not start")
	}
	s.nextAnnotation = 8
	s.ResetAnnotations()
	_ = queue.post("label-new", "")
	s.KeepAnnotations([]uint32{1})
	close(page.release)
	if _, err := queue.EvalContext(context.Background(), "barrier"); err != nil {
		t.Fatal(err)
	}
	page.mu.Lock()
	defer page.mu.Unlock()
	want := []string{"block", "window.__rcodeAnnotator?.keep([])", "label-new", "window.__rcodeAnnotator?.keep([1])", "barrier"}
	if !reflect.DeepEqual(page.calls, want) || s.nextAnnotation != 0 {
		t.Fatalf("reset was replaced by a later keep: %v; index %d", page.calls, s.nextAnnotation)
	}
}
