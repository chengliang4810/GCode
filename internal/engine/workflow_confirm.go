package engine

// 运行确认。脚本编译好之后并不等于运行：用户先看到规模、设置和脚本，
// 决定这一次允不允许跑。批准之前没有运行，也没有运行卡。

import (
	"context"
	"errors"
	"log"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/workflow"
)

// workflowConfirmRequest 是一次等待决定的确认。
type workflowConfirmRequest struct {
	confirmation contracts.WorkflowConfirmation
	decision     chan contracts.WorkflowConfirmationDecision
}

// addWorkflowConfirmation 登记一次确认并等待用户决定。
func (e *Engine) addWorkflowConfirmation(ctx context.Context, confirmation contracts.WorkflowConfirmation) (contracts.WorkflowConfirmationDecision, error) {
	confirmation.ID = id("workflow-confirm")
	confirmation.CreatedAt = time.Now()
	confirmation.Status = contracts.WorkflowConfirmationPending
	request := &workflowConfirmRequest{confirmation: confirmation, decision: make(chan contracts.WorkflowConfirmationDecision, 1)}
	e.mu.Lock()
	if e.workflowConfirms == nil {
		e.workflowConfirms = map[string]*workflowConfirmRequest{}
	}
	e.workflowConfirms[confirmation.ID] = request
	e.state.WorkflowConfirmations = append(e.state.WorkflowConfirmations, confirmation)
	if err := e.persistLocked(); err != nil {
		delete(e.workflowConfirms, confirmation.ID)
		e.state.WorkflowConfirmations = e.state.WorkflowConfirmations[:len(e.state.WorkflowConfirmations)-1]
		e.mu.Unlock()
		return contracts.WorkflowConfirmationDecision{}, err
	}
	e.publishLocked(contracts.Event{Kind: "workflow.confirmation", SessionID: confirmation.SessionID})
	e.mu.Unlock()
	select {
	case <-ctx.Done():
		e.dropWorkflowConfirmation(confirmation.ID)
		return contracts.WorkflowConfirmationDecision{}, ctx.Err()
	case decision := <-request.decision:
		return decision, nil
	}
}

// dropWorkflowConfirmation 撤销一条没人回答的确认。调用者可能已经放弃等待。
func (e *Engine) dropWorkflowConfirmation(confirmationID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.workflowConfirms == nil {
		return
	}
	if _, ok := e.workflowConfirms[confirmationID]; !ok {
		return
	}
	delete(e.workflowConfirms, confirmationID)
	e.removeWorkflowConfirmationLocked(confirmationID)
	if err := e.persistLocked(); err != nil {
		log.Printf("workflow confirmation %s: %v", confirmationID, err)
	}
	e.publishLocked(contracts.Event{Kind: "workflow.confirmation"})
}

// removeWorkflowConfirmationLocked 从快照里去掉一条确认。调用者持有状态锁。
func (e *Engine) removeWorkflowConfirmationLocked(confirmationID string) {
	for i := range e.state.WorkflowConfirmations {
		if e.state.WorkflowConfirmations[i].ID != confirmationID {
			continue
		}
		e.state.WorkflowConfirmations = append(e.state.WorkflowConfirmations[:i], e.state.WorkflowConfirmations[i+1:]...)
		return
	}
}

// workflowConfirmLocked 落实用户对一次运行确认的决定。
func (e *Engine) workflowConfirmLocked(payload *contracts.WorkflowCommand) error {
	if payload.Confirmation == nil {
		return errors.New("a workflow confirmation decision is required")
	}
	decision := *payload.Confirmation
	request := e.workflowConfirms[decision.ID]
	if request == nil {
		return errors.New("this workflow confirmation is no longer waiting")
	}
	decision.Approve = decision.Approve || decision.Decision == contracts.WorkflowDecisionOnce || decision.Decision == contracts.WorkflowDecisionSession
	if decision.Decision == "" {
		if decision.Approve {
			decision.Decision = contracts.WorkflowDecisionOnce
		} else {
			decision.Decision = contracts.WorkflowDecisionReject
		}
	}
	sessionID := request.confirmation.SessionID
	if index := e.sessionIndex(sessionID); index >= 0 {
		if decision.Approve {
			// 批准一次就是激活了按需模式：之后的运行不必再打开功能。
			e.state.Sessions[index].WorkflowActivated = true
			if decision.Decision == contracts.WorkflowDecisionSession {
				e.state.Sessions[index].WorkflowAutoApprove = true
			}
		}
	}
	delete(e.workflowConfirms, decision.ID)
	e.removeWorkflowConfirmationLocked(decision.ID)
	if err := e.persistLocked(); err != nil {
		return err
	}
	e.publishLocked(contracts.Event{Kind: "workflow.confirmation", SessionID: sessionID})
	// 通道带缓冲，发送不会在持锁时阻塞。
	request.decision <- decision
	return nil
}

// workflowSessionMode 读取一个会话的有效模式、激活与免确认状态。
func (e *Engine) workflowSessionMode(sessionID string) (mode string, activated, autoApprove bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	mode = workflow.NormalizeMode(e.state.Settings.WorkflowMode)
	index := e.sessionIndex(sessionID)
	if index < 0 {
		return mode, false, false
	}
	session := e.state.Sessions[index]
	return mode, session.WorkflowActivated, session.WorkflowAutoApprove
}

// activateWorkflowSession 记下用户在本会话里打开了工作流能力。
func (e *Engine) activateWorkflowSessionLocked(sessionID string) bool {
	index := e.sessionIndex(sessionID)
	if index < 0 {
		return false
	}
	if e.state.Sessions[index].WorkflowActivated {
		return false
	}
	e.state.Sessions[index].WorkflowActivated = true
	return true
}

// workflowCurrentMessage 返回本会话最近一条助手消息。运行卡挂在那一轮，
// 不是挂在会话末尾。
func (e *Engine) workflowCurrentMessage(sessionID string) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	messages := e.state.Messages[sessionID]
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" {
			return messages[i].ID
		}
	}
	return ""
}
