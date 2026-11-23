package contracts

import (
	"slices"
	"time"
)

// SubagentWorkflow 是工作流实例会话的代理类型。实例会话属于那次运行，
// 不是对话：它不在对话列表或智能体岛里占一条，也不接受用户直接发消息。
const SubagentWorkflow = "Workflow"

// 工作流运行状态。pending 表示已接受但尚未运行；running 包含等待问题和等待留白。
const (
	WorkflowStatusPending   = "pending"
	WorkflowStatusRunning   = "running"
	WorkflowStatusCompleted = "completed"
	WorkflowStatusErrored   = "errored"
	WorkflowStatusStopped   = "stopped"
)

// 停止原因。stopped 不等于失败；superseded 表示被修订后继替代，不显示恢复入口。
const (
	WorkflowStopUser        = "user"
	WorkflowStopModel       = "model"
	WorkflowStopProvider    = "provider"
	WorkflowStopInterrupted = "interrupted"
	WorkflowStopSuperseded  = "superseded"
)

// 全局模式。disabled 隐藏新入口与模型工具；onDemand 触发后在本会话内单向激活。
const (
	WorkflowModeDisabled = "disabled"
	WorkflowModeOnDemand = "onDemand"
	WorkflowModeAlwaysOn = "alwaysOn"
)

// 运行的来源。tool 是主代理通过工具发起的，amend 是被修订的后继，
// rerun 是从已保存定义再次运行的那一次。
const (
	WorkflowOriginTool  = "tool"
	WorkflowOriginAmend = "amend"
	WorkflowOriginRerun = "rerun"
	// WorkflowOriginHub 是从工作流中枢直接启动的运行：没有那一轮对话，
	// 也没有主代理在中间。
	WorkflowOriginHub = "hub"
)

// 阶段状态。阶段按图中声明顺序，不因状态变化重新排序。
const (
	WorkflowPhasePending = "pending"
	WorkflowPhaseRunning = "running"
	WorkflowPhaseDone    = "done"
	WorkflowPhaseFailed  = "failed"
	// WorkflowPhaseSkipped 表示这一站的条件没有成立，运行绕过了它。
	// 跳过是记录下来的事实，不是缺失。
	WorkflowPhaseSkipped = "skipped"
)

// 实例状态。executing、repairing、nudged 归入 running；queued、dispatched、waiting 归入 pending。
const (
	WorkflowActorPending = "pending"
	WorkflowActorRunning = "running"
	WorkflowActorAsking  = "asking"
	WorkflowActorDone    = "done"
	WorkflowActorFailed  = "failed"
)

// 工作流定义是可保存、可再次运行的脚本与参数契约。
type WorkflowDefinition struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	Scope       string             `json:"scope"` // project 或 global
	ProjectID   string             `json:"projectId,omitempty"`
	HostID      string             `json:"hostId,omitempty"`
	Script      string             `json:"script,omitempty"`
	Program     WorkflowProgram    `json:"program"`
	Arguments   []WorkflowArgument `json:"arguments,omitempty"`
	CreatedAt   time.Time          `json:"createdAt"`
	UpdatedAt   time.Time          `json:"updatedAt"`
}

// WorkflowArgument 声明一个运行参数。类型只允许 string、number、boolean、json。
type WorkflowArgument struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Default     string `json:"default,omitempty"`
	HasDefault  bool   `json:"hasDefault,omitempty"`
}

// WorkflowProgram 是脚本编译后的内部表示：阶段顺序、阶段内实例与分支。
// 阶段与实例都带稳定身份，投影裁剪不改变身份。
type WorkflowProgram struct {
	Phases []WorkflowPhaseSpec `json:"phases"`
}

// WorkflowPhaseSpec 声明一个阶段。Instances 里的每个实例都有自己的身份。
type WorkflowPhaseSpec struct {
	ID        string              `json:"id"`
	Name      string              `json:"name"`
	Instances []WorkflowActorSpec `json:"instances"`
	Edges     []WorkflowEdge      `json:"edges,omitempty"`
}

// WorkflowActorSpec 声明阶段内的一个子 Agent 实例。
type WorkflowActorSpec struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Prompt  string `json:"prompt,omitempty"`
	ModelID string `json:"modelId,omitempty"`
	Static  string `json:"static,omitempty"`
}

// WorkflowEdge 描述阶段之间的走向。Kind 为 next、branch 或 loop。
// When 是可选条件表达式：留空表示无条件成立。
type WorkflowEdge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Kind  string `json:"kind"`
	Label string `json:"label,omitempty"`
	When  string `json:"when,omitempty"`
}

// 脚本诊断的严重级别。只有 error 会阻止运行，warning 只提示。
const (
	WorkflowDiagnosticError   = "error"
	WorkflowDiagnosticWarning = "warning"
)

