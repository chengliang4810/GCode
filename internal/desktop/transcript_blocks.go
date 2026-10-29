package desktop

import (
	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"strings"
)

// transcriptBlock retains native specialized content outside ordinary activity trees.
func (a *App) transcriptBlock(c *ui.Context, m contracts.Message, b contracts.Block) {
	switch b.Kind {
	case "text":
		if m.Streaming {
			markdown.Stream(c, b.Text, &a.md)
		} else {
			markdown.View(c, b.Text, &a.md)
		}
	case "code":
		fence := strings.Repeat(string(rune(96)), 3)
		markdown.View(c, fence+b.Language+"\n"+b.Text+"\n"+fence, &a.md)
	case "question":
		if b.Question != nil {
			q := b.Question
			ui.Column(c).FillWidth().Padding(16).Radius(10).Border(1, a.palette.Border).Gap(10).Children(func() {
				ui.Text(c, q.Title).FontWeight(600)
				for _, option := range q.Options {
					answer := option
					btn := textButton(c, option).FillWidth()
					if q.Answer == option {
						btn.Background(a.palette.Active)
					}
					btn.Disabled(q.Answer != "").OnClick(func() { a.dispatch(contracts.Command{Kind: "request.answer", ID: q.ID, Text: answer}) })
				}
				if q.Answer != "" {
					ui.Text(c, q.Answer).TextColor(a.palette.Success)
				}
			})
		}
	case "permission":
		if b.Permission != nil {
			p := b.Permission
			ui.Column(c).Padding(16).Radius(10).Border(1, a.palette.Border).Gap(10).Children(func() {
				ui.Text(c, p.Title).FontWeight(600)
				ui.Text(c, p.Description).TextColor(a.palette.Muted)
				ui.Text(c, p.Command).Font("Geist Mono").FontSize(design.UISize(c, 12)).Selectable()
				if p.Answer == "" {
					ui.Row(c).Gap(8).Children(func() {
						for _, v := range []string{"Allow once", "Always allow", "Deny"} {
							answer := v
							textButton(c, tr(c, v)).OnClick(func() { a.dispatch(contracts.Command{Kind: "request.answer", ID: p.ID, Text: answer}) })
						}
					})
				} else {
					ui.Text(c, p.Answer).TextColor(a.palette.Muted)
				}
			})
		}
	case "todo", "todos":
		for _, todo := range b.Todos {
			ui.Row(c).Gap(8).Children(func() {
				if todo.Status == "completed" {
					icon(c, "check", 14)
				} else {
					icon(c, "clock-circle", 14)
				}
				ui.Text(c, todo.Text).FontSize(design.UISize(c, 13))
			})
		}
	case "subagent":
		if b.Subagent != nil {
			a.transcriptSubagent(c, *b.Subagent)
		}
	case "attachment", "image", "appshot":
		if b.Attachment != nil {
			if strings.HasPrefix(b.Attachment.MIME, "image/") {
				a.imageAttachment(c, *b.Attachment)
			} else {
				a.attachmentChip(c, *b.Attachment, false)
			}
		}
	case "math", "diagram":
		if b.Artifact != nil {
			a.artifactView(c, *b.Artifact, b.Kind)
		}
	case "annotation":
		if b.Annotation != nil {
			annotation := *b.Annotation
			ui.ButtonBase(c).Padding(5, 8).Radius(8).Border(1, a.palette.Border).Label(a.annotationLabel(annotation.Index)).OnClick(func() { a.workspace.FocusAnnotation(annotation.Index) }).Children(func() {
				icon(c, "cursor-square", 14)
				ui.Text(c, a.annotationLabel(annotation.Index)).FontSize(design.UISize(c, 12))
			})
		}
	default:
		if b.Text != "" {
			ui.Text(c, b.Text).Selectable()
		}
	}
}

func (a *App) transcriptSubagent(c *ui.Context, agent contracts.Subagent) {
	color := a.palette.Muted
	if agent.Status == "failed" || agent.Status == "errored" {
		color = a.palette.Danger
	}
	button := ui.ButtonBase(c).FillWidth().Height(30).Padding(0, 8).Gap(8).Radius(9).Border(1, a.activityInk(.07)).Background(a.activityInk(.03)).TextColor(color).AlignItems(ui.Center).Justify(ui.Start).Label(tr(c, "Open agent: ") + agent.Name).Tooltip(agent.Task)
	if agent.SessionID != "" {
		button.OnClick(func() { a.workspace.OpenSession(agent.SessionID) })
	}
	button.Children(func() {
		ui.Row(c).Size(18, 18).Radius(5).Background(a.activityInk(.08)).AlignItems(ui.Center).Justify(ui.Center).Children(func() { icon(c, "bot", 12) })
		ui.Text(c, tr(c, "Agent")).FontWeight(500).FontSize(design.UISize(c, 12)).SingleLine().TextColor(color)
		ui.Text(c, firstNonempty(agent.Name, agent.Task)).FontSize(design.UISize(c, 12)).SingleLine().Grow(1).TextColor(color)
		if agent.Status == "working" || agent.Status == "running" {
			ui.Spinner(c).Size(11, 11)
		}
		icon(c, "arrow-up-right", 11)
	})
}
