package contracts

import "context"

// EventResumeService replays recent event metadata and text deltas before an
// authoritative snapshot. Conversation bodies are rebuilt from that snapshot.
// An expired cursor starts with a snapshot instead.
type EventResumeService interface {
	SubscribeFrom(context.Context, uint64, ...string) (<-chan Event, error)
}