// WorkflowDiagnostic 是脚本编译的一条诊断。Path 指向出问题的位置。
type WorkflowDiagnostic struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Path     string `json:"path,omitempty"`
	Message  string `json:"message"`
}

// WorkflowRun 是一次运行的权威事实。对话运行卡、运行详情和运行目录共享同一份。
type WorkflowRun struct {
	ID           string `json:"id"`
	DefinitionID string `json:"definitionId,omitempty"`
	Name         string `json:"name"`
	SessionID    string `json:"sessionId,omitempty"`
	// MessageID 是发起这次运行的那一轮回复。运行卡挂在这一轮的轮尾，
	// 不是挂在会话的末尾。
	MessageID   string             `json:"messageId,omitempty"`
	ProjectID   string             `json:"projectId,omitempty"`
	HostID      string             `json:"hostId,omitempty"`
	Status      string             `json:"status"`
	StopReason  string             `json:"stopReason,omitempty"`
	Error       string             `json:"error,omitempty"`
	ModelID     string             `json:"modelId,omitempty"`
	Concurrency int                `json:"concurrency"`
	Result      string             `json:"result,omitempty"`
	Args        []WorkflowArgValue `json:"args,omitempty"`
	Phases      []WorkflowPhase    `json:"phases,omitempty"`
	Actors      []WorkflowActor    `json:"actors,omitempty"`
	Questions   []WorkflowQuestion `json:"questions,omitempty"`
	Holes       []WorkflowHole     `json:"holes,omitempty"`
	Artifacts   []WorkflowArtifact `json:"artifacts,omitempty"`
	Edges       []WorkflowEdge     `json:"edges,omitempty"`
	Script      string             `json:"script,omitempty"`
	StartedAt   time.Time          `json:"startedAt"`
	FinishedAt  *time.Time         `json:"finishedAt,omitempty"`
	UpdatedAt   time.Time          `json:"updatedAt"`
	Resumable   bool               `json:"resumable,omitempty"`
	Origin      string             `json:"origin,omitempty"`
}

// WorkflowArgValue 是一次运行实际收到的参数值。Kind 保留声明类型。
type WorkflowArgValue struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// WorkflowPhase 是运行中一个阶段的事实与观察。
type WorkflowPhase struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Index     int        `json:"index"`
	Status    string     `json:"status"`
	Observed  int        `json:"observed"`
	Settled   int        `json:"settled"`
	Rounds    int        `json:"rounds"`
	EnteredAt *time.Time `json:"enteredAt,omitempty"`
	SettledAt *time.Time `json:"settledAt,omitempty"`
}

