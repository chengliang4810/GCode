package desktop

import "gcode/internal/contracts"

func (a *App) openSidebarPullRequests() {
	project, found := a.footerGitProject()
	if !found || !project.GitDetected || a.sidebarUI.pullRequests != nil {
		return
	}
	if project.PullRequestsURL != "" {
		a.services.OpenURL(project.PullRequestsURL)
		return
	}
	target := project.DeviceID
	if target == "" {
		target = "local"
	}
	scope := contracts.RepoRefScope{ProjectID: project.ID, TargetID: target, RepoPath: project.Path}
	a.sidebarUI.pullRequests = &scope
	refs := a.state.RepoRefs[scope.Key()]
	if refs.State != contracts.LoadLoading {
		a.dispatch(contracts.Command{Kind: "git.refs.load", Git: &contracts.GitRequest{Scope: scope, Force: true}})
	}
}

func (a *App) finishSidebarPullRequests() {
	scope := a.sidebarUI.pullRequests
	if scope == nil {
		return
	}
	project, found := a.footerGitProject()
	if !found || project.ID != scope.ProjectID {
		a.sidebarUI.pullRequests = nil
		return
	}
	refs := a.state.RepoRefs[scope.Key()]
	if refs.State != contracts.LoadReady && refs.State != contracts.LoadError {
		return
	}
	a.sidebarUI.pullRequests = nil
	if refs.State == contracts.LoadError {
		a.errorText = refs.Error
	} else if project.PullRequestsURL == "" {
		a.errorText = a.t("This project has no supported pull request remote.")
	} else {
		a.services.OpenURL(project.PullRequestsURL)
	}
}
