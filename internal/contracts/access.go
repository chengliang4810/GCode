package contracts

type AccessMode string

const (
	AccessSupervised      AccessMode = "supervised"
	AccessAutoAcceptEdits AccessMode = "auto-accept-edits"
	AccessFullAccess      AccessMode = "full-access"
)

func (m AccessMode) Label() string {
	switch m {
	case AccessSupervised:
		return "Supervised"
	case AccessAutoAcceptEdits:
		return "Auto-accept edits"
	default:
		return "Full access"
	}
}
func (m AccessMode) Valid() bool {
	return m == AccessSupervised || m == AccessAutoAcceptEdits || m == AccessFullAccess
}
