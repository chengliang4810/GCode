package desktop

import (
	"context"
	"testing"
	"time"

	"gcode/internal/appmodel"
	"github.com/egoist/mygo/ui"
)

func TestProjectTreeGroupsAndIndependentlyCollapsesConversations(t *testing.T) {
	app := NewVisualSample()
	t.Cleanup(func() { _ = app.Close() })
	tt := ui.NewTester(app.view, 1440, 900)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})
	project, ok := tt.Find("项目 GCode")
	if !ok {
		t.Fatal("project disclosure missing")
	}
	child, ok := tt.Find("会话 整理工作区文档")
	if !ok || child.Y <= project.Y+project.H || child.X <= project.X {
		t.Fatalf("conversation is not nested below its project: project=%#v child=%#v", project, child)
	}
	nextProject, _ := tt.Find("项目 Atlas Notes")
	lastChild, _ := tt.Find("会话 检查会话恢复与流式更新")
	if nextProject.Y < lastChild.Y+lastChild.H {
		t.Fatal("next project appears before the previous project's children")
	}
	if err := tt.Click("项目 GCode"); err != nil {
		t.Fatal(err)
	}
	app.Update(SampleSnapshot())
	tt.Frame()
	if _, exists := tt.Find("会话 整理工作区文档"); exists {
		t.Fatal("snapshot refresh reopened the collapsed project")
	}
	if _, exists := tt.Find("会话 重排知识库索引"); !exists {
		t.Fatal("collapsing one project hid another project's conversations")
	}
	tt.Key(0, ui.KeyEnter)
	if _, exists := tt.Find("会话 整理工作区文档"); !exists {
		t.Fatal("Enter did not reopen the focused project disclosure")
	}
	if err := tt.Click("会话 重排知识库索引"); err != nil {
		t.Fatal(err)
	}
	app.mu.RLock()
	activeID, cwd := app.activeSessionID, app.workspaceRoot
	app.mu.RUnlock()
	if activeID != "session-atlas" || cwd != "/Users/demo/Projects/atlas-notes" {
		t.Fatalf("cross-project selection = %q, %q", activeID, cwd)
	}
}

func TestProjectTreeCreatesConversationInClickedProject(t *testing.T) {
	initial := SampleSnapshot()
	called := make(chan appmodel.NewSessionOptions, 1)
	service := &desktopServiceStub{
		newSessionFn: func(_ context.Context, options appmodel.NewSessionOptions) error {
			called <- options
			return nil
		},
		snapshotFn: func(context.Context) (appmodel.Snapshot, error) { return initial, nil },
	}
	app := Open(service, OpenOptions{Initial: initial})
	t.Cleanup(func() { _ = app.Close() })
	tt := ui.NewTester(app.view, 1440, 900)
	if err := tt.Click("新会话 · Atlas Notes"); err != nil {
		t.Fatal(err)
	}
	select {
	case options := <-called:
		if options.ProjectID != "project-atlas" {
			t.Fatalf("created in %q instead of the clicked project", options.ProjectID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("project action did not dispatch NewSession")
	}
}
