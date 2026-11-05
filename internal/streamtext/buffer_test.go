package streamtext

import (
	"strings"
	"testing"
)

func TestBufferKeepsPublishedStringsAcrossAppendReplacementAndRetry(t *testing.T) {
	var b Buffer
	first := b.Append("", "你好")
	second := b.Append(first, " world")
	replaced := b.Append("hook replacement", "!")
	retried := b.Append("", "retry")
	if first != "你好" || second != "你好 world" || replaced != "hook replacement!" || retried != "retry" {
		t.Fatalf("published text changed: %q / %q / %q / %q", first, second, replaced, retried)
	}
	for range 4096 {
		retried = b.Append(retried, ".")
	}
	if first != "你好" || second != "你好 world" || replaced != "hook replacement!" {
		t.Fatal("buffer growth changed a published snapshot")
	}
}

func BenchmarkReplyAccumulation(b *testing.B) {
	chunk := strings.Repeat("x", 100)
	for _, buffered := range []bool{false, true} {
		name := "concatenation"
		if buffered {
			name = "buffer"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				var accumulator Buffer
				var text string
				for range 1000 {
					if buffered {
						text = accumulator.Append(text, chunk)
					} else {
						text += chunk
					}
				}
				if len(text) != 100_000 {
					b.Fatal("truncated reply")
				}
			}
		})
	}
}
