package engine

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"gcode/internal/agent"
	"gcode/internal/contracts"
	"gcode/internal/terminalhost"
)

func TestStreamTextRetainsSnapshotsAndResetsAfterRetry(t *testing.T) {
	core := &Engine{
		state: contracts.Snapshot{Messages: map[string][]contracts.Message{}},
		runs:  map[string]*activeRun{"session": {id: "run"}},
	}
	var current string
	apply := func(event agent.Event) {
		t.Helper()
		if err := core.applyAgentEventLocked("session", "run", &current, event); err != nil {
			t.Fatal(err)
		}
	}
	apply(agent.Event{Kind: "message.start"})
	apply(agent.Event{Kind: "message.delta", Text: "first"})
	first := core.snapshotLocked()
	apply(agent.Event{Kind: "message.delta", Text: " second", Reasoning: "thinking"})
	second := core.snapshotLocked()
	apply(agent.Event{Kind: "message.retry"})
	apply(agent.Event{Kind: "message.delta", Text: "retry"})
	if first.Messages["session"][0].Blocks[0].Text != "first" ||
		second.Messages["session"][0].Blocks[0].Text != "first second" ||
		core.state.Messages["session"][0].Blocks[0].Text != "retry" {
		t.Fatal("stream growth or retry changed published snapshots")
	}
}

func TestPublishedSnapshotsRetainIndependentSubscriberOwnership(t *testing.T) {
	a, b := make(chan contracts.Event, 1), make(chan contracts.Event, 1)
	core := &Engine{
		state: contracts.Snapshot{
			Sessions: []contracts.Session{{ID: "session", Title: "original"}},
			Messages: map[string][]contracts.Message{"session": {{ID: "message", Blocks: []contracts.Block{{Kind: "text", Text: "original"}}}}},
		},
		subs: map[uint64]chan contracts.Event{1: a, 2: b},
	}
	core.publishLocked(contracts.Event{Kind: "state.changed"})
	first, second := <-a, <-b
	first.Snapshot.Sessions[0].Title = "changed"
	first.Snapshot.Messages["session"][0].Blocks[0].Text = "changed"
	if second.Snapshot.Sessions[0].Title != "original" ||
		second.Snapshot.Messages["session"][0].Blocks[0].Text != "original" ||
		core.state.Sessions[0].Title != "original" ||
		core.state.Messages["session"][0].Blocks[0].Text != "original" {
		t.Fatal("subscriber modified another subscriber or the engine")
	}
}

func TestUnchangedIndexAvoidsReplacementButRepairsMissingFile(t *testing.T) {
	core := openStorageTestCore(t, t.TempDir())
	t.Cleanup(func() { _ = core.Close() })
	if err := core.persistLocked(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(core.storage.ConfigDirectory(), "engine.json")
	anchor := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(path, anchor, anchor); err != nil {
		t.Fatal(err)
	}
	if err := core.persistLocked(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || !info.ModTime().Equal(anchor) {
		t.Fatalf("unchanged index was replaced: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := core.persistLocked(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("missing index was not repaired:", err)
	}
}

func TestWaitingTerminalCommandDoesNotBlockSnapshots(t *testing.T) {
	op := &sync.Mutex{}
	op.Lock()
	core := &Engine{
		terminals:   map[string]*terminalhost.Session{"terminal": {}},
		terminalOps: map[string]*sync.Mutex{"terminal": op},
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- core.Execute(ctx, contracts.Command{Kind: "terminal.input", ID: "terminal", Text: "input"})
	}()
	defer func() {
		cancel()
		op.Unlock()
		if err := <-done; err == nil {
			t.Error("canceled terminal operation was executed")
		}
	}()
	for range 20 {
		snapshot := make(chan error, 1)
		go func() { _, err := core.Snapshot(t.Context()); snapshot <- err }()
		select {
		case err := <-snapshot:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("waiting on a terminal holds the engine mutex")
		}
	}
}
