package desktop

import (
	"fmt"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func testTodos(count, active int) []contracts.Todo {
	items := make([]contracts.Todo, count)
	for i := range items {
		status := "pending"
		if i < active {
			status = "completed"
		} else if i == active {
			status = "in_progress"
		}
		items[i] = contracts.Todo{ID: fmt.Sprint(i), Text: fmt.Sprintf("Task %d", i+1), Status: status}
	}
	return items
}

func TestTodoPanelLifecycleAndFocusWindow(t *testing.T) {
	items := testTodos(10, 4)
	start, end := todoFocusWindow(items)
	if start != 3 || end != 6 {
		t.Fatalf("focus window=%d:%d", start, end)
	}
	state := &todoPanelState{}
	state.observe(todoWrite{ID: "one", Items: items})
	if !state.visible() || state.expanded {
		t.Fatal("new list must be visible and collapsed")
	}
	state.expanded = true
	state.observe(todoWrite{ID: "two", Items: testTodos(10, 5)})
	if !state.expanded {
		t.Fatal("ordinary progress update lost manual expansion")
	}
	state.dismissed = true
	state.earlier, state.later = true, true
	state.observe(state.write)
	if state.visible() {
		t.Fatal("redraw resurrected a dismissed write")
	}
	state.observe(todoWrite{ID: "three", Items: items})
	if !state.visible() || state.expanded || state.earlier || state.later {
		t.Fatal("a new invocation after dismissal must reopen with all folds reset")
	}
	state.observe(todoWrite{ID: "finished", Items: testTodos(10, 10)})
	if state.visible() {
		t.Fatal("fully completed list stayed visible")
	}
	state.observe(todoWrite{ID: "new-work", Items: items})
	if !state.visible() || state.expanded {
		t.Fatal("new work after completion must start collapsed")
	}
	state.observe(todoWrite{ID: "clear"})
	if state.visible() || state.write.ID != "clear" {
		t.Fatal("empty write did not retire the old list")
	}
}

func TestLatestTodoWriteKeepsClearBoundaryAndChatOwnership(t *testing.T) {
	messages := []contracts.Message{
		{ID: "one", Role: "assistant", Blocks: []contracts.Block{{ID: "todo", Kind: "todos", Todos: testTodos(2, 0)}}},
		{ID: "two", Role: "user", Blocks: []contracts.Block{{ID: "untrusted", Kind: "todos", Todos: testTodos(4, 1)}}},
		{ID: "three", Role: "assistant", Blocks: []contracts.Block{{ID: "clear", Kind: "todos"}}},
	}
	if got := latestTodoWrite(messages); got.ID != "three:clear" || len(got.Items) != 0 {
		t.Fatalf("empty latest write rediscovered historical work: %+v", got)
	}
	a := testApp(t, "default")
	a.state.Messages["chat-0"] = messages[:1]
	a.state.Messages["child"] = messages[2:]
	a.state.Revision++
	if !a.todoPanel("main", "chat-0").visible() || a.todoPanel("side", "child").visible() {
		t.Fatal("side conversation todos crossed ownership")
	}
}

func TestComposerAttachedGoalTodoGeometryAndInteraction(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, width := range []int{360, 740} {
			t.Run(fmt.Sprintf("dark=%t/width=%d", dark, width), func(t *testing.T) {
				a := testApp(t, "default")
				a.selectSession("chat-0")
				session := a.currentSession()
				session.Goal = &contracts.SessionGoal{ID: "goal", Objective: "Keep the current goal above the input", Status: contracts.GoalPaused}
				setGoalFixture(a, session.ID, session.Goal)
				a.state.Messages[session.ID] = []contracts.Message{{ID: "write", Role: "assistant", Blocks: []contracts.Block{
					{ID: "todo", Kind: "todos", Todos: testTodos(10, 4)},
				}}}
				a.state.Revision++
				var todoBounds, goalBounds, inputBounds ui.Rect
				tt := ui.NewTester(func(c *ui.Context) {
					c.DisableAnimations()
					a.palette = design.Palette("rcode", dark)
					c.SetTheme(a.palette.UI())
					ui.Column(c).Fill().Justify(ui.End).Children(func() {
						a.composerAttachedPanels(c, session, "main")
						ui.Column(c.Key("input-surface")).Label("Input surface").FillWidth().Height(56)
					})
				}, width, 600)
				tt.Frame()
				tt.Frame()
				if tt.HasText("Task 4") || !tt.HasText("Task 5") || !tt.HasText("4/10") {
					t.Fatal("new checklist did not start with only its active headline:", tt.Texts())
				}
				todoBounds, _ = tt.Find("Todo list")
				goalBounds, _ = tt.Find("Goal")
				inputBounds, _ = tt.Find("Input surface")
				if todoBounds.Y+todoBounds.H > goalBounds.Y+.5 || goalBounds.Y+goalBounds.H > inputBounds.Y+.5 ||
					todoBounds.X-goalBounds.X != 12 || goalBounds.X+goalBounds.W-todoBounds.X-todoBounds.W != 12 {
					t.Fatalf("attached stack drift: todo=%+v goal=%+v input=%+v", todoBounds, goalBounds, inputBounds)
				}
				inset := design.AttachedPanels().Inset
				for _, panel := range []struct {
					bounds ui.Rect
					inset  float32
				}{{goalBounds, inset}, {todoBounds, inset * 2}} {
					if panel.bounds.X != inputBounds.X+panel.inset || panel.bounds.X+panel.bounds.W != inputBounds.X+inputBounds.W-panel.inset {
						t.Fatalf("attached panel exceeds composer inset: panel=%+v input=%+v inset=%g", panel.bounds, inputBounds, panel.inset)
					}
				}
				if err := tt.Click("Expand todos"); err != nil {
					t.Fatal(err)
				}
				tt.Frame()
				tt.Frame()
				for _, text := range []string{"Task 4", "Task 5", "Task 6", "3 earlier", "4 later"} {
					if !tt.HasText(text) {
						t.Fatalf("missing focused row %q: %v", text, tt.Texts())
					}
				}
				visible, _ := tt.Find("Todo list")
				lastRow, _ := tt.Find("Task 6")
				if lastRow.Y+lastRow.H > visible.Y+visible.H {
					t.Fatalf("expanded tasks are clipped: row=%+v panel=%+v", lastRow, visible)
				}
				if err := tt.Click("Show 3 earlier todos"); err != nil {
					t.Fatal(err)
				}
				tt.Frame()
				if !tt.HasText("Task 1") {
					t.Fatal("earlier fold did not reveal original order")
				}
				if err := tt.Click("Collapse todos"); err != nil {
					t.Fatal(err)
				}
				if err := tt.Click("Dismiss todos"); err != nil {
					t.Fatal(err)
				}
				tt.Advance(time.Second)
				if tt.HasText("4/10") {
					t.Fatal("dismissed checklist remained visible")
				}
				a.state.Messages[session.ID][0].Blocks[0].ID = "new-write"
				a.state.Revision++
				tt.Frame()
				if !tt.HasText("4/10") {
					t.Fatal("new invocation did not restore the tray")
				}
			})
		}
	}
}

