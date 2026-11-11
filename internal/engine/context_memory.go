package engine

import (
	"gcode/internal/agent"
	"gcode/internal/model"
)

// View and archive share the same content-addressed session record. A failed
// write must not leave the running engine using an uncommitted projection.
func (e *Engine) replaceContextMemoryLocked(sessionID string, messages []model.Message, state agent.ContextMemory) error {
	if err := e.ensureSessionDataLocked(sessionID); err != nil {
		return err
	}
	if e.contextMemories == nil {
		e.contextMemories = map[string]agent.ContextMemory{}
	}
	oldContext, oldMemory := e.context[sessionID], e.contextMemories[sessionID]
	e.context[sessionID] = model.CloneMessages(messages)
	e.contextMemories[sessionID] = agent.CloneContextMemory(state)
	if err := e.persistLocked(sessionID); err != nil {
		e.context[sessionID], e.contextMemories[sessionID] = oldContext, oldMemory
		return err
	}
	return nil
}
