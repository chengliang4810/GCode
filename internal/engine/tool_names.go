package engine

import (
	"encoding/json"
	"strings"

	"gcode/internal/contracts"
)

// toolReadableNameLocked 给只带编号的工具行找可读的名字：TaskOutput 和
// TaskStop 的参数里只有 task_id，工作流工具的参数里只有 run_id，而界面
// 要显示的是任务名与运行名。照着会话与任务注册表换一次；换不到就返回空
// 串，让界面只显示动作词，不拿编号顶替名字。
func (e *Engine) toolReadableNameLocked(sessionID string, tool contracts.ToolCall) string {
	switch tool.Kind {
	case contracts.ToolTaskOutput, contracts.ToolTaskStop:
		return e.taskNameLocked(sessionID, tool.TaskID)
	case contracts.ToolWorkflow:
		if strings.TrimSpace(tool.Summary) != "" {
			return ""
		}
		var args struct {
			RunID string `json:"run_id"`
		}
		if json.Unmarshal([]byte(tool.Args), &args) != nil {
			return ""
		}
		return e.workflowRunNameLocked(args.RunID)
	}
	return ""
}

// taskNameLocked 是一个任务的显示名：子智能体会话用会话标题，后台命令用
// 启动时写下的描述。
func (e *Engine) taskNameLocked(sessionID, taskID string) string {
	if taskID == "" {
		return ""
	}
	for _, session := range e.state.Sessions {
		if session.ID == taskID && strings.TrimSpace(session.Title) != "" {
			return session.Title
		}
	}
	if set := e.sessionTools[sessionID]; set != nil && set.tasks != nil {
		for _, task := range set.tasks.List() {
			if task.ID == taskID && strings.TrimSpace(task.Title) != "" {
				return task.Title
			}
		}
	}
	return ""
}

// workflowRunNameLocked 是一次运行的显示名。运行没有名字时返回空串。
func (e *Engine) workflowRunNameLocked(runID string) string {
	if runID == "" {
		return ""
	}
	for _, run := range e.state.WorkflowRuns {
		if run.ID == runID {
			return strings.TrimSpace(run.Name)
		}
	}
	return ""
}
