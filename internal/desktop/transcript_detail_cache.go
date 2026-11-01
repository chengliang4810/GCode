package desktop

import (
	"unicode"
)

const (
	activityDetailsMaxEntries = 64
	activityDetailsMaxBytes   = 16 << 20
)

type activityDetailKey struct{ session, block string }
type activityDetailFont struct {
	body, code string
	size       float32
}

// Cached entries contain presentation data only. Widget focus, selection and
// scroll state remain owned by each native detail element.
type activityDetailEntry struct {
	source         activityDetailSource
	font           activityDetailFont
	lines          []thoughtLine
	width          float32
	representative int
	measured       map[int]float32
	bytes          int
	touched        uint64
}

type activityDetailsCache struct {
	entries map[activityDetailKey]*activityDetailEntry
	bytes   int
	clock   uint64
}

func (cache *activityDetailsCache) get(key activityDetailKey, source activityDetailSource, font activityDetailFont) *activityDetailEntry {
	if cache.entries == nil {
		cache.entries = make(map[activityDetailKey]*activityDetailEntry)
	}
	cache.clock++
	if entry := cache.entries[key]; entry != nil && entry.source == source && entry.font == font {
		entry.touched = cache.clock
		return entry
	}
	entry := &activityDetailEntry{source: source, font: font, measured: make(map[int]float32), touched: cache.clock}
	if previous := cache.entries[key]; previous != nil && previous.source == source {
		// A font change invalidates geometry, not Markdown parsing.
		entry.lines = previous.lines
	} else {
		entry.lines = detailLines(source)
	}
	entry.bytes = 2*(len(source.text)+len(source.command)+len(source.args)+len(source.path)+len(source.output)) + len(entry.lines)*104
	for i, line := range entry.lines {
		entry.bytes += len(line) * 64
		if width := estimateDetailWidth(line, font.size); width > entry.width {
			entry.width, entry.representative = width, i
		}
	}
	entry.width += 16
	if previous := cache.entries[key]; previous != nil {
		cache.bytes -= previous.bytes
		delete(cache.entries, key)
	}
	if entry.bytes > activityDetailsMaxBytes {
		return entry
	}
	for len(cache.entries) >= activityDetailsMaxEntries || cache.bytes+entry.bytes > activityDetailsMaxBytes {
		var oldest activityDetailKey
		oldestTime := ^uint64(0)
		for candidate, cached := range cache.entries {
			if cached.touched < oldestTime {
				oldest, oldestTime = candidate, cached.touched
			}
		}
		cache.bytes -= cache.entries[oldest].bytes
		delete(cache.entries, oldest)
	}
	cache.entries[key] = entry
	cache.bytes += entry.bytes
	return entry
}

// Estimation scans Unicode once without invoking native text layout. Exact
// widths are learned only for a representative line and rows entering view.
func estimateDetailWidth(line thoughtLine, size float32) float32 {
	var width float32
	for _, run := range line {
		var units float32
		for _, r := range run.text {
			switch {
			case r == '\t':
				units += 4 * .65
			case unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || r == '\u200d' || r == '\ufe0f':
			case r >= 0x1f000:
				units += 2
			case r >= 0x2e80:
				units += 1.1
			case run.style.code:
				units += .65
			case r == 'W' || r == 'M' || r == '@':
				units += 1
			case r == 'i' || r == 'l' || r == '.' || r == ',' || r == ' ':
				units += .4
			default:
				units += .75
			}
		}
		if run.style.bold {
			units *= 1.08
		}
		width += units * size
	}
	return width
}
