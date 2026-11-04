package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/storage"
)

func TestWorktreeDataDirectoryRealGitAndRestart(t *testing.T) {
	repository := temporaryGitRepository(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			subagentWireCall(w, model.Chat, "Write", `{"file_path":"engine-cwd.txt","content":"written in selected checkout"}`)
			return
		}
		goalResponse(w, model.Chat, "Done", nil, true)
	}))
	t.Cleanup(server.Close)
	home := t.TempDir()
	store, err := storage.Open(storage.Options{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	core, err := openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = core.Close() })
	saveModelCheckConfig(t, core, modelCheckConfig(model.Chat, server.URL))
	if err := core.Execute(context.Background(), contracts.Command{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: repository}}); err != nil {
		t.Fatal(err)
	}
	before := waitForProjectFiles(t, core)
	project := before.Projects[0]
	if !project.GitDetected || project.Branch != "main" || len(project.Worktrees) != 1 {
		t.Fatalf("project Git detection failed: %+v", project)
	}
	send := contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
		RequestID: "create-tree", ProjectID: project.ID, ModelID: "checks/unselected", Text: "Inspect checkout",
		Checkout: &contracts.CheckoutPlan{Kind: contracts.CheckoutNew, Base: "main"},
	}}
	if err := core.Execute(context.Background(), send); err != nil {
		t.Fatal(err)
	}
	after := waitIdle(t, core)
	session := after.Sessions[0]
	if filepath.Dir(session.CWD) != filepath.Join(store.Directory(), "worktrees") || session.CWD == repository ||
		!strings.HasPrefix(session.Branch, "feat/") || session.WorktreeID == "" || session.SourceContext == nil ||
		session.SourceContext.CWD != session.CWD || session.SourceContext.CheckoutID != session.WorktreeID {
		t.Fatalf("checkout selection did not reach session execution: %+v", session)
	}
	if name := filepath.Base(session.CWD); !validShortDirectoryID(name) {
		t.Fatalf("worktree directory is not a short random ID: %q", name)
	}
	actual, err := gitOutput(context.Background(), session.CWD, "symbolic-ref", "--short", "HEAD")
	if err != nil || actual != session.Branch {
		t.Fatalf("worktree was not created by Git: %q %v", actual, err)
	}
	if content, err := os.ReadFile(filepath.Join(session.CWD, "tracked.txt")); err != nil || string(content) != "baseline" {
		t.Fatalf("selected base was not checked out: %q %v", content, err)
	}
	if content, err := os.ReadFile(filepath.Join(session.CWD, "engine-cwd.txt")); err != nil || string(content) != "written in selected checkout" {
		t.Fatalf("real Agent tool did not use selected checkout: %q %v", content, err)
	}
	if _, err := os.Stat(filepath.Join(repository, "engine-cwd.txt")); !os.IsNotExist(err) {
		t.Fatal("worktree execution wrote into the original checkout")
	}
	if len(after.Projects[0].Worktrees) != 2 || after.Projects[0].Branch != "main" {
		t.Fatalf("new worktree changed main checkout: %+v", after.Projects[0])
	}
	if err := core.Execute(context.Background(), send); err != nil {
		t.Fatal(err)
	}
	deduped, _ := core.Snapshot(context.Background())
	if len(deduped.Projects[0].Worktrees) != 2 || len(deduped.Sessions) != 1 {
		t.Fatal("duplicate accepted request created another worktree")
	}
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = storage.Open(storage.Options{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	restored, _ := reopened.Snapshot(context.Background())
	if restored.Sessions[0].CWD != session.CWD || restored.Sessions[0].WorktreeID != session.WorktreeID ||
		restored.Sessions[0].SourceContext.HeadSHA != session.SourceContext.HeadSHA {
		t.Fatal("worktree ownership was not restored after restart")
	}
	if err := reopened.Execute(context.Background(), contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
		ProjectID: project.ID, ModelID: "checks/unselected", Text: "Reuse existing tree",
		Checkout: &contracts.CheckoutPlan{Kind: contracts.CheckoutReuse, Path: session.CWD, Branch: session.Branch},
	}}); err != nil {
		t.Fatal(err)
	}
	reused := waitIdle(t, reopened)
	if !sameCheckoutPath(reused.Sessions[0].CWD, session.CWD) || len(reused.Projects[0].Worktrees) != 2 {
		t.Fatal("reuse created another checkout or selected wrong directory")
	}
}

