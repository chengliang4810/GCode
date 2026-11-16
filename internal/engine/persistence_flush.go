package engine

import (
	"context"
	"log"
	"time"
)

// Flush independently of model/tool output: a quiet tool must not leave the
// preceding tool results pending until the next message arrives.
func (e *Engine) flushRunningConversations(ctx context.Context) {
	defer close(e.flushDone)
	ticker := time.NewTicker(messagePersistInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.mu.Lock()
			if !e.closed {
				var dirty []string
				for id := range e.runs {
					if _, pending := e.sessionDirty[id]; pending {
						dirty = append(dirty, id)
					}
				}
				if len(dirty) > 0 {
					if err := e.persistProgressLocked(dirty...); err != nil {
						log.Printf("flush running conversations: %v", err)
					}
				}
			}
			e.mu.Unlock()
		}
	}
}
