package workflow

import "testing"

// ledgeLamps 是一次时间线的灯心位置：站宽 168、灯心距站左缘 17。
func ledgeLampXs(count int) []float32 {
	out := make([]float32, count)
	for i := range out {
		out[i] = StationCenterX(i)
	}
	return out
}

func TestLedgeWidthGrowsWithLampsAndCountsTheRest(t *testing.T) {
	if LedgeWidth(0) != 0 {
		t.Fatalf("no lamps means no ledge: %g", LedgeWidth(0))
	}
	// 一枚灯：8 + 10 + 8 = 26。
	if got := LedgeWidth(1); got != 26 {
		t.Fatalf("one lamp: %g", got)
	}
	// 三枚灯：8 + (3*16-6) + 8 = 58。
	if got := LedgeWidth(3); got != 58 {
		t.Fatalf("three lamps: %g", got)
	}
	// 再多也只多出「还有 N 个」的宽度。
	if got := LedgeWidth(5); got != 58+LedgeMore {
		t.Fatalf("five lamps: %g", got)
	}
}

func TestFoldStationsKeepsWhatIsInView(t *testing.T) {
	lamps := ledgeLampXs(3)
	if fold := FoldStations(lamps, 0, 800, -1, 0); fold.Folded() {
		t.Fatalf("nothing is off screen: %+v", fold)
	}
	// 往回滚一段：滚到左边的站折到左檐，右边没有东西出去。
	fold := FoldStations(lamps, 400, 800, -1, 0)
	if len(fold.Left) == 0 || fold.Left[0] != 0 {
		t.Fatalf("stations scrolled off the left fold there: %+v", fold)
	}
	if len(fold.Right) != 0 {
		t.Fatalf("nothing folds right in this window: %+v", fold)
	}
}

func TestFoldStationsDoesNotFoldTheCameraTarget(t *testing.T) {
	lamps := ledgeLampXs(3)
	fold := FoldStations(lamps, 200, 800, 0, 0)
	if len(fold.Left) != 0 {
		t.Fatalf("the station the camera flies to is not folded: %+v", fold)
	}
}

func TestFoldStationsFoldsRightWhenTheViewportEnds(t *testing.T) {
	lamps := ledgeLampXs(6)
	// 视口 500 宽：最后几站在右边外面。
	fold := FoldStations(lamps, 0, 500, -1, 0)
	if len(fold.Right) == 0 {
		t.Fatalf("stations beyond the viewport fold right: %+v", fold)
	}
	for _, index := range fold.Right {
		if StationCenterX(index) <= 500 {
			t.Fatalf("station %d is inside the viewport: %+v", index, fold)
		}
	}
}

func TestFoldStationsNeedsAViewport(t *testing.T) {
	if fold := FoldStations(ledgeLampXs(4), 100, 0, -1, 0); fold.Folded() {
		t.Fatalf("an unmeasured viewport folds nothing: %+v", fold)
	}
	if fold := FoldStations(nil, 100, 500, -1, 0); fold.Folded() {
		t.Fatalf("an empty timeline folds nothing: %+v", fold)
	}
}

func TestLedgeLampsKeepsTheContentSide(t *testing.T) {
	shown, more := LedgeLamps([]int{0, 1}, LedgeLeft)
	if len(shown) != 2 || more != 0 || shown[0] != 0 || shown[1] != 1 {
		t.Fatalf("a short ledge shows every lamp: %+v %d", shown, more)
	}
	// 左檐靠内容的一端是最后一个，所以要露末三枚。
	shown, more = LedgeLamps([]int{0, 1, 2, 3, 4}, LedgeLeft)
	if more != 2 || len(shown) != 3 || shown[0] != 2 || shown[2] != 4 {
		t.Fatalf("the left ledge keeps the content-side lamps: %+v %d", shown, more)
	}
	shown, more = LedgeLamps([]int{0, 1, 2, 3, 4}, LedgeRight)
	if more != 2 || len(shown) != 3 || shown[0] != 0 || shown[2] != 2 {
		t.Fatalf("the right ledge keeps the content-side lamps: %+v %d", shown, more)
	}
}

func TestLedgeStubWidthMeasuresToTheOpenStation(t *testing.T) {
	lamps := ledgeLampXs(4)
	// 第一站折叠：檐到第二站灯心之间那段。
	fold := TimelineFold{Left: []int{0}}
	width := LedgeStubWidth(fold, LedgeLeft, lamps, 0, 800, 0)
	if width <= 0 {
		t.Fatalf("the left stub reaches the open station: %g", width)
	}
	if width < LedgeStubMin {
		t.Fatalf("a stub shorter than the minimum is not drawn: %g", width)
	}
	// 没有折叠就没有残段。
	if width := LedgeStubWidth(TimelineFold{}, LedgeLeft, lamps, 0, 800, 0); width != 0 {
		t.Fatalf("no fold means no stub: %g", width)
	}
}

func TestLedgeStubWidthLeavesTheStationSlotAlone(t *testing.T) {
	lamps := ledgeLampXs(2)
	// 右檐折的是第二站：残段从第一站的槽尾量到檐的内缘，不横穿站名。
	fold := TimelineFold{Right: []int{1}}
	width := LedgeStubWidth(fold, LedgeRight, lamps, 0, 400, 0)
	if width <= 0 {
		t.Fatalf("the stub runs from the open station's slot to the ledge: %g", width)
	}
	if want := 400 - (LedgeWidth(1) - LedgePad) - StationWidth; width != want {
		t.Fatalf("stub=%g, want %g", width, want)
	}
	// 右檐连第一站都折了：它左边没有开着的站，残段不画。
	if width := LedgeStubWidth(TimelineFold{Right: []int{0}}, LedgeRight, lamps, 0, 400, 0); width != 0 {
		t.Fatalf("no open station left of the ledge: %g", width)
	}
}
