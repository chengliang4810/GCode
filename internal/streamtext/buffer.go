// Package streamtext accumulates append-only text while retaining immutable
// string values for snapshots and events.
package streamtext

import "strings"

// Buffer must not be copied after use. Append may follow an external
// replacement (for example a retried reply or a display hook).
type Buffer struct {
	builder strings.Builder
}

func (b *Buffer) Append(current, delta string) string {
	if b.builder.String() != current {
		b.builder.Reset()
		b.builder.WriteString(current)
	}
	b.builder.WriteString(delta)
	return b.builder.String()
}
