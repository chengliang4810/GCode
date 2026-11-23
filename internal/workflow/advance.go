package workflow

// 阶段推进的纯规则。推进的事实只有两类：声明顺序和边的条件。
// 声明顺序是默认走向；条件可以让某个阶段被跳过，或者让运行回到前面的阶段。

import (
	"strconv"
	"strings"

	"gcode/internal/contracts"
)

// PhaseRoundLimit 限制一个阶段最多被进入几轮。回边不会让运行永远转圈。
const PhaseRoundLimit = 5

// 推进决定的种类。
const (
	AdvanceEnter  = "enter"
	AdvanceSkip   = "skip"
	AdvanceFinish = "finish"
)

// AdvanceDecision 是推进器给出的下一步。
type AdvanceDecision struct {
	Kind    string
	PhaseID string
	// Reason 说明为什么跳过这一站。跳过是事实，不是沉默。
	Reason string
	// ResetFrom 指出回边的落点：从这里起的阶段重新变成待执行。
	ResetFrom string
}

// Facts 把一次运行的事实摊成条件能读的名字空间。名字空间都是点分段的，
// 没有点的裸词在条件里是字面量。
func Facts(run contracts.WorkflowRun) map[string]string {
	facts := map[string]string{
		"run.status": run.Status,
		"run.name":   run.Name,
		"run.result": run.Result,
	}
	for _, phase := range run.Phases {
		facts["phase."+phase.ID+".status"] = phase.Status
		facts["phase."+phase.ID+".rounds"] = strconv.Itoa(phase.Rounds)
	}
	for _, actor := range run.Actors {
		facts["actor."+actor.ID+".status"] = actor.Status
		facts["actor."+actor.ID+".result"] = actor.Result
	}
	for _, hole := range run.Holes {
		facts["hole."+hole.ID] = hole.Text
		facts["hole."+hole.ID+".status"] = hole.Status
	}
	for _, question := range run.Questions {
		facts["question."+question.ID] = question.Answer
		facts["question."+question.ID+".status"] = question.Status
	}
	for _, argument := range run.Args {
		facts["arg."+argument.Name] = argument.Text
	}
	return facts
}

// Advance 决定运行下一步做什么。调用方按决定推进，并把跳过的阶段记成事实。
func Advance(run contracts.WorkflowRun) AdvanceDecision {
	if len(run.Phases) == 0 {
		return AdvanceDecision{Kind: AdvanceFinish}
	}
	current, entered := currentPhase(run)
	if !entered {
		// 运行总是从第一个声明的阶段开始。
		return AdvanceDecision{Kind: AdvanceEnter, PhaseID: run.Phases[0].ID}
	}
	facts := Facts(run)
	for index := current.Index + 1; index < len(run.Phases); index++ {
		candidate := run.Phases[index]
		if candidate.Status != contracts.WorkflowPhasePending {
			continue
		}
		if reason, blocked := edgeBlocked(run, current, candidate, facts); blocked {
			return AdvanceDecision{Kind: AdvanceSkip, PhaseID: candidate.ID, Reason: reason}
		}
		return AdvanceDecision{Kind: AdvanceEnter, PhaseID: candidate.ID}
	}
	// 没有后面的阶段了，看看有没有成立的回边。
	if phaseID, ok := loopTarget(run, current, facts); ok {
		return AdvanceDecision{Kind: AdvanceEnter, PhaseID: phaseID, ResetFrom: phaseID}
	}
	return AdvanceDecision{Kind: AdvanceFinish}
}

// currentPhase 找出运行当前所在的阶段：最近进入的那一站。
func currentPhase(run contracts.WorkflowRun) (contracts.WorkflowPhase, bool) {
	var current contracts.WorkflowPhase
	found := false
	for _, phase := range run.Phases {
		if phase.EnteredAt == nil {
			continue
		}
		if !found || phase.EnteredAt.After(*current.EnteredAt) {
			current, found = phase, true
		}
	}
	return current, found
}

// edgeBlocked 检查从当前阶段指向候选阶段的边。有一条边成立就不跳过，
// 有一条边明确不成立就跳过：条件是许可，不是禁令。
func edgeBlocked(run contracts.WorkflowRun, current, candidate contracts.WorkflowPhase, facts map[string]string) (string, bool) {
	for _, edge := range run.Edges {
		if edge.From != current.ID || edge.To != candidate.ID {
			continue
		}
		if edge.When == "" {
			return "", false
		}
		condition, err := ParseCondition(edge.When)
		if err != nil {
			// 条件读不懂就不替它做决定：宁可走，也不假装知道。
			return "", false
		}
		if !condition.Eval(facts) {
			return "the edge to " + candidate.ID + " did not hold", true
		}
		return "", false
	}
	return "", false
}

// loopTarget 找出成立的回边目标。回边只能回到已经走过的阶段，
// 而且那一站还没到轮次上限。
func loopTarget(run contracts.WorkflowRun, current contracts.WorkflowPhase, facts map[string]string) (string, bool) {
	for _, edge := range run.Edges {
		if edge.From != current.ID || edge.Kind != "loop" {
			continue
		}
		target, ok := phaseByID(run, edge.To)
		if !ok || target.Index >= current.Index {
			continue
		}
		if target.Rounds >= PhaseRoundLimit {
			continue
		}
		if edge.When != "" {
			condition, err := ParseCondition(edge.When)
			if err != nil || !condition.Eval(facts) {
				continue
			}
		}
		return edge.To, true
	}
	return "", false
}

// phaseByID 按身份取阶段。
func phaseByID(run contracts.WorkflowRun, phaseID string) (contracts.WorkflowPhase, bool) {
	for _, phase := range run.Phases {
		if phase.ID == phaseID {
			return phase, true
		}
	}
	return contracts.WorkflowPhase{}, false
}

// PhaseSummary 是脚本的规模说明。确认窗和工具结果都用它说话。
type PhaseSummary struct {
	Phases int
	Actors int
}

// SummarizeProgram 数脚本声明的阶段与实例。
func SummarizeProgram(program contracts.WorkflowProgram) PhaseSummary {
	summary := PhaseSummary{Phases: len(program.Phases)}
	for _, phase := range program.Phases {
		summary.Actors += len(phase.Instances)
	}
	return summary
}

// PhaseNames 列出阶段名。界面按声明顺序显示，不按状态排序。
func PhaseNames(program contracts.WorkflowProgram) []string {
	names := make([]string, 0, len(program.Phases))
	for _, phase := range program.Phases {
		name := phase.Name
		if strings.TrimSpace(name) == "" {
			name = phase.ID
		}
		names = append(names, name)
	}
	return names
}
