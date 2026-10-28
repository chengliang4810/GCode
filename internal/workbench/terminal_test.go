package workbench

import (
	"io"
	"sync"
	"sync/atomic"
	"testing"

	"gcode/internal/contracts"
)

type countedCloser struct{ calls atomic.Int32 }

func (c *countedCloser) Close() error { c.calls.Add(1); return nil }

func TestTerminalResourceClosesAcrossLateDelivery(t *testing.T) {
	for _, name := range []string{"before delivery", "after delivery", "concurrent delivery"} {
		t.Run(name, func(t *testing.T) {
			var slot closeSlot
			resource := &countedCloser{}
			switch name {
			case "before delivery":
				slot.close()
				if slot.install(resource) {
					t.Fatal("closed owner accepted a late resource")
				}
			case "after delivery":
				if !slot.install(resource) {
					t.Fatal("live owner rejected a resource")
				}
			case "concurrent delivery":
				var jobs sync.WaitGroup
				jobs.Go(func() { slot.install(resource) })
				jobs.Go(slot.close)
				jobs.Wait()
			}
			slot.close()
			slot.close()
			if got := resource.calls.Load(); got != 1 {
				t.Fatalf("resource closed %d times; want once", got)
			}
		})
	}
}

func TestTerminalSnapshotRemovalClosesAndPreservesActiveTab(t *testing.T) {
	s, _, snapshot := testWorkspace(t)
	resource := &countedCloser{}
	v := &terminalView{meta: contracts.Terminal{ID: "removed"}, conn: newStreamConn(nil, nil)}
	v.resource.install(resource)
	s.terminals[v.meta.ID] = v
	s.choose("terminal", v.meta.ID, "Terminal")
	s.OpenSurface("history")
	snapshot.Terminals = nil
	s.Update(snapshot)
	if s.ActiveKind() != "history" || s.TabCount() != 1 || len(s.terminals) != 0 {
		t.Fatalf("stale terminal remains or active tab changed: %s, %d, %d", s.ActiveKind(), s.TabCount(), len(s.terminals))
	}
	if resource.calls.Load() != 1 {
		t.Fatal("removed terminal resource was not released")
	}
}

func TestTerminalRejectsInvalidResizeAndSuppressesDuplicates(t *testing.T) {
	calls := 0
	conn := newStreamConn(nil, func(cols, rows int) { calls++ })
	t.Cleanup(func() { _ = conn.Close() })
	if err := conn.Resize(0, 24); err == nil {
		t.Fatal("zero columns accepted")
	}
	if err := conn.Resize(80, 24); err != nil {
		t.Fatal(err)
	}
	if err := conn.Resize(80, 24); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("resize callback count = %d", calls)
	}
}

func TestTerminalAltAndControlSequencesPreserveFollowingText(t *testing.T) {
	var line string
	conn := newStreamConn(func(text string) { line = text }, nil)
	if _, err := conn.Write([]byte("\x1ba\x1b[A\x1b[200~bc\x1b[201~\x1b]0;ignored\x07d\r")); err != nil {
		t.Fatal(err)
	}
	if line != "abcd" {
		t.Fatalf("backend input = %q; want abcd", line)
	}
	_ = conn.Close()
	if _, err := conn.Write([]byte("e")); err != io.ErrClosedPipe {
		t.Fatalf("input after close = %v", err)
	}
}
