package desktop

import (
	"context"
	"testing"
	"time"

	"gcode/internal/contracts"
)

func TestSubscriptionDispatchBoundsPendingUIWork(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	events := make(chan contracts.Event, 100)
	for i := range 100 {
		events <- contracts.Event{Sequence: uint64(i + 1)}
	}
	close(events)
	posted := make(chan func(), 100)
	done := make(chan struct{})
	var sequences []uint64
	go func() {
		defer close(done)
		dispatchSubscriptionEvents(ctx, events, func(fn func()) { posted <- fn }, func(e contracts.Event) { sequences = append(sequences, e.Sequence) })
	}()
	select {
	case fn := <-posted:
		// A stalled UI must leave the remaining events in the bounded core channel.
		select {
		case <-posted:
			t.Fatal("subscription queued another callback before the UI consumed the first")
		case <-time.After(20 * time.Millisecond):
		}
		fn()
	case <-time.After(time.Second):
		t.Fatal("no callback posted")
	}
	for range 99 {
		select {
		case fn := <-posted:
			fn()
		case <-time.After(time.Second):
			t.Fatal("delivery stalled")
		}
	}
	<-done
	for i, seq := range sequences {
		if seq != uint64(i+1) {
			t.Fatal("events reordered")
		}
	}
}

func TestSubscriptionDispatchCancellationReleasesWorker(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	events := make(chan contracts.Event, 2)
	events <- contracts.Event{}
	events <- contracts.Event{}
	posted := make(chan func(), 2)
	done := make(chan struct{})
	applied := false
	go func() {
		defer close(done)
		dispatchSubscriptionEvents(ctx, events, func(fn func()) { posted <- fn }, func(contracts.Event) { applied = true })
	}()
	fn := <-posted
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker waited for UI after cancellation")
	}
	fn()
	if applied {
		t.Fatal("canceled event was applied")
	}
}
