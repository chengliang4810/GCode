package workbench

import (
	"fmt"
	"gcode/internal/design"
	"net/url"
	"strings"

	"gcode/internal/assets"
	"gcode/internal/contracts"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type browser struct {
	data       contracts.BrowserTab
	address    string
	view       *mygo.WebView
	off        []func()
	zoom       float64
	annotation *annotationSession
	scripts    *scriptQueue
	body       ui.Handle
	focusBody  bool
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
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return "", fmt.Errorf("enter an HTTP or HTTPS website address")
	}
	return u.String(), nil
}

func (s *State) OpenBrowser(address string) { s.openBrowser(address) }

func (s *State) openBrowser(address string) {
	if s.closed.Load() {
		return
	}
	normalized, err := NormalizeURL(address)
	if err != nil {
		s.Error = err.Error()
		return
	}
	s.nextBrowser++
	id := fmt.Sprintf("browser-%d", s.nextBrowser)
	b := &browser{data: contracts.BrowserTab{ID: id, URL: normalized, Title: "Browser"}, address: normalized, zoom: 1, focusBody: true}
	s.browsers[id] = b
	s.choose("browser", id, "Browser")
	s.createBrowserView(b)
}

func (s *State) createBrowserView(b *browser) {
	if s.win == nil {
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
				b.data.URL = address
				b.address = address
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
			if _, err := NormalizeURL(e.URL); err != nil {
				e.PreventDefault()
				return
			}
			update(func() { b.stopAnnotation(); b.data.Loading = true; b.data.Error = "" })
		}),
	)
	if err := page.LoadURL(b.data.URL); err != nil {
		b.data.Loading = false
		b.data.Error = err.Error()
	}
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
	services := c.Services()
	ui.Row(c).Height(38).Padding(0, 8).Gap(4).AlignItems(ui.Center).BorderWidth(1, 0, 1, 0).BorderColor(p.Border).Background(p.Shell).Children(func() {
		ui.ButtonBase(c).Label("Back").Disabled(!b.data.CanGoBack).Size(24, 24).Tooltip("Back").OnClick(func() {
			if b.view != nil {
				b.stopAnnotation()
				b.focusBody = true
				b.view.Page().GoBack()
			}
		}).Children(func() { ui.Icon(c, assets.Icon("arrow-left")).Size(14, 14).TextColor(p.Muted) })
		ui.ButtonBase(c).Label("Forward").Disabled(!b.data.CanGoForward).Size(24, 24).Tooltip("Forward").OnClick(func() {
			if b.view != nil {
				b.stopAnnotation()
				b.focusBody = true
				b.view.Page().GoForward()
			}
		}).Children(func() { ui.Icon(c, assets.Icon("arrow-right")).Size(14, 14).TextColor(p.Muted) })
		ui.ButtonBase(c).Label("Reload page").Size(24, 24).Radius(6).Tooltip("Reload page").OnClick(func() {
			if b.view != nil {
				b.stopAnnotation()
				b.data.Loading = true
				b.view.Page().Reload()
			} else {
				s.navigateBrowser(b)
			}
		}).Children(func() { ui.Icon(c, assets.Icon("refresh")).Size(14, 14).TextColor(p.Muted) })
		ui.Row(c).Grow(1).Height(24).Padding(0, 8).Gap(6).AlignItems(ui.Center).Radius(6).Background(sourceInk(p.Dark, .035)).Children(func() {
			ui.Icon(c, assets.Icon("globe")).Size(12, 12).TextColor(p.Faint)
			in := ui.TextInputBase(c, &b.address).Label("Website address").Placeholder("Enter URL").Grow(1).FontSize(design.UISize(c, 11.5)).OnSubmit(func() { s.navigateBrowser(b) })
			if in.Focused() || b.address != b.data.URL {
				ui.ButtonBase(c).Label("Go to address").Size(18, 18).Radius(4).OnClick(func() { s.navigateBrowser(b) }).Children(func() { ui.Icon(c, assets.Icon("return")).Size(12, 12).TextColor(p.Muted) })
			}
		})
		label := "Annotate an element"
		if b.annotation != nil {
			label = "Stop annotating"
		}
		button := ui.ButtonBase(c).Label(label).Tooltip(label).Size(24, 24).Radius(6).Disabled(b.view == nil || b.data.Loading || b.data.Error != "" || s.OnAnnotation == nil).OnClick(func() { s.toggleAnnotation(b) })
		color := p.Muted
		if b.annotation != nil {
			button.Background(sourceInk(p.Dark, .1))
			color = p.Accent
		}
		button.Children(func() { ui.Icon(c, assets.Icon("cursor-square")).Size(14, 14).TextColor(color) })
		ui.ButtonBase(c).Label("Open in default browser").Tooltip("Open in default browser").Disabled(b.data.URL == "").Size(24, 24).Radius(6).OnClick(func() { services.OpenURL(b.data.URL) }).Children(func() { ui.Icon(c, assets.Icon("arrow-up-right")).Size(14, 14).TextColor(p.Muted) })
	})
	if b.data.Error != "" {
		ui.Column(c).Grow(1).FillWidth().Center().Gap(12).Children(func() {
			ui.Text(c, "This page could not be loaded.").FontSize(design.UISize(c, 16))
			ui.Text(c, b.data.Error).TextColor(p.Muted).MaxWidth(400)
			ui.Button(c, "Try again").OnClick(func() { s.navigateBrowser(b) })
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
		ui.Text(c, "Open a website").FontSize(design.UISize(c, 16))
		ui.Text(c, b.data.URL).TextColor(p.Muted)
	})
}

func (s *State) navigateBrowser(b *browser) {
	if s.closed.Load() || s.browsers[b.data.ID] != b {
		return
	}
	b.stopAnnotation()
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
