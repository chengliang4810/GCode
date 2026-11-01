package corerunner

import (
	"crypto/sha256"
	"io"
	"net/http"
	"strings"
	"testing"
)

type testTransport func(*http.Request) (*http.Response, error)

func (f testTransport) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestWireRecorderCapturesActualOutputBudgetAndEffort(t *testing.T) {
	request, err := http.NewRequest(http.MethodPost, "http://fixture.invalid/v1/messages",
		strings.NewReader(`{"model":"selected","max_tokens":32768,"thinking":{"type":"adaptive"},"output_config":{"effort":"max"}}`))
	if err != nil {
		t.Fatal(err)
	}
	wire := &wireRecorder{base: testTransport(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("data: {\"type\":\"message_start\",\"message\":{\"model\":\"actual-model\"}}\n\n"))}, nil
	})}
	response, err := wire.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	calls := wire.snapshot()
	if len(calls) != 1 || calls[0].MaxOutputTokens != 32768 || calls[0].ThinkingType != "adaptive" || calls[0].ReasoningEffort != "max" {
		t.Fatalf("actual request budget or reasoning was lost: %+v", calls)
	}
	if len(calls[0].Frames) != 1 || calls[0].Frames[0].Model != "actual-model" {
		t.Fatal("gateway's response model identity was lost")
	}
}

func TestWireRecorderKeepsSSEFrameDiagnosticsAcrossReadBoundaries(t *testing.T) {
	stream := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":7,\"output_tokens\":0}}}\n\n" +
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"fixture\"}}\n\n" +
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}\n\n"
	var frames []wireFrame
	var body string
	reader := &recordedBody{ReadCloser: io.NopCloser(strings.NewReader(stream)), hash: sha256.New(), done: func(_ string, text string, _ bool, result []wireFrame) {
		body, frames = text, result
	}}
	buffer := make([]byte, 11)
	for {
		if _, err := reader.Read(buffer); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
	}
	_ = reader.Close()
	if body != stream || len(frames) != 3 || frames[1].DeltaType != "text_delta" || frames[2].StopReason != "end_turn" {
		t.Fatalf("stream diagnostics lost frames: %+v", frames)
	}
	if frames[0].InputTokens == nil || *frames[0].InputTokens != 7 || frames[2].OutputTokens == nil || *frames[2].OutputTokens != 2 {
		t.Fatal("actual wire usage was lost")
	}
}

func TestWireResponseCaptureIsBounded(t *testing.T) {
	stream := strings.Repeat("x", (2<<20)+1)
	var body string
	var truncated bool
	reader := &recordedBody{ReadCloser: io.NopCloser(strings.NewReader(stream)), hash: sha256.New(), done: func(_ string, text string, cut bool, _ []wireFrame) {
		body, truncated = text, cut
	}}
	_, _ = io.Copy(io.Discard, reader)
	_ = reader.Close()
	if len(body) != 2<<20 || !truncated {
		t.Fatal("response diagnostic capture was not bounded")
	}
}

func TestWireFrameLimitPreservesTerminalUsageAndStop(t *testing.T) {
	recorder := &recordedBody{}
	token := []byte("data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"x\"}}\n\n")
	for range 10020 {
		recorder.recordFrames(token)
	}
	recorder.recordFrames([]byte("data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"max_tokens\"},\"usage\":{\"output_tokens\":32768}}\n\n"))
	recorder.recordFrames([]byte("data: {\"type\":\"message_stop\"}\n\n"))
	if len(recorder.frames) > 10016 || len(recorder.frames) < 10002 {
		t.Fatal("diagnostic frames are unbounded or terminal events were dropped")
	}
	stop := recorder.frames[len(recorder.frames)-2]
	if stop.StopReason != "max_tokens" || stop.OutputTokens == nil || *stop.OutputTokens != 32768 || recorder.frames[len(recorder.frames)-1].Type != "message_stop" {
		t.Fatal("long stream lost its actual terminating usage and stop reason")
	}
}
