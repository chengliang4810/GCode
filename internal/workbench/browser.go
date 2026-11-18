package workbench

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"gcode/internal/assets"
	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/linktarget"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type browser struct {
	data contracts.BrowserTab
	// file is the absolute path of a host file this surface renders itself
	// instead of loading a web address.
	file string
	// pageTitle is the document's own title. A preview keeps the file name as
	// its tab title but shows the document title beside the path.
	pageTitle    string
	address      string
	view         *mygo.WebView
	off          []func()
	zoom         float64
	annotation   *annotationSession
	scripts      *scriptQueue
	body         ui.Handle
	focusBody    bool
	addressInput ui.Handle
	focusAddress bool
}

// NormalizeURL accepts public HTTP(S) navigation only. App pages and local files
// are deliberately not reachable from the address bar of the external browser.
func NormalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("enter a website address")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	target, err := linktarget.Parse(raw)
	if err != nil || target.Kind != linktarget.Web {
		return "", fmt.Errorf("enter an HTTP or HTTPS website address")
	}
	return target.URL, nil
}

func (s *State) OpenBrowser(address string) { s.openBrowser(address) }

// OpenFilePreview renders a host file with the embedded browser. HTML keeps the
// document's own relative resources; PDF uses the platform webview's viewer.
func (s *State) OpenFilePreview(absolute, name string) error {
	if s.closed.Load() {
		return nil
	}
	if absolute == "" || !filepath.IsAbs(absolute) {
		return fmt.Errorf("file preview needs an absolute path")
	}
	if name == "" {
		name = filepath.Base(absolute)
	}
	for id, b := range s.browsers {
		if b.file != "" && sameHostPath(b.file, absolute) {
			s.choose("browser", id, b.data.Title)
			return nil
		}
	}
	s.nextBrowser++
	id := fmt.Sprintf("browser-%d", s.nextBrowser)
	b := &browser{
		data:      contracts.BrowserTab{ID: id, URL: absolute, Title: name},
		file:      absolute,
		address:   absolute,
		zoom:      1,
		focusBody: true,
	}
	s.browsers[id] = b
	s.choose("browser", id, name)
	s.createBrowserView(b)
	return nil
}

func (s *State) openBrowser(address string) {
	if s.closed.Load() {
		return
	}
	normalized := ""
	if strings.TrimSpace(address) != "" {
		var err error
		normalized, err = NormalizeURL(address)
		if err != nil {
			s.Error = err.Error()
			return
		}
	}
	s.nextBrowser++
	id := fmt.Sprintf("browser-%d", s.nextBrowser)
	b := &browser{data: contracts.BrowserTab{ID: id, URL: normalized, Title: "Browser"}, address: normalized, zoom: 1, focusBody: normalized != "", focusAddress: normalized == ""}
	s.browsers[id] = b
	s.choose("browser", id, "Browser")
	s.createBrowserView(b)
}

func (s *State) createBrowserView(b *browser) {
	if s.win == nil || b.data.URL == "" && b.file == "" {
		return
	}
	b.data.Loading = true
	// Subscribe before loading so even an immediate cached load is observed.
	view, err := s.win.NewWebView(mygo.WebViewOptions{})
	if err != nil {
		b.data.Loading = false
		b.data.Error = err.Error()
		return
	}
	b.view = view
	page := view.Page()
	b.scripts = newScriptQueue(s.ctx, page)
	update := func(fn func()) {
		s.win.Update(func() {
			if !s.closed.Load() && s.browsers[b.data.ID] == b && b.view == view {
				fn()
			}
		})
	}
	b.off = append(b.off,
		page.OnPageTitleUpdated(func(e *mygo.TitleEvent) {
			e.PreventDefault()
			title := e.Title
			update(func() {
				if b.file != "" {
					// A preview keeps the file name so the tab stays findable,
					// but the document's own title still describes the page.
					b.pageTitle = title
					return
				}
				b.data.Title = title
				for i := range s.tabs {
					if s.tabs[i].id == b.data.ID {
						s.tabs[i].title = title
					}
				}
			})
		}),
		page.OnDidNavigate(func(address string) {
			update(func() {
				b.stopAnnotation()
				if b.file == "" {
					b.data.URL = address
					b.address = address
				}
				b.data.Loading = page.IsLoading()
				b.data.CanGoBack = page.CanGoBack()
				b.data.CanGoForward = page.CanGoForward()
			})
		}),
		page.OnDidFinishLoad(func() {
			update(func() {
				b.data.Loading = false
				b.data.Error = ""
				b.data.CanGoBack = page.CanGoBack()
				b.data.CanGoForward = page.CanGoForward()
			})
		}),
		page.OnDidFailLoad(func(err *mygo.LoadError) {
			description := err.Description
			update(func() { b.stopAnnotation(); b.data.Loading = false; b.data.Error = description })
		}),
		page.OnWillNavigate(func(e *mygo.NavigateEvent) {
			if !b.allowsNavigation(e.URL) {
				e.PreventDefault()
				return
			}
			update(func() { b.stopAnnotation(); b.data.Loading = true; b.data.Error = "" })
		}),
	)
	if b.file != "" {
		if err := page.LoadFile(b.file); err != nil {
			b.data.Loading = false
			b.data.Error = err.Error()
		}
		return
	}
	if err := page.LoadURL(b.data.URL); err != nil {
		b.data.Loading = false
		b.data.Error = err.Error()
	}
}

