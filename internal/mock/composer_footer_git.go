package mock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"
	"time"

	"gcode/internal/contracts"
)

func (s *Server) ensureComposerFooterGitLocked(restored bool) {
	if s.state.RepoRefs == nil {
		s.state.RepoRefs = make(map[string]contracts.RepoRefs)
	}
	if s.state.CheckoutPR == nil {
		s.state.CheckoutPR = make(map[string]contracts.CheckoutPRStatus)
	}
	if s.gitJobs == nil {
		s.gitJobs = make(map[string]uint64)
	}
	for i := range s.state.Projects {
		p := &s.state.Projects[i]
		if !restored && (p.ID == "fieldnotes" || p.ID == "zeron" || p.ID == "backend") {
			p.GitDetected = true
			p.DefaultBranch = "main"
			p.CloudRepository = "zeronsh/" + p.ID
		}
	}
	for i := range s.state.Sessions {
		session := &s.state.Sessions[i]
		p, ok := s.gitProjectLocked(session.ProjectID)
		if !ok {
			continue
		}
		if session.CWD == "" {
			session.CWD = p.Path
			for _, tree := range p.Worktrees {
				if tree.ID == session.WorktreeID {
					session.CWD = tree.Path
				}
			}
		}
		// Seed only explicit deterministic observations. Restored legacy scalar
		// branches do not manufacture conversation-owned source context.
		if !restored && p.GitDetected && session.SourceContext == nil {
			if session.ID == "chat-0" {
				session.CWD, session.Branch, session.WorktreeID = "~/worktrees/fieldnotes-workspace", "fieldnotes/workspace", "workspace-tree"
			}
			if session.ID == "chat-1" {
				session.Branch = "fieldnotes/command-palette"
			}
			session.SourceContext = s.sourceContextLocked(*session, p, session.Branch)
			key, valid := contracts.SessionCheckoutPRKey(*session)
			if valid {
				s.state.CheckoutPR[key.Key()] = contracts.CheckoutPRStatus{Key: key, State: contracts.LoadReady, Summary: s.gitPRSummaryLocked(key)}
			}
		}
	}
	for key, resource := range s.state.RepoRefs {
		if resource.State == contracts.LoadLoading {
			resource.State, resource.Switching = "", ""
			s.state.RepoRefs[key] = resource
		}
	}
	for key, resource := range s.state.CheckoutPR {
		if resource.State == contracts.LoadLoading {
			resource.State, resource.Error = "", ""
			s.state.CheckoutPR[key] = resource
		}
	}
}

// Old saved fixtures predate explicit Git metadata. Migrate only the known
// deterministic seed projects, and never override an explicitly saved false.
func (s *Server) migrateComposerFooterGitProjects(data []byte) {
	var saved struct {
		Snapshot struct {
			Projects []struct {
				ID          string `json:"id"`
				GitDetected *bool  `json:"gitDetected"`
			} `json:"projects"`
		} `json:"snapshot"`
	}
	if json.Unmarshal(data, &saved) != nil {
		return
	}
	for _, old := range saved.Snapshot.Projects {
		if old.GitDetected != nil || old.ID != "fieldnotes" && old.ID != "zeron" && old.ID != "backend" {
			continue
		}
		for i := range s.state.Projects {
			project := &s.state.Projects[i]
			if project.ID == old.ID {
				project.GitDetected, project.DefaultBranch, project.CloudRepository = true, "main", "zeronsh/"+project.ID
			}
		}
	}
}

func (s *Server) gitProjectLocked(id string) (contracts.Project, bool) {
	for _, p := range s.state.Projects {
		if p.ID == id {
			return p, true
		}
	}
	return contracts.Project{}, false
}

