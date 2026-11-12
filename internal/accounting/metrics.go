// Package accounting computes durable execution metrics without depending on
// the desktop UI, persistence implementation, or a live core instance.
package accounting

import (
	"fmt"
	"sort"
	"time"

	"gcode/internal/contracts"
)

// Own computes the metrics owned by one session. An empty turnID selects the
// whole session, including maintenance requests; a non-empty turnID selects
// only that turn. Runs are deliberately filtered by their own SessionID so
// copied transcript text cannot charge the session that inherited it.
func Own(
	sessionID, turnID string,
	messages []contracts.Message,
	requests []contracts.ModelRequest,
	runs []contracts.Run,
) contracts.ExecutionMetrics {
	var metrics contracts.ExecutionMetrics
	modelIndexes := make(map[string]int)
	recordedTurns := make(map[string]bool)
	eligibleTurns := make(map[string]bool)

	for _, request := range requests {
		if request.SessionID != sessionID || turnID != "" && request.TurnID != turnID {
			continue
		}
		metrics.Totals.Add(request)
		if request.DecodeDuration > 0 {
			metrics.GenerationDuration += request.DecodeDuration
		}
		recordedTurns[request.TurnID] = true
		eligibleTurns[request.TurnID] = true

		key := request.ProviderID + "\x00" + request.ProviderName + "\x00" + request.ModelID
		index, exists := modelIndexes[key]
		if !exists {
			index = len(metrics.Models)
			modelIndexes[key] = index
			metrics.Models = append(metrics.Models, contracts.ModelMetrics{
				ProviderID: request.ProviderID,
				Provider:   request.ProviderName,
				ModelID:    request.ModelID,
			})
		}
		metrics.Models[index].Totals.Add(request)
	}

	turns := make(map[string]bool)
	ownedRuns := make([]contracts.Run, 0, len(runs))
	for _, run := range runs {
		if run.SessionID != sessionID || turnID != "" && run.ID != turnID {
			continue
		}
		ownedRuns = append(ownedRuns, run)
		eligibleTurns[run.ID] = true
		if span, ok := runSpan(run); ok {
			metrics.Spans = append(metrics.Spans, span)
		}
	}
	for _, request := range requests {
		if request.SessionID == sessionID && (turnID == "" || request.TurnID == turnID) && request.TurnID != "" {
			turns[request.TurnID] = true
		}
	}
	metrics.Turns = len(turns)
	setWallFields(&metrics, metrics.Spans)

	selectedMessages := selectMessages(sessionID, turnID, eligibleTurns, messages)
	metrics.ToolDuration = toolDuration(selectedMessages)
	metrics.Activity = activityFor(ownedRuns, selectedMessages, eligibleTurns)

	for _, run := range ownedRuns {
		if recordedTurns[run.ID] {
			continue
		}
		for _, message := range selectedMessages {
			if message.TurnID == run.ID && message.Role == "assistant" && message.ModelID != "" {
				metrics.Totals.Unrecorded = true
				break
			}
		}
	}
	return metrics
}

// Merge combines an own view with delegated child views. Model/request and
// measured duration totals are additive; wall time is a union of intervals so
// concurrent work is counted once. Turns and Activity belong to base's view
// and therefore are intentionally retained from base.
func Merge(base contracts.ExecutionMetrics, children ...contracts.ExecutionMetrics) contracts.ExecutionMetrics {
	out := base.Clone()
	if len(children) == 0 {
		return out
	}

	modelIndexes := make(map[string]int, len(out.Models))
	for i, model := range out.Models {
		modelIndexes[modelKey(model.ProviderID, model.Provider, model.ModelID)] = i
	}
	wallInputs := make([]contracts.ExecutionMetrics, 0, len(children)+1)
	wallInputs = append(wallInputs, base)
	for _, child := range children {
		out.Totals.Merge(child.Totals)
		out.GenerationDuration += child.GenerationDuration
		out.ToolDuration += child.ToolDuration
		for _, model := range child.Models {
			key := modelKey(model.ProviderID, model.Provider, model.ModelID)
			index, exists := modelIndexes[key]
			if !exists {
				index = len(out.Models)
				modelIndexes[key] = index
				out.Models = append(out.Models, model)
				continue
			}
			out.Models[index].Totals.Merge(model.Totals)
		}
		wallInputs = append(wallInputs, child)
	}
	mergeWallFields(&out, wallInputs...)
	return out
}

func modelKey(providerID, provider, modelID string) string {
	return providerID + "\x00" + provider + "\x00" + modelID
}

func selectMessages(sessionID, turnID string, eligibleTurns map[string]bool, messages []contracts.Message) []contracts.Message {
	selected := make([]contracts.Message, 0, len(messages))
	for _, message := range messages {
		if message.SessionID != "" && message.SessionID != sessionID {
			continue
		}
		if turnID != "" {
			if eligibleTurns[turnID] && message.TurnID == turnID {
				selected = append(selected, message)
			}
			continue
		}
		if eligibleTurns[message.TurnID] {
			selected = append(selected, message)
		}
	}
	return selected
}

func runSpan(run contracts.Run) (contracts.ExecutionSpan, bool) {
	if run.StartedAt.IsZero() {
		return contracts.ExecutionSpan{}, false
	}
	span := contracts.ExecutionSpan{Start: run.StartedAt}
	switch {
	case run.FinishedAt != nil:
		span.End = *run.FinishedAt
	case run.Status == "running":
		// End remains zero to preserve a live tail for Elapsed.
	default:
		// Preserve the existing wall-duration convention for historical rows
		// that have a duration but no explicit finish timestamp.
		span.End = run.StartedAt.Add(time.Duration(run.DurationMS) * time.Millisecond)
	}
	if !span.End.IsZero() && span.End.Before(span.Start) {
		span.End = span.Start
	}
	return span, true
}

