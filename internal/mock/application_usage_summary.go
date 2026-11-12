package mock

import (
	"gcode/internal/appusage"
	"gcode/internal/contracts"
)

// refreshApplicationUsageLocked rebuilds the mock's durable usage projection
// from its resident state. Tests use a complete in-memory state, so the helper
// can project every message and run while still exercising the same report DTO
// as the real engine.
func (s *Server) refreshApplicationUsageLocked() {
	records := make(map[string][]contracts.UsageRecord, len(s.state.Messages))
	for sessionID, messages := range s.state.Messages {
		records[sessionID] = contracts.UsageRecordsFromMessages(messages)
	}
	runs := contracts.UsageRuns(s.state.Runs)
	now := s.opts.Now()
	report := appusage.BuildRecords(records, runs, now)
	s.state.ApplicationUsage = &contracts.ApplicationUsage{
		Records:     records,
		Runs:        runs,
		Report:      appusage.ToContractReport(report),
		GeneratedAt: now,
	}
}
