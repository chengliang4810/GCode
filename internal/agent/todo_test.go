package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"gcode/internal/model"
)

func todoUpdateJSON(t *testing.T, update TodoUpdate) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(update)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestParseTodoUpdateValidation(t *testing.T) {
	valid := TodoUpdate{Todos: []TodoItem{{Text: "ship", Status: TodoPending}}}
	if _, err := ParseTodoUpdate(todoUpdateJSON(t, valid)); err != nil {
		t.Fatalf("valid update rejected: %v", err)
	}
	for _, raw := range []string{`{}`, `{"todos":null}`} {
		if _, err := ParseTodoUpdate(json.RawMessage(raw)); err == nil {
			t.Fatalf("missing checklist cleared valid work: %s", raw)
		}
	}
	tooMany := TodoUpdate{}
	for i := 0; i <= TodoMaxItems; i++ {
		tooMany.Todos = append(tooMany.Todos, TodoItem{Text: fmt.Sprintf("item %d", i), Status: TodoPending})
	}
	if _, err := ParseTodoUpdate(todoUpdateJSON(t, tooMany)); err == nil {
		t.Fatal("over-long list accepted")
	}
	empty := TodoUpdate{Todos: []TodoItem{{Text: "  ", Status: TodoPending}}}
	if _, err := ParseTodoUpdate(todoUpdateJSON(t, empty)); err == nil {
		t.Fatal("empty item text accepted")
	}
	badStatus := TodoUpdate{Todos: []TodoItem{{Text: "x", Status: "done"}}}
	if _, err := ParseTodoUpdate(todoUpdateJSON(t, badStatus)); err == nil {
		t.Fatal("unknown status accepted")
	}
	noReason := TodoUpdate{Todos: valid.Todos, Resolution: &TodoResolution{Kind: "blocked"}}
	if _, err := ParseTodoUpdate(todoUpdateJSON(t, noReason)); err == nil {
		t.Fatal("resolution without reason accepted")
	}
	if _, err := ParseTodoUpdate(todoUpdateJSON(t, TodoUpdate{
		Todos: valid.Todos, Resolution: &TodoResolution{Kind: "made_up", Reason: "not a resolution"},
	})); err == nil {
		t.Fatal("unknown resolution kind accepted")
	}
}

func todoHistory() []model.Message {
	return []model.Message{
		{Role: "user", Text: "build it", UserInput: true},
		{Role: "assistant", Calls: []model.Call{{ID: "c1", Name: "TodoWrite", Arguments: `{"todos":[{"text":"one","status":"in_progress"},{"text":"two","status":"pending"}],"resolution":{"kind":"blocked","reason":"waiting on review"}}`}}},
		{Role: "tool", CallID: "c1", Text: "[~] one"},
	}
}

func TestTodoStateFromHistoryLatestSuccessWins(t *testing.T) {
	history := todoHistory()
	// A later call whose result arrived first must not overwrite the newest
	// successful update; an errored result is ignored entirely.
	history = append(history,
		model.Message{Role: "assistant", Calls: []model.Call{
			{ID: "c2", Name: "TodoWrite", Arguments: `{"todos":[{"text":"older","status":"completed"}]}`},
			{ID: "c3", Name: "TodoWrite", Arguments: `{"todos":[{"text":"newer","status":"pending"}]}`},
		}},
		model.Message{Role: "tool", CallID: "c3", Text: "ok"},
		model.Message{Role: "tool", CallID: "c2", Text: "ok"},
	)
	state := todoStateFromHistory(history)
	if len(state.Current.Todos) != 1 || state.Current.Todos[0].Text != "newer" {
		t.Fatalf("late older result overwrote the newest update: %+v", state.Current)
	}
	if state.TurnsSinceWrite != 0 {
		t.Fatalf("successful write did not reset the counter: %d", state.TurnsSinceWrite)
	}

	errored := todoHistory()
	errored = append(errored,
		model.Message{Role: "assistant", Calls: []model.Call{{ID: "c4", Name: "TodoWrite", Arguments: `{"todos":[{"text":"broken","status":"pending"}]}`}}},
		model.Message{Role: "tool", CallID: "c4", Text: "invalid", IsError: true},
	)
	state = todoStateFromHistory(errored)
	if len(state.Current.Todos) != 2 {
		t.Fatalf("errored result replaced the list: %+v", state.Current)
	}
}

