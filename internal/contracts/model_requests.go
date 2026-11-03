package contracts

import (
	"time"

	"gcode/internal/model"
)

// ModelRequest is durable accounting for one model attempt, independent of
// transcript folding, compaction and the currently selected provider.
// TurnID is empty for conversation maintenance such as automatic naming.
type ModelRequest struct {
	ID             string        `json:"id"`
	SessionID      string        `json:"sessionId"`
	TurnID         string        `json:"turnId,omitempty"`
	ProviderID     string        `json:"providerId"`
	ProviderName   string        `json:"providerName"`
	ModelID        string        `json:"modelId"`
	Usage          model.Usage   `json:"usage"`
	DecodeDuration time.Duration `json:"decodeDuration"`
}

// ModelRequestTotals only pairs output tokens with their own measured decode
// duration. Averaging individual TPS values would overweight short requests.
type ModelRequestTotals struct {
	Requests, UnknownUsage, DecodeRequests int
	Input, Output, Cached                  int64
	DecodeTokens                           int64
	DecodeDuration                         time.Duration
	Unrecorded                             bool
	CacheRequests                          int
	CacheInputTokens, CacheReadTokens      int64
}

func (s *ModelRequestTotals) Add(request ModelRequest) {
	s.Requests++
	if !request.Usage.Known {
		s.UnknownUsage++
	}
	s.Input += int64(max(0, request.Usage.Input))
	s.Output += int64(max(0, request.Usage.Output))
	s.Cached += int64(max(0, request.Usage.Cached))
	if request.Usage.CacheKnown && request.Usage.Input >= 0 &&
		request.Usage.Cached >= 0 && request.Usage.Cached <= request.Usage.Input {
		s.CacheRequests++
		s.CacheInputTokens += int64(request.Usage.Input)
		s.CacheReadTokens += int64(request.Usage.Cached)
	}
	if request.Usage.OutputKnown && request.DecodeDuration > 0 {
		s.DecodeRequests++
		s.DecodeTokens += int64(max(0, request.Usage.Output))
		s.DecodeDuration += request.DecodeDuration
	}
}

func (s ModelRequestTotals) CacheHitFraction() (float64, bool) {
	if s.CacheInputTokens <= 0 {
		return 0, false
	}
	return float64(s.CacheReadTokens) / float64(s.CacheInputTokens), true
}

func (s ModelRequestTotals) TokensPerSecond() (float64, bool) {
	if s.DecodeDuration <= 0 {
		return 0, false
	}
	return float64(s.DecodeTokens) / s.DecodeDuration.Seconds(), true
}
