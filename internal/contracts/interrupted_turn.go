package contracts

import "slices"

// EditableTurnMessage returns the transcript anchor of the latest stopped or failed run.
func EditableTurnMessage(session Session, messages []Message, runs []Run) int {
	if session.ID == "" || session.Archived || session.SubagentType != "" || session.Goal != nil {
		return -1
	}
	var status string
	switch {
	case session.Status == Idle && session.LastOutcome == "interrupted":
		status = RunStatusInterrupted
	case session.LastOutcome == "failed" && session.Status != Working && session.Status != AwaitingInput:
		// Acknowledging a failure returns the session to idle, so a seen
		// failure keeps the same edit anchor an unseen one has. Only a run in
		// flight takes the anchor away.
		status = RunStatusFailed
	default:
		return -1
	}
	var latest Run
	for _, run := range slices.Backward(runs) {
		if run.SessionID == session.ID {
			latest = run
			break
		}
	}
	if latest.Status != status {
		return -1
	}
	for i, m := range slices.Backward(messages) {
		if m.Role != "user" {
			continue
		}
		if m.TurnID == latest.ID || latest.RequestID != "" && (m.RequestID == latest.RequestID || m.ID == latest.RequestID) {
			return i
		}
		break
	}
	return -1
}
