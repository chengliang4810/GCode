package workbench

func (s *State) OpenGoal(sessionID, title string) {
	if s.closed.Load() || sessionID == "" || s.RenderGoal == nil {
		return
	}
	s.choose("goal", sessionID, title)
}
