package engine

import (
	"encoding/json"
	"fmt"
	"strings"

	"gcode/internal/agent"
	"gcode/internal/contracts"
)

// Todo lists are a typed display projection of successful builtin calls.
// Keep the tool record as the source of truth, including after reopening a
// conversation; neither clients nor storage need another copy of task state.
func projectMessageTodos(message contracts.Message) contracts.Message {
	var blocks []contracts.Block
	for i, block := range message.Blocks {
		if blocks != nil {
			blocks = append(blocks, block)
		}
		tool := block.Tool
		if tool == nil || tool.Name != "TodoWrite" || tool.Status != "completed" || tool.IsError {
			continue
		}
		update, err := agent.ParseTodoUpdate(json.RawMessage(tool.Args))
		if err != nil {
			continue
		}
		if blocks == nil {
			blocks = make([]contracts.Block, i+1, len(message.Blocks)+1)
			copy(blocks, message.Blocks[:i+1])
		}
		copyTool := *tool
		copyTool.Kind = contracts.ToolTodo
		copyTool.Summary = todoProgress(update)
		list := contracts.Block{ID: block.ID + ":todos", Kind: "todos", Todos: make([]contracts.Todo, len(update.Todos))}
		for j, item := range update.Todos {
			list.Todos[j] = contracts.Todo{
				ID: fmt.Sprintf("%s:%d", list.ID, j), Text: strings.TrimSpace(item.Text), Status: string(item.Status),
			}
		}
		block.Tool, block.Todos = &copyTool, list.Todos
		blocks[len(blocks)-1] = block
		blocks = append(blocks, list)
	}
	if blocks != nil {
		message.Blocks = blocks
	}
	return message
}

func todoProgress(update agent.TodoUpdate) string {
	done := 0
	for _, item := range update.Todos {
		if item.Status == agent.TodoCompleted {
			done++
		}
	}
	return fmt.Sprintf("%d/%d", done, len(update.Todos))
}

// All snapshot paths (initial, reconnect and slow-subscriber recovery) share
// the same projection used by incremental message events.
func (e *Engine) snapshotLocked(sessionIDs ...string) contracts.Snapshot {
	snapshot := contracts.CloneSnapshot(contracts.SessionProjection(e.state, sessionIDs))
	for sessionID, messages := range snapshot.Messages {
		for i := range messages {
			messages[i] = projectMessageTodos(messages[i])
		}
		snapshot.Messages[sessionID] = messages
	}
	return snapshot
}
