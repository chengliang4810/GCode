package contracts

import "slices"

// SessionProjection selects conversation detail before cloning. An empty
// selection is a global summary; it never carries historical message bodies.
// The returned projection shares source data until CloneSnapshot is called.
func SessionProjection(source Snapshot, sessionIDs []string) Snapshot {
	out := source
	out.Messages = make(map[string][]Message, len(sessionIDs))
	for _, id := range sessionIDs {
		if messages, ok := source.Messages[id]; ok {
			out.Messages[id] = messages
		}
	}
	out.Sessions = slices.Clone(source.Sessions)
	for i := range out.Sessions {
		if !slices.Contains(sessionIDs, out.Sessions[i].ID) {
			out.Sessions[i].Goal = nil
		}
	}
	out.ModelRequests = nil
	for _, request := range source.ModelRequests {
		if slices.Contains(sessionIDs, request.SessionID) {
			out.ModelRequests = append(out.ModelRequests, request)
		}
	}
	out.Queue = nil
	for _, queued := range source.Queue {
		if slices.Contains(sessionIDs, queued.SessionID) {
			out.Queue = append(out.Queue, queued)
		}
	}
	out.Changes = nil
	for _, change := range source.Changes {
		if change.SessionID == "" || slices.Contains(sessionIDs, change.SessionID) {
			out.Changes = append(out.Changes, change)
		}
	}
	out.Terminals = nil
	for _, terminal := range source.Terminals {
		if terminal.SessionID == "" || slices.Contains(sessionIDs, terminal.SessionID) {
			out.Terminals = append(out.Terminals, terminal)
		}
	}
	return out
}
