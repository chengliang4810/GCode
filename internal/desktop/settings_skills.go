package desktop

import (
	"crypto/rand"
	"slices"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *App) editSkill(skill *contracts.UserSkill) {
	s := &a.capabilityUI
	s.skillOpen, s.skillDetailsOpen, s.skillImportOpen = true, false, false
	s.skillImport = ""
	s.skill = contracts.UserSkill{ID: "skill-" + rand.Text(), Enabled: true, Content: "---\nname: my-skill\ndescription: Describe when to use this skill.\n---\n\n# Instructions\n"}
	if skill != nil {
		s.skill = *skill
		s.skill.Files = slices.Clone(skill.Files)
	}
}

func (a *App) showSkillDetails(skill contracts.UserSkill) {
	s := &a.capabilityUI
	s.skill = skill
	s.skill.Files = slices.Clone(skill.Files)
	s.skillDetailsOpen, s.skillOpen, s.skillImportOpen = true, false, false
}

func (a *App) importSkill() {
	s := &a.capabilityUI
	s.skill = contracts.UserSkill{ID: "skill-" + rand.Text(), Enabled: true}
	s.skillImport = ""
	s.skillImportOpen, s.skillDetailsOpen, s.skillOpen = true, false, false
}

func (a *App) skillSettings(c *ui.Context) {
	s := &a.capabilityUI
	if s.queries == nil {
		s.queries = make(map[string]*string)
	}
	if s.queries["Skills"] == nil {
		s.queries["Skills"] = new(string)
	}
	ui.Row(c.Key("skill-toolbar")).FillWidth().Gap(8).Children(func() {
		controlTextInput(c.Key("resource-search-Skills"), s.queries["Skills"]).Grow(1).Label(tr(c, "Search Skills")).Placeholder(tr(c, "Search"))
		textButton(c, tr(c, "Refresh")).OnClick(func() { a.dispatch(contracts.Command{Kind: "resource.refresh"}) })
		textButton(c, tr(c, "Import skill")).OnClick(a.importSkill)
		primaryButton(c, tr(c, "Add skill")).OnClick(func() { a.editSkill(nil) })
	})
	query := strings.ToLower(strings.TrimSpace(*s.queries["Skills"]))
	found := 0
	for _, skill := range a.state.UserResources.Skills {
		if !matchesResource(query, skill.Name, skill.Description) {
			continue
		}
		found++
		ui.Row(c.Key("skill-"+skill.ID)).FillWidth().AlignItems(ui.Center).Gap(8).Padding(10, 4).BorderWidth(0, 0, 1, 0).BorderColor(a.palette.Border).Children(func() {
			controlRowButton(c.Key("details")).Grow(1).MinWidth(0).Label(tr(c, "Skill details") + " " + skill.Name).Children(func() {
				ui.Column(c).Grow(1).Gap(3).Children(func() {
					ui.Text(c, skill.Name).FontWeight(500)
					if skill.Description != "" {
						ui.Text(c, skill.Description).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted)
					}
				})
			}).OnClick(func() { a.showSkillDetails(skill) })
			enabled := skill.Enabled
			a.controlSwitch(c.Key("enabled"), &enabled, tr(c, "Enable")+" "+skill.Name, false, func(enabled bool) {
				a.dispatch(contracts.Command{Kind: "resource.skill.toggle", ID: skill.ID, Enabled: enabled})
			})
			textButton(c, tr(c, "Remove")).Label(tr(c, "Remove") + " " + skill.Name).OnClick(func() {
				a.confirmSettingsRemoval("skill", skill.ID, skill.Name)
			})
		})
	}
	if found == 0 {
		ui.Text(c, tr(c, "No skills")).TextColor(a.palette.Muted)
	}
	a.skillDetailsDialog(c)
	a.settingsDialog(c, "skill-import", tr(c, "Import skill"), &s.skillImportOpen, 600, func() {
		resourceText(c, "skill-import", "Import directory", &s.skillImport)
		textButton(c, tr(c, "Browse…")).Disabled(a.state.Runtime != nil && a.state.Runtime.Remote).OnClick(a.browseSkillDirectory)
		if a.state.Runtime != nil && a.state.Runtime.Remote {
			ui.Text(c, tr(c, "The import directory is on the remote host.")).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted)
		}
		resourceText(c, "skill-import-name", "Skill name", &s.skill.Name)
		resourceText(c, "skill-import-description", "Description", &s.skill.Description)
		ui.Row(c).FillWidth().Justify(ui.End).Gap(8).Children(func() {
			textButton(c, tr(c, "Cancel")).OnClick(func() { s.skillImportOpen = false })
			primaryButton(c, tr(c, "Import")).Disabled(strings.TrimSpace(s.skillImport) == "").OnClick(func() {
				value := s.skill
				s.skillImportOpen = false
				a.dispatch(contracts.Command{Kind: "resource.skill.import", Resource: &contracts.ResourceInput{Skill: &value, ImportPath: s.skillImport}})
			})
		})
	})
	title := tr(c, "Add skill")
	for _, saved := range a.state.UserResources.Skills {
		if saved.ID == s.skill.ID {
			title = tr(c, "Edit skill")
			break
		}
	}
	a.settingsDialog(c, "skill-editor", title, &s.skillOpen, 680, func() {
		resourceText(c, "skill-name", "Skill name", &s.skill.Name)
		resourceText(c, "skill-description", "Description", &s.skill.Description)
		controlTextArea(c.Key("skill-content"), &s.skill.Content).FillWidth().Height(260).Label("SKILL.md").Placeholder("SKILL.md")
		for _, file := range s.skill.Files {
			ui.Text(c, file).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted)
		}
		ui.Row(c).FillWidth().Justify(ui.End).Gap(8).Children(func() {
			textButton(c, tr(c, "Cancel")).OnClick(func() { s.skillOpen = false })
			primaryButton(c, tr(c, "Save")).OnClick(func() {
				value := s.skill
				s.skillOpen = false
				a.dispatch(contracts.Command{Kind: "resource.skill.save", Resource: &contracts.ResourceInput{Skill: &value}})
			})
		})
	})
}

