package desktop

import (
	"errors"

	"gcode/internal/contracts"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

// Side and central composers share the typed resource importer and native
// picker. Staging belongs to the captured side state, never the active chat.
func (a *App) sideAttachmentWork(sessionID string, state *sideComposerState, request contracts.AttachmentImportRequest, pick bool) {
	importer, ok := a.service.(contracts.AttachmentImporter)
	if !ok {
		state.Failure = "Attachments are unavailable"
		return
	}
	if pick && a.attachmentOpen {
		return
	}
	if pick {
		a.attachmentOpen = true
	}
	state.Importing++
	request = request.Clone()
	request.SessionID = sessionID
	scope, generation := a.captureSessionScope()
	window, ctx, picker := a.win, a.ctx, a.pickAttachmentFiles
	work := func() func() {
		var err error
		if pick {
			switch {
			case picker != nil:
				request.Paths, err = picker()
			case window != nil:
				request.Paths, err = mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: window, Multiple: true})
			default:
				err = errors.New("file selection is unavailable")
			}
		}
		var items []contracts.Attachment
		if err == nil && (len(request.Paths) > 0 || request.Clipboard != nil) {
			items, err = importer.ImportAttachments(ctx, request)
		}
		snapshot, snapshotErr := a.service.Snapshot(ctx, scope...)
		return func() {
			if a.closed {
				return
			}
			if pick {
				a.attachmentOpen = false
			}
			state.Importing--
			if err != nil {
				state.Failure = err.Error()
				return
			}
			if snapshotErr == nil && a.sessionGenerationCurrent(generation, scope) {
				a.apply(snapshot)
			}
			exists := false
			for _, session := range a.state.Sessions {
				if session.ID == sessionID {
					exists = true
					break
				}
			}
			if !exists {
				return
			}
			state.Attachments = append(state.Attachments, items...)
			state.Failure = ""
			state.FocusPending = len(items) > 0
		}
	}
	if window == nil {
		work()()
		return
	}
	go func() { commit := work(); window.Update(commit) }()
}

func (a *App) pasteSideAttachments(id string, state *sideComposerState, services ui.Services) bool {
	formats := services.ClipboardFormats()
	for _, preferred := range []transfer.Format{transfer.FileList, transfer.PNG} {
		for _, format := range formats {
			if format != preferred {
				continue
			}
			data := services.ReadClipboardData(preferred)
			if preferred == transfer.FileList {
				paths, err := data.Files()
				if err == nil && len(paths) > 0 {
					a.sideAttachmentWork(id, state, contracts.AttachmentImportRequest{Paths: paths}, false)
					return true
				}
			} else if bytes, err := data.Read(transfer.PNG); err == nil && len(bytes) > 0 {
				a.sideAttachmentWork(id, state, contracts.AttachmentImportRequest{Clipboard: &contracts.ClipboardAttachment{Name: "Image.png", MIME: "image/png", Data: bytes}}, false)
				return true
			}
		}
	}
	return false
}
