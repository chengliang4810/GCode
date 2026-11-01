package desktop

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/linktarget"
	"gcode/internal/linkui"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/markdown/parser"
)

func (a *App) configureWorkspaceLinks() {
	a.workspace.OnLinkConfig = func(sessionID, documentPath string) linkui.Config {
		return a.linkConfig(sessionID, documentPath, a.services)
	}
}

// transcriptScope begins the selection scope shared by every row of a
// transcript. Rows are separate list items, so one message cannot carry a
// scope of its own while its blocks are rows: the list holds one scope, and a
// selection crosses its blocks the way the reference's surface does.
func (a *App) transcriptScope(c *ui.Context, pane string, build func()) {
	root := ui.Column(c).Key("transcript-scope:"+pane).FillWidth().Grow(1).Selectable()
	selection := linkui.BeginSelection(root)
	if a.transcriptSelections == nil {
		a.transcriptSelections = map[string]*linkui.Selection{}
	}
	a.transcriptSelections[pane] = selection
	previous := a.linkSelection
	a.linkSelection = selection
	defer func() { a.linkSelection = previous }()
	root.Children(build)
	selection.Menu(c, root)
}

// Keep delayed actions bound to the paragraph's source, rather than the last
// conversation built in another pane. Options own image caches and are not copied.
func (a *App) transcriptOptions(sessionID string, user bool) *markdown.Options {
	if sessionID == "" {
		sessionID = a.sessionID
	}
	if a.linkOptions == nil {
		a.linkOptions = map[string]*markdown.Options{}
	}
	key := fmt.Sprintf("%s:%t", sessionID, user)
	options := a.linkOptions[key]
	if options == nil {
		options = &markdown.Options{}
		a.linkOptions[key] = options
	}
	base := &a.md
	if user {
		base = &a.userMD
	}
	options.Theme, options.Extensions, options.ShareSelection = base.Theme, base.Extensions, base.ShareSelection
	options.Math, options.Code, options.CodeBlock = base.Math, base.Code, base.CodeBlock
	options.HTML, options.LoadImage, options.WrapCode, options.Reveal = base.HTML, base.LoadImage, base.WrapCode, base.Reveal
	options.OnLink = func(raw string) { a.openTranscriptLink(sessionID, raw) }
	options.Block = func(c *ui.Context, document *parser.Document, index int) bool {
		previous := a.linkSource
		a.linkSource = sessionID
		defer func() { a.linkSource = previous }()
		if user {
			return a.userMarkdownBlock(c, document, index)
		}
		return a.transcriptMarkdownBlock(c, document, index)
	}
	return options
}

func (a *App) openTranscriptLink(sessionID, raw string) {
	config := a.linkConfig(sessionID, "", a.services)
	if destination, ok := config.Resolve(raw); ok {
		if destination.Kind == linktarget.Web && !config.Internal {
			config.OpenExternal(raw)
			return
		}
		config.Open(raw)
	}
}

