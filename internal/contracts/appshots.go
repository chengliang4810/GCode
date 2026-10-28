package contracts

// AppshotCapabilities describes simulated capture permissions; it never
// requests operating-system access or captures a real window.
type AppshotCapabilities struct {
	Platform        string `json:"platform"`
	GlobalShortcut  string `json:"globalShortcut"`
	WindowCapture   string `json:"windowCapture"`
	ApplicationText string `json:"applicationText"`
}
