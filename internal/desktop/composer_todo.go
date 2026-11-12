package desktop

import (
	"fmt"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

type todoWrite struct {
	ID    string
	Items []contracts.Todo
}

type todoSummary struct {
	done, total, active, next int
}

func summarizeTodos(items []contracts.Todo) todoSummary {
	s := todoSummary{total: len(items), active: -1, next: -1}
	for i, item := range items {
		if item.Status == "completed" {
			s.done++
			continue
		}
		if s.next < 0 {
			s.next = i
		}
		if item.Status == "in_progress" && s.active < 0 {
			s.active = i
		}
	}
	return s
}

func (s todoSummary) headline() int {
	if s.active >= 0 {
		return s.active
	}
	return s.next
}

func todoFocusWindow(items []contracts.Todo, summary todoSummary) (int, int) {
	if len(items) <= 6 {
		return 0, len(items)
	}
	focus := summary.headline()
	if focus < 0 {
		focus = len(items) - 1
	}
	start := min(max(focus-1, 0), len(items)-3)
	return start, start + 3
}

func latestTodoWrite(messages []contracts.Message) todoWrite {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "assistant" {
			continue
		}
		for j := len(messages[i].Blocks) - 1; j >= 0; j-- {
			block := &messages[i].Blocks[j]
			if block.Kind == "todo" || block.Kind == "todos" {
				return todoWrite{ID: messages[i].ID + ":" + block.ID, Items: block.Todos}
			}
		}
	}
	return todoWrite{}
}

// Full authoritative transcripts are available in GCode; no replay-window
// watermark or legacy history fallback is needed. Only presentation lives here.
type todoPanelState struct {
	write                    todoWrite
	summary                  todoSummary
	revision                 uint64
	observed                 bool
	expanded, earlier, later bool
	dismissed                bool
	openedAt                 time.Time
	scroll                   ui.ScrollState
}

func (s *todoPanelState) observe(write todoWrite) {
	before := s.summary
	retired := s.write.ID == "" || len(s.write.Items) == 0 || before.done == before.total
	fresh := retired || s.dismissed && write.ID != s.write.ID
	if write.ID != s.write.ID || fresh {
		s.dismissed = false
	}
	if fresh {
		s.expanded, s.earlier, s.later = false, false, false
		s.scroll = ui.ScrollState{}
	}
	s.write = write
	after := summarizeTodos(write.Items)
	s.summary = after
	if after.total == 0 || after.done == after.total {
		s.expanded = false
	}
}

func (s *todoPanelState) visible() bool {
	summary := s.summary
	return !s.dismissed && summary.total > 0 && summary.done < summary.total
}

func (a *App) todoPanel(scope, sessionID string) *todoPanelState {
	if a.todoPanels == nil {
		a.todoPanels = make(map[string]*todoPanelState)
	}
	key := scope + ":" + sessionID
	state := a.todoPanels[key]
	if state == nil {
		state = &todoPanelState{}
		a.todoPanels[key] = state
	}
	if !state.observed || state.revision != a.state.Revision {
		state.observe(latestTodoWrite(a.state.Messages[sessionID]))
		state.observed, state.revision = true, a.state.Revision
	}
	return state
}

// Each layer has a real clipped lower strip, so translucent surfaces cannot
// reveal the hidden border or accept clicks there.
func (a *App) attachedComposerPanel(c *ui.Context, key, label string, inset float32, fade bool, content func(*ui.Context)) {
	s := design.AttachedPanels()
	ui.Column(c.Key(key+":inset")).FillWidth().Shrink(0).Padding(0, inset).Children(func() {
		clip := ui.Column(c.Key(key)).Label(label).FillWidth().Shrink(0).ClipY()
		if fade {
			mounted := ui.Local(clip, "mounted-at", func() time.Time { return c.Now() })
			a.attachedFade(c, clip, *mounted)
		}
		uiState := ui.Local(clip, "height", func() float32 { return s.Height })
		clip.Height(max(0, *uiState-s.Overlap))
		clip.Children(func() {
			panel := ui.Column(c.Key(key+":surface")).FillWidth().Shrink(0).
				Radius(s.Radius, s.Radius, 0, 0).Padding(0, 2, s.Overlap, 2)
			panel = a.attachedComposerSurface(c, panel)
			panel.Children(func() { content(c) })
			height := panel.Bounds().H
			if height > 0 && height != *uiState {
				*uiState = height
				c.Invalidate()
			}
		})
	})
}

