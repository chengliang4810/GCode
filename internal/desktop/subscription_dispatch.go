package desktop

import (
	"context"

	"gcode/internal/contracts"
)

// dispatchSubscriptionEvents keeps at most one callback in the native UI queue.
// Without acknowledgement, an asynchronous post drains the bounded core stream
// into an unbounded main-thread queue, retaining every captured snapshot.
func dispatchSubscriptionEvents(ctx context.Context, events <-chan contracts.Event, post func(func()), apply func(contracts.Event)) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			applied := make(chan struct{})
			post(func() {
				defer close(applied)
				if ctx.Err() == nil {
					apply(event)
				}
			})
			select {
			case <-ctx.Done():
				return
			case <-applied:
			}
		}
	}
}
