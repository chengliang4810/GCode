package engine

import (
	"errors"
	"slices"
	"sort"

	"gcode/internal/contracts"
	"gcode/internal/reorder"
)

// reorderSessionLocked moves one conversation beside another inside the
// destination session group and renumbers that group densely. Reordering
// never changes which conversations are pinned or archived: both own their
// own sidebar sections.
func (e *Engine) reorderSessionLocked(command contracts.Command) error {
	input := command.Reorder
	index := e.sessionIndex(input.SourceID)
	if index < 0 {
		return errors.New("session not found")
	}
	source := e.state.Sessions[index]
	if !reorderableSession(source) {
		return errors.New("only active top-level conversations can be reordered")
	}
	if input.GroupID != "" && e.sessionGroupIndex(input.GroupID) < 0 {
		return errors.New("session group not found")
	}
	members := e.groupMembersLocked(input.GroupID)
	var err error
	if input.TargetID != "" {
		targetFound, sourceFound := false, false
		for _, session := range members {
			targetFound = targetFound || session.ID == input.TargetID
			sourceFound = sourceFound || session.ID == source.ID
		}
		if !targetFound {
			return errors.New("reorder target is not in the destination group")
		}
		if !sourceFound {
			// A cross-group move joins the destination list before the
			// relative move, so both endpoints exist in it.
			members = append(members, source)
		}
		members, err = reorder.Move(members, input.SourceID, input.TargetID, input.After,
			func(session contracts.Session) string { return session.ID })
		if err != nil {
			return err
		}
	} else {
		// An empty target appends the conversation to the end of the group.
		members = slices.DeleteFunc(members, func(session contracts.Session) bool { return session.ID == source.ID })
		members = append(members, source)
	}
	for rank, session := range members {
		member := &e.state.Sessions[e.sessionIndex(session.ID)]
		member.GroupID = input.GroupID
		member.GroupOrder = rank
	}
	return nil
}

// reorderableSession reports whether a conversation can take part in manual
// session ordering: only active top-level conversations own a group position.
func reorderableSession(session contracts.Session) bool {
	return session.ID != "" && !session.Pinned && !session.Archived &&
		session.ParentSessionID == "" && !session.SideChat
}

// groupMembersLocked lists the reorderable conversations of one group in
// display order, or of the ungrouped root when groupID is empty.
func (e *Engine) groupMembersLocked(groupID string) []contracts.Session {
	var members []contracts.Session
	for _, session := range e.state.Sessions {
		if session.GroupID == groupID && reorderableSession(session) {
			members = append(members, session)
		}
	}
	sort.SliceStable(members, func(i, j int) bool {
		if members[i].GroupOrder != members[j].GroupOrder {
			return members[i].GroupOrder < members[j].GroupOrder
		}
		if !members[i].UpdatedAt.Equal(members[j].UpdatedAt) {
			return members[i].UpdatedAt.After(members[j].UpdatedAt)
		}
		return members[i].ID < members[j].ID
	})
	return members
}
