package workbench

// OpenWorkflow binds the run detail tab to the requested run. The tab shows
// the same run facts the conversation card shows; it never keeps its own copy.
func (s *State) OpenWorkflow(runID, title string) {
	if runID == "" || s.RenderWorkflow == nil {
		return
	}
	s.choose("workflow", runID, title)
}

// OpenWorkflowRuns binds the run directory tab to one conversation: the list of
// workflow runs that conversation launched, not a run's own detail.
func (s *State) OpenWorkflowRuns(sessionID, title string) {
	if sessionID == "" || s.RenderWorkflowRuns == nil {
		return
	}
	s.choose("workflow-runs", sessionID, title)
}