func (a *App) skillDetailsDialog(c *ui.Context) {
	s := &a.capabilityUI
	if !s.skillDetailsOpen {
		return
	}
	for _, current := range a.state.UserResources.Skills {
		if current.ID == s.skill.ID {
			s.skill = current
			s.skill.Files = slices.Clone(current.Files)
			break
		}
	}
	a.settingsDialog(c, "skill-details", s.skill.Name, &s.skillDetailsOpen, 600, func() {
		if s.skill.Description != "" {
			ui.Text(c, s.skill.Description)
		}
		status := "Disabled"
		if s.skill.Enabled {
			status = "Enabled"
		}
		ui.Row(c).FillWidth().Gap(12).Children(func() {
			ui.Text(c, tr(c, "Status")).TextColor(a.palette.Muted)
			ui.Text(c, tr(c, status))
		})
		if s.skill.Path != "" {
			ui.Text(c, s.skill.Path).FillWidth().FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted)
		} else {
			ui.Text(c, "SKILL.md").FontWeight(500)
		}
		for _, file := range s.skill.Files {
			ui.Text(c, file).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted)
		}
		ui.Row(c).FillWidth().Justify(ui.End).Gap(8).Children(func() {
			textButton(c, tr(c, "Close")).OnClick(func() { s.skillDetailsOpen = false })
			textButton(c, tr(c, "Edit skill")).OnClick(func() {
				value := s.skill
				a.editSkill(&value)
			})
		})
	})
}

func (a *App) browseSkillDirectory() {
	window := a.win
	if window == nil {
		return
	}
	skillID := a.capabilityUI.skill.ID
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: window, Directory: true})
		window.Update(func() {
			if a.closed || !a.capabilityUI.skillImportOpen || a.capabilityUI.skill.ID != skillID {
				return
			}
			if err != nil {
				a.errorText = err.Error()
			} else if len(paths) > 0 {
				a.capabilityUI.skillImport = paths[0]
			}
		})
	}()
}
