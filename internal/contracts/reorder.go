package contracts

import (
	"errors"
	"strings"
)

// ReorderInput moves one existing item immediately before or after another.
// Model IDs are local to ProviderID; provider and group reorders leave
// ProviderID empty. GroupID is the destination session group of a session
// reorder, empty for the ungrouped root.
type ReorderInput struct {
	ProviderID string `json:"providerId,omitempty"`
	GroupID    string `json:"groupId,omitempty"`
	SourceID   string `json:"sourceId"`
	TargetID   string `json:"targetId"`
	After      bool   `json:"after"`
}

// IsReorderCommand reports whether kind carries only a ReorderInput payload.
func IsReorderCommand(kind string) bool {
	switch kind {
	case "provider.reorder", "provider.models.reorder", "group.reorder", "session.reorder", "queue.move":
		return true
	}
	return false
}

func (r ReorderInput) Validate(kind string) error {
	if strings.TrimSpace(r.SourceID) == "" {
		return errors.New("source ID is required for reordering")
	}
	if strings.TrimSpace(r.TargetID) == "" && kind != "session.reorder" {
		return errors.New("source and target IDs are required for reordering")
	}
	switch kind {
	case "provider.models.reorder":
		if strings.TrimSpace(r.ProviderID) == "" {
			return errors.New("provider ID is required for model reordering")
		}
	case "session.reorder":
		if r.ProviderID != "" {
			return errors.New("session reordering does not accept a provider scope")
		}
	default:
		if r.ProviderID != "" {
			return errors.New("provider reordering does not accept a provider scope")
		}
	}
	return nil
}
