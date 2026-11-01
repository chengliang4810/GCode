package workbench

import (
	"testing"
	"time"
)

func TestTabStripDetailUsesBackingResource(t *testing.T) {
	s, _, snapshot := testWorkspace(t)
	for _, file := range snapshot.Files {
		if !file.ReadOnly && file.Kind != "directory" && file.Kind != "image" {
			s.OpenFile(file.ID)
			if got := s.tabDetail(s.tabs[s.active]); got != file.Path {
				t.Fatalf("file detail%q differs from resource path%q", got, file.Path)
			}
			break
		}
	}
	s.OpenBrowser("https://example.com/actual-resource")
	browserTab := s.tabs[s.active]
	b := s.browsers[browserTab.id]
	if b == nil {
		t.Fatal("browser resource did not open")
	}
	b.address = "https://example.com/uncommitted-draft"
	if got := s.tabDetail(browserTab); got != b.data.URL {
		t.Errorf("browser detail%q differs from current resource URL%q", got, b.data.URL)
	}
	for _, kind := range []string{"session", "history", "diff", "changes", "terminal", "files"} {
		t.Run(kind, func(t *testing.T) {
			if got := s.tabDetail(surface{kind: kind, id: "unrelated", title: "display title is not detail"}); got != "" {
				t.Fatalf("surface without source detail generated tooltip%q", got)
			}
		})
	}
	if got := s.tabDetail(surface{kind: "file", id: "missing", title: "missing file"}); got != "" {
		t.Fatal("removed file generated fallback detail")
	}
}

func TestTabStripGenericTitleDoesNotBecomeTooltip(t *testing.T) {
	for _, kind := range []string{"session", "history", "diff", "changes", "terminal", "files"} {
		t.Run(kind, func(t *testing.T) {
			s, _, _ := testWorkspace(t)
			s.choose(kind, "eligibility-test", "Only the displayed title")
			tt := tabStripTester(s, 360, false)
			chip, ok := tt.Find("Only the displayed title")
			if !ok {
				t.Fatal("test chip did not render")
			}
			before := len(tt.Texts())
			tt.Move(chip.X+3, chip.Y+chip.H/2)
			tt.Advance(time.Second)
			if after := len(tt.Texts()); after != before {
				t.Fatalf("title-only chip added tooltip text after hover: before%d after%d", before, after)
			}
			if s.active != 0 || s.tabs[0].kind != kind {
				t.Fatal("hover changed the selected surface")
			}
		})
	}
}

func TestTabStripResourceDetailStillAppearsAtMatureHover(t *testing.T) {
	for _, kind := range []string{"file", "browser"} {
		t.Run(kind, func(t *testing.T) {
			s, _, snapshot := testWorkspace(t)
			if kind == "file" {
				for _, file := range snapshot.Files {
					if !file.ReadOnly && file.Kind != "directory" && file.Kind != "image" {
						s.OpenFile(file.ID)
						break
					}
				}
			} else {
				s.OpenBrowser("https://example.com/actual-resource")
				s.browsers[s.tabs[s.active].id].address = "https://example.com/uncommitted-draft"
			}
			if len(s.tabs) != 1 {
				t.Fatal("detail-bearing resource did not open")
			}
			detail := s.tabDetail(s.tabs[0])
			if detail == "" {
				t.Fatal("resource has no actual detail")
			}
			tt := tabStripTester(s, 360, false)
			chip, _ := tt.Find(s.tabs[0].title)
			before := len(tt.Texts())
			tt.Move(chip.X+3, chip.Y+chip.H/2)
			// This mature state preserves eligibility; it does not assert the
			// source's pending350ms delay or pointer-relative placement.
			tt.Advance(time.Second)
			if len(tt.Texts()) <= before || !tt.HasText(detail) {
				t.Fatalf("eligible%s detail did not appear at mature hover: %v", kind, tt.Texts())
			}
			if kind == "browser" && tt.HasText("uncommitted-draft") {
				t.Fatal("browser draft address appeared instead of current URL")
			}
		})
	}
}
