package contracts

import "slices"

// InterruptedTurnMessage returns the transcript anchor of the latest stopped run.
func InterruptedTurnMessage(session Session, messages []Message, runs []Run) int {
	if session.ID == "" || session.Status != Idle || session.LastOutcome != "interrupted" ||
		session.Archived || session.SubagentType != "" || session.Goal != nil {
		return -1
	}
	var latest Run
	for _, run := range slices.Backward(runs) {
		if run.SessionID == session.ID {
			latest = run
			break
		}
	}
	if latest.Status != "interrupted" {
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