// allowsNavigation accepts public HTTP(S) addresses. A file preview also follows
// the file URLs its own document loads, but the address bar stays HTTP-only.
func (b *browser) allowsNavigation(address string) bool {
	if _, err := NormalizeURL(address); err == nil {
		return true
	}
	if b.file == "" {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(address))
	return strings.HasPrefix(lower, "file:") || strings.HasPrefix(lower, "about:")
}

func (b *browser) close() {
	b.stopAnnotation()
	for _, off := range b.off {
		off()
	}
	b.off = nil
	if b.scripts != nil {
		b.scripts.close()
		b.scripts = nil
	}
	if b.view != nil {
		b.view.Destroy()
		b.view = nil
	}
}

func (s *State) browserView(c *ui.Context, id string) {
	b := s.browsers[id]
	if b == nil {
		return
	}
	p := s.palette()
	if b.annotation != nil {
		c.OnShortcut(0, ui.KeyEscape, b.stopAnnotation)
	}
	ui.Row(c).Height(38).Padding(0, 8).Gap(4).AlignItems(ui.Center).BorderWidth(1, 0, 1, 0).BorderColor(p.Border).Background(p.Shell).Children(func() {
		ui.ButtonBase(c.Key("browser-back:"+id)).Label(tr(c, "Back")).Disabled(!b.data.CanGoBack).Size(24, 24).Tooltip(tr(c, "Back")).OnClick(func() {
			if b.view != nil {
				b.stopAnnotation()
				b.focusBody = true
				b.view.Page().GoBack()
			}
		}).Children(func() { ui.Icon(c, assets.Icon("arrow-left")).Size(14, 14).TextColor(p.Muted) })
		ui.ButtonBase(c.Key("browser-forward:"+id)).Label(tr(c, "Forward")).Disabled(!b.data.CanGoForward).Size(24, 24).Tooltip(tr(c, "Forward")).OnClick(func() {
			if b.view != nil {
				b.stopAnnotation()
				b.focusBody = true
				b.view.Page().GoForward()
			}
		}).Children(func() { ui.Icon(c, assets.Icon("arrow-right")).Size(14, 14).TextColor(p.Muted) })
		ui.ButtonBase(c.Key("browser-reload:"+id)).Label(tr(c, "Reload page")).Disabled(b.data.URL == "" && b.file == "").Size(24, 24).Radius(6).Tooltip(tr(c, "Reload page")).OnClick(func() {
			if b.view != nil {
				b.stopAnnotation()
				b.data.Loading = true
				if b.file != "" {
					b.data.Error = ""
					if err := b.view.Page().LoadFile(b.file); err != nil {
						b.data.Loading = false
						b.data.Error = err.Error()
					}
					return
				}
				b.view.Page().Reload()
			} else {
				s.navigateBrowser(b)
			}
		}).Children(func() { ui.Icon(c, assets.Icon("refresh")).Size(14, 14).TextColor(p.Muted) })
		ui.Row(c).Grow(1).Height(24).Padding(0, 8).Gap(6).AlignItems(ui.Center).Radius(6).Background(sourceInk(p.Dark, .035)).Children(func() {
			if b.file != "" {
				// A preview shows the host path the file was opened from; it is
				// not an editable address.
				ui.Icon(c, assets.Icon(previewIcon(b.file))).Size(12, 12).TextColor(p.Faint)
				label := b.file
				if b.pageTitle != "" && b.pageTitle != filepath.Base(b.file) {
					label = b.file + " — " + b.pageTitle
				}
				ui.Text(c, label).FontSize(design.TextSize(c, design.TextBase)).TextColor(p.Muted).SingleLine().TruncateMiddle().Shrink(1).MinWidth(0).Tooltip(label)
				return
			}
			ui.Icon(c, assets.Icon("globe")).Size(12, 12).TextColor(p.Faint)
			in := ui.TextInputBase(c.Key("browser-address:"+id), &b.address).Bind(&b.addressInput).Label(tr(c, "Website address")).Placeholder(tr(c, "Enter URL")).Grow(1).FontSize(design.TextSize(c, design.TextBase)).OnSubmit(func() { s.navigateBrowser(b) })
			if b.focusAddress {
				in.Focus()
				b.focusAddress = false
			}
			if in.Focused() || b.address != b.data.URL {
				ui.ButtonBase(c.Key("browser-go:"+id)).Label(tr(c, "Go to address")).Size(18, 18).Radius(4).OnClick(func() { s.navigateBrowser(b) }).Children(func() { ui.Icon(c, assets.Icon("return")).Size(12, 12).TextColor(p.Muted) })
			}
		})
		if b.file == "" {
			services := c.Services()
			label := "Annotate an element"
			if b.annotation != nil {
				label = "Stop annotating"
			}
			button := ui.ButtonBase(c.Key("browser-annotate:"+id)).Label(tr(c, label)).Tooltip(tr(c, label)).Size(24, 24).Radius(6).Disabled(b.view == nil || b.data.Loading || b.data.Error != "" || s.OnAnnotation == nil).OnClick(func() { s.toggleAnnotation(b) })
			color := p.Muted
			if b.annotation != nil {
				button.Background(sourceInk(p.Dark, .1))
				color = p.Accent
			}
			button.Children(func() { ui.Icon(c, assets.Icon("cursor-square")).Size(14, 14).TextColor(color) })
			ui.ButtonBase(c.Key("browser-external:"+id)).Label(tr(c, "Open in default browser")).Tooltip(tr(c, "Open in default browser")).Disabled(b.data.URL == "").Size(24, 24).Radius(6).OnClick(func() { services.OpenURL(b.data.URL) }).Children(func() { ui.Icon(c, assets.Icon("arrow-up-right")).Size(14, 14).TextColor(p.Muted) })
		}
	})
	if b.data.Error != "" {
		ui.Column(c).Grow(1).FillWidth().Center().Gap(12).Children(func() {
			ui.Text(c, tr(c, "This page could not be loaded.")).FontSize(design.TextSize(c, design.TextLarge))
			ui.Text(c, b.data.Error).TextColor(p.Muted).MaxWidth(400)
			ui.Button(c, tr(c, "Try again")).OnClick(func() { s.navigateBrowser(b) })
		})
		return
	}
	if b.view != nil {
		page := b.view.Page()
		body := ui.WebView(c, b.view).Bind(&b.body).Grow(1).FillWidth().OnShortcut(ui.Cmd, ui.KeyEqual, func() { b.zoom = min(b.zoom+.1, 3); page.SetZoomFactor(b.zoom) }).OnShortcut(ui.Cmd, ui.KeyMinus, func() { b.zoom = max(b.zoom-.1, .25); page.SetZoomFactor(b.zoom) })
		if b.focusBody {
			body.Focus()
			b.focusBody = false
		}
		return
	}
	ui.Column(c).Grow(1).FillWidth().Center().Gap(12).Children(func() {
		ui.Text(c, tr(c, "Open a website")).FontSize(design.TextSize(c, design.TextLarge))
		ui.Text(c, b.data.URL).TextColor(p.Muted)
	})
}

func (s *State) navigateBrowser(b *browser) {
	if s.closed.Load() || s.browsers[b.data.ID] != b {
		return
	}
	b.stopAnnotation()
	if b.file != "" {
		b.focusBody = true
		b.data.Error = ""
		if b.view == nil {
			s.createBrowserView(b)
			return
		}
		b.data.Loading = true
		if err := b.view.Page().LoadFile(b.file); err != nil {
			b.data.Loading = false
			b.data.Error = err.Error()
		}
		return
	}
	address, err := NormalizeURL(b.address)
	if err != nil {
		b.data.Error = err.Error()
		return
	}
	b.address = address
	b.data.URL = address
	b.data.Error = ""
	b.focusBody = true
	if b.view == nil {
		s.createBrowserView(b)
		return
	}
	if b.view != nil {
		b.data.Loading = true
		if err := b.view.Page().LoadURL(address); err != nil {
			b.data.Error = err.Error()
			b.data.Loading = false
		}
	}
}

// previewIcon selects the tab's identity inside the preview header.
func previewIcon(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".pdf":
		return "document"
	case ".html", ".htm", ".xhtml":
		return "file-code"
	}
	return "document"
}
