package contracts

import (
	"strings"
	"testing"
)

func TestPromptReferenceRoundTrip(t *testing.T) {
	refs := []PromptReference{
		{Kind: "command", ID: "init", Label: "Init"},
		{Kind: "skill", ID: "documents", Label: "文档 🙂"},
		{Kind: "mcp", ID: "local-tools", Label: "本机工具"},
		{Kind: "skill", ID: "escaped", Label: "[文档]\\ *name* & <正文>"},
	}
	text := "你好 "
	for _, ref := range refs {
		text += ref.Markdown() + " "
	}
	parts := ParsePrompt(text)
	found := 0
	for _, part := range parts {
		if part.Reference != nil {
			if *part.Reference != refs[found] {
				t.Fatalf("identity changed: %+v", part.Reference)
			}
			found++
		}
	}
	if found != len(refs) || PlainPrompt(text) != "你好 /init @skill:documents @mcp:local-tools @skill:escaped " {
		t.Fatalf("lost mixed content: %q %+v", PlainPrompt(text), parts)
	}
}

func TestPromptReferencesRejectInvalidDestinations(t *testing.T) {
	for _, destination := range []string{"gcode://skill/..%2Fsecret", "gcode://unknown/id", "https://example.invalid/id", "gcode://skill/a%20b"} {
		t.Run(destination, func(t *testing.T) {
			text := "[label](" + destination + ")"
			for _, part := range ParsePrompt(text) {
				if part.Reference != nil {
					t.Fatal("invalid reference was accepted")
				}
			}
			if PlainPrompt(text) != text {
				t.Fatal("ordinary text was changed")
			}
		})
	}
	ref := PromptReference{Kind: "skill", ID: "docs", Label: "bad]\nlabel"}
	if strings.Contains(ref.Markdown(), "\n") {
		t.Fatal("label escaped its token")
	}
}
