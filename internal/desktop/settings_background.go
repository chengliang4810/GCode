package desktop

import (
	"bytes"
	"fmt"
	"image"
	"math"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type backgroundSettingsState struct {
	Key, Error, LoadError, DialogPath string
	Loading, Open, Dragging           bool
	Bitmap                            *ui.Bitmap
	Thumbnail                         *ui.Bitmap
	Pixels                            *image.NRGBA
	Draft, Initial                    design.BackgroundAdjustment
	StartX, StartY, Width, Height     float32
	ZoomValue                         float64
	Preview                           ui.Handle
	ReturnFocus                       ui.Handle
}

func adjustmentFromContract(a contracts.BackgroundAdjustment) design.BackgroundAdjustment {
	return design.NormalizeBackgroundAdjustment(design.BackgroundAdjustment{FocalX: a.FocalX, FocalY: a.FocalY, Zoom: a.Zoom})
}

// refreshBackground is a controller action. Call it after platform/settings
// changes; the view only reads the prepared image and never starts work.
func (a *App) refreshBackground() { a.prepareBackground(&a.background) }

// backgroundTask publishes every result on the window thread. Tests without a
// native window use the same operation synchronously and never open dialogs.
func (a *App) backgroundTask(work func() func()) {
	if a.win == nil {
		work()()
		return
	}
	window := a.win
	go func() {
		commit := work()
		window.Update(func() {
			if !a.closed {
				commit()
			}
		})
	}()
}

func (a *App) backgroundCommand(state *backgroundSettingsState, command contracts.Command, after func()) {
	command = command.Clone()
	state.Error = ""
	a.backgroundTask(func() func() {
		err := a.service.Execute(a.ctx, command)
		snapshot, snapshotErr := a.service.Snapshot(a.ctx)
		return func() {
			if err != nil {
				state.Error = err.Error()
				return
			}
			if snapshotErr != nil {
				state.Error = snapshotErr.Error()
				return
			}
			a.apply(snapshot)
			if after != nil {
				after()
			}
		}
	})
}

func (a *App) chooseBackground(state *backgroundSettingsState) {
	if a.win == nil {
		return
	}
	state.Error = ""
	window := a.win
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: window, Title: "Choose New Thread Composer Background"})
		window.Update(func() {
			if a.closed {
				return
			}
			if err != nil {
				state.Error = err.Error()
				return
			}
			if len(paths) > 0 {
				a.backgroundCommand(state, contracts.Command{Kind: "background.install", Path: paths[0]}, nil)
			}
		})
	}()
}

func (a *App) prepareBackground(state *backgroundSettingsState) {
	settings := a.state.Settings.Appearance
	dark := strings.EqualFold(settings.Mode, "dark") || strings.EqualFold(settings.Mode, "system") && a.systemDark
	key := fmt.Sprintf("%s|%s|%t", settings.BackgroundAssetRef, settings.BackgroundEffect, dark)
	if key == state.Key {
		return
	}
	state.Key = key
	state.Bitmap = nil
	state.Thumbnail = nil
	state.Pixels = nil
	state.Loading = false
	state.LoadError = ""
	if settings.BackgroundAssetRef == "" {
		return
	}
	reader, ok := a.service.(contracts.ResourceService)
	if !ok {
		state.LoadError = "Image unavailable. Choose a replacement or remove it."
		return
	}
	state.Loading = true
	light := !dark
	a.backgroundTask(func() func() {
		data, err := reader.ReadResource(a.ctx, settings.BackgroundAssetRef)
		var bitmap *ui.Bitmap
		var thumbnail *ui.Bitmap
		var pixels *image.NRGBA
		if err == nil {
			var source image.Image
			source, _, err = image.Decode(bytes.NewReader(data))
			if err == nil {
				proxy := design.BackgroundThumbnail(source, 2048)
				thumbnail = ui.NewBitmap(proxy)
				pixels = design.BackgroundEffectPixels(proxy, settings.BackgroundEffect, light)
				bitmap = ui.NewBitmap(pixels)
			}
		}
		return func() {
			if state.Key != key {
				return
			}
			state.Loading = false
			if err != nil {
				state.LoadError = "Image unavailable. Choose a replacement or remove it."
				return
			}
			state.LoadError = ""
			state.Bitmap = bitmap
			state.Thumbnail = thumbnail
			state.Pixels = pixels
		}
	})
}

