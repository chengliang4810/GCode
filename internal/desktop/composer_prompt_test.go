package desktop

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/richtext"
)

func promptTester(t *testing.T, a *App, width float32, sends, stops *int) *ui.Tester {
	t.Helper()
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(a.palette.UI())
		ui.Column(c).Width(width).Padding(300, 0, 0, 0).Children(func() {
			a.nativeComposerSurface(c, composerSurfaceProps{
				draft: &a.draft, input: &a.composer, surface: &a.composerSurface, controls: a.centralControls(),
				inputLabel: "Draft", attachLabel: "Attach", sendLabel: "Send", stopLabel: "Stop",
				hero: true, widthHint: width, canSend: true, stopWithEscape: true,
				onSend: func(string) bool { *sends++; return false }, onStop: func() { *stops++ }, onChooseAttach: func() {},
			})
		})
	}, int(width)+40, 700)
	tt.Advance(time.Second)
	return tt
}

func TestComposerPromptPickerKeyboardAndDismissal(t *testing.T) {
	a := testApp(t, "default")
	sends, stops := 0, 0
	tt := promptTester(t, a, 600, &sends, &stops)
	if err := tt.Click("Draft"); err != nil {
		t.Fatal(err)
	}
	tt.Type("/go")
	if !a.prompt.open || len(a.prompt.choices) != 1 {
		t.Fatalf("goal search did not open: %+v", a.prompt)
	}
	tt.Key(0, ui.KeyEnter)
	if sends != 0 || !strings.Contains(a.draft, "gcode://command/goal") || a.prompt.doc.Text().Len() != 2 {
		t.Fatalf("Enter did not insert one atomic reference and space: %q, sends %d", a.draft, sends)
	}
	tt.Type("完成任务")
	tt.Key(0, ui.KeyEnter)
	if sends != 1 {
		t.Fatal("second Enter did not send")
	}
	a.draft = "@"
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	if a.prompt.open || stops != 0 {
		t.Fatal("Escape stopped a run instead of closing the picker")
	}
	tt.Frame()
	if a.prompt.open {
		t.Fatal("dismissed query reopened")
	}
	tt.Type("d")
	if !a.prompt.open {
		t.Fatal("editing query did not reopen picker")
	}
}

func TestComposerPromptAtomicUndoAndSameNameIdentity(t *testing.T) {
	a := testApp(t, "default")
	a.state.UserResources.Skills = []contracts.UserSkill{
		{ID: "one", Name: "Same", Enabled: true},
		{ID: "two", Name: "Same", Enabled: true},
	}
	sends, stops := 0, 0
	tt := promptTester(t, a, 600, &sends, &stops)
	_ = tt.Click("Draft")
	tt.Type("@two")
	tt.Key(0, ui.KeyTab)
	if !strings.Contains(a.draft, "gcode://skill/two") {
		t.Fatalf("wrong same-name resource selected: %q", a.draft)
	}
	tt.Key(0, ui.KeyBackspace) // following space
	tt.Key(0, ui.KeyBackspace) // whole reference
	if a.draft != "" {
		t.Fatalf("token was not atomic: %q", a.draft)
	}
	tt.Command("undo")
	if !strings.Contains(a.draft, "gcode://skill/two") {
		t.Fatalf("native undo lost token identity: %q", a.draft)
	}
	a.prompt.doc.SetText(richtext.NewText("Same"))
	tt.Frame()
	if a.draft != "Same" {
		t.Fatalf("plain text acquired an old identity: %q", a.draft)
	}
}

func TestComposerPromptAdjacentCopiesKeepBothReferences(t *testing.T) {
	a := testApp(t, "default")
	raw := (contracts.PromptReference{Kind: "skill", ID: "docs", Label: "文档"}).Markdown()
	text := a.promptText(raw)
	text = text.Insert(text.Len(), text)
	if got := promptCanonical(text); got != raw+raw {
		t.Fatalf("adjacent references merged during serialization: %q", got)
	}
}

func TestComposerPromptIMEAndMiddleCompletion(t *testing.T) {
	a := testApp(t, "default")
	sends, stops := 0, 0
	tt := promptTester(t, a, 360, &sends, &stops)
	_ = tt.Click("Draft")
	tt.Type("/goal")
	a.prompt.doc.SetSelection(3, 3)
	tt.Frame()
	tt.Key(0, ui.KeyTab)
	if promptDisplay(a.draft) != "Goal " {
		t.Fatalf("completion left a suffix: %q", a.draft)
	}
	tt.Compose("你好", 2)
	tt.Key(0, ui.KeyEnter)
	if sends != 0 || !a.prompt.state.Composing || strings.Contains(a.draft, "你好") {
		t.Fatal("IME confirmation sent or committed the draft")
	}
}

