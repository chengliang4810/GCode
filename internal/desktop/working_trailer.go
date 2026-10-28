package desktop

import (
	"hash/fnv"
	"math"
	"strings"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

type primaryPendingSend struct {
	RequestID string
	StartedAt time.Time
	Queued    bool
}

// These are the source's decorative activity captions, not reasoning content.
var activityWords = [...]string{"Zeroning", "Thinking", "Pondering", "Scheming", "Brewing", "Weaving", "Tinkering", "Musing", "Composing", "Sifting", "Untangling", "Distilling", "Sketching", "Plotting", "Riffing", "Combobulating", "Percolating", "Marinating", "Noodling", "Puzzling", "Conjuring"}

func activityWord(sessionID string, seconds int64) string {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(sessionID))
	return activityWords[(hash.Sum64()+uint64(max(seconds, 0)/7))%uint64(len(activityWords))]
}

func (a *App) workingState(sessionID string) (word, elapsed string, visible bool) {
	if pending, ok := a.pendingSends[sessionID]; ok {
		if pending.Queued {
			return "Queued — will send automatically", "", true
		}
		return "Sending…", "", true
	}
	var session contracts.Session
	for _, candidate := range a.state.Sessions {
		if candidate.ID == sessionID {
			session = candidate
			break
		}
	}
	if session.Status != contracts.Working {
		return "", "", false
	}
	var run contracts.Run
	for _, candidate := range a.state.Runs {
		if candidate.SessionID == sessionID && !candidate.StartedAt.Before(run.StartedAt) {
			run = candidate
		}
	}
	var milliseconds int64
	if !run.StartedAt.IsZero() {
		milliseconds = max(a.now().UTC().Sub(run.StartedAt).Milliseconds(), run.DurationMS, 0)
	}
	return activityWord(sessionID, milliseconds/1000) + "…", strings.TrimPrefix(workedFor(milliseconds), "Worked for "), true
}

func (a *App) workingTrailer(c *ui.Context, sessionID string) {
	word, elapsed, visible := a.workingState(sessionID)
	if !visible {
		return
	}
	ui.Row(c).Label("Activity status: "+sessionID).FillWidth().Padding(16, 0, 0, 0).Gap(8).AlignItems(ui.Center).Children(func() {
		a.activityGlyph(c)
		color := a.palette.Muted
		if word == "Queued — will send automatically" {
			color = a.palette.Warning
		}
		ui.Text(c, word).FontSize(design.UISize(c, 12)).TextColor(color)
		if elapsed != "" {
			ui.Text(c, elapsed).FontSize(design.UISize(c, 11)).TextColor(a.palette.Faint).Margin(1, 0, 0, 0)
		}
	})
	// Elapsed time is meaningful state even when animation is disabled.
	c.After(time.Second)
}

// Native boxes preserve the original 3x3 activity glyph and its source pulse;
// this is display-only geometry, not a replacement input or focus widget.
func (a *App) activityGlyph(c *ui.Context) {
	phase := float64(c.Now().UnixNano()%int64(750*time.Millisecond)) / float64(750*time.Millisecond)
	reduced := a.reduceMotion(c)
	ui.Column(c).Gap(1.25).Shrink(0).Children(func() {
		colors := [3]ui.Color{ui.Hex("#B6D3EF"), ui.Hex("#EDB185"), ui.Hex("#F888A0")}
		for row := 0; row < 3; row++ {
			ui.Row(c).Gap(1.25).Children(func() {
				for col := 0; col < 3; col++ {
					opacity := float32(.6)
					if !reduced {
						t := phase + (2-float64(row)+math.Abs(float64(col)-1))/4
						t -= math.Floor(t)
						switch {
						case t < .45:
							opacity = 1 - .9*float32(t/.45)
						case t < .92:
							opacity = .1
						default:
							opacity = .1 + .9*float32((t-.92)/.08)
						}
					}
					ui.Box(c).Size(2.5, 2.5).Radius(1.25).Background(colors[row]).Opacity(opacity)
				}
			})
		}
	})
	if !reduced {
		c.After(time.Second / 15)
	}
}
