package desktop

func hookStatusLabel(status string) string {
	switch status {
	case "failed":
		return "Failed"
	case "blocked":
		return "Blocked"
	case "stopped":
		return "Stopped"
	default:
		return "Completed"
	}
}
