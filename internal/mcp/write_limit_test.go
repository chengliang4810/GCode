package mcp

import (
	"bytes"
	"strings"
	"testing"
)

type trackedStdioInput struct {
	bytes.Buffer
	closed bool
}

func (w *trackedStdioInput) Close() error { w.closed = true; return nil }

func TestOversizedStdioRequestIsRejectedBeforeWriting(t *testing.T) {
	input := &trackedStdioInput{}
	client := &Client{input: input, done: make(chan struct{})}
	if err := client.write(t.Context(), map[string]string{"payload": strings.Repeat("x", maxMessageBytes)}); err == nil {
		t.Fatal("oversized request accepted")
	}
	if input.Len() != 0 || input.closed {
		t.Fatal("oversized request reached or closed the transport")
	}
	if err := client.write(t.Context(), map[string]string{"method": "still-usable"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(input.String(), "still-usable") || input.closed {
		t.Fatal("local size rejection damaged the connection")
	}
}
