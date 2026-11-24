package workflow

import (
	"time"

	"gcode/internal/contracts"
)

// RunView 是一次运行的展示投影。对话运行卡、运行详情和运行目录都读同一份，
// 不各自推导状态、名册或产物。
type RunView struct {
	RunID       string
	Name        string
	Status      string
	StopReason  string
	Resumable   bool
	ModelID     string
	Concurrency int
	Origin      string
	// 发起这一轮运行的那条回复；空值表示它不是一轮对话的结果。
	MessageID  string
	SessionID  string
	StartedAt  time.Time
	FinishedAt *time.Time
	// UpdatedAt is when the run last changed; a live run's duration reads
	// from here.
	UpdatedAt time.Time
	Phases    []RunPhaseView
	Questions []contracts.WorkflowQuestion
	Holes     []contracts.WorkflowHole
	Artifacts ArtifactSlice
	// Truncated 说明卡片上的名册确实被裁剪过，详情能看全。
	Truncated bool
}

// RunPhaseView 是阶段脊柱上的一站：声明顺序、状态灯和这一站的名册。
type RunPhaseView struct {
	ID       string
	Name     string
	Index    int
	Status   string
	Observed int
	Settled  int
	Rounds   int
	Actors   []RunActorView
	Roster   Roster
}

// RunActorView 是一枚可点的实例药丸。会话缺席即它还没有自己的时间线。
type RunActorView struct {
	ID        string
	Name      string
	Status    string
	SessionID string
	ModelID   string
	Error     string
}

// ProjectRun 把一次运行投影成卡片与详情共享的展示事实。阶段按声明顺序，
// 名册有界，产物限三个，其余计数如实报告。
func ProjectRun(run contracts.WorkflowRun) RunView {
	view := RunView{
		RunID:       run.ID,
		Name:        run.Name,
		Status:      run.Status,
		StopReason:  run.StopReason,
		Resumable:   run.Resumable && CanResume(run),
		ModelID:     run.ModelID,
		Concurrency: NormalizeConcurrency(run.Concurrency),
		Origin:      run.Origin,
		MessageID:   run.MessageID,
		SessionID:   run.SessionID,
		StartedAt:   run.StartedAt,
		FinishedAt:  run.FinishedAt,
		UpdatedAt:   run.UpdatedAt,
		Questions:   append([]contracts.WorkflowQuestion(nil), run.Questions...),
		Holes:       append([]contracts.WorkflowHole(nil), run.Holes...),
		Artifacts:   CardArtifacts(run.Artifacts),
	}
	for _, phase := range run.Phases {
		actors := phaseActorList(run.Actors, phase.ID)
		host := RunPhaseView{
			ID:       phase.ID,
			Name:     phase.Name,
			Index:    phase.Index,
			Status:   phaseStatus(phase, actors),
			Observed: phase.Observed,
			Settled:  phase.Settled,
			Rounds:   phase.Rounds,
			Roster:   BuildRoster(actors),
		}
		for _, actor := range actors {
			host.Actors = append(host.Actors, RunActorView{
				ID:        actor.ID,
				Name:      actor.Name,
				Status:    ActorState(actor.Status),
				SessionID: actor.SessionID,
				ModelID:   actor.ModelID,
				Error:     actor.Error,
			})
		}
		if host.Roster.Overflow > 0 {
			view.Truncated = true
		}
		view.Phases = append(view.Phases, host)
	}
	return view
}

// phaseActorList 取某一阶段的全部实例，表外实例同样在名单里。
func phaseActorList(actors []contracts.WorkflowActor, phaseID string) []contracts.WorkflowActor {
	out := make([]contracts.WorkflowActor, 0, len(actors))
	for _, actor := range actors {
		if actor.PhaseID == phaseID {
			out = append(out, actor)
		}
	}
	return out
}

// phaseStatus 优先报告执行器记录的事实；没有记录时才按实例聚合。
func phaseStatus(phase contracts.WorkflowPhase, actors []contracts.WorkflowActor) string {
	switch phase.Status {
	case contracts.WorkflowPhaseRunning, contracts.WorkflowPhaseDone, contracts.WorkflowPhaseFailed:
		return phase.Status
	}
	return PhaseState(actors)
}

// AgentTotal 数全部实例，表外实例也算：投影有界不等于执行有界。
func (v RunView) AgentTotal() int {
	total := 0
	for _, phase := range v.Phases {
		total += len(phase.Actors)
	}
	return total
}

// AgentWorking 数仍在跑或等答案的实例，用于运行中的摘要。
func (v RunView) AgentWorking() int {
	working := 0
	for _, phase := range v.Phases {
		for _, actor := range phase.Actors {
			if actor.Status == contracts.WorkflowActorRunning || actor.Status == contracts.WorkflowActorAsking {
				working++
			}
		}
	}
	return working
}

// PendingQuestions 数仍在等主 Agent 回答的问题。
func (v RunView) PendingQuestions() int {
	count := 0
	for _, question := range v.Questions {
		if question.Status == "pending" || question.Status == "asked" {
			count++
		}
	}
	return count
}

// WaitingHoles 数仍在等补全的留白。
func (v RunView) WaitingHoles() int {
	count := 0
	for _, hole := range v.Holes {
		if hole.Status == contracts.WorkflowHoleWaiting {
			count++
		}
	}
	return count
}

// Active 报告这次运行还能自己往前走。
func (v RunView) Active() bool {
	return IsActive(v.Status)
}
