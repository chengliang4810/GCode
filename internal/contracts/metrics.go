package contracts

import (
	"slices"
	"time"
)

// ExecutionSpan records one run interval. A zero End marks work that is still
// running; finished intervals retain their recorded end time.
type ExecutionSpan struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// ModelMetrics groups request accounting by the provider identity and model
// selected for the request. Provider is the display name captured at request
// time; ProviderID is the stable provider identity.
type ModelMetrics struct {
	ProviderID string             `json:"providerId"`
	Provider   string             `json:"provider"`
	ModelID    string             `json:"modelId"`
	Totals     ModelRequestTotals `json:"totals"`
}

// ExecutionMetrics is the durable, framework-independent accounting result
// for one conversation or one turn. WallDuration excludes an open tail;
// RunningSince identifies that tail so Elapsed can include it at read time.
type ExecutionMetrics struct {
	Totals             ModelRequestTotals `json:"totals"`
	Models             []ModelMetrics     `json:"models,omitempty"`
	Turns              int                `json:"turns"`
	GenerationDuration time.Duration      `json:"generationDuration"`
	ToolDuration       time.Duration      `json:"toolDuration"`
	WallKnown          bool               `json:"wallKnown"`
	WallDuration       time.Duration      `json:"wallDuration"`
	RunningSince       time.Time          `json:"runningSince"`
	Spans              []ExecutionSpan    `json:"spans,omitempty"`
	Activity           ActivitySummary    `json:"activity"`
}

// ConversationMetrics keeps the aggregate view together with the local view
// used to build it. Turns contains delegated turn views; OwnTurns contains the
// current conversation's own turn views so a refresh can preserve them while
// child sessions are loaded independently.
type ConversationMetrics struct {
	Total    ExecutionMetrics            `json:"total"`
	Own      ExecutionMetrics            `json:"own"`
	Turns    map[string]ExecutionMetrics `json:"turns,omitempty"`
	OwnTurns map[string]ExecutionMetrics `json:"ownTurns,omitempty"`
}

// Merge adds every durable ModelRequestTotals field. Boolean completeness is
// a lower-bound property, so any unrecorded input keeps the merged result
// unrecorded.
func (s *ModelRequestTotals) Merge(other ModelRequestTotals) {
	s.Requests += other.Requests
	s.UnknownUsage += other.UnknownUsage
	s.DecodeRequests += other.DecodeRequests
	s.Input += other.Input
	s.Output += other.Output
	s.Cached += other.Cached
	s.DecodeTokens += other.DecodeTokens
	s.DecodeDuration += other.DecodeDuration
	s.Unrecorded = s.Unrecorded || other.Unrecorded
	s.CacheRequests += other.CacheRequests
	s.CacheInputTokens += other.CacheInputTokens
	s.CacheReadTokens += other.CacheReadTokens
	s.TimedRequests += other.TimedRequests
	s.FirstTokenRequests += other.FirstTokenRequests
	s.RequestDuration += other.RequestDuration
	s.FirstTokenLatency += other.FirstTokenLatency
}

// Clone returns a copy whose slices and maps do not alias the source.
func (m ExecutionMetrics) Clone() ExecutionMetrics {
	m.Models = slices.Clone(m.Models)
	m.Spans = slices.Clone(m.Spans)
	return m
}

// Elapsed returns the known closed wall time plus the currently running tail.
// A time before RunningSince contributes zero rather than a negative duration.
func (m ExecutionMetrics) Elapsed(now time.Time) (time.Duration, bool) {
	if !m.WallKnown {
		return 0, false
	}
	duration := m.WallDuration
	if duration < 0 {
		duration = 0
	}
	if !m.RunningSince.IsZero() {
		if running := now.Sub(m.RunningSince); running > 0 {
			duration += running
		}
	}
	return duration, true
}

// Clone returns an independent conversation metrics snapshot.
func (m ConversationMetrics) Clone() ConversationMetrics {
	m.Total = m.Total.Clone()
	m.Own = m.Own.Clone()
	if m.Turns != nil {
		cloned := make(map[string]ExecutionMetrics, len(m.Turns))
		for id, metrics := range m.Turns {
			cloned[id] = metrics.Clone()
		}
		m.Turns = cloned
	}
	if m.OwnTurns != nil {
		cloned := make(map[string]ExecutionMetrics, len(m.OwnTurns))
		for id, metrics := range m.OwnTurns {
			cloned[id] = metrics.Clone()
		}
		m.OwnTurns = cloned
	}
	return m
}
