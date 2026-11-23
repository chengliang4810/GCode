package desktop

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/i18n"
	"github.com/egoist/mygo/ui"
)

type automationPageState struct {
	Draft      contracts.Automation
	Interval   string
	Open, Busy bool
	Error      string
	DeleteID   string
	SelectedID string
	Filter     string
	Module     string
}

// automationModuleWorkflows 是自动化页里的工作流模块。自动化和工作流是同一类
// “已保存、可再次运行”的东西，所以切换留在同一页里，不另造一个应用或一条
// 侧栏入口。
const automationModuleWorkflows = "workflows"

func (a *App) editAutomation(value *contracts.Automation) {
	s := &a.automationUI
	s.Draft = contracts.Automation{Name: "", Mode: "cron", Cron: "0 9 * * 1-5", Timezone: "Asia/Shanghai", Enabled: true, IntervalSeconds: 3600}
	if len(a.state.Models) > 0 {
		s.Draft.ModelID = a.state.Models[0].ID
	}
	if value != nil {
		s.Draft = *value
	}
	s.Interval = strconv.FormatInt(s.Draft.IntervalSeconds, 10)
	s.Open, s.Error = true, ""
}

func (a *App) automationAction(command contracts.Command, after func()) {
	s := &a.automationUI
	if s.Busy {
		return
	}
	s.Busy, s.Error = true, ""
	command = command.Clone()
	scope, generation := a.captureSessionScope()
	a.backgroundTask(func() func() {
		err := a.service.Execute(a.ctx, command)
		snapshot, snapshotErr := a.service.Snapshot(a.ctx, scope...)
		return func() {
			s.Busy = false
			if err != nil {
				s.Error = err.Error()
				return
			}
			if snapshotErr != nil {
				s.Error = snapshotErr.Error()
				return
			}
			if a.sessionGenerationCurrent(generation, scope) {
				a.apply(snapshot)
			}
			if after != nil {
				after()
			}
		}
	})
}

