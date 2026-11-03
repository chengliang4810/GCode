package mock

import (
	"context"
	"reflect"
	"testing"
	"time"

	"gcode/internal/contracts"
)

func TestInvalidSessionReferencesDoNotPartiallyUpdate(t *testing.T) {
	s := newTestServer(t, Options{})
	ctx := context.Background()
	before, _ := s.Snapshot(ctx)
	name := "must not commit"
	unknown := "missing"
	if err := s.UpdateSession(ctx, contracts.SessionUpdate{ID: "chat-0", Title: &name, ModelID: &unknown}); err == nil {
		t.Fatal("unknown model accepted")
	}
	if _, err := s.Send(ctx, contracts.SendRequest{Text: "must not create", ModelID: "missing"}); err == nil {
		t.Fatal("unknown model created a session")
	}
	after, _ := s.Snapshot(ctx)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("invalid configuration partially mutated state")
	}
}

func TestRetryAttachmentOnlyAndArchivedGuards(t *testing.T) {
	s := newTestServer(t, Options{Instant: true})
	ctx := context.Background()
	events, _ := s.Subscribe(ctx)
	<-events
	_, err := s.Send(ctx, contracts.SendRequest{SessionID: "chat-0", Attachments: []contracts.Attachment{{ID: "image-1", Name: "layout.png", MIME: "image/png"}}})
	if err != nil {
		t.Fatal(err)
	}
	await(t, events, func(v contracts.Snapshot) bool { return v.Sessions[0].Status == contracts.Idle })
	if _, err = s.Retry(ctx, "chat-0"); err != nil {
		t.Fatalf("attachment-only retry: %v", err)
	}
	await(t, events, func(v contracts.Snapshot) bool { return v.Sessions[0].Status == contracts.Idle })
	archived := true
	if err = s.UpdateSession(ctx, contracts.SessionUpdate{ID: "chat-0", Archived: &archived}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Retry(ctx, "chat-0"); err == nil {
		t.Fatal("archived session retried")
	}
}

func TestArchiveFrozenLoadingScenarioSettlesState(t *testing.T) {
	s := newTestServer(t, Options{Scenario: "loading"})
	archived := true
	if err := s.UpdateSession(context.Background(), contracts.SessionUpdate{ID: "chat-0", Archived: &archived}); err != nil {
		t.Fatal(err)
	}
	v, _ := s.Snapshot(context.Background())
	if v.Sessions[0].Status != contracts.Idle || v.Messages["chat-0"][0].Streaming {
		t.Fatal("archiving frozen loading state retained an infinite spinner")
	}
}

func TestSavedFileUpdatesChangesData(t *testing.T) {
	s := newTestServer(t, Options{})
	ctx := context.Background()
	content := "package layout\n\nconst SidebarWidth = 300\n"
	if _, err := s.SaveFile(ctx, "file-layout", content, 1); err != nil {
		t.Fatal(err)
	}
	v, _ := s.Snapshot(ctx)
	found := false
	for _, c := range v.Changes {
		if c.Path == "internal/layout.go" {
			found = c.NewContent == content && len(c.Hunks) > 0
		}
	}
	if !found {
		t.Fatal("changes pane retained outdated editor content")
	}
}

func TestForkCannotAnswerParentsPendingRequest(t *testing.T) {
	s := newTestServer(t, Options{Scenario: "questions", TokenInterval: time.Hour})
	ctx := context.Background()
	if err := s.Execute(ctx, contracts.Command{Kind: "session.fork", SessionID: "chat-0"}); err != nil {
		t.Fatal(err)
	}
	v, _ := s.Snapshot(ctx)
	child := v.Sessions[0]
	found := false
	for _, m := range v.Messages[child.ID] {
		for _, b := range m.Blocks {
			if b.Question != nil {
				found = true
				if b.Question.ID == "question-layout" || !b.Question.Resolved {
					t.Fatal("fork retained actionable parent request identity")
				}
				if err := s.Answer(ctx, b.Question.ID, "new answer"); err == nil {
					t.Fatal("fork resumed a parent wait")
				}
			}
		}
	}
	if !found {
		t.Fatal("fork lost transcript question")
	}
	if err := s.Execute(ctx, contracts.Command{Kind: "request.answer", ID: "question-layout", QuestionAnswers: []contracts.QuestionAnswer{{Options: []string{"Keep the current layout"}}}}); err != nil {
		t.Fatal("parent question became ambiguous after fork", err)
	}
	if err := s.Stop(ctx, "chat-0"); err != nil {
		t.Fatal(err)
	}
}

func TestModelSelectionPersistsAcrossSessionAndRun(t *testing.T) {
	s := newTestServer(t, Options{TokenInterval: time.Hour})
	ctx := context.Background()
	model := "cursor-auto"
	if err := s.UpdateSession(ctx, contracts.SessionUpdate{ID: "chat-0", ModelID: &model}); err != nil {
		t.Fatal(err)
	}
	v, _ := s.Snapshot(ctx)
	if v.Sessions[0].ModelID != model {
		t.Fatal("selected model did not persist")
	}
	run, err := s.Send(ctx, contracts.SendRequest{SessionID: "chat-0", Text: "run", ModelID: model})
	if err != nil {
		t.Fatal(err)
	}
	if run.SessionID != "chat-0" || run.Status != "running" {
		t.Fatal("run did not start for the selected session")
	}
	if err = s.Stop(ctx, "chat-0"); err != nil {
		t.Fatal(err)
	}
}

func TestQueuedShorthandFreezesEffectiveSessionConfiguration(t *testing.T) {
	s := newTestServer(t, Options{TokenInterval: time.Hour})
	ctx := context.Background()
	first, err := s.Send(ctx, contracts.SendRequest{SessionID: "chat-0", Text: "first"})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := s.Snapshot(ctx)
	if _, err = s.Send(ctx, contracts.SendRequest{SessionID: "chat-0", Text: "queued"}); err != nil {
		t.Fatal(err)
	}
	model, branch := "gpt-5.4", "feature/later"
	if err = s.UpdateSession(ctx, contracts.SessionUpdate{ID: "chat-0", ModelID: &model, Branch: &branch}); err != nil {
		t.Fatal(err)
	}
	s.advanceRun("chat-0", first.ID, "done", true)
	after, _ := s.Snapshot(ctx)
	if after.Sessions[0].ModelID != before.Sessions[0].ModelID || after.Sessions[0].Branch != before.Sessions[0].Branch {
		t.Fatal("queued shorthand inherited a later session edit")
	}
	if err = s.Stop(ctx, "chat-0"); err != nil {
		t.Fatal(err)
	}
}

func TestVirtualTreeAndChangesStayCoherent(t *testing.T) {
	s := newTestServer(t, Options{})
	ctx := context.Background()
	check := func() contracts.Snapshot {
		t.Helper()
		v, _ := s.Snapshot(ctx)
		for _, change := range v.Changes {
			if change.Status == "deleted" {
				continue
			}
			for _, file := range v.Files {
				if file.ProjectID == change.ProjectID && file.Path == change.Path && file.Content != change.NewContent {
					t.Fatalf("editor/diff disagree on %s", file.Path)
				}
			}
		}
		return v
	}
	initial := check()
	if err := s.Execute(ctx, contracts.Command{Kind: "file.create", ProjectID: "fieldnotes", Path: "notes.md", Text: "# Notes\n"}); err != nil {
		t.Fatal(err)
	}
	created := check()
	if len(created.Changes) != len(initial.Changes)+1 || created.Changes[len(created.Changes)-1].Status != "added" {
		t.Fatal("create did not update Changes")
	}
	file := created.Files[len(created.Files)-1]
	if err := s.Execute(ctx, contracts.Command{Kind: "file.rename", ID: file.ID, Name: "notes.txt"}); err != nil {
		t.Fatal(err)
	}
	renamed := check()
	if renamed.Changes[len(renamed.Changes)-1].Path != "notes.txt" || renamed.Files[len(renamed.Files)-1].Language != "txt" {
		t.Fatal("rename left stale file metadata")
	}
	if err := s.Execute(ctx, contracts.Command{Kind: "file.delete", ID: file.ID}); err != nil {
		t.Fatal(err)
	}
	deleted := check()
	if len(deleted.Changes) != len(initial.Changes) {
		t.Fatal("create then delete retained a phantom change")
	}
	if err := s.Execute(ctx, contracts.Command{Kind: "file.rename", ID: "file-main", Name: "renamed.go"}); err != nil {
		t.Fatal(err)
	}
	tracked := check()
	var renamedChange contracts.Change
	for _, change := range tracked.Changes {
		if change.Path == "cmd/fieldnotes/renamed.go" {
			renamedChange = change
		}
	}
	if renamedChange.Status != "renamed" || renamedChange.OldPath != "cmd/fieldnotes/main.go" {
		t.Fatal("tracked rename lost origin")
	}
	if err := s.Execute(ctx, contracts.Command{Kind: "file.delete", ID: "file-main"}); err != nil {
		t.Fatal(err)
	}
	trackedDeleted := check()
	for _, change := range trackedDeleted.Changes {
		if change.ID == renamedChange.ID {
			if change.Status != "deleted" || change.Path != "cmd/fieldnotes/main.go" || change.NewContent != "" {
				t.Fatal("rename then delete did not remove original tracked path")
			}
			return
		}
	}
	t.Fatal("tracked deletion lost its change identity")
}