func TestTranscriptTodosRemainOnlyInToolHistory(t *testing.T) {
	for _, compact := range []bool{false, true} {
		rows := transcriptRows(contracts.Message{ID: "message", Role: "assistant", Blocks: []contracts.Block{
			{ID: "call", Kind: "tool", Tool: &contracts.ToolCall{ID: "call", Name: "TodoWrite", Status: "completed"}},
			{ID: "typed-list", Kind: "todos", Todos: testTodos(10, 4)},
		}}, compact)
		if len(rows) != 1 || len(rows[0].Activity) != 1 {
			t.Fatalf("todo list leaked into transcript rows: %+v", rows)
		}
	}
}

func TestTodoProgressDoesNotStealComposerFocus(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	tt := ui.NewTester(a.View, 1320, 880)
	if err := tt.Click("Message"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Keep editing this message")
	for _, count := range []int{1, 3, 0} {
		a.state.Messages[a.sessionID] = []contracts.Message{{ID: fmt.Sprint(count), Role: "assistant", Blocks: []contracts.Block{{ID: "todos", Kind: "todos", Todos: testTodos(count, 0)}}}}
		a.state.Revision++
		tt.Frame()
		tt.Frame()
		if !tt.Focused("Message") || a.draft != "Keep editing this message" {
			t.Fatal("background progress changed message focus or draft")
		}
	}
}

func TestAttachedPanelsStayInsideRealComposer(t *testing.T) {
	for _, width := range []int{900, 1320} {
		for _, dark := range []bool{false, true} {
			t.Run(fmt.Sprintf("width=%d/dark=%t", width, dark), func(t *testing.T) {
				a := testApp(t, "default")
				a.selectSession("chat-0")
				session := a.currentSession()
				setGoalFixture(a, session.ID, &contracts.SessionGoal{ID: "goal", Objective: "Keep the command picker above the composer.", Status: contracts.GoalPaused})
				a.state.Messages[session.ID] = []contracts.Message{{ID: "todos", Role: "assistant", Blocks: []contracts.Block{
					{ID: "todo", Kind: "todos", Todos: testTodos(2, 0)},
				}}}
				a.state.Revision++
				a.state.Settings.Appearance.FontSize = 16
				a.state.Settings.Appearance.ThemeID = "rcode-light"
				if dark {
					a.state.Settings.Appearance.ThemeID = "rcode-dark"
				}
				var surface ui.Rect
				tt := ui.NewTester(func(c *ui.Context) {
					a.View(c)
					surface = a.composerSurface.Bounds(c)
				}, width, 600)
				tt.Advance(time.Second)
				tt.Frame()
				for _, panel := range []struct {
					label string
					inset float32
				}{{"Goal", design.AttachedPanels().Inset}, {"Todo list", design.AttachedPanels().Inset * 2}} {
					bounds, ok := tt.Find(panel.label)
					if !ok || bounds.X != surface.X+panel.inset || bounds.X+bounds.W != surface.X+surface.W-panel.inset {
						t.Fatalf("attached panel exceeds actual input: %s=%+v composer=%+v inset=%g", panel.label, bounds, surface, panel.inset)
					}
				}
				writeModelSurfaceEvidence(t, fmt.Sprintf("composer-attached-width-%d-dark-%t", width, dark), tt, map[string]any{"composer": surface})
			})
		}
	}
}
