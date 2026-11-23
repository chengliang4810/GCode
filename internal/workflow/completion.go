package workflow

import (
	"time"

	"gcode/internal/contracts"
)

// 完成卡只服务「成功且有交付」的运行。失败、停止、问题或留白升级都不是交付：
// 那些运行没有完成卡，也不该被画成已经交付了什么。

// TokenTotals 是一次运行里子代理用掉的 token。Known 为假表示没有任何一次请求
// 报回可用的计数——那时界面显示 —，不拿 0 冒充已知。
type TokenTotals struct {
	Input, Output int64
	Known         bool
}

// Total is the token count a completion card shows.
func (t TokenTotals) Total() int64 { return t.Input + t.Output }

// CompletionView is what a finished run delivers, ready to draw.
type CompletionView struct {
	RunID string
	Name  string
	// Primary is the artifact the preview shows. It is absent when there is no
	// explicit primary and more than one delivery.
	Primary    contracts.WorkflowArtifact
	HasPrimary bool
	// Index is the rest of the deliveries, bounded: six rows, or five and a
	// count when there are more.
	Index ArtifactSlice
	// Delivered counts every delivery, including the ones off the index.
	Delivered int
	// Duration is how long the run took. A missing finished time leaves it
	// zero, which the card shows as an unknown.
	Duration time.Duration
	// Phases counts the phases the run actually entered, not the declared ones.
	Phases int
	// Agents counts the instances the run created.
	Agents int
	Tokens TokenTotals
}

// Completion reports whether a run has a completion card, and what it says.
// Only a completed run that really published something has one.
func Completion(run contracts.WorkflowRun, tokens TokenTotals) (CompletionView, bool) {
	if run.Status != contracts.WorkflowStatusCompleted {
		return CompletionView{}, false
	}
	if len(run.Artifacts) == 0 {
		return CompletionView{}, false
	}
	ordered := orderArtifacts(run.Artifacts)
	view := CompletionView{RunID: run.ID, Name: run.Name, Delivered: len(ordered), Tokens: tokens}
	if primary, ok := PrimaryArtifact(ordered); ok {
		view.Primary, view.HasPrimary = primary, true
	}
	rest := ordered
	if view.HasPrimary {
		rest = make([]contracts.WorkflowArtifact, 0, len(ordered)-1)
		for _, artifact := range ordered {
			if artifact.ID == view.Primary.ID {
				continue
			}
			rest = append(rest, artifact)
		}
	}
	view.Index = completionIndex(rest)
	if run.FinishedAt != nil && !run.StartedAt.IsZero() && !run.FinishedAt.Before(run.StartedAt) {
		view.Duration = run.FinishedAt.Sub(run.StartedAt)
	}
	for _, phase := range run.Phases {
		if phase.EnteredAt != nil {
			view.Phases++
		}
	}
	view.Agents = len(run.Actors)
	return view, true
}

// completionIndex bounds the delivery index: at most six rows, which becomes
// five rows plus a count when it would overflow.
func completionIndex(artifacts []contracts.WorkflowArtifact) ArtifactSlice {
	if len(artifacts) <= CompletionIndexLimit {
		return ArtifactSlice{Shown: artifacts}
	}
	return boundArtifacts(artifacts, CompletionOverflowRows)
}
