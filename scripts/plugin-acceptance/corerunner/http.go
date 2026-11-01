package corerunner

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"hash"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type wireCall struct {
	Model                string      `json:"model"`
	MaxOutputTokens      int         `json:"maxOutputTokens,omitempty"`
	ThinkingBudgetTokens int         `json:"thinkingBudgetTokens,omitempty"`
	ThinkingType         string      `json:"thinkingType,omitempty"`
	ReasoningEffort      string      `json:"reasoningEffort,omitempty"`
	Path                 string      `json:"path"`
	Status               int         `json:"status"`
	RequestSHA256        string      `json:"requestSha256"`
	ResponseSHA256       string      `json:"responseSha256"`
	ResponseID           string      `json:"responseId,omitempty"`
	ResponseBody         string      `json:"responseBody,omitempty"`
	BodyTruncated        bool        `json:"bodyTruncated,omitempty"`
	Frames               []wireFrame `json:"frames,omitempty"`
	StartedAt            time.Time   `json:"startedAt"`
	FinishedAt           time.Time   `json:"finishedAt"`
	Error                string      `json:"error,omitempty"`
}

type wireFrame struct {
	ReceivedAt   time.Time `json:"receivedAt"`
	Event        string    `json:"event,omitempty"`
	Type         string    `json:"type,omitempty"`
	DeltaType    string    `json:"deltaType,omitempty"`
	StopReason   string    `json:"stopReason,omitempty"`
	InputTokens  *int      `json:"inputTokens,omitempty"`
	OutputTokens *int      `json:"outputTokens,omitempty"`
	Model        string    `json:"model,omitempty"`
}

type wireRecorder struct {
	base  http.RoundTripper
	mu    sync.Mutex
	calls []*wireCall
}

func (w *wireRecorder) RoundTrip(request *http.Request) (*http.Response, error) {
	call := &wireCall{Path: request.URL.EscapedPath(), StartedAt: time.Now().UTC()}
	if request.GetBody != nil {
		body, err := request.GetBody()
		if err == nil {
			data, _ := io.ReadAll(io.LimitReader(body, 8<<20))
			_ = body.Close()
			sum := sha256.Sum256(data)
			call.RequestSHA256 = hex.EncodeToString(sum[:])
			var payload struct {
				Model               string `json:"model"`
				MaxTokens           int    `json:"max_tokens"`
				MaxOutputTokens     int    `json:"max_output_tokens"`
				MaxCompletionTokens int    `json:"max_completion_tokens"`
				ReasoningEffort     string `json:"reasoning_effort"`
				Reasoning           struct {
					Effort string `json:"effort"`
				} `json:"reasoning"`
				OutputConfig struct {
					Effort string `json:"effort"`
				} `json:"output_config"`
				Thinking struct {
					Type         string `json:"type"`
					BudgetTokens int    `json:"budget_tokens"`
				} `json:"thinking"`
			}
			_ = json.Unmarshal(data, &payload)
			call.Model = payload.Model
			call.MaxOutputTokens = max(payload.MaxTokens, payload.MaxOutputTokens, payload.MaxCompletionTokens)
			call.ThinkingBudgetTokens = payload.Thinking.BudgetTokens
			call.ThinkingType = payload.Thinking.Type
			call.ReasoningEffort = payload.ReasoningEffort
			if payload.Reasoning.Effort != "" {
				call.ReasoningEffort = payload.Reasoning.Effort
			}
			if payload.OutputConfig.Effort != "" {
				call.ReasoningEffort = payload.OutputConfig.Effort
			}
		}
	}
	w.mu.Lock()
	w.calls = append(w.calls, call)
	w.mu.Unlock()
	response, err := w.base.RoundTrip(request)
	w.mu.Lock()
	defer w.mu.Unlock()
	if err != nil {
		call.Error = err.Error()
		call.FinishedAt = time.Now().UTC()
		return nil, err
	}
	call.Status = response.StatusCode
	call.ResponseID = response.Header.Get("x-request-id")
	if call.ResponseID == "" {
		call.ResponseID = response.Header.Get("request-id")
	}
	response.Body = &recordedBody{ReadCloser: response.Body, hash: sha256.New(), done: func(digest, body string, truncated bool, frames []wireFrame) {
		w.mu.Lock()
		defer w.mu.Unlock()
		call.ResponseSHA256 = digest
		call.ResponseBody, call.BodyTruncated = body, truncated
		call.Frames = frames
		call.FinishedAt = time.Now().UTC()
	}}
	return response, nil
}