func (a *App) backgroundSettings(c *ui.Context) {
	owner := ui.Column(c.Key("background-settings")).FillWidth()
	state := &a.background
	p := &a.state.Settings.Appearance
	owner.Children(func() {
		meta := "No image selected"
		if p.Background != "" {
			meta = p.BackgroundName
			if state.LoadError != "" && !state.Loading {
				meta = "Image unavailable · Choose a replacement or remove it."
			}
		}
		a.settingsRow(c, "New thread background", meta, func() {
			if p.Background != "" {
				if state.Bitmap != nil {
					textButton(c, "Adjust").Bind(&state.ReturnFocus).Label("Adjust background").OnClick(func() {
						state.DialogPath = p.Background
						state.Draft = adjustmentFromContract(p.BackgroundAdjustment)
						state.ZoomValue = float64(state.Draft.Zoom)
						state.Open = true
						state.Dragging = false
					})
				}
				textButton(c, "Replace image").OnClick(func() { a.chooseBackground(state) })
				textButton(c, "Remove").Label("Remove background").OnClick(func() { a.backgroundCommand(state, contracts.Command{Kind: "background.remove"}, nil) })
			} else {
				textButton(c, "Choose image").OnClick(func() { a.chooseBackground(state) })
			}
		}, func() {
			if state.Thumbnail != nil {
				ui.Image(c, state.Thumbnail).Size(36, 36).Fit(ui.Cover).Radius(10).Clip().Border(1, a.palette.Border)
			}
		})
		if state.Error != "" {
			ui.Text(c, state.Error).FontSize(design.UISize(c, 12)).TextColor(a.palette.Danger).Margin(0, 16, 12, 16)
		}
		if p.Background != "" {
			line(c)
			a.settingsRow(c, "Background effect", backgroundEffectDescription(p.BackgroundEffect), func() {
				a.settingsChoice(c, "Background effect", &p.BackgroundEffect, []settingOption{{"none", "None"}, {"dither", "Dither"}, {"ascii", "ASCII"}, {"halftone", "Halftone"}, {"scanlines", "Scanlines"}}, nil)
			})
		}
	})
	a.backgroundAdjustmentDialog(c, state)
}

func backgroundEffectDescription(effect string) string {
	switch effect {
	case "dither":
		return "Rebuilds the artwork with a dithered color palette."
	case "ascii":
		return "Recreates the artwork with colored characters on black."
	case "halftone":
		return "Recreates the artwork with colored print dots on black."
	case "scanlines":
		return "Adds a pronounced horizontal display-line texture."
	default:
		return "Shows the original artwork."
	}
}

func drawAdjustedBackground(p *ui.Painter, bitmap *ui.Bitmap, bounds ui.Rect, adjustment design.BackgroundAdjustment, radius float32) {
	if bitmap == nil {
		return
	}
	w, h := bitmap.Size()
	fitted, ok := design.BackgroundFit(float32(w), float32(h), bounds, adjustment)
	if !ok {
		return
	}
	source := ui.Rect{X: (bounds.X - fitted.X) / fitted.W * float32(w), Y: (bounds.Y - fitted.Y) / fitted.H * float32(h), W: bounds.W / fitted.W * float32(w), H: bounds.H / fitted.H * float32(h)}
	p.Clip(bounds, radius, func() { p.ImageRegion(bitmap, source, bounds, ui.ImageOptions{}) })
}

