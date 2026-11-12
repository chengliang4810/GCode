package engine

import (
	"strings"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
)

func BenchmarkWriteTrajectory(b *testing.B) {
	core := benchmarkSessionPersistenceCore(b, 1)
	session := core.state.Sessions[0]
	record := contracts.ModelInvocation{
		SessionID: session.ID,
		Invocation: model.Invocation{
			ID: "benchmark", Attempt: 1, StartedAt: time.Unix(1, 0),
			System: strings.Repeat("request context\n", 32768),
		},
	}
	b.ReportAllocs()
	for b.Loop() {
		if err := core.writeTrajectory(session, record); err != nil {
			b.Fatal(err)
		}
	}
}
