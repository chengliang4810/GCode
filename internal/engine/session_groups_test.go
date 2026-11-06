package engine

import (
	"context"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/storage"
)

func TestSessionGroupCommandsCreateMoveAndPersist(t *testing.T) {
	home := t.TempDir()
	store, err := storage.Open(storage.Options{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	core, err := openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err = core.Execute(ctx, contracts.Command{Kind: "session.create", Name: "Grouped"}); err != nil {
		t.Fatal(err)
	}
	if err = core.Execute(ctx, contracts.Command{Kind: "group.create", Text: "Research"}); err != nil {
		t.Fatal(err)
	}
	if err = core.Execute(ctx, contracts.Command{Kind: "group.create", Text: "  "}); err == nil {
		t.Fatal("empty group title must be rejected")
	}
	snapshot, _ := core.Snapshot(ctx)
	if len(snapshot.SessionGroups) != 1 || snapshot.SessionGroups[0].Title != "Research" {
		t.Fatalf("group create failed: %+v", snapshot.SessionGroups)
	}
	group := snapshot.SessionGroups[0].ID
	session := snapshot.Sessions[0].ID
	if err = core.Execute(ctx, contracts.Command{Kind: "session.update", Session: &contracts.SessionUpdate{ID: session, GroupID: &group}}); err != nil {
		t.Fatal(err)
	}
	missing := "group-missing"
	if err = core.Execute(ctx, contracts.Command{Kind: "session.update", Session: &contracts.SessionUpdate{ID: session, GroupID: &missing}}); err == nil {
		t.Fatal("unknown group membership must be rejected")
	}
	if err = core.Execute(ctx, contracts.Command{Kind: "group.rename", ID: group, Text: "Renamed"}); err != nil {
		t.Fatal(err)
	}
	if err = core.Execute(ctx, contracts.Command{Kind: "group.rename", ID: "group-missing", Text: "X"}); err == nil {
		t.Fatal("renaming a missing group must fail")
	}
	if err = core.Close(); err != nil {
		t.Fatal(err)
	}
	// Groups and membership survive a restart through the engine index.
	store, err = storage.Open(storage.Options{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	core, err = openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	snapshot, _ = core.Snapshot(ctx)
	if len(snapshot.SessionGroups) != 1 || snapshot.SessionGroups[0].Title != "Renamed" {
		t.Fatalf("group rename did not persist: %+v", snapshot.SessionGroups)
	}
	if snapshot.Sessions[0].GroupID != group {
		t.Fatal("group membership did not persist")
	}
	if err = core.Execute(ctx, contracts.Command{Kind: "group.delete", ID: group}); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = core.Snapshot(ctx)
	if len(snapshot.SessionGroups) != 0 || snapshot.Sessions[0].GroupID != "" {
		t.Fatalf("group delete must unassign members: %+v", snapshot.SessionGroups)
	}
}

func TestGroupAndSessionReorderPersistDenseOrder(t *testing.T) {
	home := t.TempDir()
	store, err := storage.Open(storage.Options{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	core, err := openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	ctx := context.Background()
	for _, title := range []string{"First", "Second", "Third"} {
		if err = core.Execute(ctx, contracts.Command{Kind: "group.create", Text: title}); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"A", "B", "C"} {
		if err = core.Execute(ctx, contracts.Command{Kind: "session.create", Name: name}); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, _ := core.Snapshot(ctx)
	groups := snapshot.SessionGroups
	ids := []string{groups[0].ID, groups[1].ID, groups[2].ID}
	// Move the last group before the first one.
	if err = core.Execute(ctx, contracts.Command{Kind: "group.reorder", Reorder: &contracts.ReorderInput{
		SourceID: ids[2], TargetID: ids[0],
	}}); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = core.Snapshot(ctx)
	order := map[string]int{}
	for _, group := range snapshot.SessionGroups {
		order[group.ID] = group.Order
	}
	if order[ids[2]] != 0 || order[ids[0]] != 1 || order[ids[1]] != 2 {
		t.Fatalf("group reorder did not renumber densely: %v", order)
	}
	// Move a conversation into that group beside another, then reorder it.
	first, second := snapshot.Sessions[0].ID, snapshot.Sessions[1].ID
	if err = core.Execute(ctx, contracts.Command{Kind: "session.update", Session: &contracts.SessionUpdate{ID: first, GroupID: &ids[2]}}); err != nil {
		t.Fatal(err)
	}
	if err = core.Execute(ctx, contracts.Command{Kind: "session.reorder", Reorder: &contracts.ReorderInput{
		GroupID: ids[2], SourceID: second, TargetID: first,
	}}); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = core.Snapshot(ctx)
	ranks := map[string]int{}
	for _, session := range snapshot.Sessions {
		if session.ID == first || session.ID == second {
			if session.GroupID != ids[2] {
				t.Fatalf("%s did not join the destination group: %q", session.ID, session.GroupID)
			}
			ranks[session.ID] = session.GroupOrder
		}
	}
	// The dragged conversation sits immediately before its target.
	if ranks[second] >= ranks[first] || ranks[first]-ranks[second] != 1 {
		t.Fatalf("cross-group drag did not place the source beside its target: %v", ranks)
	}
	// An empty group and target append the conversation to the ungrouped root.
	if err = core.Execute(ctx, contracts.Command{Kind: "session.reorder", Reorder: &contracts.ReorderInput{
		SourceID: second,
	}}); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = core.Snapshot(ctx)
	for _, session := range snapshot.Sessions {
		if session.ID == second && session.GroupID != "" {
			t.Fatalf("reorder to the ungrouped root kept membership %q", session.GroupID)
		}
	}
}

func TestLazySessionReorderPersistsAllAffectedRecords(t *testing.T) {
	home := t.TempDir()
	store, err := storage.Open(storage.Options{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	core, err := openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err = core.Execute(ctx, contracts.Command{Kind: "group.create", Text: "Research"}); err != nil {
		t.Fatal(err)
	}
	state, _ := core.Snapshot(ctx)
	groupID := state.SessionGroups[0].ID
	for _, name := range []string{"A", "B", "C"} {
		if err = core.Execute(ctx, contracts.Command{Kind: "session.create", Name: name}); err != nil {
			t.Fatal(err)
		}
	}
	state, _ = core.Snapshot(ctx)
	byTitle := make(map[string]contracts.Session, len(state.Sessions))
	for _, session := range state.Sessions {
		byTitle[session.Title] = session
	}
	for _, name := range []string{"A", "B"} {
		group := groupID
		if err = core.Execute(ctx, contracts.Command{Kind: "session.update", Session: &contracts.SessionUpdate{ID: byTitle[name].ID, GroupID: &group}}); err != nil {
			t.Fatal(err)
		}
	}
	if err = core.Execute(ctx, contracts.Command{Kind: "session.reorder", Reorder: &contracts.ReorderInput{GroupID: groupID, SourceID: byTitle["B"].ID, TargetID: byTitle["A"].ID}}); err != nil {
		t.Fatal(err)
	}
	if err = core.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = storage.Open(storage.Options{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	core, err = openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	state, _ = core.Snapshot(ctx)
	if len(core.loadedSessions) != 0 {
		t.Fatalf("restart eagerly loaded session bodies: %d", len(core.loadedSessions))
	}
	byTitle = make(map[string]contracts.Session, len(state.Sessions))
	for _, session := range state.Sessions {
		byTitle[session.Title] = session
	}
	if err = core.Execute(ctx, contracts.Command{Kind: "session.reorder", Reorder: &contracts.ReorderInput{GroupID: groupID, SourceID: byTitle["C"].ID, TargetID: byTitle["A"].ID}}); err != nil {
		t.Fatal(err)
	}
	state, _ = core.Snapshot(ctx)
	want := make(map[string]contracts.Session, len(state.Sessions))
	for _, session := range state.Sessions {
		want[session.ID] = session
	}
	if err = core.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = storage.Open(storage.Options{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	core, err = openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	for id, expected := range want {
		view, viewErr := core.Snapshot(ctx, id)
		if viewErr != nil {
			t.Fatal(viewErr)
		}
		index := core.sessionIndex(id)
		if index < 0 {
			t.Fatalf("lazy reorder lost session %s", id)
		}
		got := view.Sessions[index]
		if got.GroupID != expected.GroupID || got.GroupOrder != expected.GroupOrder {
			t.Fatalf("lazy reorder was not written to %s: got=%+v want=%+v", id, got, expected)
		}
	}
}