// automationPage is a master/detail workspace; editing stays in the shared modal.
func (a *App) automationPage(c *ui.Context) {
	s := &a.automationUI
	if s.Module == automationModuleWorkflows {
		a.automationWorkflowPage(c)
		return
	}
	jobs := []contracts.Automation{}
	for _, job := range a.state.Automations {
		if s.Filter == "active" && !job.Enabled || s.Filter == "paused" && job.Enabled {
			continue
		}
		jobs = append(jobs, job)
	}
	slices.SortStableFunc(jobs, func(a, b contracts.Automation) int { return strings.Compare(a.ProjectID, b.ProjectID) })
	var selected *contracts.Automation
	for i := range jobs {
		if jobs[i].ID == s.SelectedID {
			selected = &jobs[i]
		}
	}
	if selected == nil && len(jobs) > 0 {
		selected = &jobs[0]
	}
	ui.Column(c.Key("automation-workspace")).Fill().Gap(0).Children(func() {
		ui.Row(c.Key("automation-header")).FillWidth().Padding(16, 20).Gap(10).AlignItems(ui.Center).Children(func() {
			icon(c, "widget", 18)
			ui.Text(c, tr(c, "Automations")).Bold()
			ui.Text(c, fmt.Sprintf("%d", len(a.state.Automations))).TextColor(c.Theme().TextMuted)
			ui.Spacer(c).Grow(1)
			buttonWithIcon(c.Key("automation-new"), tr(c, "New automation"), "plus", buttonPrimary).Disabled(s.Busy).OnClick(func() { a.editAutomation(nil) })
		})
		ui.Box(c).FillWidth().Height(1).Background(c.Theme().Border)
		a.automationModuleSwitcher(c)
		if s.Error != "" && !s.Open {
			ui.Text(c, s.Error).Margin(12, 20).TextColor(a.palette.Readable(a.palette.Danger)).Wrap()
		}
		width, _ := c.Size()
		narrow := width < 800
		var body ui.Element
		if narrow {
			body = ui.Column(c.Key("automation-body")).FillWidth().Grow(1).MinHeight(0)
		} else {
			body = ui.Row(c.Key("automation-body")).FillWidth().Grow(1).MinHeight(0)
		}
		body.AlignItems(ui.Stretch).Children(func() {
			list := ui.Column(c.Key("automation-list")).Gap(12).Padding(16).MinHeight(0)
			if narrow {
				list.FillWidth().Height(190)
			} else {
				list.Width(280).FillHeight().Shrink(0)
			}
			list.Children(func() {
				ui.Row(c).FillWidth().Gap(4).Wrap().Children(func() {
					for _, option := range []struct{ value, label string }{{"", "All"}, {"active", "Active"}, {"paused", "Paused"}} {
						option := option
						textButton(c.Key("automation-filter-"+option.value), tr(c, option.label)).OnClick(func() { s.Filter = option.value })
					}
				})
				ui.Scroll(c.Key("automation-task-list")).Grow(1).FillWidth().Gap(8).Children(func() {
					lastGroup := "\x00"
					for _, value := range jobs {
						job := value
						project := tr(c, "No project")
						for _, p := range a.state.Projects {
							if p.ID == job.ProjectID {
								project = p.Name
							}
						}
						if lastGroup != project {
							ui.Text(c, project).TextColor(c.Theme().TextMuted).Margin(0, 0, 4, 0)
							lastGroup = project
						}
						controlTileButton(c.Key("automation-select-"+job.ID), selected != nil && selected.ID == job.ID).FillWidth().Label(job.Name).OnClick(func() { s.SelectedID = job.ID }).Children(func() {
							ui.Column(c).FillWidth().Gap(6).Children(func() {
								ui.Text(c, job.Name).Bold().SingleLine()
								ui.Row(c).Gap(8).Children(func() { automationSummaryTexts(c, a.automationSummary(c, job)...) })
							})
						})
					}
					if len(jobs) == 0 {
						ui.Text(c, tr(c, "No automations")).TextColor(c.Theme().TextMuted).Wrap()
					}
				})
			})
			divider := ui.Box(c).Background(c.Theme().Border).Shrink(0)
			if narrow {
				divider.FillWidth().Height(1)
			} else {
				divider.Width(1).FillHeight()
			}
			ui.Scroll(c.Key("automation-detail")).Grow(1).MinWidth(0).Padding(24).Gap(20).Justify(ui.Start).Children(func() {
				if selected == nil {
					ui.Text(c, tr(c, "Automate your recurring work")).Bold()
					ui.Text(c, tr(c, "Create an automation to schedule a task and review its runs here.")).TextColor(c.Theme().TextMuted).Wrap()
					return
				}
				job := *selected
				ui.Column(c.Key("automation-" + job.ID)).FillWidth().Gap(12).Children(func() {
					ui.Text(c, job.Name).Bold().Wrap()
					ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
						automationSummaryTexts(c, slices.Concat(a.automationSummary(c, job), []string{job.ModelID})...)
					})
					ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
						buttonWithIcon(c.Key("automation-run-"+job.ID), tr(c, "Run now"), "action-play", buttonPlain).Disabled(s.Busy).OnClick(func() { a.automationAction(contracts.Command{Kind: "automation.run", ID: job.ID}, nil) })
						textButton(c.Key("automation-edit-"+job.ID), tr(c, "Edit")).Disabled(s.Busy).OnClick(func() { a.editAutomation(&job) })
						label := "Resume"
						if job.Enabled {
							label = "Pause"
						}
						textButton(c.Key("automation-toggle-"+job.ID), tr(c, label)).Disabled(s.Busy).OnClick(func() {
							a.automationAction(contracts.Command{Kind: "automation.toggle", ID: job.ID, Enabled: !job.Enabled}, nil)
						})
						textButton(c.Key("automation-delete-"+job.ID), tr(c, "Delete")).Disabled(s.Busy).OnClick(func() { s.DeleteID = job.ID })
					})
					if s.DeleteID == job.ID {
						ui.Text(c, tr(c, "Delete this automation and its run history? Conversations are kept.")).Wrap()
						ui.Row(c).Gap(8).Children(func() {
							textButton(c, tr(c, "Cancel")).OnClick(func() { s.DeleteID = "" })
							textButton(c, tr(c, "Confirm delete")).Disabled(s.Busy).OnClick(func() {
								a.automationAction(contracts.Command{Kind: "automation.delete", ID: job.ID}, func() { s.DeleteID = "" })
							})
						})
					}
				})
				ui.Text(c, tr(c, "Prompt")).TextColor(c.Theme().TextMuted)
				ui.Text(c, job.Prompt).Wrap()
				ui.Box(c).FillWidth().Height(1).Background(c.Theme().Border)
				ui.Text(c, tr(c, "Recent runs")).Bold()
				shown := 0
				for i := len(a.state.AutomationRuns) - 1; i >= 0; i-- {
					run := a.state.AutomationRuns[i]
					if run.AutomationID != job.ID {
						continue
					}
					shown++
					ui.Column(c.Key("automation-history-"+run.ID)).FillWidth().Padding(16).Gap(12).Radius(10).Border(1, c.Theme().Border).Background(c.Theme().Surface).Children(func() {
						ui.Row(c).FillWidth().Wrap().Gap(12).Children(func() {
							ui.Text(c, run.StartedAt.Local().Format("2006-01-02 15:04:05")).Bold()
							ui.Spacer(c).Grow(1)
							ui.Text(c, tr(c, run.Status)).TextColor(c.Theme().TextMuted)
						})
						if !run.FinishedAt.IsZero() {
							ui.Text(c, i18n.Duration(a.state.Settings.General.Language, run.FinishedAt.Sub(run.StartedAt).Milliseconds())).FontFeatures("tnum").TextColor(c.Theme().TextMuted)
						}
						if run.Error != "" {
							ui.Text(c, run.Error).Wrap().TextColor(a.palette.Readable(a.palette.Danger))
						}
						if run.SessionID != "" {
							textButton(c.Key("automation-open-"+run.ID), tr(c, "Open session")).OnClick(func() { a.selectSession(run.SessionID); a.navigate("chat/" + run.SessionID) })
						}
					})
				}
				if shown == 0 {
					ui.Text(c, tr(c, "No runs yet")).TextColor(c.Theme().TextMuted)
				}
			})
		})
	})
	title := "New automation"
	if s.Draft.ID != "" {
		title = "Edit automation"
	}
	a.settingsEditorDialog(c, "automation-editor-dialog", title, &s.Open, s.Busy, 800, s.Error, func() { a.automationForm(c) }, func() { a.automationFormActions(c) })
}