func setWallFields(metrics *contracts.ExecutionMetrics, spans []contracts.ExecutionSpan) {
	metrics.Spans = unionSpans(spans)
	metrics.WallKnown = len(metrics.Spans) > 0
	metrics.WallDuration = 0
	metrics.RunningSince = time.Time{}
	for _, span := range metrics.Spans {
		if span.End.IsZero() {
			metrics.RunningSince = span.Start
			continue
		}
		if duration := span.End.Sub(span.Start); duration > 0 {
			metrics.WallDuration += duration
		}
	}
}

func mergeWallFields(out *contracts.ExecutionMetrics, metrics ...contracts.ExecutionMetrics) {
	spans := make([]contracts.ExecutionSpan, 0)
	var scalarClosed time.Duration
	known := false
	for _, metrics := range metrics {
		if len(metrics.Spans) > 0 {
			spans = append(spans, metrics.Spans...)
			known = true
			continue
		}
		if !metrics.WallKnown {
			continue
		}
		known = true
		if metrics.WallDuration > 0 {
			scalarClosed += metrics.WallDuration
		}
		if !metrics.RunningSince.IsZero() {
			spans = append(spans, contracts.ExecutionSpan{Start: metrics.RunningSince})
		}
	}

	setWallFields(out, spans)
	out.WallKnown = known || out.WallKnown
	out.WallDuration += scalarClosed
}

func unionSpans(spans []contracts.ExecutionSpan) []contracts.ExecutionSpan {
	valid := make([]contracts.ExecutionSpan, 0, len(spans))
	for _, span := range spans {
		if span.Start.IsZero() {
			continue
		}
		if !span.End.IsZero() && span.End.Before(span.Start) {
			span.End = span.Start
		}
		valid = append(valid, span)
	}
	if len(valid) == 0 {
		return nil
	}
	sort.SliceStable(valid, func(i, j int) bool {
		if valid[i].Start.Equal(valid[j].Start) {
			if valid[i].End.IsZero() != valid[j].End.IsZero() {
				return valid[i].End.IsZero()
			}
			return valid[i].End.Before(valid[j].End)
		}
		return valid[i].Start.Before(valid[j].Start)
	})

	merged := make([]contracts.ExecutionSpan, 0, len(valid))
	for _, current := range valid {
		if len(merged) == 0 {
			merged = append(merged, current)
			continue
		}
		last := &merged[len(merged)-1]
		if last.End.IsZero() {
			// An open interval covers every later interval.
			continue
		}
		if current.Start.After(last.End) {
			merged = append(merged, current)
			continue
		}
		if current.End.IsZero() {
			last.End = time.Time{}
			continue
		}
		if current.End.After(last.End) {
			last.End = current.End
		}
	}
	return merged
}

type toolTiming struct {
	start    time.Time
	finished time.Time
	closed   bool
}

func toolDuration(messages []contracts.Message) time.Duration {
	tools := make(map[string]toolTiming)
	for messageIndex, message := range messages {
		for blockIndex, block := range message.Blocks {
			if block.Tool == nil {
				continue
			}
			tool := block.Tool
			key := tool.ID
			if key == "" {
				key = fmt.Sprintf("message:%s:%d:%d", message.ID, messageIndex, blockIndex)
			}
			current := tools[key]
			if current.start.IsZero() || !tool.StartedAt.IsZero() && tool.StartedAt.Before(current.start) {
				if !tool.StartedAt.IsZero() {
					current.start = tool.StartedAt
				}
			}
			if tool.FinishedAt != nil {
				current.finished = *tool.FinishedAt
				current.closed = true
			}
			tools[key] = current
		}
	}
	var total time.Duration
	for _, tool := range tools {
		if !tool.closed || tool.start.IsZero() {
			continue
		}
		if duration := tool.finished.Sub(tool.start); duration > 0 {
			total += duration
		}
	}
	return total
}

func activityFor(runs []contracts.Run, messages []contracts.Message, eligibleTurns map[string]bool) contracts.ActivitySummary {
	messagesByTurn := make(map[string][]contracts.Message)
	for _, message := range messages {
		messagesByTurn[message.TurnID] = append(messagesByTurn[message.TurnID], message)
	}
	var summary contracts.ActivitySummary
	accountedTurns := make(map[string]bool)
	for _, run := range runs {
		accountedTurns[run.ID] = true
		addActivity(&summary, contracts.SummarizeActivity(contracts.TurnActivity(messagesByTurn[run.ID], run.ID)))
	}
	for turnID := range eligibleTurns {
		if accountedTurns[turnID] {
			continue
		}
		addActivity(&summary, contracts.SummarizeActivity(contracts.TurnActivity(messagesByTurn[turnID], turnID)))
	}
	return summary
}

func addActivity(dst *contracts.ActivitySummary, src contracts.ActivitySummary) {
	dst.ToolCalls += src.ToolCalls
	dst.Commands += src.Commands
	dst.EditedFiles += src.EditedFiles
	dst.Reads += src.Reads
	dst.Searches += src.Searches
	dst.Fetches += src.Fetches
	dst.Todos += src.Todos
	dst.Other += src.Other
	dst.Failed += src.Failed
	dst.Thoughts += src.Thoughts
	dst.Notes += src.Notes
	dst.Subagents += src.Subagents
}
