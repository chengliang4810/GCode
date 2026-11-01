package engine

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/processutil"
	"gcode/internal/repohost"
)

type gitRepository struct {
	pullRequestsURL    string
	root, branch, head string
	projectPath        string
	projectBranch      string
	refs               []contracts.RepoRef
	trees              []contracts.Worktree
}

type checkoutResult struct {
	cwd, branch, worktreeID string
	projectID, projectPath  string
	storageRootID           string
	source                  *contracts.ConversationSourceContext
	repo                    *gitRepository
	owned, accepted         bool
	baseSHA                 string
	prompt                  string
	input                   *model.Message
}

func gitOutput(ctx context.Context, directory string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", append([]string{"-c", "core.quotePath=false", "-C", directory}, args...)...)
	processutil.HideConsole(command)
	command.WaitDelay = 2 * time.Second
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		// Git diagnostics concern only the explicit checkout command, never
		// provider credentials or environment contents.
		return "", fmt.Errorf("git %s: %s", args[0], strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func worktreeIdentity(path string) string {
	canonical := canonicalCheckoutPath(path)
	if volume := filepath.VolumeName(canonical); volume != "" {
		canonical = strings.ToLower(canonical)
	}
	return fmt.Sprintf("worktree-%x", sha256.Sum256([]byte(canonical)))[:29]
}

func canonicalCheckoutPath(path string) string {
	path = filepath.Clean(path)
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	if filepath.VolumeName(path) != "" {
		path = strings.ToLower(path)
	}
	return path
}

func sameCheckoutPath(left, right string) bool {
	return canonicalCheckoutPath(left) == canonicalCheckoutPath(right)
}

func sameRecordedPath(left, right string) bool {
	left, right = filepath.Clean(left), filepath.Clean(right)
	if filepath.VolumeName(left) != "" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func inspectGit(ctx context.Context, directory string) (*gitRepository, error) {
	root, err := gitOutput(ctx, directory, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	root = filepath.Clean(root)
	branch, _ := gitOutput(ctx, directory, "symbolic-ref", "--quiet", "--short", "HEAD")
	head, _ := gitOutput(ctx, directory, "rev-parse", "--verify", "HEAD")
	data, err := gitOutput(ctx, directory, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	repo := &gitRepository{root: root, branch: branch, head: head, projectPath: directory, projectBranch: branch}
	if remote, err := gitOutput(ctx, directory, "config", "--get", "remote.origin.url"); err == nil {
		repo.pullRequestsURL = repohost.PullRequestsURL(remote)
	}
	var tree contracts.Worktree
	for _, field := range strings.Split(data, "\x00") {
		if strings.HasPrefix(field, "worktree ") {
			if tree.Path != "" {
				repo.trees = append(repo.trees, tree)
			}
			path := filepath.Clean(strings.TrimPrefix(field, "worktree "))
			tree = contracts.Worktree{ID: worktreeIdentity(path), Path: path, Name: filepath.Base(path)}
		} else if strings.HasPrefix(field, "branch refs/heads/") {
			tree.Branch = strings.TrimPrefix(field, "branch refs/heads/")
		}
	}
	if tree.Path != "" {
		repo.trees = append(repo.trees, tree)
	}
	refs, err := gitOutput(ctx, directory, "for-each-ref", "--format=%(refname:short)", "refs/heads", "refs/remotes")
	if err != nil {
		return nil, err
	}
	for _, name := range strings.Split(refs, "\n") {
		name = strings.TrimSpace(name)
		if name == "" || strings.HasSuffix(name, "/HEAD") {
			continue
		}
		ref := contracts.RepoRef{Name: name, Current: name == branch}
		for _, tree := range repo.trees {
			if tree.Branch == name {
				ref.WorktreePath = tree.Path
				break
			}
		}
		repo.refs = append(repo.refs, ref)
	}
	return repo, nil
}

func (e *Engine) applyGitProjectLocked(projectID string, repo *gitRepository) {
	if repo == nil {
		return
	}
	for i := range e.state.Projects {
		project := &e.state.Projects[i]
		if project.ID != projectID {
			continue
		}
		project.GitDetected, project.Worktrees = true, repo.trees
		project.PullRequestsURL = repo.pullRequestsURL
		// A newly created worktree does not switch the project's main checkout.
		if sameRecordedPath(project.Path, repo.projectPath) {
			project.Branch = repo.projectBranch
		}
		project.Branches = nil
		for _, ref := range repo.refs {
			project.Branches = append(project.Branches, ref.Name)
		}
		for key, refs := range e.state.RepoRefs {
			if refs.Scope.ProjectID != projectID {
				continue
			}
			refs.Rows = append([]contracts.RepoRef(nil), repo.refs...)
			for i := range refs.Rows {
				refs.Rows[i].Current = refs.Rows[i].Name == project.Branch
			}
			e.state.RepoRefs[key] = refs
		}
	}
}

func (e *Engine) gitOperationLocked(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	key := id("git-operation")
	if e.gitOperations == nil {
		e.gitOperations = map[string]context.CancelFunc{}
	}
	e.gitOperations[key] = cancel
	e.wg.Add(1)
	return ctx, func() {
		cancel()
		e.mu.Lock()
		delete(e.gitOperations, key)
		e.mu.Unlock()
		e.wg.Done()
	}
}

func (e *Engine) projectForGitLocked(scope contracts.RepoRefScope) (contracts.Project, error) {
	if scope.TargetID != "local" || scope.CloudRepository != "" {
		return contracts.Project{}, errors.New("choose a project on this host")
	}
	for _, project := range e.state.Projects {
		if project.ID == scope.ProjectID && sameRecordedPath(project.Path, scope.RepoPath) {
			return project, nil
		}
	}
	return contracts.Project{}, errors.New("repository does not match the selected project")
}

func (e *Engine) gitCommand(ctx context.Context, command contracts.Command) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.checkLocked(ctx); err != nil {
		return err
	}
	request := *command.Git
	project, err := e.projectForGitLocked(request.Scope)
	if err != nil {
		return err
	}
	key := request.Scope.Key()
	if e.state.RepoRefs == nil {
		e.state.RepoRefs = map[string]contracts.RepoRefs{}
	}
	current := e.state.RepoRefs[key]
	for _, refs := range e.state.RepoRefs {
		if refs.Scope.ProjectID == project.ID && (refs.State == contracts.LoadLoading || refs.Switching != "") {
			return errors.New("repository operation is already running")
		}
	}
	switching := command.Kind == "git.ref.switch"
	if switching {
		if e.checkoutProjects[project.ID] != 0 {
			return errors.New("wait for checkout preparation to finish before switching branches")
		}
		for sid := range e.runs {
			if index := e.sessionIndex(sid); index >= 0 && e.state.Sessions[index].ProjectID == project.ID {
				return errors.New("stop active project sessions before switching branches")
			}
		}
		current.Switching, current.SwitchError = request.Ref, ""
	} else {
		current.State, current.Error = contracts.LoadLoading, ""
	}
	current.Scope, current.Generation = request.Scope, current.Generation+1
	e.state.RepoRefs[key] = current
	jobCtx, finish := e.gitOperationLocked(context.Background())
	e.publishLocked(contracts.Event{Kind: "git.refs.changed"})
	go func() {
		defer finish()
		var repo *gitRepository
		var jobErr error
		if switching {
			_, jobErr = gitOutput(jobCtx, project.Path, "switch", "--", request.Ref)
		}
		if jobErr == nil {
			repo, jobErr = inspectGit(jobCtx, project.Path)
		}
		e.mu.Lock()
		defer e.mu.Unlock()
		if e.closed {
			return
		}
		row := e.state.RepoRefs[key]
		if _, err := e.projectForGitLocked(request.Scope); err != nil {
			delete(e.state.RepoRefs, key)
			return
		}
		row.Switching = ""
		if jobErr != nil {
			if switching {
				row.SwitchError = jobErr.Error()
			} else {
				row.State, row.Error = contracts.LoadError, jobErr.Error()
			}
		} else {
			row.State, row.Error, row.SwitchError, row.Rows = contracts.LoadReady, "", "", repo.refs
			e.applyGitProjectLocked(project.ID, repo)
			for i := range e.state.Sessions {
				session := &e.state.Sessions[i]
				sameRoot := session.SourceContext != nil && sameRecordedPath(session.SourceContext.RepoRoot, repo.root)
				if session.ProjectID == project.ID && (sameRecordedPath(session.CWD, project.Path) || sameRoot) {
					session.Branch = repo.branch
					session.SourceContext = sourceContext(repo, session.CWD, session.WorktreeID)
				}
			}
		}
		e.state.RepoRefs[key] = row
		if err := e.persistLocked(); err != nil {
			row.State, row.Error = contracts.LoadError, "Repository state could not be saved."
			e.state.RepoRefs[key] = row
		}
		e.publishLocked(contracts.Event{Kind: "git.refs.changed"})
	}()
	return nil
}

func sourceContext(repo *gitRepository, cwd, treeID string) *contracts.ConversationSourceContext {
	if treeID == "" {
		treeID = worktreeIdentity(repo.root)
	}
	return &contracts.ConversationSourceContext{CheckoutID: treeID, RepoRoot: repo.root, CWD: cwd, Branch: repo.branch, HeadSHA: repo.head, ObservedAt: time.Now()}
}

// Checkout preparation runs on the service caller's worker, outside the state
// mutex. The UI still communicates with this core using typed Go commands.
func (e *Engine) checkoutSend(ctx context.Context, command contracts.Command) error {
	e.mu.Lock()
	if err := e.checkLocked(ctx); err != nil {
		e.mu.Unlock()
		return err
	}
	projectID := command.Send.ProjectID
	if projectID == "" {
		projectID = command.ProjectID
	}
	var project contracts.Project
	for _, candidate := range e.state.Projects {
		if candidate.ID == projectID {
			project = candidate
			break
		}
	}
	if project.ID == "" {
		e.mu.Unlock()
		if projectID == "" && command.Send.Checkout.Kind == contracts.CheckoutCurrent && command.Send.Checkout.Branch == "" {
			return e.executeCommand(ctx, command, nil)
		}
		return errors.New("choose a project before selecting a checkout")
	}
	if strings.TrimSpace(command.Send.Text) == "" && len(command.Send.Attachments) == 0 {
		e.mu.Unlock()
		return errors.New("message is empty")
	}
	if command.Send.RequestID != "" {
		for _, run := range e.state.Runs {
			if run.RequestID == command.Send.RequestID {
				e.mu.Unlock()
				return nil
			}
		}
		if e.checkoutRequests[command.Send.RequestID] {
			e.mu.Unlock()
			return errors.New("this request's checkout is already being prepared")
		}
	}
	modelID := command.Send.ModelID
	if modelID == "" && len(e.state.Models) > 0 {
		modelID = e.state.Models[0].ID
	}
	provider, selected, err := e.resolveModel(modelID)
	if err != nil {
		e.mu.Unlock()
		return err
	}
	if _, err := selected.ResolveEffort(command.Send.ReasoningLevel); err != nil {
		e.mu.Unlock()
		return err
	}
	if _, err := e.client.Bind(provider, selected); err != nil {
		e.mu.Unlock()
		return err
	}
	if command.Send.Delivery != "" && command.Send.Delivery != "follow-up" && command.Send.Delivery != "steer" {
		e.mu.Unlock()
		return errors.New("invalid message delivery")
	}
	prompt := command.Send.Text
	fields := strings.Fields(contracts.PlainPrompt(prompt))
	goal := len(fields) > 0 && fields[0] == "/goal" && len(command.Send.Attachments) == 0
	if goal {
		if _, _, err := promptResources(e.storage.ConfigDirectory(), prompt); err != nil {
			e.mu.Unlock()
			return err
		}
		objective := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(contracts.PlainPrompt(prompt)), "/goal"))
		if objective == "pause" || objective == "resume" || objective == "clear" {
			e.mu.Unlock()
			return errors.New("no goal is set in this conversation")
		}
		if err := (&contracts.GoalInput{Objective: objective}).Validate(); err != nil {
			e.mu.Unlock()
			return err
		}
	} else {
		prompt, err = expandPrompt(e.storage.ConfigDirectory(), project.Path, prompt)
		if err != nil {
			e.mu.Unlock()
			return err
		}
	}
	input, err := e.inputMessageLocked(prompt, command.Send.Attachments)
	if err != nil {
		e.mu.Unlock()
		return err
	}
	for _, refs := range e.state.RepoRefs {
		if refs.Scope.ProjectID == project.ID && refs.Switching != "" {
			e.mu.Unlock()
			return errors.New("wait for the project's branch switch to finish")
		}
	}
	dataDirectory := e.storage.Directory()
	storageRootID := e.storage.ActiveRootID()
	if command.Send.Checkout.Kind == contracts.CheckoutNew {
		dataDirectory, err = e.storage.Resolve(storageRootID)
		if err != nil {
			e.mu.Unlock()
			return err
		}
	}
	if e.checkoutProjects == nil {
		e.checkoutProjects = map[string]int{}
	}
	e.checkoutProjects[project.ID]++
	if e.checkoutRequests == nil {
		e.checkoutRequests = map[string]bool{}
	}
	if command.Send.RequestID != "" {
		e.checkoutRequests[command.Send.RequestID] = true
	}
	jobCtx, finish := e.gitOperationLocked(ctx)
	e.mu.Unlock()
	defer finish()
	defer func() {
		e.mu.Lock()
		e.checkoutProjects[project.ID]--
		delete(e.checkoutRequests, command.Send.RequestID)
		e.mu.Unlock()
	}()
	checkout, err := prepareCheckout(jobCtx, project, *command.Send.Checkout, dataDirectory)
	if err != nil {
		return errors.Join(err, cleanupUnacceptedCheckout(checkout))
	}
	checkout.projectID, checkout.projectPath = project.ID, project.Path
	checkout.storageRootID = storageRootID
	checkout.prompt, checkout.input = prompt, &input
	if err := e.executeCommand(jobCtx, command, checkout); err != nil {
		return errors.Join(err, cleanupUnacceptedCheckout(checkout))
	}
	// A non-run command cannot silently consume a newly created checkout.
	if checkout.owned && !checkout.accepted {
		return errors.Join(errors.New("the message did not accept the prepared checkout"), cleanupUnacceptedCheckout(checkout))
	}
	return nil
}

func prepareCheckout(ctx context.Context, project contracts.Project, plan contracts.CheckoutPlan, dataDirectory string) (*checkoutResult, error) {
	if plan.Kind == contracts.CheckoutCloud {
		return nil, errors.New("cloud checkouts are not available on this host")
	}
	repo, err := inspectGit(ctx, project.Path)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if plan.Kind == contracts.CheckoutCurrent && !project.GitDetected && plan.Branch == "" {
			// Plain directories remain valid projects. Other checkout choices
			// require Git and must never silently fall back to this directory.
			return &checkoutResult{cwd: project.Path}, nil
		}
		return nil, err
	}
	checkout := &checkoutResult{cwd: project.Path, branch: repo.branch, repo: repo}
	switch plan.Kind {
	case contracts.CheckoutCurrent:
		if plan.Branch != "" && plan.Branch != repo.branch {
			return nil, errors.New("the selected branch changed; refresh repository refs")
		}
	case contracts.CheckoutReuse:
		found := false
		for _, tree := range repo.trees {
			if sameCheckoutPath(tree.Path, plan.Path) && tree.Branch == plan.Branch && (plan.WorktreeID == "" || plan.WorktreeID == tree.ID) {
				checkout.cwd, checkout.branch, checkout.worktreeID, found = tree.Path, tree.Branch, tree.ID, true
				break
			}
		}
		if !found {
			return nil, errors.New("the selected worktree no longer belongs to this repository")
		}
	case contracts.CheckoutNew:
		base := plan.Base
		if base == "" {
			base = "HEAD"
		}
		sha, err := gitOutput(ctx, project.Path, "rev-parse", "--verify", "--end-of-options", base+"^{commit}")
		if err != nil {
			return nil, err
		}
		parent := filepath.Join(dataDirectory, "worktrees")
		worktrees, err := openWorktreeParent(dataDirectory)
		if err != nil {
			return nil, err
		}
		defer worktrees.Close()
		var name string
		for {
			name = newShortID()
			checkout.cwd = filepath.Join(parent, name)
			if err := worktrees.Mkdir(name, 0700); errors.Is(err, os.ErrExist) {
				continue
			} else if err != nil {
				return nil, fmt.Errorf("reserve worktree directory: %w", err)
			}
			break
		}
		checkout.branch, checkout.worktreeID = "feat/"+name, worktreeIdentity(checkout.cwd)
		checkout.baseSHA, checkout.projectPath = sha, project.Path
		if _, err := gitOutput(ctx, project.Path, "show-ref", "--verify", "--quiet", "refs/heads/"+checkout.branch); err == nil {
			_ = worktrees.Remove(name)
			return nil, errors.New("new worktree branch already exists")
		}
		checkout.owned = true
		if _, err := gitOutput(ctx, project.Path, "worktree", "add", "-b", checkout.branch, "--", checkout.cwd, sha); err != nil {
			// An empty reservation can be removed safely. If Git or another
			// process added content, the normal ownership checks retain it.
			_ = worktrees.Remove(name)
			return checkout, err
		}
	default:
		return nil, errors.New("unknown checkout kind")
	}
	selected, err := inspectGit(ctx, checkout.cwd)
	if err != nil {
		return checkout, err
	}
	if plan.Kind == contracts.CheckoutNew {
		checkout.worktreeID = worktreeIdentity(checkout.cwd)
	}
	selected.projectPath, selected.projectBranch = project.Path, repo.branch
	checkout.repo, checkout.source = selected, sourceContext(selected, checkout.cwd, checkout.worktreeID)
	return checkout, nil
}

func openWorktreeParent(dataDirectory string) (*os.Root, error) {
	info, err := os.Stat(dataDirectory)
	if err != nil {
		return nil, fmt.Errorf("access worktree data directory: %w", err)
	}
	if !filepath.IsAbs(dataDirectory) || !info.IsDir() {
		return nil, errors.New("worktree data directory must be an existing absolute directory")
	}
	root, err := os.OpenRoot(dataDirectory)
	if err != nil {
		return nil, fmt.Errorf("open worktree data directory: %w", err)
	}
	defer root.Close()
	if err := root.Mkdir("worktrees", 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, fmt.Errorf("create worktree parent: %w", err)
	}
	parent, err := openRealDirectory(root, "worktrees")
	if err != nil {
		return nil, fmt.Errorf("open worktree parent: %w", err)
	}
	return parent, nil
}

// Only a checkout created by this dispatch and not accepted by a persisted run
// is eligible. Never force deletion or remove an existing/reused worktree.
func cleanupUnacceptedCheckout(checkout *checkoutResult) error {
	if checkout == nil || !checkout.owned || checkout.accepted {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	retained := func(cause error) error {
		return fmt.Errorf("unaccepted checkout retained at %q on branch %q: %w", checkout.cwd, checkout.branch, cause)
	}
	if info, err := os.Lstat(checkout.cwd); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return retained(errors.New("checkout directory identity changed"))
		}
		branch, err := gitOutput(ctx, checkout.cwd, "symbolic-ref", "--short", "HEAD")
		if err != nil || branch != checkout.branch {
			return retained(errors.New("checkout identity changed"))
		}
		head, err := gitOutput(ctx, checkout.cwd, "rev-parse", "--verify", "HEAD")
		if err != nil || head != checkout.baseSHA {
			return retained(errors.New("checkout has new commits"))
		}
		status, err := gitOutput(ctx, checkout.cwd, "status", "--porcelain", "--untracked-files=all", "--ignored=matching")
		if err != nil || status != "" {
			return retained(errors.New("checkout contains changes or untracked/ignored files"))
		}
		if _, err := gitOutput(ctx, checkout.projectPath, "worktree", "remove", "--", checkout.cwd); err != nil {
			return retained(err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return retained(err)
	}
	head, err := gitOutput(ctx, checkout.projectPath, "rev-parse", "--verify", "refs/heads/"+checkout.branch)
	if err != nil {
		return nil // Git failed before creating the branch.
	}
	if head != checkout.baseSHA {
		return retained(errors.New("branch has new commits"))
	}
	// Compare-and-delete the branch at its exact original commit. Unlike -D,
	// this cannot remove a branch after another process advances its ref.
	if _, err := gitOutput(ctx, checkout.projectPath, "update-ref", "-d", "refs/heads/"+checkout.branch, checkout.baseSHA); err != nil {
		return retained(err)
	}
	return nil
}
