package desktop

import (
	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/linktarget"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"strings"
)

// transcriptBlock retains native specialized content outside ordinary activity trees.
func (a *App) transcriptBlock(c *ui.Context, m contracts.Message, b contracts.Block) {
	switch b.Kind {
	case "text":
		source := linktarget.NormalizeMarkdown(b.Text)
		options := a.transcriptOptions(m.SessionID, false)
		if m.Streaming {
			markdown.Stream(c, source, options)
		} else {
			markdown.View(c, source, options)
		}
	case "code":
		fence := strings.Repeat(string(rune(96)), 3)
		markdown.View(c, fence+b.Language+"\n"+b.Text+"\n"+fence, &a.md)
	case "error":
		a.activityTextDetail(c, b, false)
	case "question":
		if b.Question != nil {
			q := b.Question
			value := tr(c, "Awaiting your answer…")
			if q.Resolved && len(q.Pages) > 0 {
				value = q.Pages[0].Header
			}
			ui.Row(c.Key("question-record-"+q.ID)).FillWidth().Padding(6, 8).Radius(design.Controls().InputRadius).
				Border(1, a.palette.Border).Gap(8).AlignItems(ui.Center).Children(func() {
				icon(c, "chat-round-line", design.Controls().IconSize)
				ui.Text(c, tr(c, "Question")).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
				ui.Text(c, value).SingleLine().Grow(1).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
			})
		}
	case "permission":
		if b.Permission != nil {
			p := b.Permission
			ui.Column(c).Padding(16).Radius(10).Border(1, a.palette.Border).Gap(10).Children(func() {
				ui.Text(c, p.Title).FontWeight(600)
				ui.Text(c, p.Description).TextColor(a.palette.Muted)
				ui.Text(c, p.Command).Font(design.MonoFont(a.state.Settings.Appearance.CodeFont)).FontSize(float32(a.state.Settings.Appearance.CodeFontSize)).Selectable()
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
				ui.Text(c, a.annotationLabel(annotation.Index)).FontSize(design.TextSize(c, design.TextBase))
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
		ui.Text(c, tr(c, "Agent")).FontWeight(500).FontSize(design.TextSize(c, design.TextBase)).SingleLine().TextColor(color)
		ui.Text(c, firstNonempty(agent.Name, agent.Task)).FontSize(design.TextSize(c, design.TextBase)).SingleLine().Grow(1).TextColor(color)
		if agent.Status == "working" || agent.Status == "running" {
			ui.Spinner(c).Size(11, 11)
		}
		icon(c, "arrow-up-right", 11)
	})
}
