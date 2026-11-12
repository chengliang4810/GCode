package contracts

import (
	"context"
	"slices"
	"time"
)

// UsageRecord stores the message metadata required for application usage
// statistics without retaining transcript text or tool output.
type UsageRecord struct {
	ID         string    `json:"id"`
	TurnID     string    `json:"turnId"`
	Role       string    `json:"role"`
	CreatedAt  time.Time `json:"createdAt"`
	Usage      Usage     `json:"usage"`
	ModelID    string    `json:"modelId,omitempty"`
	ToolCount  int       `json:"toolCount"`
	BlockCount int       `json:"blockCount"`
}

// UsageRecordsFromMessages projects messages to usage metadata and drops all
// transcript and tool payloads.
func UsageRecordsFromMessages(messages []Message) []UsageRecord {
	records := make([]UsageRecord, len(messages))
	for i, message := range messages {
		toolCount := 0
		for _, block := range message.Blocks {
			if block.Tool != nil {
				toolCount++
			}
		}
		records[i] = UsageRecord{
			ID:         message.ID,
			TurnID:     message.TurnID,
			Role:       message.Role,
			CreatedAt:  message.CreatedAt,
			Usage:      message.Usage,
			ModelID:    message.ModelID,
			ToolCount:  toolCount,
			BlockCount: len(message.Blocks),
		}
	}
	return records
}

// UsageRuns projects runs to the fields required for usage statistics and
// drops request, error, summary, and timing payloads.
func UsageRuns(runs []Run) []Run {
	if runs == nil {
		return nil
	}
	projected := make([]Run, len(runs))
	for i, run := range runs {
		projected[i] = Run{
			ID:         run.ID,
			SessionID:  run.SessionID,
			Status:     run.Status,
			DurationMS: run.DurationMS,
		}
	}
	return projected
}

// ApplicationUsage is the compact, persisted application-wide usage state.
// Records are keyed by session ID and contain no transcript text. Report is
// the already-generated presentation projection; GeneratedAt is its calendar
// anchor for range and heatmap rendering.
type ApplicationUsage struct {
	Records     map[string][]UsageRecord `json:"records"`
	Runs        []Run                    `json:"runs"`
	Report      ApplicationUsageReport   `json:"report"`
	GeneratedAt time.Time                `json:"generatedAt"`
}

// Clone returns an owned copy of compact usage state and its nested report.
func (v ApplicationUsage) Clone() ApplicationUsage {
	clone := v
	if v.Records != nil {
		clone.Records = make(map[string][]UsageRecord, len(v.Records))
		for sessionID, records := range v.Records {
			clone.Records[sessionID] = slices.Clone(records)
		}
	}
	clone.Runs = slices.Clone(v.Runs)
	for i := range clone.Runs {
		if clone.Runs[i].FinishedAt != nil {
			finishedAt := *clone.Runs[i].FinishedAt
			clone.Runs[i].FinishedAt = &finishedAt
		}
	}
	clone.Report = v.Report.Clone()
	return clone
}

// ApplicationUsageService reads the generated application usage report without
// loading all session transcripts into a snapshot.
type ApplicationUsageService interface {
	ReadApplicationUsage(context.Context) (ApplicationUsage, error)
}
