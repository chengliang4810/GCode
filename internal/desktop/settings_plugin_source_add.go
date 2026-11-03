package desktop

import (
	"crypto/rand"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *App) openPluginSourceAdd() {
	s := &a.capabilityUI.plugins
	s.sourceEpoch++
	s.source = contracts.PluginMarketplaceInput{}
	s.sourcePending, s.sourceError = "", ""
	s.sourcesOpen, s.sourceOpen = true, true
}

func (a *App) addPluginMarketplace() {
	s := &a.capabilityUI.plugins
	if !s.sourcesOpen || !s.sourceOpen || s.sourcePending != "" || strings.TrimSpace(s.source.Source) == "" {
		return
	}
	s.sourcePending, s.sourceError = "marketplace-"+rand.Text(), ""
	input := contracts.PluginMarketplaceInput{Source: strings.TrimSpace(s.source.Source)}
	a.dispatch(contracts.Command{
		Kind: "plugins.source.save", RequestID: s.sourcePending,
		Plugin: &contracts.PluginInput{Marketplace: &input},
	})
}

func (a *App) pluginSourceCommandFinished(command contracts.Command, err error) {
	s := &a.capabilityUI.plugins
	if err != nil && command.Kind == "plugins.source.save" && s.sourcePending != "" && command.RequestID == s.sourcePending {
		s.sourcePending, s.sourceError = "", err.Error()
	}
}

func (a *App) syncPluginSourceOperation() {
	s := &a.capabilityUI.plugins
	if s.sourcePending == "" {
		return
	}
	for _, operation := range a.state.Plugins.Operations {
		if operation.ID != s.sourcePending || operation.State == "running" {
			continue
		}
		s.sourcePending = ""
		if operation.State == "failed" {
			s.sourceError = operation.Error
			return
		}
		s.sourcesOpen, s.sourceOpen = false, false
		s.source = contracts.PluginMarketplaceInput{}
		s.sourceError = ""
		return
	}
}

func (a *App) pluginSourceAddForm(c *ui.Context) {
	s := &a.capabilityUI.plugins
	busy := s.sourcePending != ""
	remote := a.state.Runtime != nil && a.state.Runtime.Remote
	drop := ui.Column(c.Key("plugin-source-drop")).FillWidth().Gap(16).
		Label(tr(c, "Marketplace source drop area")).Children(func() {
		controlTextInput(c.Key("plugin-source-location"), &s.source.Source).
			FillWidth().AutoFocus().Disabled(busy).
			Label(tr(c, "Marketplace source")).
			Placeholder(tr(c, "GitHub repository, Git URL, file or directory")).
			OnSubmit(a.addPluginMarketplace)
		hint := "Drop a file or folder, or choose a directory."
		if remote {
			hint = "Marketplace paths are on the remote host."
		}
		ui.Text(c, tr(c, hint)).TextColor(a.palette.Muted).
			FontSize(design.TextSize(c, design.TextBase))
	})
	if !remote && !busy {
		if paths := drop.DroppedFiles(); len(paths) > 0 {
			if len(paths) == 1 {
				s.source.Source, s.sourceError = paths[0], ""
			} else {
				s.sourceError = tr(c, "Add one marketplace at a time.")
			}
		}
	}
	if s.sourceError != "" {
		ui.Text(c.Key("plugin-source-error"), s.sourceError).FillWidth().TextColor(a.palette.Danger).Selectable()
	}
	ui.Row(c.Key("plugin-source-add-actions")).FillWidth().Wrap().Justify(ui.End).Gap(8).Children(func() {
		buttonWithIcon(c.Key("plugin-source-choose-directory"), tr(c, "Choose directory"), "folder", buttonSecondary).
			Disabled(remote || busy).OnClick(a.choosePluginMarketplaceDirectory)
		label := "Add plugin marketplace"
		if busy {
			label = "Adding marketplace…"
		}
		buttonWithIcon(c.Key("plugin-source-save"), tr(c, label), "plus", buttonPrimary).
			Disabled(busy || strings.TrimSpace(s.source.Source) == "").OnClick(a.addPluginMarketplace)
	})
}

func (a *App) choosePluginMarketplaceDirectory() {
	s := &a.capabilityUI.plugins
	if a.win == nil || !s.sourcesOpen || !s.sourceOpen || s.source.ID != "" || s.sourcePending != "" ||
		(a.state.Runtime != nil && a.state.Runtime.Remote) {
		return
	}
	window, epoch, title := a.win, s.sourceEpoch, a.t("Choose directory")
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: window, Directory: true, Title: title})
		window.Update(func() {
			current := &a.capabilityUI.plugins
			if a.closed || !current.sourcesOpen || !current.sourceOpen || current.source.ID != "" ||
				current.sourceEpoch != epoch || current.sourcePending != "" {
				return
			}
			if err != nil {
				current.sourceError = err.Error()
			} else if len(paths) > 0 {
				current.source.Source, current.sourceError = paths[0], ""
			}
		})
	}()
}