func TestTodoStateCountersAndTaskBoundary(t *testing.T) {
	history := todoHistory()
	for i := 0; i < 3; i++ {
		history = append(history, model.Message{Role: "assistant", Text: "thinking"})
	}
	state := todoStateFromHistory(history)
	// The write turn itself also counts toward the reminder cadence.
	if state.TurnsSinceWrite != 3 || state.TurnsSinceReminder != 4 {
		t.Fatalf("counters wrong: %+v", state)
	}
	if !state.unfinished() {
		t.Fatal("pending item not reported unfinished")
	}
	if state.reminderDue() {
		t.Fatal("reminder due before the interval")
	}
	// Past the interval the blocked resolution still suppresses the reminder…
	history = append(history, model.Message{Role: "assistant", Text: "waiting"})
	for i := 0; i < 9; i++ {
		history = append(history, model.Message{Role: "assistant", Text: "waiting"})
	}
	state = todoStateFromHistory(history)
	if state.TurnsSinceWrite < TodoReminderInterval || state.reminderDue() {
		t.Fatalf("resolution failed to suppress the reminder: %+v", state)
	}
	// A later question has its own task, even after the reminder interval.
	history = append(history, model.Message{Role: "user", Text: "Which checks ran?", UserInput: true})
	state = todoStateFromHistory(history)
	if state.Current != nil || state.reminderDue() || state.TurnsSinceWrite != 0 || state.TurnsSinceReminder != 0 {
		t.Fatalf("new question inherited old work: %+v", state)
	}
	if old := CurrentTodos(history[:len(history)-1], ""); old == nil || old.Resolution == nil || old.Resolution.Kind != "blocked" {
		t.Fatalf("old task lost its resolution: %+v", old)
	}
}

func TestTodoRetainedMarkerRestoresState(t *testing.T) {
	history := todoHistory()
	state := todoStateFromHistory(history)
	restored := todoMessage(state, "retained")
	after := todoStateFromHistory(append(history, restored, model.Message{Role: "assistant", Text: "continue"}))
	if after.Current == nil || len(after.Current.Todos) != 2 {
		t.Fatalf("retained marker lost the list: %+v", after.Current)
	}
	if after.TurnsSinceWrite != 1 {
		t.Fatalf("restored state lost its counter base: %d", after.TurnsSinceWrite)
	}
	if after.Current.Resolution == nil {
		t.Fatal("retained state dropped the resolution")
	}
}

func TestGoalSteeringAndInternalResultsKeepTodoOwnership(t *testing.T) {
	for _, kind := range []string{"blocked", "cancelled", "scope_changed"} {
		t.Run(kind, func(t *testing.T) {
			const goal = "original-input"
			history := todoHistory()
			history[0].InputIDs, history[0].GoalInputID = []string{goal}, goal
			history[1].GoalInputID = goal
			history[1].Calls[0].Arguments = strings.Replace(history[1].Calls[0].Arguments, "blocked", kind, 1)
			history = append(history,
				model.Message{Role: "user", Text: "Child finished."},
				model.Message{Role: "user", Text: "Keep the current criteria.", UserInput: true, InputIDs: []string{"steer"}, GoalInputID: goal},
			)
			if got := CurrentTodos(history, goal); got == nil || got.Resolution == nil || got.Resolution.Kind != kind {
				t.Fatalf("goal steering lost the checklist or its resolution: %+v", got)
			}
			if got := todoStateForTask(history, ""); got.Current != nil {
				t.Fatalf("ordinary worker inherited goal work on an internal wake: %+v", got.Current)
			}
			failed := history[1]
			failed.GoalInputID = ""
			history = append(history, model.Message{Role: "user", UserInput: true, Text: "An independent task."}, failed,
				model.Message{Role: "tool", CallID: "c1", IsError: true})
			if got := CurrentTodos(history, ""); got != nil {
				t.Fatalf("failed write activated work: %+v", got)
			}
			resumed, checkpoint := ResumeGoalTodos(history, goal)
			if resumed == nil || resumed.Resolution != nil || len(resumed.Todos) != 2 {
				t.Fatalf("explicit resume failed to reopen the goal: %+v", resumed)
			}
			history = append(history, checkpoint, model.Message{Role: "user", Text: "Child finished."})
			if state := todoStateFromHistory(history); !state.unfinished() || state.Current.Resolution != nil || state.reminderDue() {
				t.Fatalf("resumed work lost its state: %+v", state)
			}
			history = append(history, model.Message{Role: "user", UserInput: true, GoalInputID: "replacement", InputIDs: []string{"replacement"}})
			if got := CurrentTodos(history, "replacement"); got != nil {
				t.Fatalf("replacement goal inherited previous work: %+v", got)
			}
		})
	}
}

func TestOnlyRuntimeRemindersRestoreTodos(t *testing.T) {
	checkpoint := todoMessage(todoStateFromHistory(todoHistory()), "retained")
	for _, msg := range []model.Message{
		{Role: "assistant", Text: checkpoint.Text},
		{Role: "tool", Text: checkpoint.Text},
		{Role: "user", Text: checkpoint.Text, UserInput: true},
		{Role: "user", Text: checkpoint.Text, Summary: true},
	} {
		if got := CurrentTodos([]model.Message{msg}, ""); got != nil {
			t.Fatalf("ordinary %s text created a checklist: %+v", msg.Role, got)
		}
	}
	if got := CurrentTodos([]model.Message{checkpoint}, ""); got == nil || got.Resolution == nil {
		t.Fatal("host checkpoint lost its authoritative checklist")
	}
}

type todoFixtureModel func(context.Context, model.Request) model.Message

func (f todoFixtureModel) Stream(ctx context.Context, request model.Request, _ func(model.Delta) error) (model.Result, error) {
	return model.Result{Message: f(ctx, request)}, nil
}

