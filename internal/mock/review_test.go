package mock

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"gcode/internal/contracts"
)

func TestDirectoryMutationsAreAtomicAndKeepIdentities(t *testing.T) {
	s := newTestServer(t, Options{})
	ctx := context.Background()
	for _, c := range []contracts.Command{{Kind: "file.create", ProjectID: "fieldnotes", Path: "notes", Enabled: true}, {Kind: "file.create", ProjectID: "fieldnotes", Path: "notes/plan.md", Text: "# Plan"}} {
		if err := s.Execute(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	before, _ := s.Snapshot(ctx)
	var parent, child contracts.File
	for _, f := range before.Files {
		if f.Path == "notes" {
			parent = f
		}
		if f.Path == "notes/plan.md" {
			child = f
		}
	}
	if err := s.Execute(ctx, contracts.Command{Kind: "file.rename", ID: parent.ID, Name: "design"}); err != nil {
		t.Fatal(err)
	}
	renamed, _ := s.Snapshot(ctx)
	found := false
	for _, f := range renamed.Files {
		if f.ID == child.ID {
			found = f.Path == "design/plan.md" && f.ParentID == parent.ID && f.Version == child.Version+1
		}
	}
	if !found {
		t.Fatal("directory rename lost descendant identity")
	}
	if err := s.Execute(ctx, contracts.Command{Kind: "file.delete", ID: parent.ID}); err != nil {
		t.Fatal(err)
	}
	deleted, _ := s.Snapshot(ctx)
	for _, f := range deleted.Files {
		if f.ID == parent.ID || f.ID == child.ID {
			t.Fatal("directory delete retained descendant")
		}
	}
	if err := s.Execute(ctx, contracts.Command{Kind: "file.create", ProjectID: "fieldnotes", Path: "../outside"}); err == nil {
		t.Fatal("parent traversal accepted")
	}
	s.mu.Lock()
	for i := range s.state.Files {
		if s.state.Files[i].ID == "file-layout" {
			s.state.Files[i].ReadOnly = true
		}
	}
	s.mu.Unlock()
	protected, _ := s.Snapshot(ctx)
	if err := s.Execute(ctx, contracts.Command{Kind: "file.delete", ID: "dir-internal"}); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("expected read-only guard: %v", err)
	}
	after, _ := s.Snapshot(ctx)
	if !reflect.DeepEqual(protected.Files, after.Files) {
		t.Fatal("failed directory delete partially changed files")
	}
}

func TestPinReorderChangesVisibleOrder(t *testing.T) {
	s := newTestServer(t, Options{})
	ctx := context.Background()
	zero := 0
	pin := true
	if err := s.UpdateSession(ctx, contracts.SessionUpdate{ID: "chat-1", Pinned: &pin, PinOrder: &zero}); err != nil {
		t.Fatal(err)
	}
	v, _ := s.Snapshot(ctx)
	orders := map[string]int{}
	for _, session := range v.Sessions {
		if session.Pinned {
			orders[session.ID] = session.PinOrder
		}
	}
	if orders["chat-1"] != 0 || orders["chat-0"] != 1 {
		t.Fatalf("pin order did not move: %#v", orders)
	}
}

func TestCommandPayloadValidationPreventsMutation(t *testing.T) {
	s := newTestServer(t, Options{})
	ctx := context.Background()
	before, _ := s.Snapshot(ctx)
	for _, cmd := range []contracts.Command{{Kind: "session.delete", ID: "chat-0", Settings: &contracts.Settings{}}, {Kind: "message.stop"}, {Kind: "settings.update"}, {Kind: "unknown"}} {
		if err := s.Execute(ctx, cmd); err == nil {
			t.Errorf("invalid command accepted: %#v", cmd)
		}
	}
	after, _ := s.Snapshot(ctx)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("invalid tagged command changed state")
	}
}

