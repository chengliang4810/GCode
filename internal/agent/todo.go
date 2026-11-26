// The agent's own task list. Successful TodoWrite calls in execution history
// own the checklist state; user text and summaries never supply it. The loop
// reminds the model to keep the list current and refuses an early stop while
// unfinished work has no recorded resolution.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"gcode/internal/model"
)

const (
	// TodoReminderInterval is how many assistant turns without a successful
	// TodoWrite call may pass before the loop injects a reminder.
	TodoReminderInterval = 10
	// TodoMaxItems caps one update: a longer list means the model is dumping
	// work instead of tracking it.
	TodoMaxItems = 100
	// TodoClosureLimit bounds consecutive checklist reminders without a tool
	// round in both ordinary conversations and goal execution.
	TodoClosureLimit = 8
)

type TodoStatus string

const (
	TodoPending    TodoStatus = "pending"
	TodoInProgress TodoStatus = "in_progress"
	TodoCompleted  TodoStatus = "completed"
)

type TodoItem struct {
	Text   string     `json:"text"`
	Status TodoStatus `json:"status"`
}

type TodoResolution struct {
	Kind   string `json:"kind"`
	Reason string `json:"reason"`
}

type TodoUpdate struct {
	Todos      []TodoItem      `json:"todos"`
	Resolution *TodoResolution `json:"resolution,omitempty"`
}

// todoStateMarker embeds the serialized state in reminder-class messages so
// todoStateFromHistory can restore it after compaction dropped the calls.
const todoStateMarker = "gcode:todo-state "

// ParseTodoUpdate shares the tool's input validation with host presentation.
func ParseTodoUpdate(raw json.RawMessage) (TodoUpdate, error) {
	var update TodoUpdate
	if err := json.Unmarshal(raw, &update); err != nil {
		return TodoUpdate{}, fmt.Errorf("invalid TodoWrite input: %w", err)
	}
	if update.Todos == nil {
		return TodoUpdate{}, fmt.Errorf("TodoWrite requires a todos array; use [] to clear it")
	}
	if len(update.Todos) > TodoMaxItems {
		return TodoUpdate{}, fmt.Errorf("TodoWrite accepts at most %d items", TodoMaxItems)
	}
	for _, item := range update.Todos {
		if strings.TrimSpace(item.Text) == "" {
			return TodoUpdate{}, fmt.Errorf("every item needs a non-empty text")
		}
		switch item.Status {
		case TodoPending, TodoInProgress, TodoCompleted:
		default:
			return TodoUpdate{}, fmt.Errorf("item status must be pending, in_progress or completed")
		}
	}
	if update.Resolution != nil && strings.TrimSpace(update.Resolution.Reason) == "" {
		return TodoUpdate{}, fmt.Errorf("a todo resolution needs a non-empty reason")
	}
	if update.Resolution != nil {
		switch update.Resolution.Kind {
		case "blocked", "cancelled", "scope_changed":
		default:
			return TodoUpdate{}, fmt.Errorf("todo resolution kind must be blocked, cancelled or scope_changed")
		}
	}
	return update, nil
}

// validateTodoArguments applies the checklist rules before the schema check,
// so the model receives the exact reason a list was rejected instead of the
// schema's generic enum or length violation.
func validateTodoArguments(raw json.RawMessage) error {
	_, err := ParseTodoUpdate(raw)
	return err
}

// CurrentTodos returns the requested goal's checklist, or the current ordinary
// task's checklist when goalInputID is empty.
func CurrentTodos(history []model.Message, goalInputID string) *TodoUpdate {
	return todoStateForTask(history, goalInputID).Current
}

// ResumeGoalTodos reopens this goal's checklist from the full execution history.
// A paused goal can outlive both ordinary tasks and context compaction.
func ResumeGoalTodos(history []model.Message, goalInputID string) (*TodoUpdate, model.Message) {
	state := todoState{goalInputID: goalInputID}
	if goalInputID != "" {
		state.Current = CurrentTodos(history, goalInputID)
	}
	if state.Current != nil {
		state.Current.Resolution = nil
	}
	return state.Current, todoMessage(state, "resume")
}