func (a *App) composerAttachedPanels(c *ui.Context, session contracts.Session, scope string) {
	if session.ID == "" || session.SubagentType != "" {
		return
	}
	a.todoTray(c, session, scope)
	a.goalBanner(c, session)
}

func (a *App) todoTray(c *ui.Context, session contracts.Session, scope string) {
	state := a.todoPanel(scope, session.ID)
	if !state.visible() {
		return
	}
	s := design.AttachedPanels()
	inset := s.Inset
	if session.Goal != nil {
		inset *= 2
	}
	key := "todo-panel:" + scope + ":" + session.ID
	live := session.Status == contracts.Working || session.Status == contracts.AwaitingInput
	summary := state.summary
	a.attachedComposerPanel(c, key, tr(c, "Todo list"), inset, true, func(c *ui.Context) {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			label, glyph := "Expand todos", "alt-arrow-up"
			if state.expanded {
				label, glyph = "Collapse todos", "alt-arrow-down"
			}
			controlAttachedToggle(c.Key(key+":toggle"), tr(c, label)).Value(fmt.Sprintf("%d/%d", summary.done, summary.total)).Grow(1).MinWidth(0).Children(func() {
				icon(c, "checklist", s.GlyphSize)
				ui.Text(c, tr(c, "Todo")).SingleLine().FontWeight(500)
				ui.Text(c.Key(key+":count"), fmt.Sprintf("%d/%d", summary.done, summary.total)).SingleLine().TextColor(a.palette.Faint)
				if i := summary.headline(); !state.expanded && i >= 0 {
					ui.Text(c.Key(key+":headline"), state.write.Items[i].Text).Grow(1).MinWidth(0).SingleLine().TextColor(a.palette.Muted)
				} else {
					ui.Spacer(c).Grow(1)
				}
				icon(c, glyph, 13)
			}).OnClick(func() { state.expanded = !state.expanded; state.openedAt = c.Now() })
			if !live {
				inlineIconButton(c.Key(key+":dismiss"), "close", tr(c, "Dismiss todos")).OnClick(func() {
					state.dismissed, state.expanded = true, false
				})
			}
		})
		if !state.expanded {
			return
		}
		_, height := c.Size()
		rows := ui.Column(c.Key(key+":rows")).Label(tr(c, "Todo list")).FillWidth().Margin(2, 0, 6, 0)
		a.attachedFade(c, rows, state.openedAt)
		rows.Children(func() {
			controlTodoScroll(c.Key(key+":scroll"), &state.scroll, height*.3).Children(func() {
				start, end := todoFocusWindow(state.write.Items, state.summary)
				fold := func(earlier bool, count int) {
					side, noun, open := "later", "%d later", &state.later
					if earlier {
						side, noun, open = "earlier", "%d earlier", &state.earlier
					}
					action, glyph := "Show %d "+side+" todos", "alt-arrow-down"
					if earlier {
						glyph = "alt-arrow-up"
					}
					if *open {
						action = "Hide %d " + side + " todos"
						if earlier {
							glyph = "alt-arrow-down"
						} else {
							glyph = "alt-arrow-up"
						}
					}
					controlAttachedToggle(c.Key(key+":fold-"+side), fmt.Sprintf(tr(c, action), count)).FillWidth().Children(func() {
						icon(c, glyph, 12)
						ui.Text(c, fmt.Sprintf(tr(c, noun), count)).TextColor(a.palette.Faint)
					}).OnClick(func() { *open = !*open })
				}
				if start > 0 {
					fold(true, start)
				}
				for i, item := range state.write.Items {
					if i < start && !state.earlier || i >= end && !state.later {
						continue
					}
					controlTodoRow(c.Key(fmt.Sprintf("%s:item:%d", key, i)), item.Status).Children(func() {
						controlTodoGlyph(c, item.Status, live, live && !a.reduceMotion(c) && !c.Instant(), a.palette.Glyph)
						ui.Text(c.Key(fmt.Sprintf("%s:text:%d", key, i)), item.Text).Grow(1).MinWidth(0)
					})
				}
				if end < len(state.write.Items) {
					fold(false, len(state.write.Items)-end)
				}
			})
		})
	})
}

func (a *App) attachedFade(c *ui.Context, element ui.Element, started time.Time) {
	if a.reduceMotion(c) || c.Instant() || started.IsZero() {
		return
	}
	progress := min(max(float32(c.Now().Sub(started))/float32(150*time.Millisecond), 0), 1)
	element.Opacity(panelEase(progress))
	if progress < 1 {
		c.AnimationFrame()
	}
}
