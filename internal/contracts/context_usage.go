package contracts

// ContextUsage is a host-reported occupancy snapshot for one session. It is
// independent of the cumulative accounting counters on Message.Usage.
// A nil field is unreported; a non-nil Tokens pointing to zero is a real zero.
type ContextUsage struct {
	Tokens *uint64 `json:"tokens,omitempty"`
	Window *uint64 `json:"window,omitempty"`
}

// HasWindow reports whether the host supplied a usable context capacity.
func (u ContextUsage) HasWindow() bool { return u.Window != nil && *u.Window > 0 }

// Fraction returns the reported occupancy, which can exceed one. The ring's
// painted arc may clamp separately; the percentage must keep the reported value.
func (u ContextUsage) Fraction() (float64, bool) {
	if u.Tokens == nil || !u.HasWindow() {
		return 0, false
	}
	return float64(*u.Tokens) / float64(*u.Window), true
}

// Clone returns independently owned optional counters.
func (u ContextUsage) Clone() ContextUsage {
	return ContextUsage{Tokens: cloneContextCount(u.Tokens), Window: cloneContextCount(u.Window)}
}

// Merged applies a host report without erasing fields it did not report.
// A zero window is ignored; zero tokens and decreasing occupancy are valid.
func (u ContextUsage) Merged(report ContextUsage) ContextUsage {
	next := u.Clone()
	if report.Tokens != nil {
		next.Tokens = cloneContextCount(report.Tokens)
	}
	if report.HasWindow() {
		next.Window = cloneContextCount(report.Window)
	}
	return next
}

func cloneContextCount(value *uint64) *uint64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