func TestGitRefsSwitchRealCheckoutAndDirtyFailure(t *testing.T) {
	repository := temporaryGitRepository(t)
	core := modelCheckCore(t, model.Chat, "http://unused.invalid")
	runGitTest(t, repository, "switch", "-c", "topic")
	if err := os.WriteFile(filepath.Join(repository, "tracked.txt"), []byte("topic baseline"), 0600); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, repository, "add", "tracked.txt")
	runGitTest(t, repository, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "topic change")
	runGitTest(t, repository, "switch", "main")
	if err := core.Execute(context.Background(), contracts.Command{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: repository}}); err != nil {
		t.Fatal(err)
	}
	state, _ := core.Snapshot(context.Background())
	project := state.Projects[0]
	scope := contracts.RepoRefScope{ProjectID: project.ID, TargetID: "local", RepoPath: repository}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "git.refs.load", Git: &contracts.GitRequest{Scope: scope}}); err != nil {
		t.Fatal(err)
	}
	waitGitRefs(t, core, scope)
	if err := os.WriteFile(filepath.Join(repository, "tracked.txt"), []byte("user uncommitted change"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "git.ref.switch", Git: &contracts.GitRequest{Scope: scope, Ref: "topic"}}); err != nil {
		t.Fatal(err)
	}
	failed := waitGitRefs(t, core, scope)
	if failed.RepoRefs[scope.Key()].SwitchError == "" || failed.Projects[0].Branch != "main" {
		t.Fatal("dirty checkout failure was not reported")
	}
	if data, _ := os.ReadFile(filepath.Join(repository, "tracked.txt")); string(data) != "user uncommitted change" {
		t.Fatal("branch switch lost user changes")
	}
	if err := os.WriteFile(filepath.Join(repository, "tracked.txt"), []byte("baseline"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "git.ref.switch", Git: &contracts.GitRequest{Scope: scope, Ref: "topic"}}); err != nil {
		t.Fatal(err)
	}
	switched := waitGitRefs(t, core, scope)
	if switched.Projects[0].Branch != "topic" || switched.RepoRefs[scope.Key()].SwitchError != "" {
		t.Fatalf("branch switch was not applied: %+v", switched.RepoRefs[scope.Key()])
	}
	if actual := runGitTest(t, repository, "symbolic-ref", "--short", "HEAD"); actual != "topic" {
		t.Fatalf("physical branch = %q", actual)
	}
}

func TestWorktreeDefaultCurrentFollowsActualBranchAfterExternalSwitch(t *testing.T) {
	repository := temporaryGitRepository(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		goalResponse(w, model.Chat, "Done", nil, true)
	}))
	defer server.Close()
	core := modelCheckCore(t, model.Chat, server.URL)
	if err := core.Execute(t.Context(), contracts.Command{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: repository}}); err != nil {
		t.Fatal(err)
	}
	project := waitForProjectFiles(t, core).Projects[0]
	scope := contracts.RepoRefScope{ProjectID: project.ID, TargetID: "local", RepoPath: repository}
	if err := core.Execute(t.Context(), contracts.Command{Kind: "git.refs.load", Git: &contracts.GitRequest{Scope: scope}}); err != nil {
		t.Fatal(err)
	}
	waitGitRefs(t, core, scope)
	runGitTest(t, repository, "switch", "-c", "external-branch")
	stale, _ := core.Snapshot(t.Context())
	if stale.Projects[0].Branch != "main" {
		t.Fatal("fixture did not preserve the old refs cache")
	}
	send := contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
		ProjectID: project.ID, ModelID: "checks/unselected", Text: "Use the actual current checkout",
		Checkout: &contracts.CheckoutPlan{Kind: contracts.CheckoutCurrent, Branch: "main"},
	}}
	if err := core.Execute(t.Context(), send); err == nil {
		t.Fatal("an explicit stale branch silently executed somewhere else")
	}
	if state, _ := core.Snapshot(t.Context()); len(state.Sessions) != 0 || requests.Load() != 0 {
		t.Fatal("rejected explicit selection still created a conversation or called the provider")
	}
	send.Send.Checkout.Branch = ""
	if err := core.Execute(t.Context(), send); err != nil {
		t.Fatal(err)
	}
	state := waitIdle(t, core)
	session := state.Sessions[0]
	if session.Branch != "external-branch" || session.CWD != repository || session.SourceContext == nil ||
		session.SourceContext.Branch != "external-branch" || state.Projects[0].Branch != "external-branch" {
		t.Fatalf("default current checkout did not use authoritative Git state: %+v", session)
	}
}