func runTodoScenario(t *testing.T, backend todoFixtureModel, tools []Tool) []model.Message {
	t.Helper()
	var recorded []model.Message
	err := Run(context.Background(), Options{
		Model: backend,
		Tools: tools,
		Record: func(message model.Message) error {
			recorded = append(recorded, message)
			return nil
		},
		Emit:        func(Event) error { return nil },
		Pending:     func(bool) []model.Message { return nil },
		Request:     model.Request{History: []model.Message{{Role: "user", Text: "work", UserInput: true}}},
		ToolAllowed: func(string) bool { return true },
	})
	if err != nil {
		t.Fatal(err)
	}
	return recorded
}

func writeTodos(t *testing.T, id string, update TodoUpdate) model.Call {
	t.Helper()
	return model.Call{ID: id, Name: "TodoWrite", Arguments: string(todoUpdateJSON(t, update))}
}

// noopTool lets the scenario keep working without stopping: the closure path
// only fires when the model stops, while the reminder targets turns spent
// working with a stale list.
type noopTool struct{}

func (noopTool) Definition() model.Tool { return model.Tool{Name: "Noop"} }
func (noopTool) Concurrent() bool       { return true }
func (noopTool) Execute(context.Context, string, json.RawMessage) (model.ToolOutput, error) {
	return model.ToolOutput{Text: "ok"}, nil
}

func TestTodoReminderInjectedAfterIdleTurns(t *testing.T) {
	turn := 0
	recorded := runTodoScenario(t, func(_ context.Context, _ model.Request) model.Message {
		turn++
		switch {
		case turn == 1:
			return model.Message{Role: "assistant", Calls: []model.Call{writeTodos(t, "w1", TodoUpdate{Todos: []TodoItem{{Text: "only", Status: TodoInProgress}}})}}
		case turn <= 11:
			// Ten working turns without touching the list cross the interval.
			return model.Message{Role: "assistant", Calls: []model.Call{{ID: fmt.Sprintf("n%d", turn), Name: "Noop", Arguments: "{}"}}}
		case turn == 12:
			return model.Message{Role: "assistant", Calls: []model.Call{writeTodos(t, "w2", TodoUpdate{Todos: []TodoItem{{Text: "only", Status: TodoCompleted}}})}}
		default:
			return model.Message{Role: "assistant", Text: "finished for real"}
		}
	}, []Tool{todoWriteTool{}, noopTool{}})
	reminders := 0
	for _, message := range recorded {
		if strings.Contains(message.Text, "has not been updated recently") {
			reminders++
		}
	}
	if reminders != 1 {
		t.Fatalf("reminder injected %d times, want exactly one", reminders)
	}
}

func TestTodoClosureContinuesUntilListCompletes(t *testing.T) {
	turn := 0
	recorded := runTodoScenario(t, func(_ context.Context, _ model.Request) model.Message {
		turn++
		switch turn {
		case 1:
			return model.Message{Role: "assistant", Calls: []model.Call{writeTodos(t, "w1", TodoUpdate{Todos: []TodoItem{{Text: "first", Status: TodoInProgress}, {Text: "second", Status: TodoPending}}})}}
		case 2:
			// Stops with unfinished todos: the loop must push back once.
			return model.Message{Role: "assistant", Text: "all done!"}
		case 3:
			return model.Message{Role: "assistant", Calls: []model.Call{writeTodos(t, "w2", TodoUpdate{Todos: []TodoItem{{Text: "first", Status: TodoCompleted}, {Text: "second", Status: TodoCompleted}}})}}
		default:
			return model.Message{Role: "assistant", Text: "finished for real"}
		}
	}, []Tool{todoWriteTool{}})
	closures := 0
	for _, message := range recorded {
		if strings.Contains(message.Text, "stopped with unfinished todos") {
			closures++
		}
	}
	if closures != 1 {
		t.Fatalf("closure injected %d times, want exactly one", closures)
	}
	last := recorded[len(recorded)-1]
	if last.Role != "assistant" || last.Text != "finished for real" {
		t.Fatalf("run did not end on the completion turn: %+v", last)
	}
}

func TestTodoResolutionStopsWithoutClosure(t *testing.T) {
	turn := 0
	recorded := runTodoScenario(t, func(_ context.Context, _ model.Request) model.Message {
		turn++
		if turn == 1 {
			return model.Message{Role: "assistant", Calls: []model.Call{writeTodos(t, "w1", TodoUpdate{
				Todos:      []TodoItem{{Text: "blocked item", Status: TodoPending}},
				Resolution: &TodoResolution{Kind: "blocked", Reason: "upstream outage"},
			})}}
		}
		return model.Message{Role: "assistant", Text: "explained the blocker"}
	}, []Tool{todoWriteTool{}})
	for _, message := range recorded {
		if strings.Contains(message.Text, "stopped with unfinished todos") {
			t.Fatal("recorded resolution did not allow the stop")
		}
	}
	if turn != 2 {
		t.Fatalf("run continued past the resolution: %d turns", turn)
	}
}
