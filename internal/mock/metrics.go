package mock

import "gcode/internal/accounting"

func (s *Server) refreshMetricsLocked() {
	ids := make([]string, 0, len(s.state.Sessions))
	for _, session := range s.state.Sessions {
		ids = append(ids, session.ID)
	}
	accounting.Refresh(&s.state, ids)
}
