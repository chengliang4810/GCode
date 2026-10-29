//go:build windows && (amd64 || arm64)

package windows

import "testing"

func TestPointerMoveButtonTag(t *testing.T) {
	for _, test := range []struct {
		name string
		wp   uintptr
		want int
	}{
		{"none", 0, -128},
		{"primary", 0x0001, -129},
		{"secondary", 0x0002, -130},
		{"middle", 0x0010, -144},
		{"first extended", 0x0020, -160},
		{"second extended", 0x0040, -192},
		{"primary wins all", 0x0073, -243},
		{"secondary wins middle and extended", 0x0072, -242},
		{"middle wins extended", 0x0070, -240},
		{"first extended wins second", 0x0060, -224},
		{"shift and control only", 0x0004 | 0x0008, -128},
		{"keyboard flags preserve middle", 0x0004 | 0x0008 | 0x0010, -144},
		{"unrelated high bits", 0xffff0000, -128},
		{"unrelated high bits preserve secondary", 0xffff0000 | 0x0002, -130},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := pointerMoveButtonTag(test.wp); got != test.want {
				t.Fatalf("wParam %#x mapped to %d, want %d", test.wp, got, test.want)
			}
		})
	}
}
