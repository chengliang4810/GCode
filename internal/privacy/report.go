package privacy

import "gcode/third_party/magpie/pii"

type ModelStatus = pii.Status

// Report contains counts only; sensitive values and their prefixes never
// cross into events, notifications or diagnostic error messages.
type Report struct {
	Masked  int            `json:"masked"`
	Warned  int            `json:"warned"`
	Blocked int            `json:"blocked"`
	Kinds   map[string]int `json:"kinds"`
}