func (s *Server) gitScopeLocked(scope contracts.RepoRefScope) (contracts.Project, error) {
	p, ok := s.gitProjectLocked(scope.ProjectID)
	if !ok {
		return p, ErrNotFound
	}
	if !p.GitDetected {
		return p, errors.New("project is not a Git repository")
	}
	if scope.RepoPath != p.Path {
		return p, errors.New("repository path no longer matches the project")
	}
	if scope.CloudRepository != "" {
		if scope.CloudRepository != p.CloudRepository || scope.DefaultBranch != p.DefaultBranch || !s.cloudRepoAvailableLocked(p) {
			return p, errors.New("cloud repository is unavailable")
		}
	} else if scope.TargetID != p.DeviceID {
		return p, errors.New("repository target no longer matches the project")
	}
	return p, nil
}

func isCloudGitDevice(id string) bool {
	return id == "cloud" || strings.HasPrefix(id, "cloud-")
}

func (s *Server) cloudRepoAvailableLocked(p contracts.Project) bool {
	if isCloudGitDevice(p.DeviceID) {
		return true
	}
	if s.state.Integrations.Cloud.State != "ready" || p.CloudRepository == "" {
		return false
	}
	for _, repo := range s.state.Integrations.Cloud.Repositories {
		if repo.Authorized && strings.EqualFold(repo.Name, p.CloudRepository) {
			return true
		}
	}
	return false
}

func (s *Server) gitRowsLocked(p contracts.Project, cloud bool) []contracts.RepoRef {
	rows := make([]contracts.RepoRef, 0, len(p.Branches))
	for _, branch := range p.Branches {
		row := contracts.RepoRef{Name: branch, Current: branch == p.Branch}
		if cloud {
			row.Current = branch == p.DefaultBranch
		} else {
			for _, tree := range p.Worktrees {
				if tree.Branch == branch && tree.Path != p.Path {
					row.WorktreePath = tree.Path
					break
				}
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func (s *Server) composerGitCommand(ctx context.Context, command contracts.Command) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrClosed
	}
	var err error
	if command.Kind == "git.pr.refresh" {
		err = s.refreshCheckoutPRLocked(*command.Git.Source, command.Git.Force)
	} else {
		err = s.loadOrSwitchRefsLocked(command)
	}
	s.mu.Unlock()
	if err != nil {
		return err
	}
	return s.persist()
}

func (s *Server) loadOrSwitchRefsLocked(command contracts.Command) error {
	request := *command.Git
	p, err := s.gitScopeLocked(request.Scope)
	if err != nil {
		return err
	}
	key := request.Scope.Key()
	resource := s.state.RepoRefs[key]
	switching := command.Kind == "git.ref.switch"
	if !switching && resource.Switching != "" {
		return nil // refreshing cannot cancel the single in-flight switch
	}
	if switching && resource.Switching != "" {
		return errors.New("a ref switch is already in progress")
	}
	if !switching && !request.Force && resource.State != "" {
		return nil
	}
	if switching {
		found := false
		for _, row := range s.gitRowsLocked(p, false) {
			if row.Name == request.Ref {
				if row.WorktreePath != "" {
					return errors.New("ref already belongs to another worktree")
				}
				found = true
			}
		}
		if !found {
			return errors.New("ref no longer exists")
		}
		resource.Switching, resource.SwitchError = request.Ref, ""
	} else {
		resource.State, resource.Error = contracts.LoadLoading, ""
	}
	resource.Scope = request.Scope
	resource.Generation++
	s.state.RepoRefs[key] = resource
	s.gitJobs[key]++
	generation := s.gitJobs[key]
	s.publishLocked("git.refs.updated", "", "", "", "")
	finish := func() {
		if s.gitJobs[key] != generation {
			return
		}
		currentProject, err := s.gitScopeLocked(request.Scope)
		if err != nil {
			// A disconnected host/repository cannot leave its cached scope
			// Loading forever. Returning to this same scope must be able to
			// make a fresh ordinary request after it becomes reachable again.
			current := s.state.RepoRefs[key]
			current.State, current.Error, current.Switching, current.SwitchError = "", "", "", ""
			s.state.RepoRefs[key] = current
			s.publishLocked("git.refs.updated", "", "", "", "")
			return
		}
		current := s.state.RepoRefs[key]
		failure := ""
		if s.opts.GitScenario == "refs-error" && !switching {
			failure = "Could not load refs"
		}
		if s.opts.GitScenario == "switch-error" && switching {
			failure = "Local changes would be overwritten by checkout"
		}
		if switching {
			current.Switching, current.SwitchError = "", failure
			if failure == "" {
				for i := range s.state.Projects {
					if s.state.Projects[i].ID == currentProject.ID {
						s.state.Projects[i].Branch = request.Ref
						for wi := range s.state.Projects[i].Worktrees {
							if s.state.Projects[i].Worktrees[wi].Path == currentProject.Path {
								s.state.Projects[i].Worktrees[wi].Branch = request.Ref
							}
						}
						currentProject = s.state.Projects[i]
					}
				}
				current.State, current.Error, current.Rows = contracts.LoadReady, "", s.gitRowsLocked(currentProject, false)
			}
		} else if failure != "" {
			current.State, current.Error = contracts.LoadError, failure
		} else {
			current.State, current.Error, current.Rows = contracts.LoadReady, "", s.gitRowsLocked(currentProject, request.Scope.CloudRepository != "")
		}
		s.state.RepoRefs[key] = current
		s.publishLocked("git.refs.updated", "", "", "", "")
	}
	if s.opts.Instant {
		finish()
	} else {
		s.gitLaterLocked(finish)
	}
	return nil
}

func (s *Server) gitLaterLocked(finish func()) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		timer := time.NewTimer(s.opts.TokenInterval)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-s.done:
			return
		}
		s.mu.Lock()
		if !s.closed {
			finish()
		}
		s.mu.Unlock()
		_ = s.persist()
	}()
}

