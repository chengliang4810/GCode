package desktop

import (
	"fmt"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

type composerFooterGitState struct {
	Scope                  contracts.RepoRefScope
	Kind                   contracts.CheckoutKind
	Branch, Query          string
	RefOpen, CheckoutOpen  bool
	Requested              bool
	RefLoading             bool
	RefLoadBaseline        uint64
	RefReadyGeneration     uint64
	CursorRef, PendingRef  string
	Sequence               uint64
	Switching, SwitchError string
	RefError               string
	SwitchBaseline         uint64
	Cursor                 int
	HeldRef                *contracts.RepoRef
	Kinds                  map[string]contracts.CheckoutKind
}

func footerCloudDevice(id string) bool { return id == "cloud" || strings.HasPrefix(id, "cloud-") }

func footerCloudAvailable(state contracts.Snapshot, project contracts.Project) bool {
	if footerCloudDevice(project.DeviceID) {
		return true
	}
	return footerCloudRepositoryAvailable(state, project)
}

func footerCloudRepositoryAvailable(state contracts.Snapshot, project contracts.Project) bool {
	if state.Integrations.Cloud.State != "ready" || project.CloudRepository == "" {
		return false
	}
	for _, repo := range state.Integrations.Cloud.Repositories {
		if repo.Authorized && strings.EqualFold(repo.Name, project.CloudRepository) {
			return true
		}
	}
	return false
}

func (a *App) composerCloudDeviceID() string {
	if id := a.state.Integrations.Cloud.DeviceID; id != "" {
		return id
	}
	return "cloud-1"
}

// Cloud is a draft destination, not one of its per-session machines. Match the
// source's cloud_repo_for guard both when presenting and accepting the row.
func (a *App) pickComposerDevice(id string) {
	if footerCloudDevice(id) {
		project, found := a.footerGitProject()
		if !found || !footerCloudRepositoryAvailable(a.state, project) || id != a.composerCloudDeviceID() {
			return
		}
	}
	a.deviceID = id
	a.restoreModelSelection()
	a.deviceMenu = false
}

func (a *App) reconcileComposerCloudDevice() {
	if a.sessionID != "" || !footerCloudDevice(a.deviceID) {
		return
	}
	project, found := a.footerGitProject()
	if found && footerCloudAvailable(a.state, project) {
		return
	}
	a.deviceID = project.DeviceID
	if a.deviceID == "" {
		a.deviceID = "local"
	}
	a.restoreModelSelection()
}

func (a *App) footerGitProject() (contracts.Project, bool) {
	id := a.projectID
	if session := a.currentSession(); session.ID != "" {
		id = session.ProjectID
	}
	for _, project := range a.state.Projects {
		if project.ID == id {
			return project, true
		}
	}
	return contracts.Project{}, false
}

func (a *App) syncComposerFooterGit() (contracts.Project, contracts.RepoRefs, bool) {
	project, found := a.footerGitProject()
	f := &a.footerGit
	target := project.DeviceID
	if target == "" {
		target = "local"
	}
	scope := contracts.RepoRefScope{ProjectID: project.ID, TargetID: target, RepoPath: project.Path}
	cloud := (footerCloudDevice(a.deviceID) || footerCloudDevice(project.DeviceID)) && footerCloudAvailable(a.state, project)
	if cloud {
		scope.TargetID, scope.CloudRepository, scope.DefaultBranch = a.deviceID, project.CloudRepository, project.DefaultBranch
		if project.CloudRepository == "" {
			scope.TargetID = project.DeviceID
		}
		if !footerCloudDevice(scope.TargetID) {
			scope.TargetID = "cloud"
		}
	}
	if f.Scope != scope {
		wasCloud := f.Kind == contracts.CheckoutCloud
		if previous := f.selectedRef(a.state.RepoRefs[f.Scope.Key()]); previous != nil {
			copy := *previous
			f.HeldRef = &copy
		}
		if f.Kinds == nil {
			f.Kinds = make(map[string]contracts.CheckoutKind)
		}
		if f.Scope.Valid() {
			f.Kinds[f.Scope.Key()] = f.Kind
		}
		f.Scope, f.Branch, f.Query, f.Switching, f.SwitchError = scope, "", "", "", ""
		f.RefError = ""
		f.RefLoading = false
		f.RefReadyGeneration = 0
		f.CursorRef, f.PendingRef = "", ""
		f.RefOpen, f.CheckoutOpen, f.Requested, f.Cursor = false, false, false, 0
		f.Sequence++
		f.Kind = f.Kinds[scope.Key()]
		if f.Kind == "" || f.Kind == contracts.CheckoutCloud {
			f.Kind = contracts.CheckoutCurrent
		}
		if wasCloud && !cloud {
			f.Kind = contracts.CheckoutCurrent
		}
	}
	if cloud {
		f.Kind = contracts.CheckoutCloud
	}
	resource := a.state.RepoRefs[scope.Key()]
	if f.RefLoading && resource.Generation > f.RefLoadBaseline {
		f.RefLoading = false
	}
	if resource.State == contracts.LoadReady && resource.Generation != f.RefReadyGeneration {
		f.RefReadyGeneration = resource.Generation
		if f.RefOpen {
			f.Cursor = 0
			for i, row := range footerFilteredRefs(resource, f.Query) {
				if f.Branch != "" && row.Name == f.Branch || f.Branch == "" && row.Current {
					f.Cursor = min(i, 299)
					break
				}
			}
			for i, row := range footerFilteredRefs(resource, f.Query) {
				if row.Name == f.CursorRef {
					f.Cursor = min(i, 299)
					break
				}
			}
		}
	}
	if f.Switching != "" && resource.Generation > f.SwitchBaseline && resource.Switching == "" && resource.State != "" {
		if resource.SwitchError != "" {
			f.SwitchError = resource.SwitchError
		} else {
			f.Branch, f.RefOpen = f.Switching, false
		}
		f.Switching = ""
	}
	if f.PendingRef != "" && !f.RefLoading && resource.State != contracts.LoadLoading {
		name := f.PendingRef
		f.PendingRef = ""
		if found && project.GitDetected && a.sessionID == "" {
			matched := false
			for _, row := range resource.Rows {
				if row.Name == name {
					matched = true
					a.pickComposerFooterRef(row)
					break
				}
			}
			if !matched {
				f.SwitchError = a.t("Selected ref is no longer available. Choose another ref.")
			}
		}
	}
	return project, resource, found && project.GitDetected
}

func (f *composerFooterGitState) selectedRef(resource contracts.RepoRefs) *contracts.RepoRef {
	for _, row := range resource.Rows {
		if f.Branch != "" && row.Name == f.Branch || f.Branch == "" && row.Current {
			copy := row
			return &copy
		}
	}
	return nil
}

func (f *composerFooterGitState) effectiveRef(resource contracts.RepoRefs) string {
	if f.Branch != "" {
		return f.Branch
	}
	if row := f.selectedRef(resource); row != nil {
		return row.Name
	}
	return ""
}

func (f *composerFooterGitState) displayRef(resource contracts.RepoRefs) *contracts.RepoRef {
	if row := f.selectedRef(resource); row != nil {
		return row
	}
	if resource.State == "" || resource.State == contracts.LoadLoading {
		return f.HeldRef
	}
	return nil
}

func (a *App) ensureComposerFooterRefs(force bool) {
	_, resource, git := a.syncComposerFooterGit()
	f := &a.footerGit
	if !git || a.currentSession().ID != "" {
		return
	}
	if f.RefLoading || resource.State == contracts.LoadLoading || resource.Switching != "" || f.Switching != "" {
		f.Requested = true
		return
	}
	if !force && f.Requested {
		return
	}
	if force {
		if selected := f.selectedRef(resource); selected != nil {
			copy := *selected
			f.HeldRef = &copy
		}
	}
	f.Requested = true
	f.RefLoading, f.RefLoadBaseline = true, resource.Generation
	f.RefError = ""
	f.Sequence++
	a.dispatch(contracts.Command{Kind: "git.refs.load", Version: f.Sequence, Git: &contracts.GitRequest{Scope: f.Scope, Force: force || resource.State != ""}})
	a.syncComposerFooterGit()
}

func (a *App) composerFooterCheckoutPlan() *contracts.CheckoutPlan {
	// A selected ID already identifies a conversation, even before its row arrives.
	if a.sessionID != "" {
		return nil
	}
	_, resource, git := a.syncComposerFooterGit()
	if !git {
		return nil
	}
	f := &a.footerGit
	name := f.effectiveRef(resource)
	switch f.Kind {
	case contracts.CheckoutNew:
		// With no explicit base, resolve HEAD in the core at send time.
		return &contracts.CheckoutPlan{Kind: contracts.CheckoutNew, Base: f.Branch}
	case contracts.CheckoutCloud:
		return &contracts.CheckoutPlan{Kind: contracts.CheckoutCloud, Branch: name}
	default:
		// Cached rows retain the destination while refreshing. The core still
		// validates its path and branch against the actual repository.
		if row := f.selectedRef(resource); row != nil {
			if row.Current {
				// The default follows the actual checkout, not its cached label.
				// An explicit ref selection still pins and validates its branch.
				return &contracts.CheckoutPlan{Kind: contracts.CheckoutCurrent, Branch: f.Branch}
			}
			if row.WorktreePath != "" {
				return &contracts.CheckoutPlan{Kind: contracts.CheckoutReuse, Path: row.WorktreePath, Branch: row.Name}
			}
		}
		return &contracts.CheckoutPlan{Kind: contracts.CheckoutCurrent, Branch: f.Branch}
	}
}

func (a *App) pickComposerFooterRef(row contracts.RepoRef) {
	if a.currentSession().ID != "" {
		return
	}
	_, resource, git := a.syncComposerFooterGit()
	f := &a.footerGit
	if !git || f.Switching != "" || f.PendingRef != "" {
		return
	}
	if f.RefLoading || resource.State == contracts.LoadLoading {
		// Keep the user's choice while the owned refresh finishes, rather than
		// racing a git switch against the core's active repository operation.
		f.PendingRef, f.SwitchError = row.Name, ""
		return
	}
	if f.Kind == contracts.CheckoutCloud || f.Kind == contracts.CheckoutNew || row.Current || row.WorktreePath != "" {
		f.Branch, f.RefOpen, f.SwitchError = row.Name, false, ""
		if row.WorktreePath != "" && f.Kind != contracts.CheckoutCloud {
			f.Kind = contracts.CheckoutCurrent
		}
		return
	}
	f.Switching, f.SwitchError = row.Name, ""
	f.SwitchBaseline = resource.Generation
	f.Sequence++
	a.dispatch(contracts.Command{Kind: "git.ref.switch", Version: f.Sequence, Git: &contracts.GitRequest{Scope: f.Scope, Ref: row.Name}})
}

func (a *App) composerFooterGitBusy() bool {
	return a.sessionID == "" && (a.footerGit.PendingRef != "" || a.footerGit.Switching != "")
}

func (a *App) pickComposerFooterCheckout(kind contracts.CheckoutKind) {
	if a.currentSession().ID != "" {
		return
	}
	_, resource, _ := a.syncComposerFooterGit()
	f := &a.footerGit
	if kind == contracts.CheckoutCurrent && f.Kind == contracts.CheckoutNew {
		if row := f.selectedRef(resource); row != nil && !row.Current && row.WorktreePath == "" {
			f.Branch = ""
		}
	}
	f.Kind, f.CheckoutOpen = kind, false
}

func (a *App) composerFooterGitCommandFinished(command contracts.Command, err error) bool {
	if !contracts.IsComposerGitCommand(command.Kind) {
		return false
	}
	if command.Git == nil {
		return true
	}
	f := &a.footerGit
	if command.Git.Scope != f.Scope || command.Version != f.Sequence {
		return true
	}
	if err != nil {
		if command.Kind == "git.ref.switch" {
			f.Switching, f.SwitchError, f.RefOpen = "", err.Error(), true
		} else {
			// An RPC rejection may precede a resource snapshot. Keep the error
			// until an explicit retry rather than retrying on every render.
			f.RefError = err.Error()
			f.RefLoading = false
		}
		return true
	}
	return true
}

func (a *App) composerFooterGit(c *ui.Context) {
	project, _, git := a.syncComposerFooterGit()
	if !git {
		return
	}
	f := &a.footerGit
	if session := a.currentSession(); session.ID != "" {
		label, glyph := "Local checkout", "folder"
		if footerCloudDevice(session.DeviceID) {
			label, glyph = "Cloud", "cloud"
		} else if session.CWD != "" && session.CWD != project.Path {
			label, glyph = "Worktree", "worktree"
		}
		footerGitLabel(c, a.palette, glyph, tr(c, label), "Session checkout", 160)
		branch := session.Branch
		if branch == "" {
			branch = tr(c, "No ref")
		}
		footerGitLabel(c, a.palette, "git-branch", branch, "Session ref", 0)
		return
	}
	a.ensureComposerFooterRefs(false)
	resource := a.state.RepoRefs[f.Scope.Key()]
	refLabel := f.Branch
	if refLabel == "" {
		if row := f.displayRef(resource); row != nil {
			refLabel = row.Name
		}
	}
	if refLabel == "" {
		refLabel = tr(c, "Select ref")
	} else if f.Kind == contracts.CheckoutNew || f.Kind == contracts.CheckoutCloud {
		refLabel = tr(c, "From ") + refLabel
	}
	if f.Kind != contracts.CheckoutCloud {
		label, glyph := "Current checkout", "folder"
		if row := f.displayRef(resource); row != nil && row.WorktreePath != "" {
			label, glyph = "Current worktree", "worktree"
		}
		if f.Kind == contracts.CheckoutNew {
			label, glyph = "New worktree", "worktree"
		}
		checkout := a.footerGitTrigger(c, glyph, tr(c, label), "Select checkout", f.CheckoutOpen)
		checkout.OnClick(func() {
			f.CheckoutOpen, f.RefOpen, f.Cursor = !f.CheckoutOpen, false, 0
			if f.Kind == contracts.CheckoutNew {
				f.Cursor = 1
			}
		})
		a.footerCheckoutPopover(c, checkout)
	}
	ref := a.footerGitTrigger(c, "git-branch", refLabel, "Select ref", f.RefOpen)
	ref.OnClick(func() {
		f.RefOpen, f.CheckoutOpen, f.Query, f.Cursor = !f.RefOpen, false, "", 0
		if f.RefOpen {
			f.CursorRef = ""
			for i, row := range footerFilteredRefs(resource, "") {
				if f.Branch != "" && row.Name == f.Branch || f.Branch == "" && row.Current {
					f.Cursor = min(i, 299)
					break
				}
			}
			if rows := footerFilteredRefs(resource, ""); len(rows) > f.Cursor {
				f.CursorRef = rows[f.Cursor].Name
			}
			a.ensureComposerFooterRefs(true)
		}
	})
	a.footerRefPopover(c, ref)
}

func footerGitLabel(c *ui.Context, p design.Tokens, glyph, text, label string, maxWidth float32) {
	row := ui.Row(c).Height(20).MinWidth(0).Shrink(1).Padding(0, 8).Gap(6).AlignItems(ui.Center).TextColor(p.Muted.Alpha(.6)).Label(tr(c, label))
	if maxWidth > 0 {
		row.MaxWidth(maxWidth)
	}
	row.Children(func() {
		icon(c, glyph, 12)
		ui.Text(c, text).MinWidth(0).Shrink(1).SingleLine().FontSize(design.TextSize(c, design.TextBase)).FontWeight(500)
	})
}

func (a *App) footerGitTrigger(c *ui.Context, glyph, text, label string, open bool) ui.Element {
	b := ui.ButtonBase(c.Key(label)).Height(20).MinWidth(0).MaxWidth(280).Shrink(1).Padding(0, 8).Gap(6).Radius(6).AlignItems(ui.Center).TextColor(a.palette.Muted.Alpha(.7)).Label(tr(c, label)).Value(text)
	if open || b.Hovered() {
		b.Background(a.palette.Hover)
	}
	b.Children(func() {
		icon(c, glyph, 12)
		ui.Text(c, text).MinWidth(0).Shrink(1).SingleLine().FontSize(design.TextSize(c, design.TextBase)).FontWeight(500)
		icon(c, "alt-arrow-down", 12)
	})
	return b
}

// popover.rs::popover_card uses the same opaque input surface and shadow_lg
// as the model picker, with a 4px inset and 2px menu gap.
func (a *App) footerGitPopoverCard(c *ui.Context, panel ui.Element) {
	palette := a.centralControls().modelPopupPalette(c)
	panel.Padding(4).Gap(2).Radius(12).FontSize(design.TextSize(c, design.TextBase)).FontWeight(500).TextColor(palette.Text).Clip()
	if !a.frosted(c) {
		panel.Shadow(0, 10, 2*15, -3, ui.RGBA(0, 0, 0, .1)).Shadow(0, 4, 2*6, -4, ui.RGBA(0, 0, 0, .1))
	}
	a.floatingSurface(c, panel, 12)
}

func (a *App) footerCheckoutPopover(c *ui.Context, anchor ui.Element) {
	f := &a.footerGit
	ui.PopoverBase(c, anchor, &f.CheckoutOpen, func(panel ui.Element) {
		a.footerGitPopoverCard(c, panel)
		panel.Label(tr(c, "Checkout picker")).Width(224).Focusable().AutoFocus()
		a.footerGitPopoverPosition(c, panel, anchor)
		local := "Current checkout"
		if row := f.selectedRef(a.state.RepoRefs[f.Scope.Key()]); row != nil && row.WorktreePath != "" {
			local = "Current worktree"
		}
		items := []struct {
			kind         contracts.CheckoutKind
			label, glyph string
		}{{contracts.CheckoutCurrent, local, "folder"}, {contracts.CheckoutNew, "New worktree", "worktree"}}
		c.OnShortcut(0, ui.KeyDown, func() { f.Cursor = (f.Cursor + 1) % len(items) })
		c.OnShortcut(0, ui.KeyUp, func() { f.Cursor = (f.Cursor + len(items) - 1) % len(items) })
		c.OnShortcut(0, ui.KeyEnter, func() { a.pickComposerFooterCheckout(items[f.Cursor].kind) })
		for i, item := range items {
			item := item
			button := ui.ButtonBase(c.Key("checkout-"+string(item.kind))).Height(30).FillWidth().Padding(0, 8).Gap(8).Radius(8).Justify(ui.Start).Label(tr(c, item.label))
			if button.Hovered() || f.Kind == item.kind || f.Cursor == i {
				button.Background(a.palette.Hover)
			}
			button.OnClick(func() { a.pickComposerFooterCheckout(item.kind) }).Children(func() {
				icon(c, item.glyph, 14)
				ui.Text(c, tr(c, item.label)).FontSize(design.TextSize(c, design.TextBase))
			})
		}
	})
}

func (a *App) footerGitPopoverPosition(c *ui.Context, panel, anchor ui.Element) float32 {
	budget, openBelow := composerMenuPlacement(c, panel, anchor, a.sessionID == "", false)
	if openBelow {
		panel.AttachTo(anchor, ui.AnchorBottomStart, ui.AnchorTopStart).Margin(6, 0, 0, 0)
	} else {
		panel.AttachTo(anchor, ui.AnchorTopStart, ui.AnchorBottomStart).Margin(0, 0, 6, 0)
	}
	panel.SnapToWindow(8).MaxHeight(budget)
	return budget
}

func (a *App) footerRefPopover(c *ui.Context, anchor ui.Element) {
	f := &a.footerGit
	ui.PopoverBase(c, anchor, &f.RefOpen, func(panel ui.Element) {
		a.footerGitPopoverCard(c, panel)
		panel.Label(tr(c, "Ref picker")).Width(320)
		budget := a.footerGitPopoverPosition(c, panel, anchor)
		search := ui.TextInputBase(c.Key("composer-ref-search"), &f.Query).Label(tr(c, "Search refs")).Placeholder(tr(c, "Search refs…")).Height(32).FillWidth().Padding(0, 8).FontSize(design.TextSize(c, design.TextBase)).AutoFocus().OnChange(func() { f.Cursor, f.CursorRef = 0, "" })
		composing := search.Composing()
		search.OnTextKey(func(key ui.TextKey) bool {
			pressed := key.Key
			if key.Mods == ui.Ctrl && (pressed == ui.KeyN || pressed == ui.KeyP) {
				pressed = ui.KeyDown
				if key.Key == ui.KeyP {
					pressed = ui.KeyUp
				}
			} else if key.Mods != 0 && (pressed != ui.KeyEnter || key.Mods != ui.Cmd && key.Mods != ui.Ctrl) {
				return false
			}
			if pressed != ui.KeyEnter && pressed != ui.KeyUp && pressed != ui.KeyDown {
				return false
			}
			if composing {
				return pressed == ui.KeyEnter
			}
			f.Query = key.Text()
			rows := footerFilteredRefs(a.state.RepoRefs[f.Scope.Key()], f.Query)
			count := min(len(rows), 300)
			if count > 0 && f.Switching == "" && f.PendingRef == "" {
				f.Cursor = max(0, min(f.Cursor, count-1))
				switch pressed {
				case ui.KeyDown:
					f.Cursor = (f.Cursor + 1) % count
				case ui.KeyUp:
					f.Cursor = (f.Cursor + count - 1) % count
				case ui.KeyEnter:
					a.pickComposerFooterRef(rows[f.Cursor])
				}
				f.CursorRef = rows[f.Cursor].Name
			}
			return true
		})
		resource := a.state.RepoRefs[f.Scope.Key()]
		refError := ""
		if resource.State == contracts.LoadError && !f.RefLoading {
			refError = resource.Error
		}
		if f.RefError != "" {
			refError = f.RefError
		}
		if resource.State == contracts.LoadReady || len(resource.Rows) > 0 {
			rows := footerFilteredRefs(resource, f.Query)
			if len(rows) == 0 {
				ui.Text(c, tr(c, "No refs found.")).FontSize(design.TextSize(c, design.TextBase)).Padding(8)
			}
			if len(rows) > 0 {
				// pickers.rs caps the filtered rows and keyboard cursor at 300.
				f.Cursor = max(0, min(f.Cursor, min(len(rows), 300)-1))
				f.CursorRef = rows[f.Cursor].Name
				ui.Scroll(c).FillWidth().MaxHeight(max(float32(0), min(budget-144, 224))).Gap(2).Children(func() {
					for i, row := range rows[:min(len(rows), 300)] {
						row := row
						b := ui.ButtonBase(c.Key(row.Name)).FillWidth().Height(32).Padding(0, 8).Gap(8).Radius(7).Justify(ui.Start).Label(tr(c, "Ref ") + row.Name).Disabled(f.Switching != "" || f.PendingRef != "")
						if b.Hovered() || f.Branch == row.Name || f.Cursor == i {
							b.Background(a.palette.Hover)
						}
						if f.Cursor == i {
							b.ScrollIntoView()
						}
						b.OnClick(func() { a.pickComposerFooterRef(row) }).Children(func() {
							ui.Text(c, row.Name).Grow(1).MinWidth(0).SingleLine().FontSize(design.TextSize(c, design.TextBase))
							tag := ""
							if row.Current {
								tag = "current"
							} else if row.WorktreePath != "" {
								tag = "worktree"
							}
							if f.Switching == row.Name || f.PendingRef == row.Name {
								tag = "switching…"
							}
							if tag != "" {
								ui.Text(c, tr(c, tag)).FontSize(design.TextSize(c, design.TextXS)).TextColor(a.palette.Muted)
							}
						})
					}
				})
				if len(rows) > 300 {
					ui.Text(c, fmt.Sprintf(tr(c, "Showing 300 of %d refs"), len(rows))).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Faint).Padding(4, 8)
				}
			}
		} else if refError == "" {
			ui.Column(c.Key("composer-ref-loading")).Role(ui.RoleGroup).FillWidth().Children(func() {
				for range 4 {
					ui.Box(c).Height(18).FillWidth().Margin(5, 8).Radius(4).Background(a.palette.Text.Alpha(.08))
				}
			})
		}
		if refError != "" {
			ui.Text(c.Key("composer-ref-error"), refError).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Danger).Padding(8)
			textButton(c, tr(c, "Retry refs")).OnClick(func() { a.ensureComposerFooterRefs(true) })
		}
		if f.SwitchError != "" {
			ui.Text(c, f.SwitchError).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Danger).Padding(4, 8)
		}
	})
}

func footerFilteredRefs(resource contracts.RepoRefs, query string) []contracts.RepoRef {
	query = strings.ToLower(strings.TrimSpace(query))
	var prefix, remainder []contracts.RepoRef
	for _, row := range resource.Rows {
		name := strings.ToLower(row.Name)
		if query != "" && strings.HasPrefix(name, query) {
			prefix = append(prefix, row)
		} else if strings.Contains(name, query) {
			remainder = append(remainder, row)
		}
	}
	return append(prefix, remainder...)
}