// todoState is the checklist plus the turn counters the reminders derive from.
// It is rebuilt from history every round; nothing else stores it.
type todoState struct {
	goalInputID string
	// Exported for the retained-state snapshot embedded in reminder messages.
	Current            *TodoUpdate `json:"current"`
	TurnsSinceWrite    int         `json:"turns_since_write"`
	TurnsSinceReminder int         `json:"turns_since_reminder"`
}

// Ordinary submissions and new goals start independent checklists. Steering an
// active goal and internal notifications remain part of the existing task.
func todoStateFromHistory(history []model.Message) todoState {
	// Compaction may inspect a historical prefix ending before the live task.
	for _, message := range slices.Backward(history) {
		if message.UserInput || message.Role == "assistant" || message.GoalInputID != "" || isTodoCheckpoint(message) {
			return todoStateForTask(history, message.GoalInputID)
		}
	}
	return todoState{}
}

func todoStateForTask(history []model.Message, goalInputID string) todoState {
	if goalInputID == "" {
		for i, message := range slices.Backward(history) {
			if message.UserInput || message.GoalInputID != "" {
				history = history[i+1:]
				break
			}
		}
	}
	state := todoState{goalInputID: goalInputID}
	type ordered struct {
		order int
		raw   json.RawMessage
	}
	calls := map[string]ordered{}
	nextCall, latestSuccess := 0, 0
	for i := range history {
		message := history[i]
		if message.UserInput || (message.Role == "assistant" && message.GoalInputID != goalInputID) {
			clear(calls)
			continue
		}
		if isTodoCheckpoint(message) {
			if message.GoalInputID != goalInputID {
				clear(calls)
				continue
			}
			if restored := parseTodoStateMessage(message.Text); restored != nil {
				state = *restored
				state.goalInputID = goalInputID
				clear(calls)
				continue
			}
		}
		if message.Role == "assistant" {
			state.TurnsSinceWrite++
			state.TurnsSinceReminder++
			for _, call := range message.Calls {
				if call.Name == "TodoWrite" {
					nextCall++
					calls[call.ID] = ordered{order: nextCall, raw: json.RawMessage(call.Arguments)}
				}
			}
		}
		if message.Role != "tool" {
			continue
		}
		call, ok := calls[message.CallID]
		delete(calls, message.CallID)
		if !ok || message.IsError {
			continue
		}
		// Concurrent tools may return out of order; a late result of an
		// earlier call must not overwrite the model's last successful update.
		if call.order <= latestSuccess {
			continue
		}
		update, err := ParseTodoUpdate(call.raw)
		if err != nil {
			continue
		}
		state.Current = &update
		state.TurnsSinceWrite = 0
		latestSuccess = call.order
	}
	return state
}

func isTodoCheckpoint(message model.Message) bool {
	return message.Role == "user" && !message.UserInput && !message.Summary &&
		strings.HasPrefix(message.Text, "<system-reminder>") && strings.Contains(message.Text, "<!-- "+todoStateMarker)
}

// parseTodoStateMessage extracts the state embedded by todoMessage. The
// marker line carries the full state JSON between the marker and the closing
// comment.
func parseTodoStateMessage(text string) *todoState {
	start := strings.Index(text, "<!-- "+todoStateMarker)
	if start < 0 {
		return nil
	}
	rest := text[start+len("<!-- ")+len(todoStateMarker):]
	end := strings.Index(rest, "-->")
	if end < 0 {
		return nil
	}
	var state todoState
	if err := json.Unmarshal([]byte(strings.TrimSpace(rest[:end])), &state); err != nil {
		return nil
	}
	return &state
}

func (s todoState) unfinished() bool {
	return s.Current != nil && func() bool {
		for _, item := range s.Current.Todos {
			if item.Status != TodoCompleted {
				return true
			}
		}
		return false
	}()
}

func (s todoState) reminderDue() bool {
	return s.unfinished() && s.Current.Resolution == nil &&
		s.TurnsSinceWrite >= TodoReminderInterval && s.TurnsSinceReminder >= TodoReminderInterval
}

// render serializes the current list for the model.
func (s todoState) render() string {
	if s.Current == nil {
		return "null"
	}
	data, err := json.MarshalIndent(s.Current, "", "  ")
	if err != nil {
		return "null"
	}
	return string(data)
}