func (s *Server) sourceContextLocked(session contracts.Session, project contracts.Project, branch string) *contracts.ConversationSourceContext {
	if !project.GitDetected || session.CWD == "" {
		return nil
	}
	checkout := "checkout:" + project.ID + ":" + session.DeviceID
	root := project.Path
	for _, tree := range project.Worktrees {
		if tree.Path == session.CWD || tree.ID == session.WorktreeID && session.WorktreeID != "" {
			checkout, root = tree.ID, tree.Path
			break
		}
	}
	if isCloudGitDevice(session.DeviceID) {
		checkout, root = "checkout:"+session.ID, session.CWD
	}
	return &contracts.ConversationSourceContext{CheckoutID: checkout, RepoRoot: root, CWD: session.CWD, Branch: branch, ObservedAt: s.opts.Now().UTC()}
}

// resolveNewCheckoutLocked returns the authoritative new conversation target.
// It never uses a display label or an arbitrary last worktree.
func (s *Server) resolveNewCheckoutLocked(project contracts.Project, sessionID string, request contracts.SendRequest) (contracts.Session, error) {
	session := contracts.Session{ID: sessionID, ProjectID: project.ID, DeviceID: request.DeviceID, CWD: project.Path, Branch: project.Branch}
	if session.DeviceID == "" {
		session.DeviceID = project.DeviceID
	}
	if session.DeviceID == "" {
		session.DeviceID = "local"
	}
	plan := request.Checkout
	if plan == nil {
		// Legacy callers can still explicitly address an existing worktree.
		plan = &contracts.CheckoutPlan{Kind: contracts.CheckoutCurrent, Branch: request.Branch}
		if request.WorktreeID != "" {
			for _, tree := range project.Worktrees {
				if tree.ID == request.WorktreeID {
					plan = &contracts.CheckoutPlan{Kind: contracts.CheckoutReuse, Path: tree.Path, Branch: tree.Branch, WorktreeID: tree.ID}
				}
			}
		}
	}
	if err := plan.Validate(); err != nil {
		return session, err
	}
	if project.ID != "" && plan.Kind != contracts.CheckoutCloud {
		session.DeviceID = project.DeviceID
	}
	switch plan.Kind {
	case contracts.CheckoutCurrent:
		if plan.Branch != "" {
			session.Branch = plan.Branch
		}
	case contracts.CheckoutReuse:
		found := false
		for _, tree := range project.Worktrees {
			if tree.Path == plan.Path && tree.Branch == plan.Branch && (plan.WorktreeID == "" || tree.ID == plan.WorktreeID) {
				session.CWD, session.Branch, session.WorktreeID = tree.Path, tree.Branch, tree.ID
				found = true
			}
		}
		if !project.GitDetected || !found {
			return session, errors.New("selected worktree is no longer available")
		}
	case contracts.CheckoutNew:
		if !project.GitDetected {
			return session, errors.New("new worktree requires a Git project")
		}
		if s.opts.GitScenario == "worktree-error" {
			return session, errors.New("could not create worktree")
		}
		base := plan.Base
		if base == "" {
			base = "HEAD"
		}
		if base != "HEAD" && !slices.Contains(project.Branches, base) {
			return session, errors.New("worktree base no longer exists")
		}
		branch := "zeron/" + sessionID
		location := project.Path + "-worktrees"
		if s.state.Settings.General.CustomWorktreeLocation && s.state.Settings.General.WorktreePath != "" {
			location = s.state.Settings.General.WorktreePath
		}
		tree := contracts.Worktree{ID: s.idLocked("worktree"), Name: branch, Path: path.Join(location, sessionID), Branch: branch, BaseRef: base}
		for i := range s.state.Projects {
			if s.state.Projects[i].ID == project.ID {
				s.state.Projects[i].Worktrees = append(s.state.Projects[i].Worktrees, tree)
				s.state.Projects[i].Branches = append(s.state.Projects[i].Branches, branch)
				project = s.state.Projects[i]
			}
		}
		session.CWD, session.Branch, session.WorktreeID = tree.Path, tree.Branch, tree.ID
	case contracts.CheckoutCloud:
		if !s.cloudRepoAvailableLocked(project) {
			return session, errors.New("cloud repository is unavailable")
		}
		device := "cloud-" + sessionID
		s.state.Devices = append(s.state.Devices, contracts.Device{ID: device, Name: "Cloud", Platform: "linux", Status: "connected", LastSeen: s.opts.Now().UTC()})
		// Each deterministic Cloud machine installs the supported Cloud agents.
		// Accounts and usage belong to their own target and are not borrowed.
		var providers []contracts.ProviderDetails
		for _, provider := range s.state.Integrations.Providers {
			if provider.TargetID == "cloud-1" {
				providers = append(providers, contracts.ProviderDetails{
					ProviderID: provider.ProviderID, TargetID: device,
					Installed: provider.Installed, Enabled: provider.Enabled,
					Version: provider.Version, LatestVersion: provider.LatestVersion,
					UpdatePolicy: "off", AccountsState: contracts.LoadReady,
					DollarSkills: provider.DollarSkills, SeparateSlash: provider.SeparateSlash,
					NativeSubagents: provider.NativeSubagents, CanSwitchSubagents: provider.CanSwitchSubagents,
					LoginKind: provider.LoginKind, SingleLogin: provider.SingleLogin,
					LoginOptions: slices.Clone(provider.LoginOptions),
				})
			}
		}
		s.state.Integrations.Providers = append(s.state.Integrations.Providers, providers...)
		session.DeviceID, session.CWD = device, "/workspace/"+project.Name
		session.Branch = plan.Branch
		if session.Branch == "" {
			session.Branch = project.DefaultBranch
		}
	}
	session.SourceContext = s.sourceContextLocked(session, project, session.Branch)
	return session, nil
}