func (w *wireRecorder) snapshot() []wireCall {
	w.mu.Lock()
	defer w.mu.Unlock()
	result := make([]wireCall, 0, len(w.calls))
	for _, call := range w.calls {
		result = append(result, *call)
	}
	return result
}

type recordedBody struct {
	io.ReadCloser
	hash      hash.Hash
	body      bytes.Buffer
	truncated bool
	pending   string
	event     string
	frames    []wireFrame
	once      sync.Once
	done      func(string, string, bool, []wireFrame)
}

func (b *recordedBody) Read(data []byte) (int, error) {
	n, err := b.ReadCloser.Read(data)
	if n > 0 {
		b.recordFrames(data[:n])
		_, _ = b.hash.Write(data[:n])
		remaining := (2 << 20) - b.body.Len()
		if remaining < n {
			b.truncated = true
		}
		if remaining > 0 {
			_, _ = b.body.Write(data[:min(n, remaining)])
		}
	}
	if err == io.EOF {
		b.finish()
	}
	return n, err
}
func (b *recordedBody) Close() error { err := b.ReadCloser.Close(); b.finish(); return err }
func (b *recordedBody) finish() {
	b.once.Do(func() { b.done(hex.EncodeToString(b.hash.Sum(nil)), b.body.String(), b.truncated, b.frames) })
}

func (b *recordedBody) recordFrames(data []byte) {
	b.pending += string(data)
	for {
		line, rest, ok := strings.Cut(b.pending, "\n")
		if !ok {
			if len(b.pending) > 256<<10 {
				b.pending = ""
			}
			return
		}
		b.pending = rest
		line = strings.TrimSuffix(line, "\r")
		if strings.HasPrefix(line, "event:") {
			b.event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var payload struct {
			Type  string `json:"type"`
			Delta struct {
				Type       string `json:"type"`
				StopReason string `json:"stop_reason"`
			} `json:"delta"`
			Usage struct {
				Input  *int `json:"input_tokens"`
				Output *int `json:"output_tokens"`
			} `json:"usage"`
			Message struct {
				Model string `json:"model"`
				Usage struct {
					Input  *int `json:"input_tokens"`
					Output *int `json:"output_tokens"`
				} `json:"usage"`
			} `json:"message"`
		}
		if json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &payload) != nil {
			continue
		}
		frame := wireFrame{ReceivedAt: time.Now().UTC(), Event: b.event, Type: payload.Type,
			DeltaType: payload.Delta.Type, StopReason: payload.Delta.StopReason, InputTokens: payload.Usage.Input, OutputTokens: payload.Usage.Output,
			Model: payload.Message.Model}
		if frame.InputTokens == nil {
			frame.InputTokens = payload.Message.Usage.Input
		}
		if frame.OutputTokens == nil {
			frame.OutputTokens = payload.Message.Usage.Output
		}
		if len(b.frames) < 10000 {
			b.frames = append(b.frames, frame)
		} else if frame.Type == "message_delta" || frame.Type == "message_stop" || frame.StopReason != "" || frame.OutputTokens != nil {
			// A long token stream may fill the diagnostic prefix before the
			// terminating usage/stop frames arrive. Keep a bounded terminal tail.
			if len(b.frames) < 10016 {
				b.frames = append(b.frames, frame)
			} else {
				copy(b.frames[10000:], b.frames[10001:])
				b.frames[len(b.frames)-1] = frame
			}
		}
	}
}
