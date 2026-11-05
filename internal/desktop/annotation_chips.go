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
	hovered := uint32(0)
	for i, annotation := range a.annotations {
		index, item := i, annotation
		chip := ui.Row(c).Padding(4, 7).Gap(5).Radius(7).Border(1, a.palette.Border).Tooltip(item.Title + " · " + item.Element)
		if chip.Hovered() {
			hovered = item.Index
		}
		chip.Children(func() {
			icon(c, "cursor-square", 13)
			ui.Text(c, a.annotationLabel(item.Index)).FontSize(design.TextSize(c, design.TextSmall))
			chipDismissButton(c, tr(c, "Remove ")+a.annotationLabel(item.Index)).OnClick(func() { a.removeAnnotation(index) })
		})
	}
	// The framework polls for the pointer, so the chip under it is read each
	// frame: the workspace follows the annotation it names, and drops the
	// focus when no chip is under the pointer.
	if hovered != a.hoveredAnnotation {
		if hovered != 0 {
			a.workspace.FocusAnnotation(hovered)
		} else {
			a.workspace.FocusAnnotation(0)
		}
		a.hoveredAnnotation = hovered
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
