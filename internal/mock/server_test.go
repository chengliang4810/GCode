package mock

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"gcode/internal/contracts"
)

func newTestServer(t *testing.T, opts Options) *Server {
	t.Helper()
	s, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s
}

func await(t *testing.T, events <-chan contracts.Event, predicate func(contracts.Snapshot) bool) contracts.Snapshot {
	t.Helper()
	timer := time.NewTimer(4 * time.Second)
	defer timer.Stop()
	for {
		select {
		case e, ok := <-events:
			if !ok {
				t.Fatal("subscription closed before expected state")
			}
			if e.Snapshot != nil && predicate(*e.Snapshot) {
				return *e.Snapshot
			}
		case <-timer.C:
			t.Fatal("timed out waiting for state")
		}
	}
}

func TestSnapshotIsolation(t *testing.T) {
	s := newTestServer(t, Options{Scenario: "questions"})
	ctx := context.Background()
	before, err := s.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	copy1, err := s.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	copy1.Projects[0].Branches[0] = "mutated"
	copy1.Messages["chat-0"][0].Blocks[0].Text = "mutated"
	copy1.Models[0].ReasoningLevels[0] = "mutated"
	copy1.Providers[0].Models[0] = "mutated"
	copy1.Settings.Shortcuts[0].Keys = "mutated"
	copy1.Changes[0].Hunks[0].Lines[0].Text = "mutated"
	copy1.Commits[0].Refs[0] = "mutated"
	ms := copy1.Messages["chat-0"]
	q := ms[len(ms)-1].Blocks[0].Question
	q.Options[0] = "mutated"
	copy1.Messages["chat-0"] = nil
	after, err := s.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("caller mutation escaped into backend state")
	}
}

func TestSubscriptionCancellationAndIsolation(t *testing.T) {
	s := newTestServer(t, Options{})
	ctx, cancel := context.WithCancel(context.Background())
	first, err := s.Subscribe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Subscribe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	a := <-first
	b := <-second
	a.Snapshot.Projects[0].Name = "mutated"
	if b.Snapshot.Projects[0].Name == "mutated" {
		t.Fatal("subscribers share mutable snapshots")
	}
	cancel()
	select {
	case _, ok := <-first:
		if ok {
			t.Fatal("cancelled subscription remained open")
		}
	case <-time.After(time.Second):
		t.Fatal("cancel did not close subscription")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case _, ok := <-second:
		if ok {
			t.Fatal("closed backend retained subscription")
		}
	case <-time.After(time.Second):
		t.Fatal("close did not terminate subscription")
	}
	if _, err := s.Snapshot(context.Background()); !errors.Is(err, ErrClosed) {
		t.Fatalf("snapshot after close: %v", err)
	}
}

func TestAtomicCreateAndSend(t *testing.T) {
	s := newTestServer(t, Options{Instant: true})
	ctx := context.Background()
	events, err := s.Subscribe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	<-events
	if err := s.Execute(ctx, contracts.Command{Kind: "message.send", ProjectID: "fieldnotes", Send: &contracts.SendRequest{Text: "Review the new workspace"}}); err != nil {
		t.Fatal(err)
	}
	got := await(t, events, func(v contracts.Snapshot) bool {
		return strings.HasPrefix(v.Sessions[0].ID, "session-") && v.Sessions[0].LastOutcome == "completed"
	})
	newSession := got.Sessions[0]
	if newSession.ProjectID != "fieldnotes" {
		t.Fatal("new session lost project")
	}
	messages := got.Messages[newSession.ID]
	if len(messages) != 2 || messages[0].Role != "user" || messages[1].Role != "assistant" || messages[1].Streaming {
		t.Fatalf("unexpected turn: %#v", messages)
	}
	if len(messages[1].Blocks) == 0 || messages[1].Blocks[0].Text == "" {
		t.Fatal("assistant output missing")
	}
}