func (a *App) backgroundAdjustmentDialog(c *ui.Context, state *backgroundSettingsState) {
	ui.DialogBase(c.Key("background-adjustment"), &state.Open, func(back, panel ui.Element) {
		vw, vh := c.Size()
		p := a.state.Settings.Appearance
		back.Background(ui.Hex("#000000").Alpha(.55))
		panel.Width(min(max(vw-40, 1), 600)).Padding(20).Radius(12).Background(a.palette.Dialog).Border(1, a.palette.Border)
		ready := state.Bitmap != nil && !state.Loading && state.DialogPath == p.Background
		closeDialog := func() { state.Open = false; state.Dragging = false; state.ReturnFocus.Focus(c) }
		apply := func() {
			if !ready {
				return
			}
			draft := contracts.BackgroundAdjustment{FocalX: state.Draft.FocalX, FocalY: state.Draft.FocalY, Zoom: state.Draft.Zoom}
			a.backgroundCommand(state, contracts.Command{Kind: "background.adjust", Path: state.DialogPath, Adjustment: &draft}, closeDialog)
		}
		zoom := func(value, x, y float32) {
			if !ready {
				return
			}
			w, h := state.Bitmap.Size()
			state.Draft = design.ZoomBackground(float32(w), float32(h), ui.Rect{W: state.Width, H: state.Height}, state.Draft, value, x, y)
			state.ZoomValue = float64(state.Draft.Zoom)
			state.Dragging = false
		}
		c.OnShortcut(ui.Cmd, ui.KeyEnter, apply)
		ui.Row(c).FillWidth().Gap(16).AlignItems(ui.Start).Children(func() {
			ui.Column(c).Grow(1).Gap(4).Children(func() {
				ui.Text(c, "Adjust background").FontSize(design.UISize(c, 18)).FontWeight(600)
				ui.Text(c, "Drag to reposition. Scroll or pinch to zoom.").FontSize(design.UISize(c, 13)).TextColor(a.palette.Muted)
			})
			iconButton(c, "close", "Close background adjustment").Border(1, a.palette.Border).Radius(7).OnClick(closeDialog)
		})
		pw, ph := design.BackgroundPreviewSize(vw, vh, float32(a.state.Settings.SidebarWidth), !a.sidebar)
		state.Width, state.Height = pw, ph
		ui.Row(c).FillWidth().Height(260).Margin(16, 0, 0, 0).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			preview := ui.Box(c.Key("background-preview")).Size(pw+2, ph+2).Radius(12).Border(1, a.palette.Border).Background(a.palette.Background).Clip().Bind(&state.Preview).Focusable().AutoFocus().Label("Background crop preview")
			preview.Draw(func(painter *ui.Painter, r ui.Rect) {
				inner := ui.Rect{X: r.X + 1, Y: r.Y + 1, W: r.W - 2, H: r.H - 2}
				if ready {
					drawAdjustedBackground(painter, state.Bitmap, inner, state.Draft, 11)
				}
				if state.Dragging {
					for _, fraction := range []float32{1.0 / 3, 2.0 / 3} {
						painter.Fill(ui.Rect{X: inner.X + inner.W*fraction, Y: inner.Y, W: 1, H: inner.H}, ui.Hex("#ffffff").Alpha(.22), 0)
						painter.Fill(ui.Rect{X: inner.X, Y: inner.Y + inner.H*fraction, W: inner.W, H: 1}, ui.Hex("#ffffff").Alpha(.22), 0)
					}
				}
			})
			preview.HandleInput(func(event ui.InputEvent) bool {
				if !ready {
					return false
				}
				w, h := state.Bitmap.Size()
				bounds := ui.Rect{W: pw, H: ph}
				switch event.Kind {
				case ui.InputPointerDown:
					if event.Button != 0 {
						return false
					}
					state.Initial = state.Draft
					state.StartX, state.StartY = event.X, event.Y
					state.Dragging = true
					state.Preview.Focus(c)
					return true
				case ui.InputPointerMove:
					if state.Dragging {
						state.Draft = design.PanBackground(float32(w), float32(h), bounds, state.Initial, event.X-state.StartX, event.Y-state.StartY)
						return true
					}
				case ui.InputPointerUp:
					state.Dragging = false
					return true
				case ui.InputScroll:
					delta := min(max(-event.DY*.0025, -2), 2)
					zoom(state.Draft.Zoom*float32(math.Exp(float64(delta))), event.X, event.Y)
					return true
				case ui.InputZoom:
					zoom(state.Draft.Zoom*event.Scale, event.X, event.Y)
					return true
				case ui.InputKeyDown:
					step := float32(8)
					if event.Mods&ui.Shift != 0 {
						step = 24
					}
					dx, dy := float32(0), float32(0)
					switch event.Key {
					case ui.KeyLeft:
						dx = step
					case ui.KeyRight:
						dx = -step
					case ui.KeyUp:
						dy = step
					case ui.KeyDown:
						dy = -step
					case ui.KeyEqual:
						zoom(state.Draft.Zoom+.1, pw/2, ph/2)
						return true
					case ui.KeyMinus:
						zoom(state.Draft.Zoom-.1, pw/2, ph/2)
						return true
					default:
						return false
					}
					state.Draft = design.PanBackground(float32(w), float32(h), bounds, state.Draft, dx, dy)
					return true
				}
				return false
			})
			if !ready {
				preview.Children(func() {
					message := "Preparing preview…"
					if state.DialogPath != p.Background {
						message = "Background changed. Close and adjust the new image."
					} else if state.LoadError != "" {
						message = "Image unavailable. Choose a replacement or remove it."
					}
					ui.Text(c, message).Absolute().Left(12).Right(12).Top(ph/2 - 8).FontSize(design.UISize(c, 11.5)).TextColor(a.palette.Muted)
				})
			}
		})
		ui.Row(c).FillWidth().Margin(14, 0, 0, 0).AlignItems(ui.Center).Gap(12).Children(func() {
			textButton(c, "Reset").Label("Reset background adjustment").OnClick(func() {
				state.Draft = design.DefaultBackgroundAdjustment()
				state.ZoomValue = 1
				state.Dragging = false
			})
			ui.Spacer(c)
			textButton(c, "−").Label("Background zoom out").Disabled(!ready).OnClick(func() { zoom(state.Draft.Zoom-.1, pw/2, ph/2) })
			ui.StepSlider(c.Key("background-zoom"), &state.ZoomValue, 1, 4, .1).Width(168).Label("Background zoom").Disabled(!ready).OnChange(func() { zoom(float32(state.ZoomValue), pw/2, ph/2) })
			ui.Text(c, fmt.Sprintf("%.0f%%", state.Draft.Zoom*100)).Width(52).FontSize(design.UISize(c, 11.5)).TextColor(a.palette.Muted)
			textButton(c, "+").Label("Background zoom in").Disabled(!ready).OnClick(func() { zoom(state.Draft.Zoom+.1, pw/2, ph/2) })
		})
		ui.Box(c).FillWidth().Height(1).Background(a.palette.Border).Margin(18, 0, 12, 0)
		ui.Row(c).FillWidth().Justify(ui.End).Gap(8).Children(func() {
			textButton(c, "Cancel").Label("Cancel background adjustment").Height(34).Padding(0, 13).OnClick(closeDialog)
			ui.PrimaryButton(c, "Apply").Label("Apply background adjustment").Height(34).Padding(0, 14).Disabled(!ready).OnClick(apply)
		})
	})
}
