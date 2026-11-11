package mock

import (
	"cmp"
	"errors"
	"slices"

	"gcode/internal/contracts"
	"gcode/internal/reorder"
)

// reorderSessionLocked moves one conversation beside another inside its
// destination group and renumbers that group densely. The mock mirrors the
// engine contract; pinned and archived conversations stay out of plain rows.
func (s *Server) reorderSessionLocked(command contracts.Command) error {
	input := command.Reorder
	index := slices.IndexFunc(s.state.Sessions, func(session contracts.Session) bool { return session.ID == input.SourceID })
	if index < 0 {
		return errors.New("session not found")
	}
	source := s.state.Sessions[index]
	if !mockReorderableSession(source) {
		return errors.New("only active top-level conversations can be reordered")
	}
	if input.GroupID != "" && s.sessionGroupIndexLocked(input.GroupID) < 0 {
		return errors.New("unknown session group")
	}
	next := contracts.CloneSnapshot(s.state)
	members := make([]contracts.Session, 0, len(next.Sessions))
	for _, session := range next.Sessions {
		if session.GroupID == input.GroupID && mockReorderableSession(session) {
			members = append(members, session)
		}
	}
	slices.SortStableFunc(members, func(a, b contracts.Session) int {
		if a.GroupOrder != b.GroupOrder {
			return cmp.Compare(a.GroupOrder, b.GroupOrder)
		}
		if !a.UpdatedAt.Equal(b.UpdatedAt) {
			return b.UpdatedAt.Compare(a.UpdatedAt)
		}
		return cmp.Compare(a.ID, b.ID)
	})
	if input.TargetID != "" {
		targetFound, sourceFound := false, false
		for _, member := range members {
			targetFound = targetFound || member.ID == input.TargetID
			sourceFound = sourceFound || member.ID == input.SourceID
		}
		if !targetFound {
			return errors.New("reorder target is not in the destination group")
		}
		if !sourceFound {
			// A cross-group move brings the conversation into the destination
			// list first so the relative move has both endpoints.
			members = append(members, source)
		}
		moved, err := reorder.Move(members, input.SourceID, input.TargetID, input.After,
			func(session contracts.Session) string { return session.ID })
		if err != nil {
			return err
		}
		members = moved
	} else {
		members = slices.DeleteFunc(members, func(session contracts.Session) bool { return session.ID == input.SourceID })
		members = append(members, source)
	}
	for rank, member := range members {
		for i := range next.Sessions {
			if next.Sessions[i].ID == member.ID {
				next.Sessions[i].GroupID = input.GroupID
				next.Sessions[i].GroupOrder = rank
				break
			}
		}
	}
	s.state = next
	s.publishLocked("session.updated", input.SourceID, "", "", "")
	return s.persist()
}

func mockReorderableSession(session contracts.Session) bool {
	return session.ID != "" && !session.Pinned && !session.Archived &&
		session.ParentSessionID == "" && !session.SideChat
}

func (s *Server) reorderProjectSessionLocked(input *contracts.ReorderInput) error {
	index := s.sessionIndexLocked(input.SourceID)
	if index < 0 {
		return errors.New("session not found")
	}
	source := s.state.Sessions[index]
	if source.ProjectID == "" || !mockReorderableSession(source) {
		return errors.New("only active top-level project conversations can be reordered")
	}
	var members []contracts.Session
	for _, session := range s.state.Sessions {
		if session.ProjectID == source.ProjectID && mockReorderableSession(session) {
			members = append(members, session)
		}
	}
	slices.SortStableFunc(members, contracts.CompareProjectSessions)
	members, err := reorder.Move(members, input.SourceID, input.TargetID, input.After,
		func(session contracts.Session) string { return session.ID })
	if err != nil {
		return err
	}
	for rank, member := range members {
		s.state.Sessions[s.sessionIndexLocked(member.ID)].ProjectOrder = rank + 1
	}
	s.publishLocked("session.updated", input.SourceID, "", "", "")
	return s.persist()
}
