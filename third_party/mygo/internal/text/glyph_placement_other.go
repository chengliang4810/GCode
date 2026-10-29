//go:build !windows || (!amd64 && !arm64)

package text

func (*System) quarterEligible(*Font, float32, bool, uint8) bool { return false }