func (a *App) linkConfig(sessionID, documentPath string, services ui.Services) linkui.Config {
	parse := func(raw string) (linktarget.Target, error) {
		if documentPath != "" {
			return linktarget.ParsePreview(raw, documentPath)
		}
		return linktarget.Parse(raw)
	}
	resolve := func(raw string) (linkui.Destination, bool) {
		target, err := parse(raw)
		if err != nil {
			return linkui.Destination{}, false
		}
		var source contracts.Session
		for _, session := range a.state.Sessions {
			if session.ID == sessionID && !session.Archived {
				source = session
				break
			}
		}
		if source.ID == "" || !a.workspace.LinkSourceAvailable(sessionID) || target.Kind == linktarget.Anchor {
			return linkui.Destination{}, false
		}
		destination := linkui.Destination{Raw: raw, Tooltip: target.URL, Kind: target.Kind}
		if target.Kind != linktarget.File {
			return destination, true
		}
		root, err := a.workspace.LinkRoot(sessionID)
		if err != nil {
			return linkui.Destination{}, false
		}
		absolute, _, _, err := linktarget.ResolveFile(target.Path, root)
		if err != nil {
			return linkui.Destination{}, false
		}
		destination.Tooltip, destination.Label = absolute, path.Base(absolute)
		if target.Line > 0 {
			destination.Tooltip += fmt.Sprintf(":%d", target.Line)
			if target.Column > 0 {
				destination.Tooltip += fmt.Sprintf(":%d", target.Column)
			}
		} else if target.Anchor != "" {
			destination.Tooltip += "#" + target.Anchor
		}
		destination.Local = a.state.Runtime == nil || !a.state.Runtime.Remote
		if source.DeviceID != "" {
			destination.Local = false
			for _, device := range a.state.Devices {
				if device.ID == source.DeviceID && device.Current {
					destination.Local = a.state.Runtime == nil || !a.state.Runtime.Remote
				}
			}
		}
		for _, project := range a.state.Projects {
			if project.ID != source.ProjectID {
				continue
			}
			var matches []string
			for _, file := range a.state.Files {
				if file.ProjectID != project.ID || file.Kind == "directory" {
					continue
				}
				candidate, _, _, candidateErr := linktarget.ResolveFile(file.Path, project.Path)
				if candidateErr != nil {
					continue
				}
				if sameLinkPath(candidate, absolute) {
					destination.KnownFile = true
				}
				if strings.EqualFold(path.Base(candidate), path.Base(absolute)) {
					matches = append(matches, candidate)
				}
			}
			destination.Label = disambiguatedLinkLabel(absolute, matches)
		}
		return destination, true
	}
	config := linkui.Config{
		Resolve: resolve, Internal: !a.state.Settings.General.OpenLinksExternally, Selection: a.linkSelection,
		Open: func(raw string) {
			var err error
			if documentPath == "" {
				err = a.workspace.OpenLink(sessionID, raw)
			} else {
				err = a.workspace.OpenPreviewLink(sessionID, documentPath, raw)
			}
			if err != nil {
				a.errorText = err.Error()
			}
		},
		SetInternal: func(internal bool) {
			a.state.Settings.General.OpenLinksExternally = !internal
			a.saveSettings()
		},
	}
	absolutePath := func(raw string) (string, bool) {
		destination, ok := resolve(raw)
		if !ok || !destination.Local || destination.Kind != linktarget.File || a.win == nil {
			return "", false
		}
		target, err := parse(raw)
		if err != nil {
			return "", false
		}
		root, err := a.workspace.LinkRoot(sessionID)
		if err != nil {
			return "", false
		}
		absolute, _, _, err := linktarget.ResolveFile(target.Path, root)
		return absolute, err == nil
	}
	config.OpenExternal = func(raw string) {
		target, err := parse(raw)
		if _, ok := resolve(raw); !ok || err != nil {
			return
		}
		if target.Kind == linktarget.Web || target.Kind == linktarget.Mail {
			services.OpenURL(target.URL)
		} else if absolute, ok := absolutePath(raw); ok {
			a.backgroundTask(func() func() {
				err := mygo.Shell.OpenPath(filepath.FromSlash(absolute))
				return func() {
					if err != nil {
						a.errorText = err.Error()
					}
				}
			})
		}
	}
	config.Reveal = func(raw string) {
		if absolute, ok := absolutePath(raw); ok {
			a.backgroundTask(func() func() {
				mygo.Shell.ShowItemInFolder(filepath.FromSlash(absolute))
				return func() {}
			})
		}
	}
	return config
}

func sameLinkPath(a, b string) bool {
	if len(a) > 1 && a[1] == ':' {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func disambiguatedLinkLabel(absolute string, matches []string) string {
	parts := strings.Split(absolute, "/")
	for n := 1; n <= len(parts); n++ {
		label := strings.Join(parts[len(parts)-n:], "/")
		unique := true
		for _, other := range matches {
			otherParts := strings.Split(other, "/")
			if len(otherParts) < n || sameLinkPath(other, absolute) {
				continue
			}
			suffix := strings.Join(otherParts[len(otherParts)-n:], "/")
			same := suffix == label
			if len(absolute) > 1 && absolute[1] == ':' {
				same = strings.EqualFold(suffix, label)
			}
			if same {
				unique = false
				break
			}
		}
		if unique {
			return label
		}
	}
	return absolute
}

func (a *App) linkMarkdownBlock(c *ui.Context, document *parser.Document, index int, theme *markdown.Theme) bool {
	config := a.linkConfig(a.linkSource, "", c.Services())
	return linkui.Block(c, document, index, config, theme)
}
