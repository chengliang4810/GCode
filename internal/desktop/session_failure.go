package desktop

import (
	"time"

	"gcode/internal/contracts"
)

// acknowledgeSessionFailure reports a failure the user has now seen.
//
// The failed marker on a conversation, and on the project that owns it, says
// "a run failed and nobody looked yet". The transcript shows the failure as
// soon as its conversation is open, so from that moment the marker has nothing
// left to say and the core returns the conversation to idle. A failure in a
// conversation the user has not opened keeps its marker until they do.
//
// The core owns the state: the desktop only reports that the failure was read,
// and LastOutcome keeps naming the failed outcome for the transcript, the
// menu bar and the failed turn's edit anchor.
func (a *App) acknowledgeSessionFailure(session contracts.Session) {
	if session.ID == "" {
		return
	}
	if session.Status != contracts.Errored {
		delete(a.acknowledgedFailures, session.ID)
		return
	}
	// One failure is acknowledged once. The timestamp tells a new failure from
	// the one the core already settled, so a repeated frame never asks again.
	if seen, ok := a.acknowledgedFailures[session.ID]; ok && seen.Equal(session.UpdatedAt) {
		return
	}
	if a.acknowledgedFailures == nil {
		a.acknowledgedFailures = make(map[string]time.Time)
	}
	a.acknowledgedFailures[session.ID] = session.UpdatedAt
	a.dispatch(contracts.Command{Kind: "session.acknowledge", SessionID: session.ID})
}

// acknowledgeVisibleFailures reports the failure of the conversation that is
// on screen: the user is reading the transcript that shows it.
func (a *App) acknowledgeVisibleFailures() {
	if a.sessionID != "" {
		a.acknowledgeSessionFailureByID(a.sessionID)
	}
}

func (a *App) acknowledgeSessionFailureByID(id string) {
	for _, session := range a.state.Sessions {
		if session.ID == id {
			a.acknowledgeSessionFailure(session)
			return
		}
	}
}
