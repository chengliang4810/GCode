package desktop

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

func workflowArtifactPreview(artifact contracts.WorkflowArtifact) (title, content string, isMarkdown bool) {
	title = strings.TrimSpace(artifact.Name)
	isMarkdown = strings.EqualFold(filepath.Ext(title), ".md") ||
		strings.EqualFold(filepath.Ext(artifact.Path), ".md") ||
		strings.EqualFold(strings.TrimSpace(strings.SplitN(artifact.MIME, ";", 2)[0]), "text/markdown")
	content = artifact.Content
	return
}

func (a *App) workflowArtifactView(c *ui.Context, key string) {
	runID, name, _ := strings.Cut(key, "/")
	run, _ := a.workflowRun(runID)
	var versions []contracts.WorkflowArtifact
	for _, artifact := range run.Artifacts {
		if artifact.Name == name {
			versions = append(versions, artifact)
		}
	}
	slices.SortStableFunc(versions, func(left, right contracts.WorkflowArtifact) int {
		return right.Version - left.Version
	})
	ui.Column(c.Key("workflow-artifact-pane:" + key)).Fill().Padding(16).Gap(10).Children(func() {
		if len(versions) == 0 {
			ui.Text(c, tr(c, "No artifacts")).TextColor(a.palette.Muted)
			return
		}
		index := 0
		for i, artifact := range versions {
			if artifact.Version == a.workflowUI.artifactVersions[key] {
				index = i
				break
			}
		}
		artifact := versions[index]
		title, content, isMarkdown := workflowArtifactPreview(artifact)
		selectVersion := func(index int) {
			if a.workflowUI.artifactVersions == nil {
				a.workflowUI.artifactVersions = make(map[string]int)
			}
			a.workflowUI.artifactVersions[key] = versions[index].Version
		}
		ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, title).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").Tooltip(title).FontWeight(600)
			iconButton(c.Key("run:"+key), "alt-arrow-left", tr(c, "Open run details")).OnClick(func() { a.openWorkflowRun(runID) })
			if content != "" {
				services := c.Services()
				iconButton(c.Key("copy:"+key), "copy", tr(c, "Copy")).OnClick(func() { services.WriteClipboard(content) })
			}
		})
		if len(versions) > 1 {
			ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
				iconButton(c.Key("older:"+key), "alt-arrow-left", tr(c, "Previous version")).Disabled(index+1 == len(versions)).
					OnClick(func() { selectVersion(index + 1) })
				ui.Text(c, fmt.Sprintf("v%d", artifact.Version)).FontFeatures("tnum")
				iconButton(c.Key("newer:"+key), "alt-arrow-right", tr(c, "Next version")).Disabled(index == 0).
					OnClick(func() { selectVersion(index - 1) })
			})
		}
		if artifact.Path != "" {
			ui.Text(c, artifact.Path).FillWidth().Wrap().Selectable().TextColor(a.palette.Muted)
		}
		ui.Divider(c)
		ui.Scroll(c.Key("preview:" + artifact.ID)).FillWidth().Grow(1).Children(func() {
			if strings.TrimSpace(content) == "" {
				message := "No preview available for this file."
				if strings.TrimSpace(artifact.Path) == "" {
					message = "This artifact was published without content or a file."
				}
				ui.Text(c, tr(c, message)).FillWidth().Wrap().TextColor(a.palette.Muted)
				return
			}
			if isMarkdown {
				options := markdown.Options{Theme: a.md.Theme, Extensions: a.md.Extensions, CodeBlock: design.MarkdownCodeBlock}
				markdown.View(c, content, &options).FillWidth().Selectable()
				return
			}
			ui.Text(c, content).FillWidth().Wrap().Selectable().
				FontSize(design.TextSize(c, design.TextSmall))
		})
	})
}
