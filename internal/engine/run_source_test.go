package engine

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
)

func TestRunSourceRefreshesFollowUpAndQueuedTaskFromActualCWD(t *testing.T) {
	repository := temporaryGitRepository(t)
	subdirectory := filepath.Join(repository, "subproject")
	if err := os.Mkdir(subdirectory, 0700); err != nil {
		t.Fatal(err)
	}
	var core *Engine
	var requests atomic.Int32
	observations := make(chan contracts.Session, 3)
	releaseSecond := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		number := requests.Add(1)
		state, err := core.Snapshot(r.Context())
		if err != nil || len(state.Sessions) != 1 {
			http.Error(w, "conversation unavailable", http.StatusInternalServerError)
			return
		}
		observations <- state.Sessions[0]
		if number == 2 {
			select {
			case <-releaseSecond:
			case <-r.Context().Done():
				return
			}
		}
		goalResponse(w, model.Chat, "Done", nil, true)
	}))
	t.Cleanup(server.Close)
	core = modelCheckCore(t, model.Chat, server.URL)
	if err := core.Execute(t.Context(), contracts.Command{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: subdirectory}}); err != nil {
		t.Fatal(err)
	}
	state := waitForProjectFiles(t, core)
	project := state.Projects[0]
	send := func(sessionID, text string) {
		t.Helper()
		if err := core.Execute(t.Context(), contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
			ProjectID: project.ID, SessionID: sessionID, ModelID: "checks/unselected", Text: text,
		}}); err != nil {
			t.Fatal(err)
		}
	}
	observe := func(branch, head string) contracts.Session {
		t.Helper()
		select {
		case session := <-observations:
			source := session.SourceContext
			if source == nil || source.Branch != branch || session.Branch != branch || source.HeadSHA != head ||
				!sameCheckoutPath(source.RepoRoot, repository) || !sameCheckoutPath(source.CWD, subdirectory) {
				t.Fatalf("model request observed stale or incorrect source: session=%+v source=%+v", session, source)
			}
			return session
		case <-time.After(10 * time.Second):
			t.Fatal("model request did not observe refreshed source")
			return contracts.Session{}
		}
	}
	firstHead := runGitTest(t, repository, "rev-parse", "HEAD")
	send("", "First task")
	first := observe("main", firstHead)
	waitIdle(t, core, first.ID)
	runGitTest(t, repository, "switch", "-c", "outside-follow-up")
	if err := os.WriteFile(filepath.Join(repository, "tracked.txt"), []byte("next commit"), 0600); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, repository, "add", "tracked.txt")
	runGitTest(t, repository, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "follow-up source")
	secondHead := runGitTest(t, repository, "rev-parse", "HEAD")
	send(first.ID, "Follow-up task")
	second := observe("outside-follow-up", secondHead)
	if !second.SourceContext.ObservedAt.After(first.SourceContext.ObservedAt) {
		t.Fatal("follow-up kept the earlier observation time")
	}
	runGitTest(t, repository, "switch", "-c", "outside-queued-task")
	send(first.ID, "Queued task")
	close(releaseSecond)
	third := observe("outside-queued-task", secondHead)
	if !third.SourceContext.ObservedAt.After(second.SourceContext.ObservedAt) {
		t.Fatal("queued task kept the preceding task's observation")
	}
	finished := waitIdle(t, core, first.ID)
	if len(finished.Runs) != 3 || len(finished.Queue) != 0 {
		t.Fatalf("follow-up/queued execution did not settle: runs=%+v queue=%+v", finished.Runs, finished.Queue)
	}
}

func TestRunSourcePersistsOnlyTheObservedSession(t *testing.T) {
	home, repository := t.TempDir(), temporaryGitRepository(t)
	core := openStorageTestCore(t, home)
	t.Cleanup(func() { _ = core.Close() })
	core.mu.Lock()
	core.state.Projects = append(core.state.Projects, contracts.Project{ID: "project", Path: repository, Branch: "project-observation"})
	other := newTestSession(t, core, "project", "Other conversation")
	session := newTestSession(t, core, "project", "Observed conversation")
	run := &activeRun{id: "observed-run", cancel: func() {}}
	core.runs[session.ID] = run
	core.mu.Unlock()
	if err := core.refreshRunSource(t.Context(), session, run, inspectRunSource); err != nil {
		t.Fatal(err)
	}
	core.mu.Lock()
	delete(core.runs, session.ID)
	observed := core.state.Sessions[core.sessionIndex(session.ID)]
	unchanged := core.state.Sessions[core.sessionIndex(other.ID)]
	projectBranch := core.state.Projects[0].Branch
	core.mu.Unlock()
	if observed.SourceContext == nil || observed.SourceContext.Branch != "main" || observed.WorktreeID != "" {
		t.Fatalf("refresh changed checkout selection or did not observe source: %+v", observed)
	}
	if !reflect.DeepEqual(unchanged, other) || projectBranch != "project-observation" {
		t.Fatal("run observation leaked into another conversation or project")
	}
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	restored := openStorageTestCore(t, home)
	t.Cleanup(func() { _ = restored.Close() })
	state, err := restored.Snapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range state.Sessions {
		if candidate.ID != session.ID || candidate.SourceContext == nil {
			continue
		}
		actual, expected := *candidate.SourceContext, *observed.SourceContext
		if !actual.ObservedAt.Equal(expected.ObservedAt) {
			t.Fatalf("persisted observation time changed: got %s, want %s", actual.ObservedAt, expected.ObservedAt)
		}
		actual.ObservedAt, expected.ObservedAt = time.Time{}, time.Time{}
		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("persisted source changed: got %+v, want %+v", actual, expected)
		}
		return
	}
	t.Fatal("latest run source was not restored from disk")
}

