package desktop

import (
	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func (a *App) attachAnnotation(annotation contracts.Annotation) {
	annotation = annotation.Bounded()
	a.annotations = append(a.annotations, annotation)
	a.route = "chat"
	a.focusComposer = true
}
func (a *App) annotationChips(c *ui.Context) {
	for i, annotation := range a.annotations {
		index, item := i, annotation
		ui.Row(c).Padding(4, 7).Gap(5).Radius(7).Border(1, a.palette.Border).Tooltip(item.Title + " · " + item.Element).OnHover(func(hovered bool) {
			if hovered {
				a.workspace.FocusAnnotation(item.Index)
			} else {
				a.workspace.FocusAnnotation(0)
			}
		}).Children(func() {
			icon(c, "cursor-square", 13)
			ui.Text(c, a.annotationLabel(item.Index)).FontSize(design.TextSize(c, design.TextSmall))
			iconButton(c, "close", tr(c, "Remove ")+a.annotationLabel(item.Index)).Size(16, 16).OnClick(func() { a.removeAnnotation(index) })
		})
	}
}

func (a *App) removeAnnotation(index int) {
	if index < 0 || index >= len(a.annotations) {
		return
	}
	a.annotations = append(a.annotations[:index], a.annotations[index+1:]...)
	indices := make([]uint32, 0, len(a.annotations))
	for _, annotation := range a.annotations {
		indices = append(indices, annotation.Index)
	}
	a.workspace.KeepAnnotations(indices)
}
