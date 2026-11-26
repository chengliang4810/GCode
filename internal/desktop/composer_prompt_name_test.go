package desktop

import (
	"testing"

	"gcode/internal/contracts"
)

// 候选行的名称是主标识：描述再长也不能把名称压成一个省略号。修复前名称
// 与描述同处一条弹性分配里，长描述按内容宽度分走绝大部分空间，名称只剩
// 几像素宽，候选行看起来只有描述没有名字。
func TestComposerPromptChoiceKeepsNameVisible(t *testing.T) {
	const (
		long  = "When you need a browser, read this Skill by default. Use it to open and operate websites, fill forms, click buttons, take screenshots, extract page data, sign in, and perform other browser automation."
		other = "Design and run a workflow that keeps every station and run card readable in a narrow window."
	)
	for _, width := range []float32{600, 480} {
		a := testApp(t, "default")
		a.state.UserResources.Skills = []contracts.UserSkill{
			{ID: "browser", Name: "ego-browser", Description: long, Enabled: true},
			{ID: "short", Name: "Run", Description: other, Enabled: true},
		}
		sends, stops := 0, 0
		tt := promptTester(t, a, width, &sends, &stops)
		if err := tt.Click("Draft"); err != nil {
			t.Fatal(err)
		}
		tt.Type("/")
		tt.Frame()

		wide, ok := tt.Find(long)
		if !ok {
			t.Fatalf("%g: long description was not rendered", width)
		}
		narrow, ok := tt.Find(other)
		if !ok {
			t.Fatalf("%g: second description was not rendered", width)
		}

		// 名称按自身内容定宽，描述紧跟其后：名字越宽，描述起点越靠右。名称若
		// 被压缩成同一个省略号，两条起点会挤在同一处。
		if narrow.X >= wide.X {
			t.Fatalf("%g: long name did not reserve room: ego-browser starts %g, Run starts %g", width, wide.X, narrow.X)
		}
		if gap := wide.X - narrow.X; gap < 40 {
			t.Fatalf("%g: name width did not reach the description: gap %g", width, gap)
		}
		// 名称至少要留在行内可读的位置，而不是贴着左边缘的省略号。
		if wide.X < 90 {
			t.Fatalf("%g: long description left %g px for the name", width, wide.X)
		}
	}
}
