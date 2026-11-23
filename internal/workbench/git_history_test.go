package workbench

import (
	"context"
	"errors"
	"fmt"
	"gcode/internal/contracts"
	localize "gcode/internal/i18n/view"
	"github.com/egoist/mygo/ui"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type historyTestService struct {
	contracts.Service
	requests []contracts.GitHistoryRequest
	result   contracts.GitHistoryResult
	err      error
}

func (s *historyTestService) ListGitHistory(_ context.Context, r contracts.GitHistoryRequest) (contracts.GitHistoryResult, error) {
	s.requests = append(s.requests, r)
	return s.result, s.err
}

func TestGitHistorySurfaceLoadsRefreshesAndScopes(t *testing.T) {
	s, backend, snapshot := testWorkspace(t)
	service := &historyTestService{Service: backend, result: contracts.GitHistoryResult{Repository: true, Commits: []contracts.Commit{{SHA: "1234567890", Subject: "real host commit", Author: "host author", AuthoredAt: time.Now(), Refs: []string{"HEAD -> main"}}}}}
	s.backend = service
	snapshot.Sessions = []contracts.Session{{ID: "one", CWD: "/host/one"}, {ID: "two", CWD: "/host/two"}}
	s.Update(snapshot)
	s.SwitchSession("one")
	s.OpenSurface("history")
	if len(service.requests) != 1 || service.requests[0].SessionID != "one" {
		t.Fatalf("wrong scope: %+v", service.requests)
	}
	tt := ui.NewTester(s.historyView, 900, 600)
	if !tt.HasText("real host commit") || !tt.HasText("host author") {
		t.Fatalf("missing real data: %v", tt.Texts())
	}
	if err := tt.Click("Refresh"); err != nil {
		t.Fatal(err)
	}
	if len(service.requests) != 2 {
		t.Fatal("refresh did not query host")
	}
	s.Update(snapshot)
	if len(service.requests) != 2 {
		t.Fatal("unrelated snapshot reloaded git")
	}
	s.historySearch = "not present"
	tt.Frame()
	if !tt.HasText("No matching commits.") {
		t.Fatal("missing search empty state")
	}
	s.SwitchSession("two")
	service.result.Commits[0].Subject = "second repository"
	s.OpenSurface("history")
	tt.Frame()
	if len(service.requests) != 3 || service.requests[2].SessionID != "two" || !tt.HasText("second repository") {
		t.Fatal("switch retained previous repository")
	}
	service.err = errors.New("git error sentinel")
	s.refreshGitHistory(true)
	tt.Frame()
	if !tt.HasText("Could not load Git history") || !tt.HasText("git error sentinel") {
		t.Fatal("load error hidden")
	}
	service.err = nil
	service.result = contracts.GitHistoryResult{}
	s.refreshGitHistory(true)
	tt.Frame()
	if !tt.HasText("The current directory is not a Git repository.") {
		t.Fatal("non-repo state missing")
	}
	service.result.Repository = true
	s.refreshGitHistory(true)
	tt.Frame()
	if !tt.HasText("This repository has no commits yet.") {
		t.Fatal("empty repo state missing")
	}
}

func TestGitHistoryThemesWidthsAndDetails(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, width := range []int{420, 1100} {
			for _, language := range []string{"en", "zh-CN"} {
				t.Run(fmt.Sprintf("dark=%t/%d/%s", dark, width, language), func(t *testing.T) {
					s, _, _ := testWorkspace(t)
					s.gitHistory.result = contracts.GitHistoryResult{Repository: true, Commits: []contracts.Commit{{SHA: "1234567890abcdef", Subject: "真实提交标题", Author: "提交作者", AuthoredAt: time.Now(), Refs: []string{"HEAD -> main"}}}}
					tt := ui.NewTester(func(c *ui.Context) {
						localize.Set(c, language)
						theme := ui.LightTheme()
						if dark {
							theme = ui.DarkTheme()
						}
						c.SetTheme(theme)
						ui.Column(c).Fill().Children(func() { s.historyView(c) })
					}, width, 600)
					if !tt.HasText("真实提交标题") || !tt.HasText("提交作者") {
						t.Fatalf("history not rendered: %v", tt.Texts())
					}
					if err := tt.Click("真实提交标题"); err != nil {
						t.Fatal(err)
					}
					tt.Frame()
					if !s.commitOpen || !tt.HasText("1234567890abcdef") {
						t.Fatal("commit details did not open")
					}
					if dir := os.Getenv("GCODE_HISTORY_EVIDENCE"); dir != "" {
						if err := os.MkdirAll(dir, 0755); err != nil {
							t.Fatal(err)
						}
						f, err := os.Create(filepath.Join(dir, fmt.Sprintf("history-%t-%d-%s.png", dark, width, language)))
						if err != nil {
							t.Fatal(err)
						}
						defer f.Close()
						if err := png.Encode(f, tt.Image()); err != nil {
							t.Fatal(err)
						}
					}
				})
			}
		}
	}
}
