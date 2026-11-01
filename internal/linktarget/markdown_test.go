package linktarget_test

import (
	"testing"

	"gcode/internal/linktarget"
)

func TestNormalizeMarkdown(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, src, want string }{
		{"backslashes", `[Read](C:\repo\a.go:12:5)`, `[Read](<C:/repo/a.go:12:5>)`},
		{"spaces", `[Read](C:/My Project/a.go)`, `[Read](<C:/My Project/a.go>)`},
		{"label unchanged", `[C:\label.go](C:\repo\a.go)`, `[C:\label.go](<C:/repo/a.go>)`},
		{"angle", `[Read](<C:\My Project\a.go>)`, `[Read](<C:/My Project/a.go>)`},
		{"title", `[Read](C:\My Project\a.go "title")`, `[Read](<C:/My Project/a.go> "title")`},
		{"angle title", `[Read](<C:\My Project\a.go> "title")`, `[Read](<C:/My Project/a.go> "title")`},
		{"parentheses", `[Read](C:\My (Project)\a.go)`, `[Read](<C:/My (Project)/a.go>)`},
		{"two links", `[a](C:\a.go) [b](D:\b.go)`, `[a](<C:/a.go>) [b](<D:/b.go>)`},
		{"web unchanged", `[Read](https://example.com/a)`, `[Read](https://example.com/a)`},
		{"relative unchanged", `[Read](src\a.go)`, `[Read](src\a.go)`},
		{"inline unchanged", "`[Read](C:\\a.go)` [Read](C:\\a.go)", "`[Read](C:\\a.go)` [Read](<C:/a.go>)"},
		{"multitick unchanged", "`` `[Read](C:\\a.go)` ``", "`` `[Read](C:\\a.go)` ``"},
		{"fence unchanged", "```md\n[Read](C:\\a.go)\n```\n[Read](C:\\a.go)", "```md\n[Read](C:\\a.go)\n```\n[Read](<C:/a.go>)"},
		{"tilde fence unchanged", "~~~md\n[Read](C:\\a.go)\n~~~\n", "~~~md\n[Read](C:\\a.go)\n~~~\n"},
		{"long fence", "````md\n```\n[Read](C:\\a.go)\n````\n", "````md\n```\n[Read](C:\\a.go)\n````\n"},
		{"multiline code", "`code\n[Read](C:\\a.go)\n`", "`code\n[Read](C:\\a.go)\n`"},
		{"escaped syntax", `\[Read\](C:\a.go)`, `\[Read\](C:\a.go)`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := linktarget.NormalizeMarkdown(tt.src); got != tt.want {
				t.Fatalf("NormalizeMarkdown(%q) = %q, want %q", tt.src, got, tt.want)
			}
		})
	}
}