func TestThemeImportLifecycleAndSnapshotOwnership(t *testing.T) {
	s := newTestServer(t, Options{})
	ctx := context.Background()
	if err := s.Execute(ctx, contracts.Command{Kind: "theme.analyze", Path: "themes/missing-theme.json", Name: "linked"}); err != nil {
		t.Fatal(err)
	}
	preview, _ := s.Snapshot(ctx)
	if preview.ThemeImport == nil || !preview.ThemeImport.Ready || len(preview.ThemeImport.Variants) != 2 {
		t.Fatal("theme preview missing")
	}
	preview.ThemeImport.Variants[0].Name = "mutated"
	if err := s.Execute(ctx, contracts.Command{Kind: "theme.select", ID: "github-light", Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if err := s.Execute(ctx, contracts.Command{Kind: "theme.import"}); err != nil {
		t.Fatal(err)
	}
	imported, _ := s.Snapshot(ctx)
	if imported.ThemeImport != nil || len(imported.ThemeLibrary) != 1 || len(imported.ThemeLibrary[0].VariantIDs) != 1 {
		t.Fatal("selected variants were not imported")
	}
	entry := imported.ThemeLibrary[0]
	if err := s.Execute(ctx, contracts.Command{Kind: "theme.reload", ID: entry.ID}); err != nil {
		t.Fatal(err)
	}
	reloaded, _ := s.Snapshot(ctx)
	if reloaded.ThemeLibrary[0].Warning == "" || len(reloaded.ThemeLibrary[0].VariantIDs) != 1 {
		t.Fatal("failed reload discarded compiled palette")
	}
	if err := s.Execute(ctx, contracts.Command{Kind: "theme.duplicate", ID: entry.ID}); err != nil {
		t.Fatal(err)
	}
	duplicated, _ := s.Snapshot(ctx)
	if len(duplicated.ThemeLibrary) != 2 || duplicated.ThemeLibrary[1].SourceKind != "editable" {
		t.Fatal("editable duplicate missing")
	}
	if err := s.Execute(ctx, contracts.Command{Kind: "theme.remove", ID: entry.ID}); err != nil {
		t.Fatal(err)
	}
	after, _ := s.Snapshot(ctx)
	if len(after.ThemeLibrary) != 1 {
		t.Fatal("remove did not update library")
	}
	if err := s.Execute(ctx, contracts.Command{Kind: "theme.analyze", Path: "broken.json"}); err != nil {
		t.Fatal(err)
	}
	invalid, _ := s.Snapshot(ctx)
	if invalid.ThemeImport.Error == "" || invalid.ThemeImport.Ready {
		t.Fatal("invalid theme was accepted")
	}
	if err := s.Execute(ctx, contracts.Command{Kind: "theme.import"}); err == nil {
		t.Fatal("invalid preview imported")
	}
}

func TestRelatedSessionsPreserveContextAndParent(t *testing.T) {
	s := newTestServer(t, Options{})
	ctx := context.Background()
	created, _ := s.Snapshot(ctx)
	if err := s.Execute(ctx, contracts.Command{Kind: "session.fork", SessionID: "chat-0"}); err != nil {
		t.Fatal(err)
	}
	forked, _ := s.Snapshot(ctx)
	fork := forked.Sessions[0]
	if fork.ParentSessionID != "chat-0" || fork.SideChat || len(forked.Messages[fork.ID]) != len(created.Messages["chat-0"]) {
		t.Fatal("fork lost context or parent association")
	}
	for i, m := range forked.Messages[fork.ID] {
		if m.ID == created.Messages["chat-0"][i].ID || m.SessionID != fork.ID || m.Streaming {
			t.Fatal("fork reused mutable/current-run message identity")
		}
	}
	if err := s.Execute(ctx, contracts.Command{Kind: "session.sidechat", SessionID: "chat-0"}); err != nil {
		t.Fatal(err)
	}
	side, _ := s.Snapshot(ctx)
	child := side.Sessions[0]
	if child.ParentSessionID != "chat-0" || !child.SideChat || len(side.Messages[child.ID]) != 0 {
		t.Fatal("side chat is not an empty related session")
	}
}

func TestQueueRetainsConfigurationAndArchiveCancels(t *testing.T) {
	s := newTestServer(t, Options{TokenInterval: time.Hour})
	ctx := context.Background()
	before, _ := s.Snapshot(ctx)
	original := before.Sessions[0]
	first, err := s.Send(ctx, contracts.SendRequest{SessionID: "chat-0", Text: "first"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Send(ctx, contracts.SendRequest{SessionID: "chat-0", Text: "second", ModelID: "gpt-5.4", ReasoningLevel: "Extra high", Branch: "feature/next", DeviceID: "macbook"})
	if err != nil {
		t.Fatal(err)
	}
	s.advanceRun("chat-0", first.ID, "done", true)
	v, _ := s.Snapshot(ctx)
	session := v.Sessions[0]
	if session.ModelID != "gpt-5.4" || session.ReasoningLevel != "Extra high" {
		t.Fatal("queued run lost frozen model/reasoning configuration")
	}
	if session.Branch != original.Branch || session.DeviceID != original.DeviceID || session.CWD != original.CWD || session.WorktreeID != original.WorktreeID {
		t.Fatal("queued draft settings migrated an established conversation")
	}
	active := s.runs["chat-0"].id
	archived := true
	if err := s.UpdateSession(ctx, contracts.SessionUpdate{ID: "chat-0", Archived: &archived}); err != nil {
		t.Fatal(err)
	}
	if s.advanceRun("chat-0", active, "late", true) {
		t.Fatal("archived run accepted late output")
	}
	after, _ := s.Snapshot(ctx)
	if after.Sessions[0].Status != contracts.Idle {
		t.Fatal("archive kept active status")
	}
}
