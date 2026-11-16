package engine

import "sync"

// stateMutex serializes mutations through transaction while view protects the
// live state. Persistence releases view while retaining transaction. Progress
// saves admit live reads; transactional saves retain a read barrier until the
// caller settles. No mutation can invalidate a caller's save-failure rollback.
// Always acquire transaction before view for mutations.
type stateMutex struct {
	transaction sync.Mutex
	view        sync.Mutex
}

func (m *stateMutex) Lock() {
	m.transaction.Lock()
	m.view.Lock()
}

func (m *stateMutex) Unlock() {
	m.view.Unlock()
	m.transaction.Unlock()
}

func (m *stateMutex) TryLock() bool {
	if !m.transaction.TryLock() {
		return false
	}
	if !m.view.TryLock() {
		m.transaction.Unlock()
		return false
	}
	return true
}