func TestWorktreeRejectsInvalidSelectionAndUnavailableDataDirectory(t *testing.T) {
	repository := temporaryGitRepository(t)
	project := contracts.Project{Path: repository, GitDetected: true}
	for _, test := range []struct {
		name string
		plan contracts.CheckoutPlan
		path string
	}{
		{name: "missing base", plan: contracts.CheckoutPlan{Kind: contracts.CheckoutNew, Base: "not-a-real-ref"}, path: t.TempDir()},
		{name: "relative data directory", plan: contracts.CheckoutPlan{Kind: contracts.CheckoutNew}, path: "relative"},
		{name: "missing data directory", plan: contracts.CheckoutPlan{Kind: contracts.CheckoutNew}, path: filepath.Join(t.TempDir(), "unavailable")},
		{name: "foreign tree", plan: contracts.CheckoutPlan{Kind: contracts.CheckoutReuse, Path: t.TempDir(), Branch: "main"}, path: t.TempDir()},
		{name: "changed branch", plan: contracts.CheckoutPlan{Kind: contracts.CheckoutCurrent, Branch: "topic"}, path: t.TempDir()},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := prepareCheckout(context.Background(), project, test.plan, test.path)
			if err == nil {
				t.Fatal("invalid checkout was silently accepted")
			}
			if test.name == "missing data directory" {
				if _, err := os.Stat(test.path); !os.IsNotExist(err) {
					t.Fatal("unavailable data root was silently recreated")
				}
			}
		})
	}
}

