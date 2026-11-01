package engine

import (
	"context"
	"errors"

	"gcode/internal/agent"
	"gcode/internal/contracts"
)

func (e *Engine) signalSubagentStarted(child *subagentExecution) {
	if child == nil || child.started == nil {
		return
	}
	child.startSignal.Do(func() { close(child.started) })
}

func (e *Engine) waitSubagentStarted(run *activeRun) {
	if run != nil && run.subagent != nil && run.subagent.started != nil {
		<-run.subagent.started
	}
}

// Background delegation retains the parent's Start rules until this boundary
// completes, then accepted background work continues on its own context.
func (e *Engine) subagentStartHook(ctx context.Context, session contracts.Session, child *subagentExecution) (agent.HookResult, error) {
	defer e.signalSubagentStarted(child)
	if child == nil || child.startHook == nil {
		return agent.HookResult{}, nil
	}
	startContext := child.startContext
	if startContext == nil {
		startContext = ctx
	}
	result, err := child.startHook(startContext, agent.HookInput{Event: contracts.HookSubagentStart, AgentID: session.ID, SubagentType: child.profile.ID})
	if err != nil && child.background && startContext.Err() != nil && ctx.Err() == nil {
		return agent.HookResult{}, nil
	}
	// The creation already succeeded; Start cannot reverse it.
	result.Deny = false
	if errors.Is(err, context.Canceled) && ctx.Err() != nil {
		return result, ctx.Err()
	}
	return result, err
}
