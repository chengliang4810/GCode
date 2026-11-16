package engine

import (
	"testing"
	"time"

	"gcode/internal/model"
)

func TestRunningConversationFlushesWithoutAnotherMessage(t *testing.T) {
	core := openStorageTestCore(t, t.TempDir())
	defer core.Close()
	core.mu.Lock()
	session := newTestSession(t, core, "", "quiet tool")
	if err := core.persistLocked(session.ID); err != nil {
		core.mu.Unlock()
		t.Fatal(err)
	}
	before := core.sessionRecords[session.ID]
	core.runs[session.ID] = &activeRun{cancel: func() {}}
	core.history[session.ID] = append(core.history[session.ID], model.Message{Role: "user", Text: "pending while tool waits"})
	core.markSessionDirtyLocked(session.ID)
	core.mu.Unlock()
	defer func() {
		core.mu.Lock()
		delete(core.runs, session.ID)
		core.mu.Unlock()
	}()
	deadline := time.NewTimer(6 * time.Second)
	defer deadline.Stop()
	poll := time.NewTicker(20 * time.Millisecond)
	defer poll.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatal("quiet running conversation was not persisted")
		case <-poll.C:
			core.mu.Lock()
			record := core.sessionRecords[session.ID]
			saved, err := core.readSessionDisk(session, record)
			core.mu.Unlock()
			if record != before && err == nil && len(saved.History) == 1 && saved.History[0].Text == "pending while tool waits" {
				return
			}
		}
	}
}
