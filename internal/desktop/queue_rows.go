package desktop

import (
	"gcode/internal/contracts"

	"github.com/egoist/mygo/ui"
)

// queueRowDrag identifies a queued row while it is being dragged. The session
// keeps a drag inside the queue it came from.
type queueRowDrag struct {
	SessionID string
	ID        string
}

// queueRows draws the queued messages above the composer. A row can be
// dragged before or after another row of the same session; Send now stops the
// live run and sends the row at once. Editing takes the row back into the
// composer. A steering row is absent here on purpose: it is already the user's
// message and the conversation shows it, waiting for its step boundary.
func (a *App) queueRows(c *ui.Context, sessionID string) {
	real := a.state.Runtime != nil
	for _, queued := range a.state.Queue {
		if queued.SessionID != sessionID || queued.Delivery == "steer" {
			continue
		}
		q := queued
		row := ui.Row(c).Padding(6, 24).Gap(10).AlignItems(ui.Center).Children(func() {
			icon(c, "queue-drag-handle", 14)
			ui.Text(c, q.Text).Grow(1).SingleLine().TextColor(a.palette.Muted)
			if real {
				iconButton(c, "queue-send", tr(c, "Send now (interrupt)")).OnClick(func() {
					a.dispatch(contracts.Command{Kind: "queue.send", ID: q.ID})
				})
				iconButton(c, "arrow-up", tr(c, "Send as steering")).OnClick(func() {
					a.dispatch(contracts.Command{Kind: "queue.steer", ID: q.ID})
				})
			}
			iconButton(c, "pen", tr(c, "Edit queued message")).OnClick(func() {
				a.draft = q.Text
				a.attachments = append([]contracts.Attachment(nil), q.Attachments...)
				a.annotations = append([]contracts.Annotation(nil), q.Annotations...)
				if q.ModelID != "" {
					a.modelID = q.ModelID
				}
				if q.ReasoningLevel != "" {
					a.reasoning = q.ReasoningLevel
				}
				if q.DeviceID != "" {
					a.deviceID = q.DeviceID
				}
				if q.Branch != "" {
					a.branch = q.Branch
				}
				if q.WorktreeID != "" {
					a.checkout = "New worktree"
				} else {
					a.checkout = "Current checkout"
				}
				a.normalizeReasoning()
				a.dispatch(contracts.Command{Kind: "queue.remove", ID: q.ID})
			})
			iconButton(c, "close", tr(c, "Remove queued message")).OnClick(func() {
				a.dispatch(contracts.Command{Kind: "queue.remove", ID: q.ID})
			})
		})
		a.queueDragTarget(c, row, q.SessionID, q.ID)
	}
}

// queueDragTarget makes the whole row a drag source. Dropping it over another
// row of the same session moves it before or after that row.
func (a *App) queueDragTarget(c *ui.Context, row ui.Element, sessionID, id string) ui.Element {
	real := a.state.Runtime != nil
	row = controlReorderRow(c, row, queueRowDrag{SessionID: sessionID, ID: id}, !real)
	dragged, after, dropped := controlReorderTarget[queueRowDrag](c, row, func(value queueRowDrag) bool {
		return real && value.SessionID == sessionID && value.ID != id
	})
	if dropped {
		a.dispatch(contracts.Command{Kind: "queue.move", Reorder: &contracts.ReorderInput{
			SourceID: dragged.ID, TargetID: id, After: after,
		}})
	}
	return row
}
