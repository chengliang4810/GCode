package workbench

// OpenTrajectory binds the diagnostic tab to the requested conversation,
// independently of the conversation currently displayed in the main pane.
func (s *State) OpenTrajectory(sessionID, title string) {
	if sessionID == "" || s.RenderTrajectory == nil {
		return
	}
	s.choose("trajectory", sessionID, title)
}
