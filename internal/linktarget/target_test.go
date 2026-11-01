package linktarget_test

import (
	"testing"

	"gcode/internal/linktarget"
)

func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, raw string
		want      linktarget.Target
	}{
		{"relative", "internal/main.go", linktarget.Target{Kind: linktarget.File, Path: "internal/main.go"}},
		{"basename", "README.md", linktarget.Target{Kind: linktarget.File, Path: "README.md"}},
		{"posix", "/tmp/中文.go:12:5", linktarget.Target{Kind: linktarget.File, Path: "/tmp/中文.go", Line: 12, Column: 5}},
		{"windows", `C:\My Project\main.go:12:5`, linktarget.Target{Kind: linktarget.File, Path: "C:/My Project/main.go", Line: 12, Column: 5}},
		{"windows slash", "D:/repo/main.go#L12C5", linktarget.Target{Kind: linktarget.File, Path: "D:/repo/main.go", Line: 12, Column: 5}},
		{"file URL", "file:///tmp/a%20b.go#L3", linktarget.Target{Kind: linktarget.File, Path: "/tmp/a b.go", Line: 3}},
		{"windows URL", "file:///C:/My%20Project/a.go:3:2", linktarget.Target{Kind: linktarget.File, Path: "C:/My Project/a.go", Line: 3, Column: 2}},
		{"colon range", "a.go:12-15", linktarget.Target{Kind: linktarget.File, Path: "a.go", Line: 12}},
		{"colon column range", "a.go:12:5-15:8", linktarget.Target{Kind: linktarget.File, Path: "a.go", Line: 12, Column: 5}},
		{"hash range", "a.go#L12-L15", linktarget.Target{Kind: linktarget.File, Path: "a.go", Line: 12}},
		{"hash column range", "a.go#L12C5-L15C8", linktarget.Target{Kind: linktarget.File, Path: "a.go", Line: 12, Column: 5}},
		{"same line range", "a.go#L12C5-L12C8", linktarget.Target{Kind: linktarget.File, Path: "a.go", Line: 12, Column: 5}},
		{"web", "https://example.com/a?q=x#L2", linktarget.Target{Kind: linktarget.Web, URL: "https://example.com/a?q=x#L2"}},
		{"uppercase web", "HTTPS://example.com/path", linktarget.Target{Kind: linktarget.Web, URL: "HTTPS://example.com/path"}},
		{"web encoded", "http://example.com/a%20b", linktarget.Target{Kind: linktarget.Web, URL: "http://example.com/a%20b"}},
		{"ipv6", "http://[::1]:8080/", linktarget.Target{Kind: linktarget.Web, URL: "http://[::1]:8080/"}},
		{"anchor", "#中文-heading", linktarget.Target{Kind: linktarget.Anchor, Anchor: "中文-heading"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := linktarget.Parse(tt.raw)
			if err != nil || got != tt.want {
				t.Fatalf("Parse(%q) = %#v, %v; want %#v", tt.raw, got, err, tt.want)
			}
		})
	}
}

func TestParseRejectsAmbiguousDestinations(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, raw string }{
		{"empty", ""}, {"surrounding space", " a.go"}, {"newline", "a\n.go"},
		{"nul", "a\x00.go"}, {"unc", `\\host\share\a.go`}, {"slash unc", "//host/a.go"},
		{"drive relative", "C:main.go"}, {"dot", "./a.go"}, {"parent", "../a.go"},
		{"numeric drive relative", "C:12"}, {"numeric drive relative column", "C:12:5"},
		{"interior dot", "src/./a.go"}, {"interior parent", "src/../a.go"},
		{"backslash parent", `src\..\a.go`}, {"query", "a.go?line=2"}, {"bad hash", "a.go#section"},
		{"zero line", "a.go:0"}, {"zero column", "a.go:1:0"}, {"zero hash line", "a.go#L0"},
		{"reverse range", "a.go:12-3"}, {"reverse hash range", "a.go#L12-L3"},
		{"reverse columns", "a.go:12:8-12:5"}, {"zero end", "a.go:1-0"},
		{"huge line", "a.go:9999999999999999999999999999"},
		{"bad numeric suffix", "a.go:12x"}, {"negative line", "a.go:-1"},
		{"remote file URL", "file://host/a.go"}, {"file opaque", "file:C:/a.go"},
		{"file relative URL", "file:foo"}, {"file query", "file:///a.go?q=1"},
		{"file encoded parent", "file:///a/%2e%2e/b.go"}, {"file bad escape", "file:///a%zz.go"},
		{"unsupported scheme", "javascript:alert(1)"}, {"mail chat", "mailto:a@example.com"},
		{"missing web host", "https://"}, {"web opaque", "https:example.com"},
		{"credentials", "https://user:pass@example.com"}, {"blank user", "https://@example.com"},
		{"web space", "https://example.com/a b"}, {"web unicode space", "https://example.com/a\u2003b"},
		{"web tab", "https://example.com/\tb"}, {"web backslash", `https://example.com\a`},
		{"web bad percent", "https://example.com/%zz"}, {"web bad fragment", "https://example.com/#%zz"},
		{"web bad query", "https://example.com/?q=%zz"},
		{"bad port", "https://example.com:abc"}, {"large port", "https://example.com:65536"},
		{"zero port", "https://example.com:0"}, {"empty port", "https://example.com:"},
		{"unbracketed ipv6", "https://::1/"}, {"invalid ipv6", "https://[wat]/"},
		{"empty anchor", "#"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got, err := linktarget.Parse(tt.raw); err == nil {
				t.Fatalf("Parse(%q) unexpectedly accepted %#v", tt.raw, got)
			}
		})
	}
}

