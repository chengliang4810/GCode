package engine

import (
	"testing"

	"gcode/internal/contracts"
)

func TestEventResumeReplaysGapThenOwnedSnapshot(t *testing.T) {
	core := openStorageTestCore(t, t.TempDir())
	defer core.Close()
	session := newTestSession(t, core, "", "resume")
	core.mu.Lock()
	core.publishLocked(contracts.Event{Kind: "run.started", SessionID: session.ID})
	cursor := core.sequence
	core.publishLocked(contracts.Event{Kind: "message.delta", SessionID: session.ID, Text: "missed text"})
	core.publishLocked(contracts.Event{Kind: "run.finished", SessionID: session.ID})
	core.mu.Unlock()
	stream, err := core.SubscribeFrom(t.Context(), cursor, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	first, second, snapshot := <-stream, <-stream, <-stream
	if first.Text != "missed text" || first.Sequence != cursor+1 || second.Kind != "run.finished" || snapshot.Snapshot == nil {
		t.Fatalf("resume order: %+v %+v %+v", first, second, snapshot)
	}
	snapshot.Snapshot.Sessions[0].Title = "consumer mutation"
	core.mu.Lock()
	defer core.mu.Unlock()
	if core.state.Sessions[core.sessionIndex(session.ID)].Title == "consumer mutation" {
		t.Fatal("resumed snapshot shares mutable state")
	}
}

func TestEventResumeExpiredCursorRebuildsAndJournalIsBounded(t *testing.T) {
	core := openStorageTestCore(t, t.TempDir())
	defer core.Close()
	core.mu.Lock()
	core.publishLocked(contracts.Event{Kind: "first"})
	cursor := core.sequence
	for range 130 {
		core.publishLocked(contracts.Event{Kind: "later"})
	}
	if len(core.recentEvents) != 128 {
		t.Fatal("event journal is not bounded")
	}
	core.mu.Unlock()
	stream, err := core.SubscribeFrom(t.Context(), cursor)
	if err != nil {
		t.Fatal(err)
	}
	if first := <-stream; first.Kind != "snapshot" || first.Snapshot == nil {
		t.Fatalf("expired cursor did not rebuild: %+v", first)
	}
}

func TestEventResumeAcceptsZeroCursor(t *testing.T) {
	core := &Engine{
		state: contracts.Snapshot{}, subs: make(map[uint64]chan contracts.Event),
		done: make(chan struct{}),
	}
	core.mu.Lock()
	core.publishLocked(contracts.Event{Kind: "first"})
	core.mu.Unlock()
	stream, err := core.SubscribeFrom(t.Context(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if first := <-stream; first.Kind != "first" || first.Sequence != 1 {
		t.Fatalf("zero cursor skipped its first event: %+v", first)
	}
}
