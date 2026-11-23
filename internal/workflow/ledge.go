package workflow

// 边檐：时间线横向滚动时，滚到视口外的站折叠到那一侧，用同一枚小灯表示，
// 点一下把那一站带回来。这里只算几何，不画东西。

const (
	// LedgeLamp 是边檐上一枚灯的直径。
	LedgeLamp float32 = 10
	// LedgePitch 是相邻两枚灯的间距。
	LedgePitch float32 = 16
	// LedgePad 是边檐靠边与靠内容两侧的内边距。
	LedgePad float32 = 8
	// LedgeFade 是轨道在檐旁淡出的宽度：檐要落在干净的底上。
	LedgeFade float32 = 40
	// LedgeMaxLamps 是边檐上最多露几枚灯。
	LedgeMaxLamps = 3
	// LedgeMore 是「还有 N 个」占的宽度。
	LedgeMore float32 = 26
	// LedgeStubMin 是檐到第一座开着的站之间那段轨道的最短长度：更短就画成一点，不画。
	LedgeStubMin float32 = 6
)

// 边檐在哪一侧。
const (
	LedgeLeft  = "left"
	LedgeRight = "right"
)

// TimelineFold 是滚到视口外的站，按站的下标升序分到两侧。
type TimelineFold struct {
	Left  []int
	Right []int
}

// Folded reports whether any station is folded away.
func (f TimelineFold) Folded() bool { return len(f.Left)+len(f.Right) > 0 }

// LedgeWidth 是一侧边檐占的宽（含两侧内边距）。没有灯就没有边檐。
func LedgeWidth(count int) float32 {
	if count <= 0 {
		return 0
	}
	lamps := float32(min(count, LedgeMaxLamps))*LedgePitch - (LedgePitch - LedgeLamp)
	width := LedgePad + lamps + LedgePad
	if count > LedgeMaxLamps {
		width += LedgeMore
	}
	return width
}

// FoldStations 找出滚到视口外的站。折叠是不动点：边檐越宽，压在它底下的站
// 越多，所以要反复算到集合不再变化。keep 是镜头正飞向的站：它按陈旧的滚动量
// 可能算在视野外，但正要进来，两侧都不收它。
func FoldStations(lamps []float32, scrollLeft, viewport float32, keep int, inset float32) TimelineFold {
	if len(lamps) == 0 || viewport <= 0 {
		return TimelineFold{}
	}
	fold := TimelineFold{}
	for pass := 0; pass < len(lamps)+1; pass++ {
		leftWidth := float32(0)
		if len(fold.Left) > 0 {
			leftWidth = LedgeWidth(len(fold.Left)) + LedgeFade
		}
		rightWidth := float32(0)
		if len(fold.Right) > 0 {
			rightWidth = LedgeWidth(len(fold.Right)) + LedgeFade
		}
		left := make([]int, 0, len(lamps))
		right := make([]int, 0, len(lamps))
		for index, lamp := range lamps {
			x := inset + lamp - scrollLeft
			if index == keep {
				continue
			}
			if x < leftWidth {
				left = append(left, index)
				continue
			}
			if x > viewport-rightWidth {
				right = append(right, index)
			}
		}
		if len(left) == len(fold.Left) && len(right) == len(fold.Right) {
			break
		}
		fold = TimelineFold{Left: left, Right: right}
	}
	return fold
}

// LedgeLamps 是边檐上真正露出的灯（最多 LedgeMaxLamps 枚，靠内容的一端优先）
// 与没露出的数量。
func LedgeLamps(indexes []int, side string) (shown []int, more int) {
	if len(indexes) <= LedgeMaxLamps {
		return append([]int(nil), indexes...), 0
	}
	more = len(indexes) - LedgeMaxLamps
	if side == LedgeLeft {
		return append([]int(nil), indexes[more:]...), more
	}
	return append([]int(nil), indexes[:LedgeMaxLamps]...), more
}

// LedgeStubWidth 是檐到最近一座开着的站之间那段轨道的宽。左檐从檐的内缘量到
// 那站的灯前；右檐从那站的槽尾量到檐的内缘（用槽而不是灯，免得轨道横穿站名）。
// 短于 LedgeStubMin 就不画。
func LedgeStubWidth(fold TimelineFold, side string, lamps []float32, scrollLeft, viewport, inset float32) float32 {
	indexes := fold.Left
	if side == LedgeRight {
		indexes = fold.Right
	}
	if len(indexes) == 0 {
		return 0
	}
	inner := LedgeWidth(len(indexes)) - LedgePad
	var width float32
	if side == LedgeLeft {
		open := indexes[len(indexes)-1] + 1
		if open >= len(lamps) {
			return 0
		}
		width = inset + lamps[open] - scrollLeft - LedgeStubMin - inner
	} else {
		open := indexes[0] - 1
		if open < 0 {
			return 0
		}
		width = viewport - inner - (inset + StationX(open) + StationWidth - scrollLeft)
	}
	if width < LedgeStubMin {
		return 0
	}
	return width
}