func TestStopRejectsStaleUpdates(t *testing.T) {
	s := newTestServer(t, Options{TokenInterval: time.Hour})
	ctx := context.Background()
	run, err := s.Send(ctx, contracts.SendRequest{SessionID: "chat-0", Text: "Update this workspace"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Stop(ctx, "chat-0"); err != nil {
		t.Fatal(err)
	}
	before, _ := s.Snapshot(ctx)
	if s.advanceRun("chat-0", run.ID, "late result", true) {
		t.Fatal("stale run accepted an update")
	}
	after, _ := s.Snapshot(ctx)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("cancelled run changed state")
	}
	if after.Sessions[0].Status != contracts.Idle || after.Messages["chat-0"][len(after.Messages["chat-0"])-1].Streaming {
		t.Fatal("stop did not settle visible state")
	}
}

func TestQueueDrainsAndRetryDoesNotRepeatUserMessage(t *testing.T) {
	s := newTestServer(t, Options{TokenInterval: time.Hour})
	ctx := context.Background()
	events, _ := s.Subscribe(ctx)
	<-events
	first, err := s.Send(ctx, contracts.SendRequest{SessionID: "chat-0", Text: "first"})
	if err != nil {
		t.Fatal(err)
	}
	queued, err := s.Send(ctx, contracts.SendRequest{SessionID: "chat-0", Text: "second"})
	if err != nil {
		t.Fatal(err)
	}
	if queued.Status != "queued" {
		t.Fatal("busy session did not queue")
	}
	s.advanceRun("chat-0", first.ID, "finished first", true)
	v, _ := s.Snapshot(ctx)
	if len(v.Queue) != 0 {
		t.Fatal("completed run did not drain queue")
	}
	active := s.runs["chat-0"].id
	s.advanceRun("chat-0", active, "finished second", true)
	before, _ := s.Snapshot(ctx)
	countUser := func(v contracts.Snapshot) int {
		n := 0
		for _, m := range v.Messages["chat-0"] {
			if m.Role == "user" {
				n++
			}
		}
		return n
	}
	n := countUser(before)
	run, err := s.Retry(ctx, "chat-0")
	if err != nil {
		t.Fatal(err)
	}
	s.advanceRun("chat-0", run.ID, "regenerated", true)
	after, _ := s.Snapshot(ctx)
	if countUser(after) != n {
		t.Fatal("retry duplicated the user's message")
	}
}

func TestFileSaveConflictAndReadOnly(t *testing.T) {
	s := newTestServer(t, Options{})
	ctx := context.Background()
	saved, err := s.SaveFile(ctx, "file-layout", "package layout\n", 1)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Version != 2 || !saved.Modified {
		t.Fatal("save did not advance file version")
	}
	if _, err = s.SaveFile(ctx, "file-layout", "old editor content", 1); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict: %v", err)
	}
	if _, err = s.SaveFile(ctx, "file-lock", "new", 1); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("expected read-only: %v", err)
	}
	v, _ := s.Snapshot(ctx)
	for _, f := range v.Files {
		if f.ID == "file-layout" && f.Content != "package layout\n" {
			t.Fatal("stale save overwrote content")
		}
	}
}

