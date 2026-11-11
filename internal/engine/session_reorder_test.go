package engine

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"gcode/internal/contracts"
)

func TestProjectSessionReorderPersistsLazyRecordsAndKeepsOtherOrders(t *testing.T) {
	home := t.TempDir()
	core := openStorageTestCore(t, home)
	defer func() { _ = core.Close() }()
	for _, path := range []string{t.TempDir(), t.TempDir()} {
		if err := core.Execute(t.Context(), contracts.Command{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: path}}); err != nil {
			t.Fatal(err)
		}
	}
	state, _ := core.Snapshot(t.Context())
	project, other := state.Projects[0].ID, state.Projects[1].ID
	for _, name := range []string{"A", "B", "C", "Pinned"} {
		if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create", ProjectID: project, Name: name}); err != nil {
			t.Fatal(err)
		}
	}
	if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create", ProjectID: other, Name: "Other"}); err != nil {
		t.Fatal(err)
	}
	if err := core.Execute(t.Context(), contracts.Command{Kind: "group.create", Text: "Independent group"}); err != nil {
		t.Fatal(err)
	}
	state, _ = core.Snapshot(t.Context())
	byTitle := map[string]contracts.Session{}
	for _, session := range state.Sessions {
		byTitle[session.Title] = session
	}
	group := state.SessionGroups[0].ID
	pinned := true
	for _, update := range []contracts.SessionUpdate{
		{ID: byTitle["Pinned"].ID, Pinned: &pinned},
		{ID: byTitle["A"].ID, GroupID: &group},
	} {
		if err := core.Execute(t.Context(), contracts.Command{Kind: "session.update", Session: &update}); err != nil {
			t.Fatal(err)
		}
	}
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	core = openStorageTestCore(t, home)
	move := func(source, target string, after bool) {
		t.Helper()
		if err := core.Execute(t.Context(), contracts.Command{Kind: "session.project.reorder", Reorder: &contracts.ReorderInput{
			SourceID: byTitle[source].ID, TargetID: byTitle[target].ID, After: after,
		}}); err != nil {
			t.Fatal(err)
		}
	}
	move("A", "C", false)
	move("B", "C", false)
	assertOrder := func(want []string) {
		t.Helper()
		state, err := core.Snapshot(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		var members []contracts.Session
		for _, session := range state.Sessions {
			if session.ProjectID == project && !session.Pinned {
				members = append(members, session)
			}
			if session.Title == "A" && session.GroupID != group {
				t.Fatal("project reorder changed custom group membership")
			}
			if session.Title == "Pinned" && (!session.Pinned || session.ProjectOrder != 0) {
				t.Fatal("project reorder changed a pinned conversation")
			}
			if session.Title == "Other" && session.ProjectOrder != 0 {
				t.Fatal("project reorder changed a different project")
			}
		}
		slices.SortStableFunc(members, contracts.CompareProjectSessions)
		var got []string
		for _, member := range members {
			got = append(got, member.Title)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("project order=%v, want %v", got, want)
		}
	}
	assertOrder([]string{"A", "B", "C"})
	// Reading lazy bodies after the compound save must retain every rank.
	for _, name := range []string{"A", "B", "C"} {
		if _, err := core.Snapshot(t.Context(), byTitle[name].ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	core = openStorageTestCore(t, home)
	assertOrder([]string{"A", "B", "C"})
	core.mu.Lock()
	core.state.Sessions[core.sessionIndex(byTitle["C"].ID)].UpdatedAt = time.Now().Add(time.Hour)
	core.mu.Unlock()
	assertOrder([]string{"A", "B", "C"})
	before, _ := core.Snapshot(t.Context())
	for _, input := range []contracts.ReorderInput{
		{SourceID: byTitle["A"].ID, TargetID: byTitle["Other"].ID},
		{SourceID: byTitle["Pinned"].ID, TargetID: byTitle["A"].ID},
		{SourceID: byTitle["A"].ID, TargetID: byTitle["Pinned"].ID},
	} {
		if err := core.Execute(t.Context(), contracts.Command{Kind: "session.project.reorder", Reorder: &input}); err == nil {
			t.Fatal("invalid project move was accepted")
		}
	}
	after, _ := core.Snapshot(t.Context())
	if !reflect.DeepEqual(before.Sessions, after.Sessions) {
		t.Fatal("invalid moves changed conversation state")
	}
	if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create", ProjectID: project, Name: "New"}); err != nil {
		t.Fatal(err)
	}
	assertOrder([]string{"New", "A", "B", "C"})
}
