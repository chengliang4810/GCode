package i18n

import "testing"

func TestDurationUsesWholeSecondsThroughDays(t *testing.T) {
	for _, test := range []struct {
		name    string
		ms      int64
		english string
		chinese string
	}{
		{"negative", -1, "0s", "0秒"},
		{"zero", 0, "0s", "0秒"},
		{"subsecond", 999, "0s", "0秒"},
		{"truncate fraction", 4199, "4s", "4秒"},
		{"last second", 59999, "59s", "59秒"},
		{"minute rollover", 60000, "1m 00s", "1分 00秒"},
		{"minute and seconds", 65000, "1m 05s", "1分 05秒"},
		{"last minute", 3599999, "59m 59s", "59分 59秒"},
		{"hour rollover", 3600000, "1h 00m 00s", "1小时 00分 00秒"},
		{"hours", 3661999, "1h 01m 01s", "1小时 01分 01秒"},
		{"last hour", 86399999, "23h 59m 59s", "23小时 59分 59秒"},
		{"day rollover", 86400000, "1d 00h 00m 00s", "1天 00小时 00分 00秒"},
		{"days", 183845999, "2d 03h 04m 05s", "2天 03小时 04分 05秒"},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, language := range []string{English, Chinese} {
				want := test.english
				if language == Chinese {
					want = test.chinese
				}
				if got := Duration(language, test.ms); got != want {
					t.Fatalf("%s duration = %q, want %q", language, got, want)
				}
			}
		})
	}
}
