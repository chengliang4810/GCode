package model

import (
	"strings"
	"testing"
)

func BenchmarkStreamAccumulation(b *testing.B) {
	const chunks = 800
	piece := strings.Repeat("x", 128)
	for _, name := range []string{"text", "chat arguments", "messages blocks"} {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(chunks * int64(len(piece)))
			chatPayload := `{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"` + piece + `"}}]}}]}`
			messagesPayload := `{"type":"content_block_delta","index":0,"delta":{"text":"` + piece + `"}}`
			for b.Loop() {
				switch name {
				case "text":
					state := newStreamState()
					for range chunks {
						state.add(Delta{Text: piece})
					}
					if len(state.snapshot().Message.Text) != chunks*len(piece) {
						b.Fatal("lost streamed text")
					}
				case "chat arguments":
					decoder := chatAdapter{}.decoder()
					for range chunks {
						if _, err := decoder.consume(chatPayload); err != nil {
							b.Fatal(err)
						}
					}
				case "messages blocks":
					decoder := messagesAdapter{}.decoder()
					for range chunks {
						if _, err := decoder.consume(messagesPayload); err != nil {
							b.Fatal(err)
						}
					}
					if len(decoder.snapshot().Message.Text) != chunks*len(piece) {
						b.Fatal("lost streamed text")
					}
				}
			}
		})
	}
}
