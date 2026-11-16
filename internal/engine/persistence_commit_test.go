package engine

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
)

func TestPersistenceIOAllowsSnapshotsAndSerializesMutations(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit", true: "rollback"}[fail], func(t *testing.T) {
			core := openStorageTestCore(t, t.TempDir())
			defer core.Close()
			core.mu.Lock()
			session := newTestSession(t, core, "", "original")
			if err := core.persistLocked(session.ID); err != nil {
				core.mu.Unlock()
				t.Fatal(err)
			}
			core.mu.Unlock()
			started, release := make(chan struct{}), make(chan struct{})
			done := make(chan error, 1)
			closed := false
			defer func() {
				if !closed {
					close(release)
				}
			}()
			injected := errors.New("injected index failure")
			go func() {
				core.mu.Lock()
				defer core.mu.Unlock()
				prior := core.state.Sessions[0].Title
				core.state.Sessions[0].Title = "pending"
				err := core.persistWithWriterLocked([]string{session.ID}, true, func(writer *Engine, required ...string) error {
					close(started)
					<-release
					if fail {
						return injected
					}
					return writer.commitPersistence(required...)
				})
				if err != nil {
					core.state.Sessions[0].Title = prior
				}
				done <- err
			}()
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("commit did not start")
			}
			// The real public snapshot path, including a loaded conversation's
			// body, must finish while the writer remains deliberately blocked.
			snapshotDone := make(chan error, 1)
			go func() {
				_, err := core.Snapshot(t.Context(), session.ID)
				snapshotDone <- err
			}()
			select {
			case err := <-snapshotDone:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("snapshot waited for persistence I/O")
			}
			if core.mu.TryLock() {
				core.mu.Unlock()
				t.Fatal("another mutation entered an in-flight transaction")
			}
			second := make(chan error, 1)
			go func() {
				core.mu.Lock()
				defer core.mu.Unlock()
				core.state.Sessions[0].Title += " then second"
				second <- core.persistLocked(session.ID)
			}()
			close(release)
			closed = true
			if err := <-done; (fail && !errors.Is(err, injected)) || (!fail && err != nil) {
				t.Fatalf("first commit: %v", err)
			}
			if err := <-second; err != nil {
				t.Fatal(err)
			}
			core.mu.Lock()
			saved, err := core.readSessionDisk(session, core.sessionRecords[session.ID])
			core.mu.Unlock()
			want := "pending then second"
			if fail {
				want = "original then second"
			}
			if err != nil || saved.Session.Title != want {
				t.Fatalf("commit order / rollback lost a mutation: title=%q err=%v", saved.Session.Title, err)
			}
		})
	}
}

func TestPersistenceIndexFailurePreservesCommittedRecordOutsideLock(t *testing.T) {
	core := openStorageTestCore(t, t.TempDir())
	defer core.Close()
	core.mu.Lock()
	session := newTestSession(t, core, "", "record")
	if err := core.persistLocked(session.ID); err != nil {
		core.mu.Unlock()
		t.Fatal(err)
	}
	old := core.sessionRecords[session.ID]
	core.history[session.ID] = []model.Message{{Role: "user", Text: "new unsaved body"}}
	index := filepath.Join(core.storage.ConfigDirectory(), "engine.json")
	if err := os.Remove(index); err != nil {
		core.mu.Unlock()
		t.Fatal(err)
	}
	if err := os.Mkdir(index, 0700); err != nil {
		core.mu.Unlock()
		t.Fatal(err)
	}
	err := core.persistLocked(session.ID)
	record := core.sessionRecords[session.ID]
	_, dirty := core.sessionDirty[session.ID]
	saved, readErr := core.readSessionDisk(session, record)
	core.mu.Unlock()
	if err == nil || readErr != nil || record != old || !dirty || len(saved.History) != 0 {
		t.Fatalf("failed index changed committed history: err=%v read=%v record=%s dirty=%v", err, readErr, record, dirty)
	}
	if err := os.Remove(index); err != nil {
		t.Fatal(err)
	}
	core.mu.Lock()
	err = core.persistLocked(session.ID)
	core.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
}

func TestPersistenceCloseWaitsForAcceptedCommit(t *testing.T) {
	home := t.TempDir()
	core := openStorageTestCore(t, home)
	core.mu.Lock()
	session := newTestSession(t, core, "", "close during save")
	core.mu.Unlock()
	started, release := make(chan struct{}), make(chan struct{})
	commitDone := make(chan error, 1)
	go func() {
		core.mu.Lock()
		defer core.mu.Unlock()
		commitDone <- core.persistWithWriterLocked([]string{session.ID}, false, func(writer *Engine, required ...string) error {
			close(started)
			<-release
			return writer.commitPersistence(required...)
		})
	}()
	<-started
	closeDone := make(chan error, 1)
	go func() { closeDone <- core.Close() }()
	close(release)
	if err := <-commitDone; err != nil {
		t.Fatal(err)
	}
	if err := <-closeDone; err != nil {
		t.Fatal(err)
	}
	core = openStorageTestCore(t, home)
	defer core.Close()
	snapshot, err := core.Snapshot(t.Context(), session.ID)
	if err != nil || len(snapshot.Sessions) != 1 || snapshot.Sessions[0].Title != session.Title {
		t.Fatalf("shutdown lost the accepted save: %+v %v", snapshot.Sessions, err)
	}
}

func TestPersistenceTransactionSnapshotWaitsForRollback(t *testing.T) {
	core := openStorageTestCore(t, t.TempDir())
	defer core.Close()
	core.mu.Lock()
	session := newTestSession(t, core, "", "committed")
	if err := core.persistLocked(session.ID); err != nil {
		core.mu.Unlock()
		t.Fatal(err)
	}
	core.mu.Unlock()
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		core.mu.Lock()
		defer core.mu.Unlock()
		defer close(done)
		core.state.Sessions[0].Title = "must not escape"
		_ = core.persistWithWriterLocked([]string{session.ID}, false, func(*Engine, ...string) error {
			close(started)
			<-release
			return errors.New("injected failure")
		})
		core.state.Sessions[0].Title = "committed"
	}()
	<-started
	observed := make(chan contracts.Snapshot, 1)
	go func() {
		snapshot, _ := core.Snapshot(t.Context(), session.ID)
		observed <- snapshot
	}()
	select {
	case <-observed:
		close(release)
		<-done
		t.Fatal("transaction snapshot escaped before rollback")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	<-done
	if snapshot := <-observed; len(snapshot.Sessions) != 1 || snapshot.Sessions[0].Title != "committed" {
		t.Fatalf("snapshot observed the failed transaction: %+v", snapshot.Sessions)
	}
}
