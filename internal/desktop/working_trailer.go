package desktop

import (
	"fmt"
	"math"
	"strings"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/i18n"
	"gcode/internal/model"
	"github.com/egoist/mygo/ui"
)

type primaryPendingSend struct {
	RequestID  string
	DraftEpoch uint64
	StartedAt  time.Time
	Queued     bool
	// Steer tells that the queued message joins the live response instead
	// of waiting for its end.
	Steer bool
}

// A run's status is a plain string in the shared contract: the engine marks a
// run that is stopping before it finishes, and the UI reads it to say so.
const (
	runStatusRunning     = "running"
	runStatusStopping    = "stopping"
	runStatusFailed      = "failed"
	runStatusInterrupted = "interrupted"
)

// phaseText localizes an activity caption. A nil context keeps the source text
// for the callers that read the state without a frame (the trailer always has
// one).
func phaseText(c *ui.Context, key string) string {
	if c == nil {
		return key
	}
	return tr(c, key)
}

// latestRun is the session's most recent run, whose turn the trailer describes.
func (a *App) latestRun(sessionID string) (contracts.Run, bool) {
	var run contracts.Run
	found := false
	for _, candidate := range a.state.Runs {
		if candidate.SessionID != sessionID {
			continue
		}
		if !found || !candidate.StartedAt.Before(run.StartedAt) {
			run = candidate
			found = true
		}
	}
	return run, found
}

// sessionRunStopping reports whether the session's run is already being
// stopped, so the controls say so instead of asking again.
func (a *App) sessionRunStopping(sessionID string) bool {
	run, ok := a.latestRun(sessionID)
	return ok && run.Status == runStatusStopping
}

// sessionRunInFlight reports whether the session has a run executing or
// stopping, which is when a retry would be refused.
func (a *App) sessionRunInFlight(sessionID string) bool {
	run, ok := a.latestRun(sessionID)
	return ok && (run.Status == runStatusRunning || run.Status == runStatusStopping)
}

// phaseLabel names the step the session's latest run is on, from the activity
// that run actually produced instead of a decorative word.
func (a *App) phaseLabel(c *ui.Context, sessionID string) string {
	run, ok := a.latestRun(sessionID)
	if !ok {
		return phaseText(c, "Waiting for the model")
	}
	if run.Status == runStatusStopping {
		return phaseText(c, "Stopping…")
	}
	if run.Status == runStatusRunning && run.Compacting {
		return phaseText(c, "Compressing context")
	}
	// TurnActivity keeps the turn's work blocks: the last one is the step the
	// run is on now.
	blocks := contracts.TurnActivity(a.state.Messages[sessionID], run.ID)
	if len(blocks) == 0 {
		return phaseText(c, "Waiting for the model")
	}
	last := blocks[len(blocks)-1]
	switch {
	case last.Tool != nil:
		return toolPhaseLabel(c, last.Tool, blocks)
	case last.Kind == "reasoning":
		return phaseText(c, "Thinking")
	default:
		return phaseText(c, "Waiting for the model")
	}
}

// toolPhaseLabel names a tool call from what it does and what it touches.
func toolPhaseLabel(c *ui.Context, tool *contracts.ToolCall, turn []contracts.Block) string {
	switch category := tool.Category(); category {
	case contracts.ToolExec:
		// The run's own description says what the command is for, which reads
		// better than the whole command line. The core already publishes it as
		// the call's summary; the command is the fallback, and the tool name is
		// the last one so the label is never empty.
		target := tool.Summary
		if target == "" || target == tool.Name {
			target = tool.Command
		}
		if target == "" {
			target = tool.Name
		}
		return phaseTarget(target)
	case contracts.ToolReadFile:
		target := tool.Path
		if target == "" {
			target = tool.Summary
		}
		return fmt.Sprintf(phaseText(c, "Reading %s"), phaseTarget(target))
	case contracts.ToolEditFile, contracts.ToolWriteFile, contracts.ToolApplyPatch:
		return fmt.Sprintf(phaseText(c, "Editing %d files"), editedFileCount(turn))
	case contracts.ToolSearch, contracts.ToolGlob:
		target := tool.Summary
		if target == "" {
			target = tool.Path
		}
		return fmt.Sprintf(phaseText(c, "Searching %s"), phaseTarget(target))
	default:
		label, detail, _ := activityTitle(contracts.Block{Tool: tool})
		if category == contracts.ToolTodo {
			detail = tool.Summary
		}
		caption := phaseText(c, label)
		if detail = phaseTarget(detail); detail != "" {
			return caption + " " + detail
		}
		return caption
	}
}

// phaseTarget keeps a phase caption on one line and within a readable width:
// a shell command is often multi-line and a description can be long, and the
// trailer is a single status row rather than a log.
func phaseTarget(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return ""
	}
	const limit = 60
	if len([]rune(text)) <= limit {
		return text
	}
	return string([]rune(text)[:limit]) + "…"
}