func TestWorktreeRejectsLinkedManagedParent(t *testing.T) {
	for _, name := range []string{"relative link inside root", "external link"} {
		t.Run(name, func(t *testing.T) {
			root, target := t.TempDir(), t.TempDir()
			linkTarget := target
			if name == "relative link inside root" {
				target = filepath.Join(root, "other")
				if err := os.Mkdir(target, 0700); err != nil {
					t.Fatal(err)
				}
				linkTarget = "other"
			}
			sentinel := filepath.Join(target, "keep.txt")
			if err := os.WriteFile(sentinel, []byte("preserve user data"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(linkTarget, filepath.Join(root, "worktrees")); err != nil {
				if runtime.GOOS == "windows" {
					t.Skipf("directory symlink creation unavailable: %v", err)
				}
				t.Fatal(err)
			}
			parent, err := openWorktreeParent(root)
			if err == nil {
				_ = parent.Close()
				t.Fatal("new worktree followed a linked managed parent")
			}
			entries, err := os.ReadDir(target)
			if err != nil || len(entries) != 1 || entries[0].Name() != "keep.txt" {
				t.Fatalf("worktree reservation wrote into a linked target: %v, %v", entries, err)
			}
			if data, err := os.ReadFile(sentinel); err != nil || string(data) != "preserve user data" {
				t.Fatalf("linked target data changed: %q, %v", data, err)
			}
		})
	}
}

func TestWorktreeParentCreatesOnlyUnderExistingRoot(t *testing.T) {
	root := t.TempDir()
	parent, err := openWorktreeParent(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := parent.Mkdir("reserved", 0700); err != nil {
		_ = parent.Close()
		t.Fatal(err)
	}
	if err := parent.Close(); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join(root, "worktrees", "reserved")); err != nil || !info.IsDir() {
		t.Fatalf("returned worktree parent did not own the expected directory: %v", err)
	}
	missing := filepath.Join(t.TempDir(), "unavailable")
	if parent, err := openWorktreeParent(missing); err == nil {
		_ = parent.Close()
		t.Fatal("missing data root was accepted")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("opening a worktree parent recreated the missing data root")
	}
}

func TestWorktreeNewDirectoriesFollowRootAndReuseKeepsOriginalPath(t *testing.T) {
	repository := temporaryGitRepository(t)
	project := contracts.Project{Path: repository, GitDetected: true}
	firstRoot, secondRoot := t.TempDir(), t.TempDir()
	first, err := prepareCheckout(context.Background(), project, contracts.CheckoutPlan{Kind: contracts.CheckoutNew}, firstRoot)
	if err != nil {
		t.Fatal(err)
	}
	second, err := prepareCheckout(context.Background(), project, contracts.CheckoutPlan{Kind: contracts.CheckoutNew}, secondRoot)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(first.cwd) != filepath.Join(firstRoot, "worktrees") ||
		filepath.Dir(second.cwd) != filepath.Join(secondRoot, "worktrees") ||
		!validShortDirectoryID(filepath.Base(first.cwd)) || !validShortDirectoryID(filepath.Base(second.cwd)) ||
		first.branch == second.branch {
		t.Fatalf("new worktrees did not use independently owned data directories: first=%+v second=%+v", first, second)
	}
	reused, err := prepareCheckout(context.Background(), project, contracts.CheckoutPlan{
		Kind: contracts.CheckoutReuse, Path: first.cwd, Branch: first.branch, WorktreeID: first.worktreeID,
	}, secondRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !sameCheckoutPath(reused.cwd, first.cwd) || reused.owned {
		t.Fatal("reused worktree moved to the current data root or acquired deletion ownership")
	}
	if data, err := os.ReadFile(filepath.Join(first.cwd, "tracked.txt")); err != nil || string(data) != "baseline" {
		t.Fatalf("original worktree was changed by creating or reusing another tree: %q %v", data, err)
	}
}

func TestWorktreeRejectedMessagesDoNotCreateCheckouts(t *testing.T) {
	repository := temporaryGitRepository(t)
	core := modelCheckCore(t, model.Chat, "http://unused.invalid")
	if err := core.Execute(context.Background(), contracts.Command{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: repository}}); err != nil {
		t.Fatal(err)
	}
	state, _ := core.Snapshot(context.Background())
	parent := filepath.Join(core.storage.Directory(), "worktrees")
	for _, test := range []struct {
		name, text, effort string
		attachments        []contracts.Attachment
	}{
		{name: "missing command", text: "/does-not-exist"},
		{name: "missing skill", text: "/skill:does-not-exist"},
		{name: "missing attachment", text: "Read this", attachments: []contracts.Attachment{{ID: "missing"}}},
		{name: "invalid effort", text: "Inspect", effort: "high"},
		{name: "goal control without session", text: "/goal pause"},
	} {
		t.Run(test.name, func(t *testing.T) {
			for retry := 0; retry < 2; retry++ {
				err := core.Execute(context.Background(), contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
					ProjectID: state.Projects[0].ID, ModelID: "checks/unselected", Text: test.text,
					ReasoningLevel: test.effort, Attachments: test.attachments,
					Checkout: &contracts.CheckoutPlan{Kind: contracts.CheckoutNew},
				}})
				if err == nil {
					t.Fatal("invalid submission was accepted")
				}
			}
			if _, err := os.Stat(parent); !os.IsNotExist(err) {
				t.Fatal("invalid submission reached worktree creation")
			}
			repo, err := inspectGit(context.Background(), repository)
			if err != nil || len(repo.trees) != 1 || len(repo.refs) != 1 {
				t.Fatalf("rejected submission left Git artifacts: %+v %v", repo, err)
			}
		})
	}
}

func TestWorktreePersistFailureRollsBackOnlyOwnedCheckout(t *testing.T) {
	repository := temporaryGitRepository(t)
	core := modelCheckCore(t, model.Chat, "http://unused.invalid")
	if err := core.Execute(context.Background(), contracts.Command{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: repository}}); err != nil {
		t.Fatal(err)
	}
	state, _ := core.Snapshot(context.Background())
	path := filepath.Join(core.storage.ConfigDirectory(), "engine.json")
	backup := path + ".fixture-backup"
	if err := os.Rename(path, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Remove(path)
		_ = os.Rename(backup, path)
	}()
	for retry := 0; retry < 2; retry++ {
		err := core.Execute(context.Background(), contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
			ProjectID: state.Projects[0].ID, ModelID: "checks/unselected", Text: "Inspect",
			Checkout: &contracts.CheckoutPlan{Kind: contracts.CheckoutNew},
		}})
		if err == nil {
			t.Fatal("run was accepted despite persistence failure")
		}
		repo, err := inspectGit(context.Background(), repository)
		if err != nil || len(repo.trees) != 1 || len(repo.refs) != 1 {
			t.Fatalf("failed acceptance left worktree/branch: %+v %v", repo, err)
		}
		entries, err := os.ReadDir(filepath.Join(core.storage.Directory(), "worktrees"))
		if err != nil || len(entries) != 0 {
			t.Fatalf("rollback left checkout files: %v %v", entries, err)
		}
		current, _ := core.Snapshot(context.Background())
		if len(current.Sessions) != 0 || len(current.Runs) != 0 {
			t.Fatal("failed acceptance left phantom session/run")
		}
	}
}