func (s *Server) captureSessionSourceLocked(session *contracts.Session) {
	project, ok := s.gitProjectLocked(session.ProjectID)
	if !ok || !project.GitDetected {
		return
	}
	branch := session.Branch
	if session.CWD == project.Path {
		branch = project.Branch
	} else {
		for _, tree := range project.Worktrees {
			if tree.Path == session.CWD {
				branch = tree.Branch
			}
		}
	}
	session.Branch = branch
	session.SourceContext = s.sourceContextLocked(*session, project, branch)
}

func (s *Server) gitPRSummaryLocked(key contracts.CheckoutPRKey) *contracts.ChangeRequestSummary {
	for _, session := range s.state.Sessions {
		owned, ok := contracts.SessionCheckoutPRKey(session)
		if !ok || owned != key {
			continue
		}
		project, ok := s.gitProjectLocked(session.ProjectID)
		if !ok {
			continue
		}
		for _, pr := range s.state.PullRequests {
			if pr.Repository == project.CloudRepository && pr.HeadBranch == key.Branch {
				return &contracts.ChangeRequestSummary{Provider: "github", Number: pr.Number, Title: pr.Title, URL: fmt.Sprintf("https://github.com/%s/pull/%d", pr.Repository, pr.Number), State: pr.Status, BaseRef: pr.BaseBranch, HeadRef: pr.HeadBranch, DetailID: pr.ID}
			}
		}
	}
	return nil
}