func TestPersistenceRestoresMutationsInIndependentStatePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "desktop.json")
	s, err := New(Options{StatePath: path, TokenInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	name := "Renamed workspace"
	if err = s.UpdateSession(ctx, contracts.SessionUpdate{ID: "chat-0", Title: &name}); err != nil {
		t.Fatal(err)
	}
	settings := DefaultSettings()
	settings.Appearance.FontSize = 18
	settings.SidebarWidth = 300
	if err = s.UpdateSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Send(ctx, contracts.SendRequest{SessionID: "chat-0", Text: "working when closed"}); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	restored := newTestServer(t, Options{StatePath: path})
	v, _ := restored.Snapshot(ctx)
	if v.Sessions[0].Title != name || v.Settings.SidebarWidth != 300 || v.Settings.Appearance.FontSize != 18 {
		t.Fatal("persisted mutations were lost")
	}
	if v.Sessions[0].Status != contracts.Idle {
		t.Fatal("restart preserved an impossible running task")
	}
	other := newTestServer(t, Options{})
	baseline, _ := other.Snapshot(ctx)
	if baseline.Sessions[0].Title == name {
		t.Fatal("a second backend inherited state without its own StatePath")
	}
}

func TestAnswersAndSettingsNormalization(t *testing.T) {
	t.Run("question", func(t *testing.T) {
		s := newTestServer(t, Options{Scenario: "questions", Instant: true})
		events, _ := s.Subscribe(context.Background())
		<-events
		if err := s.Answer(context.Background(), "question-layout", "Keep the current layout"); err != nil {
			t.Fatal(err)
		}
		got := await(t, events, func(v contracts.Snapshot) bool { return v.Sessions[0].Status == contracts.Idle })
		found := false
		for _, m := range got.Messages["chat-0"] {
			for _, b := range m.Blocks {
				if b.Question != nil && b.Question.ID == "question-layout" {
					found = b.Question.Answer == "Keep the current layout"
				}
			}
		}
		if !found {
			t.Fatal("answer not retained")
		}
		if err := s.Answer(context.Background(), "question-layout", "again"); err == nil {
			t.Fatal("duplicate answer accepted")
		}
	})
	t.Run("settings", func(t *testing.T) {
		s := newTestServer(t, Options{})
		v := DefaultSettings()
		v.SidebarWidth = 1
		v.Appearance.FontSize = 999
		v.Appearance.TranscriptWidth = 2000
		if err := s.UpdateSettings(context.Background(), v); err != nil {
			t.Fatal(err)
		}
		got, _ := s.Snapshot(context.Background())
		if got.Settings.SidebarWidth != 242 || got.Settings.Appearance.FontSize != 20 || got.Settings.Appearance.TranscriptWidth != 1200 {
			t.Fatal("unsafe geometry was not normalized")
		}
	})
}

func TestTerminalHasNoCommandExecution(t *testing.T) {
	s := newTestServer(t, Options{})
	output, err := s.TerminalInput(context.Background(), "terminal-1", "echo hello")
	if err != nil || !strings.Contains(output, "hello\r\n") {
		t.Fatalf("echo: %s %v", output, err)
	}
	output, err = s.TerminalInput(context.Background(), "terminal-1", "powershell -Command Remove-Item something")
	if err != nil || !strings.Contains(output, "powershell: command not found") {
		t.Fatal("terminal should reject unscripted commands")
	}
}

func TestStoppedRequestsCannotResumeAndDenyDoesNotRun(t *testing.T) {
	t.Run("cancelled", func(t *testing.T) {
		s := newTestServer(t, Options{Scenario: "permissions"})
		ctx := context.Background()
		if err := s.Stop(ctx, "chat-0"); err != nil {
			t.Fatal(err)
		}
		if err := s.Answer(ctx, "permission-tests", "Allow once"); err == nil {
			t.Fatal("cancelled request resumed a stopped run")
		}
	})
	t.Run("denied", func(t *testing.T) {
		s := newTestServer(t, Options{Scenario: "permissions"})
		ctx := context.Background()
		before, _ := s.Snapshot(ctx)
		if err := s.Answer(ctx, "permission-tests", "Deny"); err != nil {
			t.Fatal(err)
		}
		v, _ := s.Snapshot(ctx)
		if v.Sessions[0].Status != contracts.Idle || len(v.Runs) != len(before.Runs) {
			t.Fatal("denying a command started work")
		}
		last := v.Messages["chat-0"][len(v.Messages["chat-0"])-1]
		if !strings.Contains(last.Blocks[0].Text, "won’t run") {
			t.Fatal("denial was not acknowledged")
		}
	})
}

func TestConcurrentSnapshotsAndUpdates(t *testing.T) {
	s := newTestServer(t, Options{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 30 {
				v, err := s.Snapshot(context.Background())
				if err != nil {
					t.Error(err)
					return
				}
				v.Settings.General.CompactMode = !v.Settings.General.CompactMode
				if err = s.UpdateSettings(context.Background(), v.Settings); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func BenchmarkStressSnapshot(b *testing.B) {
	s, err := New(Options{Scenario: "stress"})
	if err != nil {
		b.Fatal(err)
	}
	defer s.Close()
	b.ResetTimer()
	for b.Loop() {
		if _, err = s.Snapshot(context.Background()); err != nil {
			b.Fatal(err)
		}
	}
}
