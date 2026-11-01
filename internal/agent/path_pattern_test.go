package agent

import "testing"

func TestPathPatternSemantics(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		path    string
		want    bool
	}{
		{name: "bare suffix recursively", pattern: "*.go", path: "deep/main.go", want: true},
		{name: "bare literal recursively", pattern: "go.mod", path: "nested/go.mod", want: true},
		{name: "immediate child", pattern: "src/*.go", path: "src/main.go", want: true},
		{name: "star stays inside segment", pattern: "src/*.go", path: "src/deep/main.go"},
		{name: "double star zero directories", pattern: "src/**/*.go", path: "src/main.go", want: true},
		{name: "double star many directories", pattern: "src/**/*.go", path: "src/a/b/main.go", want: true},
		{name: "literal prefix", pattern: "src/**/*.go", path: "other/main.go"},
		{name: "alternatives", pattern: "**/*.{go,ts}", path: "src/main.ts", want: true},
		{name: "alternative excluded", pattern: "**/*.{go,ts}", path: "src/main.rs"},
		{name: "nested alternatives", pattern: "{src,{lib,test}}/*.go", path: "test/x.go", want: true},
		{name: "unicode single character", pattern: "文件?.[gt]s", path: "文件甲.ts", want: true},
		{name: "negated class", pattern: "*.[^g]o", path: "main.go"},
		{name: "windows separators", pattern: `src\**\*.go`, path: "src/deep/main.go", want: true},
		{name: "current directory prefix", pattern: "./src/*.go", path: "src/main.go", want: true},
		{name: "case sensitive", pattern: "*.GO", path: "main.go"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pattern, err := compilePathPattern(tc.pattern)
			if err != nil {
				t.Fatal(err)
			}
			if got := pattern.matches(tc.path); got != tc.want {
				t.Fatalf("%q matched %q = %v, want %v", tc.pattern, tc.path, got, tc.want)
			}
		})
	}
}

func TestPathPatternRejectsInvalidPatterns(t *testing.T) {
	for _, pattern := range []string{
		"", "[", "src/[", "{go,ts", "go}", "{go}", "src/**suffix/*.go",
		"../*.go", "/absolute/*.go", `C:\work\*.go`, "src//*.go",
		"{a,b}{a,b}{a,b}{a,b}{a,b}{a,b}{a,b}",
	} {
		t.Run(pattern, func(t *testing.T) {
			if _, err := compilePathPattern(pattern); err == nil {
				t.Fatalf("invalid pattern accepted: %q", pattern)
			}
		})
	}
}
