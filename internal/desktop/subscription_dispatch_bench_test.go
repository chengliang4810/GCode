package desktop

import (
	"context"
	"strings"
	"testing"
	"time"

	"gcode/internal/contracts"
)

// A stalled UI and a burst of owned snapshots reproduce the asynchronous
// dispatch boundary without touching an installed app or a user's data.
func BenchmarkSubscriptionDispatchStalledUI(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		ctx, cancel := context.WithCancel(context.Background())
		events := make(chan contracts.Event)
		posted := make(chan func(), 512)
		produced := make(chan struct{})
		go func() {
			defer close(produced)
			defer close(events)
			for i := range 512 {
				snapshot := contracts.Snapshot{Messages: map[string][]contracts.Message{"session": {{ID: "message", Blocks: []contracts.Block{{Kind: "text", Text: strings.Repeat("x", 64<<10)}}}}}}
				select {
				case events <- contracts.Event{Sequence: uint64(i + 1), Snapshot: &snapshot}:
				case <-ctx.Done():
					return
				}
			}
		}()
		done := make(chan struct{})
		go func() {
			defer close(done)
			dispatchSubscriptionEvents(ctx, events, func(fn func()) { posted <- fn }, func(contracts.Event) {})
		}()
		// No UI callback runs during the burst.
		time.Sleep(30 * time.Millisecond)
		cancel()
		<-done
		<-produced
		b.ReportMetric(float64(len(posted)), "pending-ui-callbacks")
		// Keep captured snapshots reachable through measurement.
		for len(posted) > 0 {
			fn := <-posted
			fn()
		}
	}
}