// todoMessage builds the internal reminder-class user message. Host-generated
// context keeps UserInput false: providers receive the text, engines treat it
// as system guidance rather than a human submission.
func todoMessage(state todoState, kind string) model.Message {
	if kind == "reminder" || kind == "closure" {
		state.TurnsSinceReminder = 0
	}
	var instruction string
	switch kind {
	case "reminder":
		instruction = "The todo list has not been updated recently. Keep it aligned with your actual progress: mark finished items completed and the current item in_progress. Use TodoWrite only when relevant. Do not mention this internal reminder to the user."
	case "closure":
		instruction = "You stopped with unfinished todos. Continue the work and update TodoWrite before concluding. If genuinely blocked, cancelled, or out of scope, keep unfinished statuses and call TodoWrite with resolution={kind:blocked|cancelled|scope_changed,reason:<specific reason>}, then explain the remaining work to the user. Never mark unfinished work completed just to end the turn."
	case "resume":
		instruction = "The user explicitly resumed this goal. Use its saved checklist below and update it as work progresses."
	default:
		instruction = "Current todo state retained across compaction. This supersedes older todo lists; later successful TodoWrite calls replace it."
	}
	snapshot, err := json.Marshal(state)
	if err != nil {
		snapshot = []byte("{}")
	}
	body := fmt.Sprintf("%s\nCurrent todo list:\n%s\n<!-- %s%s -->", instruction, state.render(), todoStateMarker, snapshot)
	return model.Message{Role: "user", GoalInputID: state.goalInputID, Text: "<system-reminder>\n" + body + "\n</system-reminder>"}
}

type todoWriteTool struct{}

func (todoWriteTool) Definition() model.Tool {
	status := map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "completed"}, "description": "Where this item stands. Exactly one item should be in_progress."}
	return model.Tool{
		Name: "TodoWrite",
		Description: "Record or update the checklist for the current user task or active goal. Each update replaces that task's whole list. Leave unrelated historical items with their original task. " +
			"Use it for work with several steps; skip it for a single action. Update status as work progresses and immediately after completion. " +
			"Before ending, complete the remaining work, or preserve unfinished statuses and provide resolution={kind:blocked|cancelled|scope_changed,reason:<specific reason>}. " +
			"Explain that resolution to the user. Never mark unfinished work completed.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"todos": map[string]any{
					"type":        "array",
					"description": "The complete list, in the order you intend to work.",
					"items":       map[string]any{"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string", "description": "What to do, in the imperative."}, "status": status}, "required": []string{"text", "status"}},
				},
				"resolution": map[string]any{
					"type":        "object",
					"description": "Explicitly explain why remaining work cannot continue. Applies to this full list only; omit on a later update to resume normal completion checks.",
					"properties": map[string]any{
						"kind":   map[string]any{"type": "string", "enum": []string{"blocked", "cancelled", "scope_changed"}},
						"reason": map[string]any{"type": "string", "minLength": 1},
					},
					"required":             []string{"kind", "reason"},
					"additionalProperties": false,
				},
			},
			"required":             []string{"todos"},
			"additionalProperties": false,
		},
	}
}

func (todoWriteTool) Concurrent() bool { return true }

func (todoWriteTool) Execute(_ context.Context, _ string, raw json.RawMessage) (model.ToolOutput, error) {
	update, err := ParseTodoUpdate(raw)
	if err != nil {
		return model.ToolOutput{}, err
	}
	rendered, counts := renderTodos(update.Todos)
	return model.ToolOutput{Text: fmt.Sprintf("%s\n\n%d completed, %d in progress, %d pending", rendered, counts[0], counts[1], counts[2])}, nil
}

// renderTodos formats the tool result the model reads back, in list order.
// The host projects the validated arguments into a typed native checklist.
func renderTodos(items []TodoItem) (string, [3]int) {
	var lines []string
	counts := [3]int{}
	for _, item := range items {
		text := strings.TrimSpace(item.Text)
		switch item.Status {
		case TodoCompleted:
			lines = append(lines, "[x] "+text)
			counts[0]++
		case TodoInProgress:
			lines = append(lines, "[~] "+text)
			counts[1]++
		default:
			lines = append(lines, "[ ] "+text)
			counts[2]++
		}
	}
	return strings.Join(lines, "\n"), counts
}
