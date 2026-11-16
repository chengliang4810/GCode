package engine

import (
	"context"
	"gcode/internal/contracts"
	"strings"
	"testing"
)

func BenchmarkSnapshotDuringPersistenceIO(b *testing.B) {
	core := benchmarkSessionPersistenceCore(b, 1)
	core.mu.Lock()
	core.state.Messages[core.state.Sessions[0].ID] = []contracts.Message{{
		ID: "large-message", Blocks: []contracts.Block{{Kind: "text", Text: strings.Repeat("x", 512<<10)}},
	}}
	started, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	core.mu.Unlock()
	go func() {
		core.mu.Lock()
		defer core.mu.Unlock()
		done <- core.persistWithWriterLocked([]string{core.state.Sessions[0].ID}, true, func(writer *Engine, required ...string) error {
			close(started)
			<-release
			return writer.commitPersistence(required...)
		})
	}()
	<-started
	b.Cleanup(func() {
		close(release)
		if err := <-done; err != nil {
			b.Fatal(err)
		}
	})
	b.ReportAllocs()
	for b.Loop() {
		if _, err := core.Snapshot(context.Background(), core.state.Sessions[0].ID); err != nil {
			b.Fatal(err)
		}
	}
}
