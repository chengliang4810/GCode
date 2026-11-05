package model

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// ErrStreamInterrupted means the response ended before its protocol completion event.
var ErrStreamInterrupted = errors.New("model stream ended before completion")

var errOutputLimit = errors.New("model output limit reached; incomplete tool calls were not executed")

type streamReadError struct{ cause error }

func (e *streamReadError) Error() string { return e.cause.Error() }
func (e *streamReadError) Unwrap() error { return e.cause }

type streamDecoder interface {
	consume(string) (Delta, error)
	completed() bool
	snapshot() Result
	finish() (Result, error)
}

type streamState struct {
	result         Result
	calls          map[int]*streamCall
	text           strings.Builder
	reasoning      strings.Builder
	complete       bool
	stop           StopReason
	stopErr        error
	inputReported  bool
	outputReported bool
	cacheReported  bool
}

func newStreamState() streamState {
	return streamState{
		result: Result{Message: Message{Role: "assistant"}},
		calls:  make(map[int]*streamCall),
	}
}

func (s *streamState) completed() bool  { return s.complete }
func (s *streamState) snapshot() Result { return s.result }

func (s *streamState) add(delta Delta) {
	// Builders only append: strings already returned by snapshot remain
	// immutable, while each delta still reaches emit immediately.
	if delta.Text != "" {
		s.text.WriteString(delta.Text)
		s.result.Message.Text = s.text.String()
	}
	if delta.Reasoning != "" {
		s.reasoning.WriteString(delta.Reasoning)
		s.result.Message.Reasoning = s.reasoning.String()
	}
}

type streamCall struct {
	Call
	name      strings.Builder
	arguments strings.Builder
}

func newStreamCall(call Call) *streamCall {
	return &streamCall{Call: call}
}

func (c *streamCall) append(name, arguments string) {
	if name != "" {
		if c.name.Len() == 0 {
			c.name.WriteString(c.Name)
		}
		c.name.WriteString(name)
		c.Name = c.name.String()
	}
	if arguments != "" {
		if c.arguments.Len() == 0 {
			c.arguments.WriteString(c.Arguments)
		}
		c.arguments.WriteString(arguments)
		c.Arguments = c.arguments.String()
	}
}

func (s *streamState) recordUsage(input, output, cached *int) {
	if input != nil {
		s.inputReported = true
		s.result.Usage.Input = max(s.result.Usage.Input, *input)
	}
	if output != nil {
		s.outputReported = true
		s.result.Usage.Output = max(s.result.Usage.Output, *output)
	}
	if cached != nil && *cached >= 0 {
		s.cacheReported = true
		s.result.Usage.Cached = max(s.result.Usage.Cached, *cached)
	}
	s.result.Usage.Known = s.inputReported && s.outputReported
	s.result.Usage.OutputKnown = s.outputReported
	s.result.Usage.CacheKnown = s.inputReported && s.cacheReported && s.result.Usage.Cached <= s.result.Usage.Input
}

func (s *streamState) finish() (Result, error) {
	if !s.complete {
		return s.result, ErrStreamInterrupted
	}
	if s.stopErr != nil {
		return s.result, s.stopErr
	}
	if s.stop != StopComplete && s.stop != StopTools {
		return s.result, errors.New("model stream omitted its finish reason")
	}
	// Validate the entire batch before publishing any executable calls.
	calls := make([]Call, 0, len(s.calls))
	seen := make(map[string]bool, len(s.calls))
	for _, index := range sortedKeys(s.calls) {
		call := s.calls[index].Call
		if call.Arguments == "" {
			call.Arguments = "{}"
		}
		var args map[string]json.RawMessage
		if call.ID == "" || call.Name == "" ||
			json.Unmarshal([]byte(call.Arguments), &args) != nil || args == nil {
			return s.result, errors.New("model returned an incomplete tool call")
		}
		if seen[call.ID] {
			return s.result, errors.New("model returned duplicate tool call IDs")
		}
		seen[call.ID] = true
		calls = append(calls, call)
	}
	s.result.Stop = s.stop
	switch {
	case len(calls) > 0:
		s.result.Stop = StopTools
	case s.stop == StopTools:
		return s.result, errors.New("model omitted calls for a tool stop")
	}
	s.result.Message.Calls = calls
	return s.result, nil
}

// decodeStream owns SSE framing only. Adapters own all provider event decoding.
func decodeStream(reader io.Reader, decoder streamDecoder, emit func(Delta) error) (result Result, err error) {
	started := time.Now()
	var firstOutput time.Time
	defer func() {
		if !firstOutput.IsZero() {
			result.DecodeDuration = time.Since(firstOutput)
			result.FirstTokenObserved = true
			result.FirstTokenLatency = firstOutput.Sub(started)
		}
	}()
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 8<<20)
	var data []string
	process := func() error {
		if len(data) == 0 {
			return nil
		}
		payload := strings.Join(data, "\n")
		data = nil
		delta, err := decoder.consume(payload)
		if err != nil {
			return err
		}
		if firstOutput.IsZero() && (delta.Output || delta.Text != "" || delta.Reasoning != "") {
			firstOutput = time.Now()
		}
		if emit != nil && (delta.Output || delta.Text != "" || delta.Reasoning != "") {
			return emit(delta)
		}
		return nil
	}
	finish := func() (Result, error) {
		before := decoder.snapshot().Message.Text
		result, err := decoder.finish()
		if err == nil && len(result.Message.Text) > len(before) && emit != nil {
			err = emit(Delta{Text: strings.TrimPrefix(result.Message.Text, before)})
		}
		if err != nil {
			result.Message.Calls = nil
			result.Message.Continuation = nil
		}
		return result, err
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := process(); err != nil {
				return decoder.snapshot(), err
			}
			if decoder.completed() {
				return finish()
			}
		} else if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return decoder.snapshot(), fmt.Errorf("%w: %v", ErrStreamInterrupted, err)
		}
		return decoder.snapshot(), &streamReadError{cause: err}
	}
	if err := process(); err != nil {
		return decoder.snapshot(), err
	}
	return finish()
}

func sortedKeys[V any](values map[int]V) []int {
	keys := make([]int, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Ints(keys)
	return keys
}