// automationSummary returns the schedule reading in parts so the caller renders
// one text per part and the row gap divides them.
func (a *App) automationSummary(c *ui.Context, job contracts.Automation) []string {
	if !job.Enabled {
		return []string{tr(c, "Paused")}
	}
	if !job.NextAt.IsZero() {
		loc, err := time.LoadLocation(job.Timezone)
		if err != nil {
			loc = time.Local
		}
		return []string{tr(c, "Next run"), job.NextAt.In(loc).Format("01-02 15:04")}
	}
	switch job.Mode {
	case "cron":
		return []string{job.Cron, job.Timezone}
	case "interval":
		return []string{tr(c, "After completion"), i18n.Duration(a.state.Settings.General.Language, job.IntervalSeconds*1000)}
	default:
		return []string{tr(c, "Manual")}
	}
}

func automationSummaryTexts(c *ui.Context, parts ...string) {
	for _, part := range parts {
		ui.Text(c, part).TextColor(c.Theme().TextMuted)
	}
}

func (a *App) automationForm(c *ui.Context) {
	s := &a.automationUI
	d := &s.Draft
	resourceText(c, "automation-name", "Automation name", &d.Name)
	ui.Column(c).FillWidth().Gap(4).Children(func() {
		ui.Text(c, tr(c, "Prompt")).TextColor(a.palette.Muted)
		controlTextArea(c.Key("automation-prompt"), &d.Prompt).Height(140).FillWidth().Label(tr(c, "Automation prompt"))
	})
	projects := []settingOption{{"", "No project"}}
	for _, p := range a.state.Projects {
		projects = append(projects, settingOption{p.ID, p.Name})
	}
	models := []settingOption{}
	for _, m := range a.state.Models {
		models = append(models, settingOption{m.ID, m.Label})
	}
	levels := []settingOption{{"", "Default"}}
	for _, m := range a.state.Models {
		if m.ID == d.ModelID {
			for _, level := range m.ReasoningLevels {
				levels = append(levels, settingOption{level, level})
			}
		}
	}
	ui.Row(c.Key("automation-context-fields")).FillWidth().Wrap().Gap(12).Children(func() {
		automationField(c, 160, func() { a.automationSelect(c, "automation-project", "Project", &d.ProjectID, projects) })
		automationField(c, 220, func() { a.automationSelect(c, "automation-model", "Model", &d.ModelID, models) })
		automationField(c, 150, func() { a.automationSelect(c, "automation-reasoning", "Reasoning effort", &d.ReasoningLevel, levels) })
	})
	targets := []settingOption{{"", "Create a session"}}
	for _, session := range a.state.Sessions {
		if !session.Archived && session.SubagentType == "" {
			targets = append(targets, settingOption{session.ID, session.Title})
		}
	}
	automationSection(c, "Session behavior", func() {
		a.automationSelect(c, "automation-target", "Bind existing session", &d.TargetSessionID, targets)
		controlCheckbox(c.Key("automation-continuous"), &d.Continuous, tr(c, "Continue the same session"))
	})
	automationSection(c, "Trigger settings", func() {
		ui.Row(c.Key("automation-trigger-fields")).FillWidth().Wrap().Gap(12).Children(func() {
			automationField(c, 150, func() {
				a.automationSelect(c, "automation-mode", "Schedule", &d.Mode, []settingOption{{"cron", "Cron"}, {"interval", "After completion"}, {"manual", "Manual"}})
			})
			switch d.Mode {
			case "cron":
				automationField(c, 200, func() { resourceText(c, "automation-cron", "Cron expression (five fields)", &d.Cron) })
				automationField(c, 150, func() { resourceText(c, "automation-timezone", "Timezone", &d.Timezone) })
			case "interval":
				automationField(c, 230, func() { resourceText(c, "automation-interval", "Interval in seconds (minimum 60)", &s.Interval) })
			}
		})
		if d.Mode == "cron" {
			ui.Text(c, tr(c, "Example: 0 9 * * 1-5 runs at 09:00 on weekdays.")).Wrap().TextColor(a.palette.Muted)
		}
		ui.Row(c).FillWidth().Wrap().Gap(12).Children(func() {
			controlCheckbox(c.Key("automation-once"), &d.Once, tr(c, "Run once"))
			controlCheckbox(c.Key("automation-enabled"), &d.Enabled, tr(c, "Enabled"))
		})
	})
}

