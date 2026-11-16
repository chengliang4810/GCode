package engine

import (
	"maps"
	"slices"
)

// persistLocked requires mu on entry and returns with it still held. The
// transaction gate remains held throughout preparation, I/O and installation,
// preserving commit order and the caller's rollback boundary. Transactional
// snapshots wait for the caller to finish, including post-save settlement.
func (e *Engine) persistLocked(required ...string) error {
	return e.persistWithWriterLocked(required, false, (*Engine).commitPersistence)
}

// persistProgressLocked saves nonterminal execution progress. Unlike a command
// or final settlement, this state is already safe to display before durability.
func (e *Engine) persistProgressLocked(required ...string) error {
	return e.persistWithWriterLocked(required, true, (*Engine).commitPersistence)
}

// The writer argument is the persistence I/O boundary, allowing tests to hold
// or fail a commit without sleeping or modifying the user's filesystem.
func (e *Engine) persistWithWriterLocked(required []string, readLive bool, commit func(*Engine, ...string) error) error {
	e.markSessionDirtyLocked(required...)
	e.refreshStatisticsLocked()

	// Record bodies, settings and providers stay immutable behind transaction.
	// The writer owns the only slices/maps it changes, so encoding does not
	// need a second deep copy of messages, images or execution history.
	state := e.state
	state.Sessions = slices.Clone(state.Sessions)
	state.Terminals = slices.Clone(state.Terminals)
	writer := &Engine{
		storage: e.storage, state: state, providers: e.providers,
		history: e.history, context: e.context, contextMemories: e.contextMemories,
		prompts: e.prompts, loadedSessions: e.loadedSessions,
		sessionRecords: e.sessionRecords, sessionHashes: e.sessionHashes,
		sessionDirty: maps.Clone(e.sessionDirty),
		indexHash:    e.indexHash, indexHashKnown: e.indexHashKnown,
	}

	err := e.withStateUnlocked(readLive, func() error { return commit(writer, required...) })
	e.persistErrors = writer.persistErrors
	for i := range e.state.Sessions {
		e.state.Sessions[i].StorageError = writer.state.Sessions[i].StorageError
	}
	// Terminal recovery/error state is useful even if the final index failed;
	// output references only change after the writer commits the index.
	e.state.Terminals = writer.state.Terminals
	if err == nil {
		e.sessionRecords, e.sessionHashes = writer.sessionRecords, writer.sessionHashes
		e.sessionDirty = writer.sessionDirty
		e.indexHash, e.indexHashKnown = writer.indexHash, writer.indexHashKnown
	}
	return err
}

// withStateUnlocked keeps the transaction gate while running I/O. Live reads
// are allowed only when no uncommitted state has been installed by the caller.
func (e *Engine) withStateUnlocked(readLive bool, commit func() error) error {
	e.persistLiveRead = readLive
	e.mu.view.Unlock()
	defer func() {
		e.mu.view.Lock()
		e.persistLiveRead = false
	}()
	return commit()
}
