package workbench

import (
	"context"
	"fmt"
	"path"
	"strings"

	"gcode/internal/assets"
	"gcode/internal/contracts"
	"gcode/internal/linktarget"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

type fileNavigation struct {
	id                          string
	target                      linktarget.Target
	sourceSession, absolutePath string
	byteColumn                  bool
}

func (s *State) LinkSourceAvailable(sessionID string) bool {
	_, err := s.linkSession(sessionID)
	return err == nil
}

func isMarkdown(name string) bool {
	extension := strings.ToLower(path.Ext(name))
	return extension == ".md" || extension == ".markdown"
}

// LinkRoot returns the source conversation's working directory, using host path
// syntax. It never reads a path on the UI machine.
func (s *State) LinkRoot(sessionID string) (string, error) {
	session, err := s.linkSession(sessionID)
	if err != nil {
		return "", err
	}
	if session.CWD != "" {
		return session.CWD, nil
	}
	return s.projectRoot(session)
}

func (s *State) projectRoot(session contracts.Session) (string, error) {
	for _, project := range s.snapshot.Projects {
		if project.ID == session.ProjectID {
			for _, worktree := range project.Worktrees {
				if worktree.ID == session.WorktreeID {
					return worktree.Path, nil
				}
			}
			return project.Path, nil
		}
	}
	return "", fmt.Errorf("conversation has no working directory")
}

// linkSession accepts only the visible workspace's root or descendants.
func (s *State) linkSession(id string) (contracts.Session, error) {
	var source contracts.Session
	sessions := make(map[string]contracts.Session, len(s.snapshot.Sessions))
	for _, session := range s.snapshot.Sessions {
		sessions[session.ID] = session
		if session.ID == id {
			source = session
		}
	}
	if s.closed.Load() || source.ID == "" || source.Archived {
		return source, fmt.Errorf("link source conversation is unavailable")
	}
	seen := map[string]bool{}
	for current := source; current.ID != "" && !seen[current.ID]; current = sessions[current.ParentSessionID] {
		if current.ID == s.scopeID {
			return source, nil
		}
		seen[current.ID] = true
	}
	return source, fmt.Errorf("link source does not belong to the current workspace")
}

// OpenLink opens a transcript destination in this workspace without changing the
// central conversation. File reads retain the exact source conversation.
func (s *State) OpenLink(sessionID, raw string) error {
	target, err := linktarget.Parse(raw)
	if err != nil {
		return err
	}
	return s.openLinkTarget(sessionID, target)
}

func (s *State) openLinkTarget(sessionID string, target linktarget.Target) error {
	if _, err := s.linkSession(sessionID); err != nil {
		return err
	}
	if target.Kind == linktarget.Web {
		s.openBrowser(target.URL)
		return nil
	}
	if target.Kind != linktarget.File {
		return fmt.Errorf("this link needs a document preview")
	}
	s.cancelLink()
	root, err := s.LinkRoot(sessionID)
	if err != nil {
		return err
	}
	absolute, _, _, err := linktarget.ResolveFile(target.Path, root)
	if err != nil {
		return err
	}
	if service, ok := s.backend.(contracts.LinkedFileService); ok {
		ctx, cancel := context.WithCancel(s.ctx)
		s.linkCancel = cancel
		generation, scope := s.linkGeneration, s.scopeID
		finish := func(result contracts.LinkedFileResult, readErr error) {
			if s.closed.Load() || ctx.Err() != nil || generation != s.linkGeneration || scope != s.scopeID {
				return
			}
			defer cancel()
			s.linkCancel = nil
			if _, err := s.linkSession(sessionID); err != nil {
				return
			}
			if readErr != nil {
				s.Error = readErr.Error()
				return
			}
			s.Error = ""
			s.openDocument(result.File, sessionID, result.AbsolutePath, true)
			s.navigateDocument(s.documents[result.File.ID], target)
			s.choose("file", result.File.ID, result.File.Name)
		}
		request := contracts.LinkedFileRequest{SessionID: sessionID, Path: target.Path}
		if s.win == nil {
			result, readErr := service.ReadLinkedFile(ctx, request)
			finish(result, readErr)
			return readErr
		}
		win := s.win
		go func() {
			result, readErr := service.ReadLinkedFile(ctx, request)
			if ctx.Err() == nil {
				win.Update(func() { finish(result, readErr) })
			}
		}()
		return nil
	}
	// Deterministic backends can only expose their existing snapshot files.
	source, _ := s.linkSession(sessionID)
	projectRoot, err := s.projectRoot(source)
	if err != nil {
		return err
	}
	for _, file := range s.snapshot.Files {
		if file.ProjectID != source.ProjectID {
			continue
		}
		fileAbsolute, _, _, resolveErr := linktarget.ResolveFile(file.Path, projectRoot)
		if resolveErr != nil || !sameHostPath(fileAbsolute, absolute) || file.Kind == "directory" {
			continue
		}
		s.pendingNavigation = &fileNavigation{id: file.ID, target: target, sourceSession: sessionID, absolutePath: absolute}
		s.OpenFile(file.ID)
		s.applyPendingNavigation(file.ID)
		return nil
	}
	return fmt.Errorf("linked file is unavailable: %s", target.Path)
}

func sameHostPath(a, b string) bool {
	if len(a) >= 2 && a[1] == ':' {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func (s *State) cancelLink() {
	if s.linkCancel != nil {
		s.linkCancel()
		s.linkCancel = nil
	}
	s.linkGeneration++
}

func (s *State) applyPendingNavigation(id string) {
	navigation := s.pendingNavigation
	if navigation == nil || navigation.id != id || s.documents[id] == nil {
		return
	}
	s.pendingNavigation = nil
	d := s.documents[id]
	if navigation.sourceSession != "" {
		d.sourceSession, d.absolutePath = navigation.sourceSession, navigation.absolutePath
	}
	if navigation.byteColumn {
		d.preview, d.focus = false, true
		d.editor.GoTo(editor.Pos{Line: navigation.target.Line - 1, Col: max(0, navigation.target.Column)})
	} else {
		s.navigateDocument(d, navigation.target)
	}
}

func (s *State) navigateDocument(d *document, target linktarget.Target) {
	if d == nil {
		return
	}
	d.focus = true
	if target.Line > 0 {
		d.preview = false
		d.editor.GoTo(editor.Pos{Line: target.Line - 1, Col: linktarget.CharacterColumn(d.editor.Buffer.String(), target.Line, target.Column)})
	} else if isMarkdown(d.file.Path) {
		d.preview = true
		d.markdown.Anchor = target.Anchor
		d.markdown.AnchorVersion++
	}
}

func (s *State) openDocument(file contracts.File, sessionID, absolute string, linked bool) {
	if d := s.documents[file.ID]; d != nil {
		if absolute != "" {
			d.absolutePath, d.sourceSession = absolute, sessionID
		}
		if linked {
			if d.editor.Buffer.Modified() {
				if file.Content != d.file.Content {
					d.conflict = true
				}
			} else {
				if file.Content != d.editor.Buffer.String() {
					d.editor.Buffer.Reset(file.Content)
				}
				imageChanged := file.Kind == "image" && (imageResourceChanged(d.file, file) || d.image == nil)
				d.file, d.deleted, d.conflict = file, false, false
				if imageChanged {
					d.image, d.imageError = nil, ""
					s.loadDocumentImage(d)
				}
			}
		}
		return
	}
	buffer := editor.NewBuffer(file.Content)
	buffer.SetLanguage(editor.LanguageFor(file.Path))
	d := &document{file: file, editor: &editor.State{Buffer: buffer, ReadOnly: file.ReadOnly, Label: file.Name, NoScrollPastEnd: true},
		preview: isMarkdown(file.Path), sourceSession: sessionID, absolutePath: absolute, linked: linked}
	for _, known := range s.snapshot.Files {
		if known.ID == file.ID {
			d.linked = false
			break
		}
	}
	s.documents[file.ID] = d
	if d.absolutePath == "" {
		source, _ := s.linkSession(sessionID)
		if root, err := s.projectRoot(source); err == nil {
			d.absolutePath, _, _, _ = linktarget.ResolveFile(file.Path, root)
		}
	}
	if file.Kind == "image" {
		s.loadDocumentImage(d)
	}
}

func imageResourceChanged(previous, next contracts.File) bool {
	return previous.Kind != next.Kind || previous.Version != next.Version || previous.AssetRef != next.AssetRef
}

func (s *State) loadDocumentImage(d *document) {
	read, finish := s.documentImageRequest(d)
	if s.win == nil {
		data, err := read(s.ctx)
		finish(data, err)
		return
	}
	ctx, win := s.ctx, s.win
	go func() {
		data, err := read(ctx)
		if ctx.Err() == nil {
			win.Update(func() { finish(data, err) })
		}
	}()
}

// documentImageRequest snapshots resource ownership on the UI thread. Its reader
// handles bytes only; completion must run on the UI thread.
func (s *State) documentImageRequest(d *document) (func(context.Context) ([]byte, error), func([]byte, error)) {
	d.imageGeneration++
	generation := d.imageGeneration
	ref, scope, id, version := d.file.AssetRef, s.scopeID, d.file.ID, d.file.Version
	service, hasResources := s.backend.(contracts.ResourceService)
	read := func(ctx context.Context) ([]byte, error) {
		if strings.HasPrefix(ref, "resource://") {
			if hasResources {
				return service.ReadResource(ctx, ref)
			}
			return nil, fmt.Errorf("image resource is unavailable")
		}
		// Only bundled fixtures use application assets.
		return assets.Read(ref)
	}
	finish := func(data []byte, err error) {
		current := s.documents[id]
		if scope != s.scopeID {
			current = s.scopes[scope].documents[id]
		}
		if s.closed.Load() || current != d || d.imageGeneration != generation || d.file.AssetRef != ref || d.file.Version != version {
			return
		}
		if err == nil {
			if strings.HasSuffix(strings.ToLower(ref), ".svg") {
				d.image, err = ui.ParseSVG(data)
			} else {
				d.image, err = ui.DecodeBitmap(data)
			}
		}
		if err != nil {
			d.imageError = err.Error()
		}
	}
	return read, finish
}

func (s *State) openPreviewLink(d *document, raw string, openExternal func(string)) error {
	if _, err := s.linkSession(d.sourceSession); err != nil {
		return err
	}
	target, err := linktarget.ParsePreview(raw, d.absolutePath)
	if err != nil {
		return err
	}
	switch target.Kind {
	case linktarget.Mail:
		openExternal(target.URL)
		return nil
	case linktarget.Anchor:
		d.markdown.Anchor = target.Anchor
		d.markdown.AnchorVersion++
		return nil
	default:
		return s.openLinkTarget(d.sourceSession, target)
	}
}

// OpenPreviewLink routes links relative to a document on the source host.
// External actions are supplied by the desktop's shared link configuration.
func (s *State) OpenPreviewLink(sessionID, documentPath, raw string) error {
	if _, err := s.linkSession(sessionID); err != nil {
		return err
	}
	target, err := linktarget.ParsePreview(raw, documentPath)
	if err != nil {
		return err
	}
	if target.Kind == linktarget.Anchor {
		for _, d := range s.documents {
			if d.sourceSession == sessionID && sameHostPath(d.absolutePath, documentPath) {
				d.markdown.Anchor = target.Anchor
				d.markdown.AnchorVersion++
				return nil
			}
		}
		return fmt.Errorf("preview document is unavailable")
	}
	if target.Kind == linktarget.Mail {
		if s.OnLinkConfig != nil {
			config := s.OnLinkConfig(sessionID, documentPath)
			if config.OpenExternal != nil {
				config.OpenExternal(raw)
				return nil
			}
		}
		return fmt.Errorf("mail links are unavailable")
	}
	return s.openLinkTarget(sessionID, target)
}
