package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/storage"
)

func benchmarkSessionPersistenceCore(b *testing.B, count int) *Engine {
	b.Helper()
	home := b.TempDir()
	store, err := storage.Open(storage.Options{Home: home})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = store.Close() })
	if err := os.MkdirAll(filepath.Join(store.Directory(), "sessions"), 0700); err != nil {
		b.Fatal(err)
	}
	core := &Engine{
		storage:        store,
		sessionHashes:  make(map[string][32]byte),
		sessionRecords: make(map[string]string),
		loadedSessions: make(map[string]bool),
		history:        make(map[string][]model.Message),
		context:        make(map[string][]model.Message),
		state: contracts.Snapshot{
			SchemaVersion: contracts.SchemaVersion,
			Revision:      1,
			Messages:      make(map[string][]contracts.Message),
			ContextUsage:  make(map[string]contracts.ContextUsage),
		},
	}
	ids := make([]string, 0, count)
	fixtureText := strings.Repeat("x", 32<<10)
	for i := range count {
		id := fmt.Sprintf("%012d", i)
		if err := os.Mkdir(filepath.Join(store.Directory(), "sessions", id), 0700); err != nil {
			b.Fatal(err)
		}
		core.state.Sessions = append(core.state.Sessions, contracts.Session{
			ID: id, StorageRootID: store.ActiveRootID(), Title: "Conversation", DeviceID: "local",
		})
		core.state.Messages[id] = []contracts.Message{{
			ID: fmt.Sprintf("message-%012d", i), SessionID: id, Role: "user",
			Blocks: []contracts.Block{{ID: fmt.Sprintf("block-%012d", i), Kind: "text", Text: fixtureText}},
		}}
		core.history[id] = []model.Message{{Role: "user", Text: fixtureText}}
		core.context[id] = []model.Message{{Role: "user", Text: fixtureText}}
		core.loadedSessions[id] = true
		ids = append(ids, id)
	}
	if err := core.persistLocked(ids...); err != nil {
		b.Fatal(err)
	}
	return core
}

func BenchmarkPersistDirtySessionAmong100(b *testing.B) {
	core := benchmarkSessionPersistenceCore(b, 100)
	b.ReportAllocs()
	iteration := 0
	for b.Loop() {
		iteration++
		id := core.state.Sessions[0].ID
		core.state.Sessions[0].Title = fmt.Sprintf("Conversation %d", iteration)
		if err := core.persistLocked(id); err != nil {
			b.Fatal(err)
		}
	}
}
