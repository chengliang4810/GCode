package desktop

import (
	"strings"
	"testing"
)

func BenchmarkActivityDetailTenThousandLinesColdCache(b *testing.B) {
	source := activityDetailSource{output: strings.Repeat("short\n", 10000)}
	font := activityDetailFont{body: "Geist", code: "Geist Mono", size: 13}
	b.ReportAllocs()
	for b.Loop() {
		var cache activityDetailsCache
		cache.get(activityDetailKey{block: "large"}, source, font)
	}
}

func BenchmarkActivityDetailTenThousandLinesWarmCache(b *testing.B) {
	source := activityDetailSource{output: strings.Repeat("short\n", 10000)}
	font := activityDetailFont{body: "Geist", code: "Geist Mono", size: 13}
	var cache activityDetailsCache
	key := activityDetailKey{block: "large"}
	cache.get(key, source, font)
	b.ReportAllocs()
	for b.Loop() {
		cache.get(key, source, font)
	}
}