func TestResolveFile(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, file, root, absolute, relative string
		inside, fail                         bool
	}{
		{"posix relative", "src/a.go", "/repo", "/repo/src/a.go", "src/a.go", true, false},
		{"posix root", "/repo", "/repo", "/repo", ".", true, false},
		{"posix slash", "/a.go", "/", "/a.go", "a.go", true, false},
		{"posix slash same", "/", "/", "/", ".", true, false},
		{"posix case", "/Repo/a.go", "/repo", "/Repo/a.go", "", false, false},
		{"prefix boundary", "/repo-other/a.go", "/repo", "/repo-other/a.go", "", false, false},
		{"external", "/outside/a.go", "/repo", "/outside/a.go", "", false, false},
		{"parent lexical", "../a.go", "/repo/sub", "/repo/a.go", "", false, false},
		{"windows relative", `src\a.go`, `C:\repo`, "C:/repo/src/a.go", "src/a.go", true, false},
		{"windows insensitive", "c:/REPO/A.go", "C:/repo", "c:/REPO/A.go", "A.go", true, false},
		{"windows unicode fold", "C:/K/a.go", "C:/K", "C:/K/a.go", "a.go", true, false},
		{"windows root", "a.go", "C:/", "C:/a.go", "a.go", true, false},
		{"windows root same", "C:/", "C:/", "C:/", ".", true, false},
		{"other drive", "D:/repo/a.go", "C:/repo", "D:/repo/a.go", "", false, false},
		{"different syntax", "/repo/a.go", "C:/repo", "", "", false, true},
		{"relative root", "a.go", "repo", "", "", false, true},
		{"unc root", "a.go", "//host/repo", "", "", false, true},
		{"ambiguous file", "C:a.go", "C:/repo", "", "", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			absolute, relative, inside, err := linktarget.ResolveFile(tt.file, tt.root)
			if (err != nil) != tt.fail || absolute != tt.absolute || relative != tt.relative || inside != tt.inside {
				t.Fatalf("ResolveFile(%q, %q) = %q, %q, %v, %v", tt.file, tt.root, absolute, relative, inside, err)
			}
		})
	}
}

func TestCharacterColumn(t *testing.T) {
	t.Parallel()
	text := "first\r\nA中🙂e\u0301!\r\nlast"
	tests := []struct {
		name         string
		line, column int
		want         int
	}{
		{"first", 2, 1, 0}, {"ascii", 2, 2, 1}, {"chinese", 2, 3, 4},
		{"emoji", 2, 4, 8}, {"combining mark", 2, 5, 9}, {"punctuation", 2, 6, 11},
		{"line end", 2, 7, 12}, {"clamp", 2, 100, 12},
		{"zero line", 0, 4, 0}, {"zero column", 2, 0, 0}, {"missing line", 10, 2, 0},
		{"last line", 3, 3, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := linktarget.CharacterColumn(text, tt.line, tt.column); got != tt.want {
				t.Fatalf("CharacterColumn(line=%d, column=%d) = %d, want %d", tt.line, tt.column, got, tt.want)
			}
		})
	}
}

func TestParsePreview(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, raw, document string
		want                linktarget.Target
		fail                bool
	}{
		{"relative", "./next.md", "/repo/docs/readme.md", linktarget.Target{Kind: linktarget.File, Path: "/repo/docs/next.md"}, false},
		{"parent", "../main.go:12:5", "/repo/docs/readme.md", linktarget.Target{Kind: linktarget.File, Path: "/repo/main.go", Line: 12, Column: 5}, false},
		{"windows", `..\main.go#L12`, `C:\repo\docs\README.MD`, linktarget.Target{Kind: linktarget.File, Path: "C:/repo/main.go", Line: 12}, false},
		{"drive root document", "next.md", "C:/README.md", linktarget.Target{Kind: linktarget.File, Path: "C:/next.md"}, false},
		{"anchor", "#heading", "/repo/README.md", linktarget.Target{Kind: linktarget.Anchor, Anchor: "heading"}, false},
		{"file anchor", "next.md#heading", "/repo/README.md", linktarget.Target{Kind: linktarget.File, Path: "/repo/next.md", Anchor: "heading"}, false},
		{"mail", "mailto:a@example.com?subject=Hello", "/repo/README.md", linktarget.Target{Kind: linktarget.Mail, URL: "mailto:a@example.com?subject=Hello"}, false},
		{"web", "https://example.com", "/repo/README.md", linktarget.Target{Kind: linktarget.Web, URL: "https://example.com"}, false},
		{"absolute posix", "/repo/main.go", "/repo/README.md", linktarget.Target{}, true},
		{"absolute windows", "C:/repo/main.go", "C:/repo/README.md", linktarget.Target{}, true},
		{"file URL", "file:///repo/main.go", "/repo/README.md", linktarget.Target{}, true},
		{"query", "next.md?x=y", "/repo/README.md", linktarget.Target{}, true},
		{"bad document", "next.md", "README.md", linktarget.Target{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := linktarget.ParsePreview(tt.raw, tt.document)
			if (err != nil) != tt.fail || got != tt.want {
				t.Fatalf("ParsePreview(%q, %q) = %#v, %v; want %#v, fail %v", tt.raw, tt.document, got, err, tt.want, tt.fail)
			}
		})
	}
}