func automationField(c *ui.Context, basis float32, draw func()) {
	ui.Column(c).Basis(basis).Grow(1).MinWidth(0).Children(draw)
}

func automationSection(c *ui.Context, title string, draw func()) {
	ui.Column(c.Key(title)).FillWidth().Padding(8, 0, 0, 0).Gap(8).Children(func() {
		ui.Text(c, tr(c, title)).FontSize(design.TextSize(c, design.TextBase)).FontWeight(600)
		draw()
	})
}

func (a *App) automationSelect(c *ui.Context, key, label string, value *string, options []settingOption) {
	ui.Column(c.Key(key + "-field")).FillWidth().Gap(4).Children(func() {
		ui.Text(c, tr(c, label)).TextColor(a.palette.Muted)
		a.resourceSelect(c, key, label, value, options)
	})
}

func (a *App) automationsView(c *ui.Context) { a.automationPage(c) }

// automationModuleSwitcher 是自动化页顶部的模块切换。自动化和工作流是同一类
// “已保存、可再次运行”的东西，所以切换留在同一页里：不另造独立应用，也不占
// 一条侧栏入口。
func (a *App) automationModuleSwitcher(c *ui.Context) {
	s := &a.automationUI
	ui.Row(c.Key("automation-modules")).FillWidth().Padding(10, 20, 0, 0).Gap(6).Children(func() {
		for _, module := range []struct{ key, id, label string }{
			{"automation-module-automations", "", "Automations"},
			{"automation-module-workflows", automationModuleWorkflows, "Workflows"},
		} {
			module := module
			controlNavButton(c.Key(module.key), tr(c, module.label), s.Module == module.id).Children(func() {
				ui.Text(c, tr(c, module.label))
			}).OnClick(func() { s.Module = module.id })
		}
	})
}

// automationWorkflowPage 是自动化页里的工作流模块：已保存的工作流中枢。
func (a *App) automationWorkflowPage(c *ui.Context) {
	ui.Column(c.Key("automation-workflow-module")).Fill().Gap(0).Children(func() {
		ui.Row(c.Key("automation-workflow-header")).FillWidth().Padding(16, 20).Gap(10).AlignItems(ui.Center).Children(func() {
			icon(c, "checklist", 18)
			ui.Text(c, tr(c, "Workflows")).Bold()
			ui.Text(c, fmt.Sprintf("%d", len(a.state.WorkflowDefinitions))).TextColor(c.Theme().TextMuted)
			ui.Spacer(c).Grow(1)
			buttonWithIcon(c.Key("automation-new-workflow"), tr(c, "New workflow"), "plus", buttonPrimary).OnClick(func() { a.newWorkflowConversation() })
		})
		ui.Box(c).FillWidth().Height(1).Background(c.Theme().Border)
		a.automationModuleSwitcher(c)
		ui.Scroll(c.Key("automation-workflow-body")).FillWidth().Grow(1).MinHeight(0).Padding(16, 20).Gap(16).Justify(ui.Start).Children(func() {
			a.workflowHubView(c)
		})
	})
}

func (a *App) automationFormActions(c *ui.Context) {
	s := &a.automationUI
	d := &s.Draft
	ui.Row(c).FillWidth().Justify(ui.End).Gap(8).Children(func() {
		textButton(c.Key("automation-cancel"), tr(c, "Cancel")).Disabled(s.Busy).OnClick(func() { s.Open = false })
		primaryButton(c.Key("automation-save"), tr(c, "Save")).Disabled(s.Busy).OnClick(func() {
			if d.Mode == "interval" {
				n, err := strconv.ParseInt(s.Interval, 10, 64)
				if err != nil {
					s.Error = fmt.Sprint(err)
					return
				}
				d.IntervalSeconds = n
			}
			a.automationAction(contracts.Command{Kind: "automation.save", Automation: d}, func() { s.Open = false })
		})
	})
}
