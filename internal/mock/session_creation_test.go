package mock

import (
	"context"
	"path/filepath"
	"testing"

	"gcode/internal/contracts"
)

func TestBranchCreationRequestPersistsAndRetriesWithoutDuplicates(t *testing.T) {
	ctx := context.Background()
	options := Options{StatePath: filepath.Join(t.TempDir(), "state.json"), Instant: true}
	s, err := New(options)
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	parentIndex := s.sessionIndexLocked("chat-0")
	s.state.Sessions[parentIndex].Goal = &contracts.SessionGoal{ID: "parent-goal", Status: contracts.GoalPaused}
	s.state.Sessions[parentIndex].SubagentType = "explore"
	s.state.Sessions[parentIndex].RunningSubagents = 2
	s.mu.Unlock()
	command := contracts.Command{Kind: "session.sidechat", SessionID: "chat-0", RequestID: "side-creation-1", Name: "侧边聊天"}
	if err := s.Execute(ctx, command); err != nil {
		t.Fatal(err)
	}
	before, _ := s.Snapshot(ctx)
	child := before.Sessions[0]
	if child.CreationRequestID != command.RequestID || child.ParentSessionID != command.SessionID {
		t.Fatal("creation result lost request identity")
	}
	if child.Goal != nil || child.SubagentType != "" || child.RunningSubagents != 0 {
		t.Fatal("side chat inherited authoritative execution state")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = New(options)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Execute(ctx, command); err != nil {
		t.Fatal(err)
	}
	after, _ := s.Snapshot(ctx)
	if len(after.Sessions) != len(before.Sessions) {
		t.Fatal("repeating the creation request duplicated a session")
	}
	for _, session := range after.Sessions {
		if session.CreationRequestID == command.RequestID && session.ID != child.ID {
			t.Fatal("retry changed created session identity")
		}
	}
	conflict := command
	conflict.Kind = "session.fork"
	if err := s.Execute(ctx, conflict); err == nil {
		t.Fatal("same request ID was accepted for a different operation")
	}
	if err := s.Execute(ctx, contracts.Command{Kind: "session.fork", SessionID: child.ID}); err != nil {
		t.Fatal(err)
	}
	legacy, _ := s.Snapshot(ctx)
	if legacy.Sessions[0].CreationRequestID != "" {
		t.Fatal("a subsequent fork inherited its parent's creation request")
	}
}