// WorkflowActor 是阶段内一个子 Agent 实例的事实。表外实例也在名单里，只是不出现在卡片表内。
type WorkflowActor struct {
	ID         string     `json:"id"`
	PhaseID    string     `json:"phaseId"`
	Name       string     `json:"name"`
	Status     string     `json:"status"`
	ModelID    string     `json:"modelId,omitempty"`
	SessionID  string     `json:"sessionId,omitempty"`
	Error      string     `json:"error,omitempty"`
	Result     string     `json:"result,omitempty"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

// WorkflowQuestion 是 Actor 向主 Agent 升级的问题，UI 只展示，不在 Actor 行下直接要答案。
type WorkflowQuestion struct {
	ID         string     `json:"id"`
	RunID      string     `json:"runId"`
	ActorID    string     `json:"actorId,omitempty"`
	Text       string     `json:"text"`
	Context    string     `json:"context,omitempty"`
	Status     string     `json:"status"`
	Answer     string     `json:"answer,omitempty"`
	AskedAt    time.Time  `json:"askedAt"`
	ResolvedAt *time.Time `json:"resolvedAt,omitempty"`
}

// 留白状态。waiting 表示还在等人补全，filled 表示已经补上。
// 留白的类型不上界面：界面只分「还在等」和「已经补上」。
const (
	WorkflowHoleWaiting = "waiting"
	WorkflowHoleFilled  = "filled"
)

// WorkflowHole 是等待补全的留白。补全扩展同一个 run，不新建 run。
type WorkflowHole struct {
	ID        string     `json:"id"`
	RunID     string     `json:"runId"`
	PhaseID   string     `json:"phaseId,omitempty"`
	ParentID  string     `json:"parentId,omitempty"`
	Title     string     `json:"title"`
	Text      string     `json:"text,omitempty"`
	Status    string     `json:"status"`
	CreatedAt time.Time  `json:"createdAt"`
	FilledAt  *time.Time `json:"filledAt,omitempty"`
}

// WorkflowArtifact 是已发布的交付物。产物不等于脚本 return 或内部 report。
type WorkflowArtifact struct {
	ID          string    `json:"id"`
	RunID       string    `json:"runId"`
	Name        string    `json:"name"`
	MIME        string    `json:"mime,omitempty"`
	Path        string    `json:"path,omitempty"`
	Content     string    `json:"content,omitempty"`
	Primary     bool      `json:"primary,omitempty"`
	Version     int       `json:"version"`
	Order       int       `json:"order"`
	Size        int64     `json:"size,omitempty"`
	PublishedAt time.Time `json:"publishedAt"`
}

// WorkflowRunRequest 是启动一次运行的请求。Program 已被编译成内部表示。
type WorkflowRunRequest struct {
	DefinitionID string          `json:"definitionId,omitempty"`
	Program      WorkflowProgram `json:"program"`
	Name         string          `json:"name,omitempty"`
	SessionID    string          `json:"sessionId,omitempty"`
	// MessageID 是发起这次运行的那一轮回复。运行卡挂在那一轮的轮尾。
	MessageID   string             `json:"messageId,omitempty"`
	ProjectID   string             `json:"projectId,omitempty"`
	HostID      string             `json:"hostId,omitempty"`
	ModelID     string             `json:"modelId,omitempty"`
	Concurrency int                `json:"concurrency,omitempty"`
	Args        []WorkflowArgValue `json:"args,omitempty"`
	Script      string             `json:"script,omitempty"`
	Origin      string             `json:"origin,omitempty"`
}

// WorkflowSettingsChange 是一次设置改动。空字段表示不改。
type WorkflowSettingsChange struct {
	RunID       string `json:"runId"`
	ModelID     string `json:"modelId,omitempty"`
	HasModel    bool   `json:"hasModel,omitempty"`
	Concurrency int    `json:"concurrency,omitempty"`
	HasConc     bool   `json:"hasConc,omitempty"`
}

// WorkflowQuestionAnswer 是主 Agent 对升级问题的回答。
type WorkflowQuestionAnswer struct {
	RunID string `json:"runId"`
	ID    string `json:"id"`
	Text  string `json:"text"`
}

// WorkflowHoleFill 是对留白的补全。补全扩展当前 run。
type WorkflowHoleFill struct {
	RunID string `json:"runId"`
	ID    string `json:"id"`
	Text  string `json:"text"`
}

// WorkflowDefinitionSave 是保存工作流定义的请求。
type WorkflowDefinitionSave struct {
	ID          string             `json:"id,omitempty"`
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	Scope       string             `json:"scope"`
	ProjectID   string             `json:"projectId,omitempty"`
	HostID      string             `json:"hostId,omitempty"`
	Script      string             `json:"script,omitempty"`
	Program     WorkflowProgram    `json:"program"`
	Arguments   []WorkflowArgument `json:"arguments,omitempty"`
}

// WorkflowCommand 是工作流命令的载荷。具体动作由 Command.Kind 决定。
type WorkflowCommand struct {
	Run        *WorkflowRunRequest     `json:"run,omitempty"`
	Settings   *WorkflowSettingsChange `json:"settings,omitempty"`
	Question   *WorkflowQuestionAnswer `json:"question,omitempty"`
	Hole       *WorkflowHoleFill       `json:"hole,omitempty"`
	Definition *WorkflowDefinitionSave `json:"definition,omitempty"`
	Scope      string                  `json:"scope,omitempty"`
	ProjectID  string                  `json:"projectId,omitempty"`
	HostID     string                  `json:"hostId,omitempty"`
	RunID      string                  `json:"runId,omitempty"`
	ID         string                  `json:"id,omitempty"`
}

// Clone freezes a workflow payload before it crosses a goroutine or queue boundary.
func (c *WorkflowCommand) Clone() *WorkflowCommand {
	if c == nil {
		return nil
	}
	v := *c
	v.Run = c.Run.clone()
	v.Settings = copyPointer(c.Settings)
	v.Question = copyPointer(c.Question)
	v.Hole = copyPointer(c.Hole)
	if c.Definition != nil {
		d := *c.Definition
		d.Program = c.Definition.Program.clone()
		d.Arguments = slices.Clone(c.Definition.Arguments)
		v.Definition = &d
	}
	return &v
}

func (r *WorkflowRunRequest) clone() *WorkflowRunRequest {
	if r == nil {
		return nil
	}
	v := *r
	v.Program = r.Program.clone()
	v.Args = slices.Clone(r.Args)
	return &v
}

func (p WorkflowProgram) clone() WorkflowProgram {
	if len(p.Phases) == 0 {
		return WorkflowProgram{}
	}
	phases := make([]WorkflowPhaseSpec, len(p.Phases))
	for i, phase := range p.Phases {
		v := phase
		v.Instances = slices.Clone(phase.Instances)
		v.Edges = slices.Clone(phase.Edges)
		phases[i] = v
	}
	return WorkflowProgram{Phases: phases}
}
