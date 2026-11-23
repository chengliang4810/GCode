package engine

// /workflow 命令入口。命令本身不启动任何运行：它按全局模式打开本会话的
// 工作流能力，再把用户那句话展开成给主 Agent 的设计提示。真正的运行仍然
// 要经过脚本编译与用户确认。

import (
	"errors"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/workflow"
)

// workflowCommandName 是内置命令名，同时是保留字：同名用户命令既不列出
// 也不展开，内置命令的语义不能被插件顶掉。
const workflowCommandName = "workflow"

// workflowCommandText 从一段输入里认出 /workflow 命令。命令目录插入的引用
// 芯片和手敲的行首 /workflow 都算一次命令。正文保留其余引用，交给正常的
// 资源展开处理。
func workflowCommandText(text string) (body string, requested bool) {
	var out strings.Builder
	for _, part := range contracts.ParsePrompt(text) {
		if part.Reference != nil && part.Reference.Kind == "command" && part.Reference.ID == workflowCommandName {
			requested = true
			continue
		}
		out.WriteString(text[part.Start:part.End])
	}
	if requested {
		return strings.TrimSpace(out.String()), true
	}
	trimmed := strings.TrimSpace(out.String())
	if trimmed == "/"+workflowCommandName {
		return "", true
	}
	if rest, ok := strings.CutPrefix(trimmed, "/"+workflowCommandName+" "); ok {
		return strings.TrimSpace(rest), true
	}
	return "", false
}

// workflowActivateSession 按全局模式处理一次命令。关闭模式直接拒绝：命令
// 不能绕过设置。其余模式把本会话记成已激活，激活在该会话内一直保持。
func (e *Engine) workflowActivateSession(session *contracts.Session) error {
	mode := workflow.NormalizeMode(e.state.Settings.WorkflowMode)
	if mode == contracts.WorkflowModeDisabled {
		return errors.New("workflows are turned off in Settings")
	}
	session.WorkflowActivated = true
	return nil
}

// activateWorkflowSessionForRunLocked 让一次运行所属的会话拿到工作流能力。
// 中枢直接启动和恢复运行都算用户动作：按需模式这时就该打开。关闭模式不动
// 会话，工具仍然不出现。
func (e *Engine) activateWorkflowSessionForRunLocked(sessionID string) {
	if sessionID == "" {
		return
	}
	if workflow.NormalizeMode(e.state.Settings.WorkflowMode) == contracts.WorkflowModeDisabled {
		return
	}
	index := e.sessionIndex(sessionID)
	if index < 0 || e.state.Sessions[index].WorkflowActivated {
		return
	}
	e.state.Sessions[index].WorkflowActivated = true
}

// activateWorkflowSessionWithHistoryLocked 恢复带工作流历史的会话时重新激活它。
// 之前用过工作流的会话不因为重启而失去这个能力。
func (e *Engine) activateWorkflowSessionWithHistoryLocked(sessionID string) {
	if workflow.NormalizeMode(e.state.Settings.WorkflowMode) == contracts.WorkflowModeDisabled {
		return
	}
	index := e.sessionIndex(sessionID)
	if index < 0 || e.state.Sessions[index].WorkflowActivated {
		return
	}
	for _, run := range e.state.WorkflowRuns {
		if run.SessionID == sessionID {
			e.state.Sessions[index].WorkflowActivated = true
			return
		}
	}
}

// workflowCommandPrompt 是主 Agent 看到的提示词。它要求先定拓扑再写脚本，
// 并指向唯一的创建工具；请求正文就是用户写的那句话。
func workflowCommandPrompt(body string) string {
	var out strings.Builder
	out.WriteString("Design and launch a dynamic workflow for this request:\n\n")
	if body != "" {
		out.WriteString(body)
		out.WriteString("\n\n")
	}
	out.WriteString("A workflow runs a program: phases in declaration order, instances inside a phase, and optional edges that may carry a when condition. ")
	out.WriteString("Decide the topology before writing the script: how many instances, which of them share a phase, and what each one returns. ")
	out.WriteString("Then write the script as a JSON object with phases and instances and call the CreateWorkflow tool. ")
	out.WriteString("CreateWorkflow compiles the script and validates its arguments before starting it in the background. A separate script check is optional.")
	return out.String()
}
