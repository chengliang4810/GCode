package desktop

// hookStatusLabel names a hook record's outcome. Only the host's explicit
// success value reads as completion: a missing or unknown status stays
// unnamed, because reporting it as "Completed" would invent a result.
func hookStatusLabel(status string) string {
	switch status {
	case "completed":
		return "Completed"
	case "failed":
		return "Failed"
	case "blocked":
		return "Blocked"
	case "stopped":
		return "Stopped"
	}
	// A missing or unknown value is absent data, not an outcome: it keeps the
	// same neutral placeholder the rest of the interface uses for one.
	return "—"
}
