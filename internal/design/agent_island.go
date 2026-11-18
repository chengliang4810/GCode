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
	// islandExpandedRadius is the expanded island's outer corner radius, and
	// islandCollapsedRadius the collapsed one's.
	islandExpandedRadius  float32 = 24
	islandCollapsedRadius float32 = 12
	// IslandCardPadding is the padding the expanded island's inner card leaves
	// on every side.
	IslandCardPadding float32 = 14
	// IslandCardRadius keeps the card's corners concentric with the island's:
	// the outer radius minus that padding, 24 − 14 = 10. A larger inner radius
	// would bulge past the outer corner.
	IslandCardRadius float32 = islandExpandedRadius - IslandCardPadding
)

func IslandRadius(expanded bool) float32 {
	if expanded {
		return islandExpandedRadius
	}
	return islandCollapsedRadius
}