func TestRunSourceDiscardsLateCanceledAndFailedObservations(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*Engine, context.CancelFunc)
		fail   bool
	}{
		{name: "new run", mutate: func(e *Engine, _ context.CancelFunc) { e.runs["session"] = &activeRun{id: "next"} }},
		{name: "removed session", mutate: func(e *Engine, _ context.CancelFunc) { e.state.Sessions = nil }},
		{name: "different cwd", mutate: func(e *Engine, _ context.CancelFunc) { e.state.Sessions[0].CWD = "/other-checkout" }},
		{name: "different checkout", mutate: func(e *Engine, _ context.CancelFunc) { e.state.Sessions[0].WorktreeID = "other" }},
		{name: "newer observation", mutate: func(e *Engine, _ context.CancelFunc) {
			e.state.Sessions[0].SourceContext = &contracts.ConversationSourceContext{Branch: "newer", ObservedAt: time.Now().Add(time.Second)}
		}},
		{name: "canceled", mutate: func(_ *Engine, cancel context.CancelFunc) { cancel() }},
		{name: "closed", mutate: func(e *Engine, _ context.CancelFunc) { e.closed = true }},
		{name: "failed probe", fail: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			run := &activeRun{id: "current"}
			session := contracts.Session{ID: "session", CWD: "/checkout", WorktreeID: "checkout", Branch: "old",
				SourceContext: &contracts.ConversationSourceContext{Branch: "old", ObservedAt: time.Unix(100, 0)}}
			core := &Engine{runs: map[string]*activeRun{"session": run}, state: contracts.Snapshot{Sessions: []contracts.Session{session}}}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var expected []contracts.Session
			inspect := func(context.Context, string) (*gitRepository, error) {
				if !core.mu.TryLock() {
					t.Fatal("Git inspection held the state lock")
				}
				if test.mutate != nil {
					test.mutate(core, cancel)
				}
				expected = append([]contracts.Session(nil), core.state.Sessions...)
				core.mu.Unlock()
				if test.fail {
					return nil, errors.New("Git unavailable")
				}
				return &gitRepository{root: "/checkout", branch: "late", head: "late-sha"}, nil
			}
			if err := core.refreshRunSource(ctx, session, run, inspect); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(core.state.Sessions, expected) {
				t.Fatalf("discarded observation rewrote source: %+v", core.state.Sessions)
			}
		})
	}
}

func TestRunSourceSupportsDetachedAndUnbornHEAD(t *testing.T) {
	detached := temporaryGitRepository(t)
	head := runGitTest(t, detached, "rev-parse", "HEAD")
	runGitTest(t, detached, "checkout", "--detach")
	unborn := t.TempDir()
	runGitTest(t, unborn, "init", "-b", "unborn")
	for _, test := range []struct{ path, branch, head string }{
		{path: detached, head: head}, {path: unborn, branch: "unborn"},
	} {
		repo, err := inspectRunSource(t.Context(), test.path)
		if err != nil || repo.branch != test.branch || repo.head != test.head {
			t.Fatalf("source inspection = %+v, %v; want branch %q, head %q", repo, err, test.branch, test.head)
		}
	}
}

func TestRunSourcePartialGitFailureKeepsObservation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fault injection uses a POSIX Git wrapper")
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	for _, failingArgument := range []string{"symbolic-ref", "--verify"} {
		t.Run(failingArgument, func(t *testing.T) {
			repository := temporaryGitRepository(t)
			bin := t.TempDir()
			wrapper := "#!/bin/sh\nfor arg do\nif [ \"$arg\" = '" + failingArgument + "' ]; then exit 128; fi\ndone\nexec '" + strings.ReplaceAll(realGit, "'", "'\\''") + "' \"$@\"\n"
			if err := os.WriteFile(filepath.Join(bin, "git"), []byte(wrapper), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			session := contracts.Session{ID: "session", CWD: repository, Branch: "old", SourceContext: &contracts.ConversationSourceContext{
				Branch: "old", HeadSHA: "old-head", ObservedAt: time.Unix(100, 0),
			}}
			run := &activeRun{id: "run"}
			core := &Engine{runs: map[string]*activeRun{session.ID: run}, state: contracts.Snapshot{Sessions: []contracts.Session{session}}}
			if err := core.refreshRunSource(t.Context(), session, run, inspectRunSource); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(core.state.Sessions[0], session) {
				t.Fatal("a partial Git failure cleared source or refreshed its time")
			}
		})
	}
}

func TestRunSourceProbeHasDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		core := &Engine{}
		start := time.Now()
		probe := func(ctx context.Context, _ string) (*gitRepository, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		if err := core.refreshRunSource(t.Context(), contracts.Session{}, &activeRun{}, probe); err != nil {
			t.Fatal(err)
		}
		if elapsed := time.Since(start); elapsed != runSourceTimeout {
			t.Fatalf("probe deadline = %s, want %s", elapsed, runSourceTimeout)
		}
	})
}