// Call after removing the session row. Shared checkout snapshots remain while
// another conversation owns that source, but last-owner deletion invalidates
// pending refreshes before dropping the cache.
func (s *Server) deleteComposerFooterGitSessionLocked(session contracts.Session) {
	key, valid := contracts.SessionCheckoutPRKey(session)
	if !valid {
		return
	}
	for _, other := range s.state.Sessions {
		if candidate, ok := contracts.SessionCheckoutPRKey(other); ok && candidate == key {
			return
		}
	}
	delete(s.state.CheckoutPR, key.Key())
	s.gitJobs["pr:"+key.Key()]++
}

func (s *Server) refreshCheckoutPRLocked(key contracts.CheckoutPRKey, force bool) error {
	owned := false
	for _, session := range s.state.Sessions {
		candidate, ok := contracts.SessionCheckoutPRKey(session)
		owned = owned || ok && candidate == key
	}
	if !owned {
		return errors.New("conversation source is no longer available")
	}
	encoded := key.Key()
	resource := s.state.CheckoutPR[encoded]
	if !force && resource.State != "" {
		return nil
	}
	resource.Key, resource.State, resource.Error = key, contracts.LoadLoading, ""
	resource.Generation++
	s.state.CheckoutPR[encoded] = resource
	jobKey := "pr:" + encoded
	s.gitJobs[jobKey]++
	generation := s.gitJobs[jobKey]
	finish := func() {
		if s.gitJobs[jobKey] != generation {
			return
		}
		current := s.state.CheckoutPR[encoded]
		if s.opts.GitScenario == "pr-error" {
			current.State, current.Error = contracts.LoadError, "Could not refresh pull request"
		} else {
			current.State, current.Error, current.Summary = contracts.LoadReady, "", s.gitPRSummaryLocked(key)
		}
		s.state.CheckoutPR[encoded] = current
		s.publishLocked("git.pr.updated", "", "", "", "")
	}
	s.publishLocked("git.pr.updated", "", "", "", "")
	if s.opts.Instant {
		finish()
	} else {
		s.gitLaterLocked(finish)
	}
	return nil
}

func cloneComposerFooterGitSnapshots(v *contracts.Snapshot) {
	v.RepoRefs = maps.Clone(v.RepoRefs)
	for key, resource := range v.RepoRefs {
		resource.Rows = slices.Clone(resource.Rows)
		v.RepoRefs[key] = resource
	}
	v.CheckoutPR = maps.Clone(v.CheckoutPR)
	for key, resource := range v.CheckoutPR {
		if resource.Summary != nil {
			copy := *resource.Summary
			resource.Summary = &copy
		}
		v.CheckoutPR[key] = resource
	}
	for i := range v.Sessions {
		if v.Sessions[i].SourceContext != nil {
			copy := *v.Sessions[i].SourceContext
			v.Sessions[i].SourceContext = &copy
		}
	}
}
