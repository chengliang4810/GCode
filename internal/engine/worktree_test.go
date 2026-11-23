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
	summary, err := core.Snapshot(context.Background())
	if err != nil || len(summary.Sessions) != 1 {
		t.Fatalf("conversation was not created: %+v, %v", summary.Sessions, err)
	}
	viewCtx, viewCancel := context.WithCancel(context.Background())
	defer viewCancel()
	view, err := core.Subscribe(viewCtx, summary.Sessions[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	<-view
	after := waitIdle(t, core, summary.Sessions[0].ID)
	session := after.Sessions[0]
	if filepath.Dir(session.CWD) != filepath.Join(store.Directory(), "worktrees", filepath.Base(repository)) || session.CWD == repository ||
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
	deduped, _ := core.Snapshot(context.Background(), session.ID)
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
	reopenedSummary, err := reopened.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("reopened conversation was not found: %+v, %v", reopenedSummary.Sessions, err)
	}
	var reusedSessionID string
	for _, candidate := range reopenedSummary.Sessions {
		if candidate.ID != session.ID {
			reusedSessionID = candidate.ID
			break
		}
	}
	if reusedSessionID == "" {
		t.Fatalf("reused conversation was not created: %+v", reopenedSummary.Sessions)
	}
	reused := waitIdle(t, reopened, reusedSessionID)
	var reusedSession contracts.Session
	for _, candidate := range reused.Sessions {
		if candidate.ID == reusedSessionID {
			reusedSession = candidate
			break
		}
	}
	if reusedSession.ID == "" || !sameCheckoutPath(reusedSession.CWD, session.CWD) || len(reused.Projects[0].Worktrees) != 2 {
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
	summary, err := core.Snapshot(t.Context())
	if err != nil || len(summary.Sessions) != 1 {
		t.Fatalf("conversation was not created: %+v, %v", summary.Sessions, err)
	}
	state := waitIdle(t, core, summary.Sessions[0].ID)
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

func TestWorktreeNewRejectsSourceRepositoryNesting(t *testing.T) {
	for _, name := range []string{
		"data inside repository", "data equals repository", "data alias into repository",
		"case alias data inside repository", "case alias data equals repository",
		"source alias", "source subdirectory", "registered source worktree", "group equals source root",
	} {
		t.Run(name, func(t *testing.T) {
			repository := temporaryGitRepository(t)
			source, data := repository, filepath.Join(repository, "data")
			switch name {
			case "data equals repository":
				data = repository
			case "case alias data inside repository", "case alias data equals repository":
				source = filepath.Join(t.TempDir(), "RePoSiToRy")
				if err := os.Rename(repository, source); err != nil {
					t.Fatal(err)
				}
				alias := filepath.Join(filepath.Dir(source), strings.ToLower(filepath.Base(source)))
				sourceInfo, err := os.Stat(source)
				if err != nil {
					t.Fatal(err)
				}
				aliasInfo, err := os.Stat(alias)
				if os.IsNotExist(err) {
					t.Skip("test filesystem is case-sensitive")
				}
				if err != nil {
					t.Fatal(err)
				}
				if !os.SameFile(sourceInfo, aliasInfo) {
					t.Skip("case alias does not identify the same repository")
				}
				data = alias
				if name == "case alias data inside repository" {
					data = filepath.Join(alias, "data")
				}
			case "data alias into repository", "source alias":
				alias := filepath.Join(t.TempDir(), "alias")
				target := repository
				if name == "data alias into repository" {
					target = data
					if err := os.Mkdir(target, 0700); err != nil {
						t.Fatal(err)
					}
					data = alias
				} else {
					source = alias
				}
				if err := os.Symlink(target, alias); err != nil {
					if runtime.GOOS == "windows" {
						t.Skipf("directory symlink creation unavailable: %v", err)
					}
					t.Fatal(err)
				}
			case "source subdirectory":
				source = filepath.Join(repository, "subproject")
				if err := os.Mkdir(source, 0700); err != nil {
					t.Fatal(err)
				}
			case "registered source worktree":
				linked := filepath.Join(t.TempDir(), "linked")
				runGitTest(t, repository, "worktree", "add", "-b", "source-tree", "--", linked, "HEAD")
				source, data = filepath.Join(linked, "subproject"), filepath.Join(linked, "data")
				if err := os.Mkdir(source, 0700); err != nil {
					t.Fatal(err)
				}
			case "group equals source root":
				data = t.TempDir()
				source = filepath.Join(data, "worktrees", "repository")
				if err := os.Mkdir(filepath.Dir(source), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(repository, source); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.MkdirAll(data, 0700); err != nil {
				t.Fatal(err)
			}
			repo, err := inspectGit(t.Context(), source)
			if err != nil {
				t.Fatal(err)
			}
			managed := filepath.Join(data, "worktrees")
			group := filepath.Join(managed, worktreeProjectName(filepath.Base(repo.root)))
			if name == "group equals source root" {
				groupInfo, err := os.Stat(group)
				if err != nil {
					t.Fatal(err)
				}
				repoInfo, err := os.Stat(repo.root)
				if err != nil {
					t.Fatal(err)
				}
				if !os.SameFile(groupInfo, repoInfo) {
					t.Fatal("test grouping must identify the source root itself")
				}
			}
			_, managedBefore := os.Lstat(managed)
			_, groupBefore := os.Lstat(group)
			refsBefore := runGitTest(t, source, "for-each-ref", "--format=%(refname):%(objectname)", "refs/heads")
			treesBefore := runGitTest(t, source, "worktree", "list", "--porcelain")
			statusBefore := runGitTest(t, source, "status", "--porcelain", "--untracked-files=all")
			project := contracts.Project{Path: source, GitDetected: true}
			checkout, err := prepareCheckout(t.Context(), project, contracts.CheckoutPlan{Kind: contracts.CheckoutNew}, data)
			if err == nil || !strings.Contains(err.Error(), "must be outside source repository") {
				t.Fatalf("nested checkout was not explicitly rejected: checkout=%+v error=%v", checkout, err)
			}
			for _, path := range []struct {
				name   string
				before error
			}{{managed, managedBefore}, {group, groupBefore}} {
				if os.IsNotExist(path.before) {
					if _, err := os.Lstat(path.name); !os.IsNotExist(err) {
						t.Fatalf("rejected checkout left a directory at %q: %v", path.name, err)
					}
				}
			}
			if refs := runGitTest(t, source, "for-each-ref", "--format=%(refname):%(objectname)", "refs/heads"); refs != refsBefore {
				t.Fatalf("rejected checkout changed branches: before=%q after=%q", refsBefore, refs)
			}
			if trees := runGitTest(t, source, "worktree", "list", "--porcelain"); trees != treesBefore {
				t.Fatalf("rejected checkout changed registered worktrees: before=%q after=%q", treesBefore, trees)
			}
			if status := runGitTest(t, source, "status", "--porcelain", "--untracked-files=all"); status != statusBefore {
				t.Fatalf("rejected checkout changed repository files: before=%q after=%q", statusBefore, status)
			}
		})
	}
}

func TestWorktreeNewAllowsSiblingAndExternalData(t *testing.T) {
	for _, name := range []string{"sibling with repository prefix", "external", "external alias", "registered source worktree", "separate git directory"} {
		t.Run(name, func(t *testing.T) {
			repository := temporaryGitRepository(t)
			data := t.TempDir()
			switch name {
			case "sibling with repository prefix":
				parent := t.TempDir()
				source := filepath.Join(parent, "repository")
				if err := os.Rename(repository, source); err != nil {
					t.Fatal(err)
				}
				repository, data = source, source+"-data"
			case "external alias":
				alias := filepath.Join(t.TempDir(), "alias")
				if err := os.Symlink(data, alias); err != nil {
					if runtime.GOOS == "windows" {
						t.Skipf("directory symlink creation unavailable: %v", err)
					}
					t.Fatal(err)
				}
				data = alias
			case "registered source worktree":
				linked := filepath.Join(t.TempDir(), "linked")
				runGitTest(t, repository, "worktree", "add", "-b", "source-tree", "--", linked, "HEAD")
				repository, data = linked, filepath.Dir(linked)
			case "separate git directory":
				// The common-dir's parent is not a checkout and remains a valid data root.
				runGitTest(t, repository, "init", "--separate-git-dir", filepath.Join(data, "metadata"))
			}
			if err := os.MkdirAll(data, 0700); err != nil {
				t.Fatal(err)
			}
			project := contracts.Project{Path: repository, GitDetected: true}
			checkout, err := prepareCheckout(t.Context(), project, contracts.CheckoutPlan{Kind: contracts.CheckoutNew}, data)
			if err != nil {
				t.Fatal(err)
			}
			if filepath.Dir(checkout.cwd) != filepath.Join(data, "worktrees", worktreeProjectName(filepath.Base(repository))) ||
				!validShortDirectoryID(filepath.Base(checkout.cwd)) || checkout.branch != "feat/"+filepath.Base(checkout.cwd) {
				t.Fatalf("external checkout layout changed: %+v", checkout)
			}
			if err := cleanupUnacceptedCheckout(checkout); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(checkout.cwd); !os.IsNotExist(err) {
				t.Fatalf("rollback retained the unaccepted checkout: %v", err)
			}
			if branches := runGitTest(t, repository, "branch", "--list", checkout.branch); branches != "" {
				t.Fatalf("rollback retained branch %q: %s", checkout.branch, branches)
			}
		})
	}
}

func TestWorktreeProjectName(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ name, input, want string }{
		{name: "repository", input: "GCode", want: "GCode"},
		{name: "unicode and space", input: "我的 项目", want: "我的-项目"},
		{name: "separators", input: `../a\b:c*?`, want: "a-b-c"},
		{name: "empty", input: "", want: "project"},
		{name: "dots", input: "..", want: "project"},
		{name: "device", input: "CON.txt", want: "_CON.txt"},
		{name: "port", input: "lpt1", want: "_lpt1"},
		{name: "ordinary prefix", input: "console", want: "console"},
		{name: "long unicode", input: strings.Repeat("项", 100), want: strings.Repeat("项", 33)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := worktreeProjectName(test.input); got != test.want {
				t.Fatalf("project directory = %q, want %q", got, test.want)
			}
		})
	}
}

func TestWorktreeNewGroupsByRepositoryNotProjectTitle(t *testing.T) {
	data := t.TempDir()
	seen := map[string]bool{}
	for _, name := range []string{"GCode", "GCode", "项目 alpha", "OtherProject"} {
		repository := filepath.Join(t.TempDir(), name)
		if err := os.Rename(temporaryGitRepository(t), repository); err != nil {
			t.Fatal(err)
		}
		subdirectory := filepath.Join(repository, "subproject")
		if err := os.Mkdir(subdirectory, 0700); err != nil {
			t.Fatal(err)
		}
		project := contracts.Project{Path: subdirectory, Name: "Renamed display title", GitDetected: true}
		checkout, err := prepareCheckout(t.Context(), project, contracts.CheckoutPlan{Kind: contracts.CheckoutNew}, data)
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Dir(checkout.cwd) != filepath.Join(data, "worktrees", worktreeProjectName(name)) || seen[checkout.cwd] ||
			!validShortDirectoryID(filepath.Base(checkout.cwd)) || checkout.branch != "feat/"+filepath.Base(checkout.cwd) {
			t.Fatalf("checkout did not use a unique repository grouping: %+v", checkout)
		}
		seen[checkout.cwd] = true
		if err := cleanupUnacceptedCheckout(checkout); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWorktreeRejectsLinkedProjectParent(t *testing.T) {
	for _, kind := range []string{"internal link", "external link", "file"} {
		t.Run(kind, func(t *testing.T) {
			root, target := t.TempDir(), t.TempDir()
			managed := filepath.Join(root, "worktrees")
			if err := os.Mkdir(managed, 0700); err != nil {
				t.Fatal(err)
			}
			if kind == "internal link" {
				target = filepath.Join(managed, "other-project")
				if err := os.Mkdir(target, 0700); err != nil {
					t.Fatal(err)
				}
			}
			sentinel := filepath.Join(target, "keep.txt")
			if err := os.WriteFile(sentinel, []byte("preserve"), 0600); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(managed, "GCode")
			if kind == "file" {
				if err := os.WriteFile(path, []byte("not a directory"), 0600); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Symlink(target, path); err != nil {
				if runtime.GOOS == "windows" {
					t.Skipf("symlink creation unavailable: %v", err)
				}
				t.Fatal(err)
			}
			if parent, err := openWorktreeParent(root, "GCode"); err == nil {
				_ = parent.Close()
				t.Fatal("project directory alias or file was accepted")
			}
			if entries, err := os.ReadDir(target); err != nil || len(entries) != 1 || entries[0].Name() != "keep.txt" {
				t.Fatalf("target was changed: %v %v", entries, err)
			}
			if body, err := os.ReadFile(sentinel); err != nil || string(body) != "preserve" {
				t.Fatalf("sentinel was changed: %q %v", body, err)
			}
		})
	}
}

func TestWorktreeRejectsInvalidProjectDirectory(t *testing.T) {
	for _, name := range []string{"", ".", "..", "../escape", `a\b`, "a/b", "/absolute"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if parent, err := openWorktreeParent(root, name); err == nil {
				_ = parent.Close()
				t.Fatal("invalid directory component was accepted")
			}
			if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
				t.Fatalf("invalid name created directories: %v %v", entries, err)
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
			parent, err := openWorktreeParent(root, "GCode")
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
	parent, err := openWorktreeParent(root, "GCode")
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
	if info, err := os.Stat(filepath.Join(root, "worktrees", "GCode", "reserved")); err != nil || !info.IsDir() {
		t.Fatalf("returned worktree parent did not own the expected directory: %v", err)
	}
	missing := filepath.Join(t.TempDir(), "unavailable")
	if parent, err := openWorktreeParent(missing, "GCode"); err == nil {
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
	if filepath.Dir(first.cwd) != filepath.Join(firstRoot, "worktrees", filepath.Base(repository)) ||
		filepath.Dir(second.cwd) != filepath.Join(secondRoot, "worktrees", filepath.Base(repository)) ||
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

func TestWorktreeExistingNestedCurrentAndReuseKeepPaths(t *testing.T) {
	repository := temporaryGitRepository(t)
	data := filepath.Join(repository, "data")
	original := filepath.Join(data, "worktrees", "existing-checkout")
	runGitTest(t, repository, "worktree", "add", "-b", "existing", "--", original, "HEAD")
	project := contracts.Project{Path: repository, GitDetected: true}
	for _, test := range []struct {
		name string
		plan contracts.CheckoutPlan
		cwd  string
	}{
		{name: "current", plan: contracts.CheckoutPlan{Kind: contracts.CheckoutCurrent}, cwd: repository},
		{name: "reuse", plan: contracts.CheckoutPlan{Kind: contracts.CheckoutReuse, Path: original, Branch: "existing"}, cwd: original},
	} {
		t.Run(test.name, func(t *testing.T) {
			checkout, err := prepareCheckout(t.Context(), project, test.plan, data)
			if err != nil {
				t.Fatal(err)
			}
			if !sameCheckoutPath(checkout.cwd, test.cwd) || checkout.owned {
				t.Fatalf("existing checkout moved or acquired cleanup ownership: %+v", checkout)
			}
			if err := cleanupUnacceptedCheckout(checkout); err != nil {
				t.Fatal(err)
			}
			if body, err := os.ReadFile(filepath.Join(checkout.cwd, "tracked.txt")); err != nil || string(body) != "baseline" {
				t.Fatalf("existing checkout changed: %q %v", body, err)
			}
		})
	}
	if _, err := os.Lstat(filepath.Join(data, "worktrees", worktreeProjectName(filepath.Base(repository)))); !os.IsNotExist(err) {
		t.Fatalf("current/reuse created the new project grouping: %v", err)
	}
	if branch := runGitTest(t, original, "symbolic-ref", "--short", "HEAD"); branch != "existing" {
		t.Fatalf("reused worktree branch changed: %q", branch)
	}
}

func TestWorktreeReuseKeepsRegisteredPathOutsideProjectGrouping(t *testing.T) {
	repository := temporaryGitRepository(t)
	data := t.TempDir()
	original := filepath.Join(data, "worktrees", "existing-checkout")
	runGitTest(t, repository, "worktree", "add", "-b", "existing", "--", original, "HEAD")
	project := contracts.Project{Path: repository, GitDetected: true}
	reused, err := prepareCheckout(t.Context(), project, contracts.CheckoutPlan{
		Kind: contracts.CheckoutReuse, Path: original, Branch: "existing",
	}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !sameCheckoutPath(reused.cwd, original) || reused.owned {
		t.Fatalf("registered checkout was relocated or acquired cleanup ownership: %+v", reused)
	}
	if err := cleanupUnacceptedCheckout(reused); err != nil {
		t.Fatal(err)
	}
	if body, err := os.ReadFile(filepath.Join(original, "tracked.txt")); err != nil || string(body) != "baseline" {
		t.Fatalf("existing worktree changed: %q %v", body, err)
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
		entries, err := os.ReadDir(filepath.Join(core.storage.Directory(), "worktrees", filepath.Base(repository)))
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
	summary, err := core.Snapshot(context.Background())
	if err != nil || len(summary.Sessions) != 1 {
		t.Fatalf("goal session was not created: %+v, %v", summary.Sessions, err)
	}
	sessionID := summary.Sessions[0].ID
	after, err := core.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	var session contracts.Session
	for _, candidate := range after.Sessions {
		if candidate.ID == sessionID {
			session = candidate
			break
		}
	}
	if session.ID == "" {
		t.Fatalf("goal session was not returned: %+v", after.Sessions)
	}
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