// editedFileCount is how many distinct files the turn has changed.
func editedFileCount(turn []contracts.Block) int {
	paths := map[string]bool{}
	for _, block := range turn {
		if block.Tool == nil {
			continue
		}
		switch block.Tool.Category() {
		case contracts.ToolEditFile, contracts.ToolWriteFile, contracts.ToolApplyPatch:
			if block.Tool.Path != "" {
				paths[block.Tool.Path] = true
			}
		}
	}
	return max(len(paths), 1)
}

// workingState reports the session's activity for callers without a frame, in
// the source language, so the trailer is their only localized reader.
func (a *App) workingState(sessionID string) (word, elapsed string, visible bool) {
	return a.workingStateFor(nil, sessionID)
}

func (a *App) workingStateFor(c *ui.Context, sessionID string) (word, elapsed string, visible bool) {
	// A plain in-flight send owns only its bubble; queueing and steering stay
	// in the trailer because those need an explanation, not "sending" itself.
	if pending, ok := a.pendingSends[sessionID]; ok {
		if pending.Steer {
			return phaseText(c, "Steering the running response"), "", true
		}
		if pending.Queued {
			return phaseText(c, "Queued — will send automatically"), "", true
		}
		return "", "", false
	}
	if side := a.sideComposers[sessionID]; side != nil && side.Pending != nil {
		if side.Pending.Steer {
			return phaseText(c, "Steering the running response"), "", true
		}
		if side.Pending.Queued {
			return phaseText(c, "Queued — will send automatically"), "", true
		}
		return "", "", false
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
	word = a.phaseLabel(c, sessionID)
	run, ok := a.latestRun(sessionID)
	if !(ok && run.Status == runStatusRunning && run.Compacting) && !strings.HasSuffix(word, "…") {
		word += "…"
	}
	if !ok || run.StartedAt.IsZero() {
		// Without a real run there is no elapsed work to report.
		return word, "", true
	}
	milliseconds := max(a.now().UTC().Sub(run.StartedAt).Milliseconds(), run.DurationMS, 0)
	return word, i18n.Duration(a.state.Settings.General.Language, milliseconds), true
}

func (a *App) workingTrailer(c *ui.Context, sessionID string) {
	word, elapsed, visible := a.workingStateFor(c, sessionID)
	if !visible {
		return
	}
	// ZCode's turn loading slot keeps the first two retries as ordinary
	// loading. From retry three it shows a stable counter, never a countdown.
	for _, session := range a.state.Sessions {
		if session.ID != sessionID || session.Status != contracts.Working || session.ModelRetry == nil {
			continue
		}
		retry := session.ModelRetry
		waiting := retry.Reason == model.RetryWaitingForNetwork
		if !waiting && retry.Attempt < 3 {
			continue
		}
		caption := fmt.Sprintf(tr(c, "Reconnecting... %d/%d"), retry.Attempt, retry.MaxRetries)
		if waiting {
			caption = tr(c, "Waiting for network")
		}
		tooltip := caption
		if retry.HTTPStatus > 0 {
			tooltip += fmt.Sprintf(", HTTP %d", retry.HTTPStatus)
		}
		row := ui.Row(c).Label(tr(c, "Activity status: ")+sessionID).FillWidth().Padding(16, 0, 0, 0)
		row.Children(func() {
			label := ui.Text(c.Key("model-retry:"+sessionID), caption).Tooltip(tooltip).
				FontSize(design.TextSize(c, design.TextBase)).FontWeight(500).TextColor(a.palette.Muted)
			if !a.reduceMotion(c) {
				now := c.Now()
				started := ui.Local(row, "retry-shimmer-start", func() time.Time { return now })
				a.activityShimmerInk(c, label, caption, activityShimmerPhase(*started, now), 500, a.palette.Faint, a.palette.Muted)
				c.After(time.Second / 30)
			}
		})
		return
	}
	caption := tr(c, word)
	if strings.HasSuffix(word, "…") {
		caption = tr(c, strings.TrimSuffix(word, "…")) + "…"
	}
	ui.Row(c).Label(tr(c, "Activity status: ")+sessionID).FillWidth().Padding(16, 0, 0, 0).Gap(8).AlignItems(ui.Center).Children(func() {
		a.activityGlyph(c)
		color := a.palette.Muted
		if word == "Queued — will send automatically" {
			color = a.palette.Warning
		}
		ui.Text(c, caption).FontSize(design.TextSize(c, design.TextBase)).TextColor(color).SingleLine().MinWidth(0).Shrink(1)
		if elapsed != "" {
			ui.Text(c, elapsed).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted).Margin(1, 0, 0, 0).FontFeatures("tnum")
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
