package desktop

import (
	"bytes"
	"errors"
	"strconv"
	"strings"

	"gcode/internal/contracts"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

func (a *App) chooseAttachments() { a.attachmentWork(contracts.AttachmentImportRequest{}, true) }

func (a *App) attachmentScope() string {
	if a.sessionID != "" {
		return "session:" + a.sessionID
	}
	return "draft:" + strconv.FormatUint(a.draftEpoch, 10)
}
func (a *App) importAttachments(request contracts.AttachmentImportRequest) {
	a.attachmentWork(request, false)
}

func (a *App) attachmentWork(request contracts.AttachmentImportRequest, pick bool) {
	importer, ok := a.service.(contracts.AttachmentImporter)
	if !ok {
		a.errorText = "Attachments are unavailable"
		return
	}
	request = request.Clone()
	request.SessionID = a.sessionID
	if pick && a.attachmentOpen {
		return
	}
	a.attachmentOpen = pick
	scope, epoch := a.attachmentScope(), a.draftEpoch
	sessionScope, sessionGeneration := a.captureSessionScope()
	if a.attachmentImports == nil {
		a.attachmentImports = make(map[string]int)
	}
	a.attachmentImports[scope]++
	origin, window, ctx, picker := a.sessionID, a.win, a.ctx, a.pickAttachmentFiles
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
		snapshot, snapshotErr := a.service.Snapshot(ctx, sessionScope...)
		return func() {
			if a.closed {
				return
			}
			a.attachmentOpen = false
			a.attachmentImports[scope]--
			if a.attachmentImports[scope] == 0 {
				delete(a.attachmentImports, scope)
			}
			if origin == "" && epoch != a.draftEpoch {
				return
			}
			if err != nil {
				a.errorText = err.Error()
				return
			}
			if snapshotErr == nil && a.sessionGenerationCurrent(sessionGeneration, sessionScope) {
				a.apply(snapshot)
			}
			if origin != "" && !a.hasSession(origin) {
				return
			}
			if origin == a.sessionID {
				a.attachments = append(a.attachments, items...)
				a.focusComposer = len(items) > 0
			} else {
				if a.sessionStates == nil {
					a.sessionStates = make(map[string]sessionViewState)
				}
				view := a.sessionStates[origin]
				view.Attachments = append(view.Attachments, items...)
				a.sessionStates[origin] = view
			}
		}
	}
	if window == nil {
		work()()
		return
	}
	go func() { commit := work(); window.Update(commit) }()
}

func (a *App) pasteAttachments(services ui.Services) bool {
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
					a.importAttachments(contracts.AttachmentImportRequest{Paths: paths})
					return true
				}
			} else if bytes, err := data.Read(transfer.PNG); err == nil && len(bytes) > 0 {
				a.importAttachments(contracts.AttachmentImportRequest{Clipboard: &contracts.ClipboardAttachment{Name: "Image.png", MIME: "image/png", Data: bytes}})
				return true
			}
		}
	}
	return false
}

func snapshotMediaRefs(snapshot contracts.Snapshot) []string {
	var refs []string
	for _, attachment := range snapshot.Attachments {
		if strings.HasPrefix(attachment.MIME, "image/") {
			refs = append(refs, attachment.AssetRef)
		}
	}
	return refs
}

// Resource decoding starts on state transitions, never in a drawing callback.
func (a *App) prefetchMedia(refs []string) {
	reader, ok := a.service.(contracts.ResourceService)
	if !ok {
		return
	}
	if a.mediaLoading == nil {
		a.mediaLoading = make(map[string]bool)
	}
	for _, ref := range refs {
		if ref == "" || strings.HasPrefix(ref, "asset://") || a.mediaLoading[ref] {
			continue
		}
		if hasMedia(ref) {
			continue
		}
		a.mediaLoading[ref] = true
		window, ctx := a.win, a.ctx
		work := func() media {
			data, err := reader.ReadResource(ctx, ref)
			if err != nil {
				return media{}
			}
			prefix := bytes.TrimSpace(data)
			if bytes.HasPrefix(prefix, []byte("<svg")) || bytes.HasPrefix(prefix, []byte("<?xml")) {
				svg, _ := ui.ParseSVG(data)
				return media{svg: svg}
			}
			bitmap, _ := ui.DecodeBitmap(data)
			return media{bitmap: bitmap}
		}
		commit := func(result media) {
			if a.closed {
				return
			}
			delete(a.mediaLoading, ref)
			storeMedia(ref, result)
		}
		if window == nil {
			commit(work())
		} else {
			go func() { result := work(); window.Update(func() { commit(result) }) }()
		}
	}
}
