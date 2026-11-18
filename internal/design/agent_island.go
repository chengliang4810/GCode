package design

// IslandGeometry keeps the top-edge companion separate from normal window chrome.
func IslandGeometry(expanded bool, count int) (width, height int) {
	if !expanded {
		if count > 0 {
			return 298, 32
		}
		return 210, 28
	}
	if count == 0 {
		return 340, 152
	}
	return 440, min(400, 56+count*112)
}

const (
	IslandCardRadius  float32 = 12
	IslandCardPadding float32 = 14
)

func IslandRadius(expanded bool) float32 {
	if expanded {
		return 24
	}
	return 12
}