func TestComposerPromptMouseSelectionKeepsPickerMounted(t *testing.T) {
	a := testApp(t, "default")
	sends, stops := 0, 0
	tt := promptTester(t, a, 600, &sends, &stops)
	_ = tt.Click("Draft")
	tt.Type("/in")
	if err := tt.Click("Init"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(a.draft, "gcode://command/init") || !tt.Focused("Draft") {
		t.Fatalf("pointer selection lost token or focus: %q", a.draft)
	}
}

func TestComposerPromptCompletionPreservesFollowingProse(t *testing.T) {
	for _, test := range []struct {
		name, draft, query, want string
		caret                    int
	}{
		{"bare trigger", "@正文", "", "文档 正文", 1},
		{"partial query", "/go完成任务", "go", "Goal 完成任务", 3},
		{"candidate suffix", "/goal完成任务", "go", "Goal 完成任务", 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := testApp(t, "default")
			ref := contracts.PromptReference{Kind: "command", ID: "goal", Label: "Goal"}
			if test.query == "" {
				ref = contracts.PromptReference{Kind: "skill", ID: "docs", Label: "文档"}
			}
			s := &promptEditor{start: 0, end: test.caret, query: test.query}
			a.syncPrompt(s, test.draft)
			draft := test.draft
			a.completePrompt(s, promptChoice{ref: ref}, &draft)
			if got := contracts.DisplayPrompt(draft); got != test.want {
				t.Fatalf("following prose was changed: %q, want %q", got, test.want)
			}
		})
	}
}

func TestComposerPromptPickerAboveAndCoversGoalTodos(t *testing.T) {
	for _, test := range []struct {
		name, trigger    string
		width, top       float32
		hero, side, dark bool
	}{
		{"home-light", "/", 600, 350, true, false, false},
		{"home-narrow-dark", "@", 360, 350, true, false, true},
		{"thread-short-dark", "$", 600, 130, false, false, true},
		{"side-narrow-light", "/", 360, 350, false, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := testApp(t, "default")
			a.selectSession("chat-0")
			a.palette = design.Palette("rcode", test.dark)
			for i := range 20 {
				id := fmt.Sprintf("choice-%02d", i)
				a.state.UserResources.Commands = append(a.state.UserResources.Commands, contracts.UserCommand{ID: id, Name: id, Enabled: true})
				a.state.UserResources.Skills = append(a.state.UserResources.Skills, contracts.UserSkill{ID: id, Name: id, Enabled: true})
			}
			session := a.currentSession()
			session.Goal = &contracts.SessionGoal{ID: "overlap", Objective: "Goal behind completions", Status: contracts.GoalPaused, Iteration: 1}
			todos := contracts.Block{Kind: "todos", Todos: []contracts.Todo{{Text: "Todo behind completions", Status: "pending"}}}
			controller := a.centralControls()
			draft, input, surface := &a.draft, &a.composer, &a.composerSurface
			if test.side {
				state := a.sideComposer(session.ID)
				controller = a.sideControls(session.ID, state)
				draft, input, surface = &state.Draft, &state.Input, &state.Surface
			}
			var panel, composer ui.Rect
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(a.palette.UI())
				c.Root().Background(a.palette.Background)
				// The clipped composer is built before the goal/todos, so the
				// popup must escape clipping and paint over later normal content.
				ui.Column(c).Fill().Children(func() {
					ui.Column(c).Absolute().Top(test.top).Left(20).Width(test.width).Height(130).Clip().Children(func() {
						a.nativeComposerSurface(c, composerSurfaceProps{
							draft: draft, input: input, surface: surface, controls: controller,
							inputLabel: "Overlay draft", attachLabel: "Attach", sendLabel: "Send", stopLabel: "Stop",
							hero: test.hero, widthHint: test.width,
							onSend: func(string) bool { return false }, onChooseAttach: func() {},
						})
					})
					ui.Column(c).Absolute().Top(max(20, test.top-220)).Left(20).Width(test.width).Children(func() {
						a.goalBanner(c, session)
						a.transcriptBlock(c, contracts.Message{}, todos)
					})
				})
				panel = controller.prompt.panel.Bounds(c)
				composer = surface.Bounds(c)
			}, int(test.width)+40, 700)
			tt.Advance(time.Second)
			if err := tt.Click("Overlay draft"); err != nil {
				t.Fatal(err)
			}
			tt.Type(test.trigger)
			tt.Advance(time.Second)
			if panel.H <= 0 || panel.Y < 0 || panel.Y+panel.H > composer.Y {
				t.Fatalf("completion list did not stay above input: panel=%+v input=%+v", panel, composer)
			}
			goal, _ := tt.Find("Show goal details")
			todo, _ := tt.Find("Todo behind completions")
			overlap := func(a, b ui.Rect) bool { return a.X < b.X+b.W && b.X < a.X+a.W && a.Y < b.Y+b.H && b.Y < a.Y+a.H }
			if !overlap(panel, goal) || !overlap(panel, todo) {
				t.Fatalf("fixture did not cover both goal and todo: panel=%+v goal=%+v todo=%+v", panel, goal, todo)
			}
			var selected *promptChoice
			for _, choice := range controller.prompt.choices {
				bounds, ok := tt.Find(choice.ref.Label)
				if ok && overlap(bounds, goal) {
					value := choice
					selected = &value
					break
				}
			}
			if selected == nil {
				t.Fatal("no candidate overlaps the goal control")
			}
			if err := tt.Click(selected.ref.Label); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(*draft, selected.ref.Markdown()) || tt.HasText("Clear goal") {
				t.Fatalf("popup click was clipped or reached the underlying goal: draft=%q", *draft)
			}
		})
	}
}