func TestWorktreeRollbackPreservesChangedAndReusedCheckouts(t *testing.T) {
	repository := temporaryGitRepository(t)
	checkout, err := prepareCheckout(context.Background(), contracts.Project{Path: repository, GitDetected: true},
		contracts.CheckoutPlan{Kind: contracts.CheckoutNew}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(checkout.cwd, "user.txt"), []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := cleanupUnacceptedCheckout(checkout); err == nil || !strings.Contains(err.Error(), filepath.Base(checkout.cwd)) {
		t.Fatal("dirty checkout cleanup did not identify retained path")
	}
	if data, _ := os.ReadFile(filepath.Join(checkout.cwd, "user.txt")); string(data) != "preserve" {
		t.Fatal("cleanup removed user changes")
	}
	reused, err := prepareCheckout(context.Background(), contracts.Project{Path: repository, GitDetected: true},
		contracts.CheckoutPlan{Kind: contracts.CheckoutReuse, Path: checkout.cwd, Branch: checkout.branch}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := cleanupUnacceptedCheckout(reused); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(reused.cwd, "user.txt")); err != nil {
		t.Fatal("cleanup touched reused worktree")
	}
}

func TestWorktreeCurrentKeepsProjectSubdirectory(t *testing.T) {
	repository := temporaryGitRepository(t)
	subdirectory := filepath.Join(repository, "subproject")
	if err := os.Mkdir(subdirectory, 0700); err != nil {
		t.Fatal(err)
	}
	checkout, err := prepareCheckout(context.Background(), contracts.Project{Path: subdirectory, GitDetected: true},
		contracts.CheckoutPlan{Kind: contracts.CheckoutCurrent, Branch: "main"}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if checkout.cwd != subdirectory || !sameCheckoutPath(checkout.source.RepoRoot, repository) {
		t.Fatal("current checkout moved subdirectory project to the Git root")
	}
}

func TestWorktreeGoalAcceptsPreparedCheckout(t *testing.T) {
	repository := temporaryGitRepository(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		goalResponse(w, model.Chat, "Done", nil, true)
	}))
	t.Cleanup(server.Close)
	core := modelCheckCore(t, model.Chat, server.URL)
	if err := core.Execute(context.Background(), contracts.Command{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: repository}}); err != nil {
		t.Fatal(err)
	}
	state, _ := core.Snapshot(context.Background())
	if err := core.Execute(context.Background(), contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
		ProjectID: state.Projects[0].ID, ModelID: "checks/unselected", Text: "/goal Inspect checkout",
		Checkout: &contracts.CheckoutPlan{Kind: contracts.CheckoutNew},
	}}); err != nil {
		t.Fatal(err)
	}
	after, _ := core.Snapshot(context.Background())
	session := after.Sessions[0]
	if session.Goal == nil || session.CWD == repository || session.WorktreeID == "" ||
		session.SourceContext == nil || !strings.HasPrefix(session.Branch, "feat/") {
		t.Fatalf("goal ignored the prepared checkout: %+v", session)
	}
	if actual := runGitTest(t, session.CWD, "symbolic-ref", "--short", "HEAD"); actual != session.Branch {
		t.Fatalf("goal checkout was cleaned up after acceptance: %q", actual)
	}
}

func validShortDirectoryID(value string) bool {
	if len(value) != 12 {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') {
			return false
		}
	}
	return true
}

func temporaryGitRepository(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatal("Git is required for the real checkout tests")
	}
	directory := t.TempDir()
	runGitTest(t, directory, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(directory, "tracked.txt"), []byte("baseline"), 0600); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, directory, "add", "tracked.txt")
	runGitTest(t, directory, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "initial")
	return directory
}

func runGitTest(t *testing.T, directory string, args ...string) string {
	t.Helper()
	output, err := gitOutput(context.Background(), directory, args...)
	if err != nil {
		t.Fatal(err)
	}
	return output
}

func waitGitRefs(t *testing.T, core *Engine, scope contracts.RepoRefScope) contracts.Snapshot {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		state, err := core.Snapshot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		refs := state.RepoRefs[scope.Key()]
		if refs.State != contracts.LoadLoading && refs.Switching == "" {
			return state
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("Git operation did not finish")
	return contracts.Snapshot{}
}
