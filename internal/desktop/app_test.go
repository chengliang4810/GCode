package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gcode/internal/appmodel"
	"github.com/alecthomas/chroma/v2"
	"github.com/egoist/mygo/ui"
)

type workspaceProbe struct {
	snapshotCalls chan string
	readCalls     chan string
	editorCalls   chan string
}

type desktopServiceStub struct {
	snapshotFn     func(context.Context) (appmodel.Snapshot, error)
	newSessionFn   func(context.Context, appmodel.NewSessionOptions) error
	sendFn         func(context.Context, string, string, appmodel.SendMode) error
	saveSettingsFn func(context.Context, appmodel.Settings) error
	saveProviderFn func(context.Context, appmodel.Provider, string) error
}

func (s *desktopServiceStub) Snapshot(ctx context.Context) (appmodel.Snapshot, error) {
	if s.snapshotFn != nil {
		return s.snapshotFn(ctx)
	}
	return appmodel.Snapshot{}, nil
}

func (s *desktopServiceStub) AddProject(context.Context, string) error { return nil }

func (s *desktopServiceStub) NewSession(ctx context.Context, options appmodel.NewSessionOptions) error {
	if s.newSessionFn != nil {
		return s.newSessionFn(ctx, options)
	}
	return nil
}

func (s *desktopServiceStub) Send(ctx context.Context, sessionID, text string, mode appmodel.SendMode) error {
	if s.sendFn != nil {
		return s.sendFn(ctx, sessionID, text, mode)
	}
	return nil
}

func (s *desktopServiceStub) Stop(context.Context, string) error                { return nil }
func (s *desktopServiceStub) Withdraw(context.Context, string, string) error    { return nil }
func (s *desktopServiceStub) ResumeQueue(context.Context, string) error         { return nil }
func (s *desktopServiceStub) SetProvider(context.Context, string, string) error { return nil }
func (s *desktopServiceStub) SetFullAccess(context.Context, string, bool) error { return nil }
func (s *desktopServiceStub) ReplyPermission(context.Context, string, string, string) error {
	return nil
}
func (s *desktopServiceStub) ReplyAsk(context.Context, string, string, []string) error {
	return nil
}
func (s *desktopServiceStub) SaveSettings(ctx context.Context, settings appmodel.Settings) error {
	if s.saveSettingsFn != nil {
		return s.saveSettingsFn(ctx, settings)
	}
	return nil
}
func (s *desktopServiceStub) SaveProvider(ctx context.Context, provider appmodel.Provider, key string) error {
	if s.saveProviderFn != nil {
		return s.saveProviderFn(ctx, provider, key)
	}
	return nil
}
func (s *desktopServiceStub) RemoveProvider(context.Context, string) error { return nil }
func (s *desktopServiceStub) Close(context.Context) error                  { return nil }

func waitForDesktopState(t *testing.T, description string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if condition() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", description)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (p *workspaceProbe) Snapshot(_ context.Context, cwd string) (WorkspaceSnapshot, error) {
	p.snapshotCalls <- cwd
	return WorkspaceSnapshot{Files: []appmodel.FileEntry{{Name: "main.go", Path: "main.go"}}}, nil
}

func (p *workspaceProbe) ReadFile(_ context.Context, cwd, path string) (appmodel.FilePreview, error) {
	p.readCalls <- filepath.Join(cwd, path)
	return appmodel.FilePreview{Path: path, Text: "package main"}, nil
}

func (p *workspaceProbe) OpenEditor(_ context.Context, cwd, path string) error {
	p.editorCalls <- filepath.Join(cwd, path)
	return nil
}

func TestVisualSampleRendersLightDarkAndNarrow(t *testing.T) {
	app := NewVisualSample()
	tt := ui.NewTester(app.view, 1280, 800)
	if !tt.HasText("示例数据") {
		t.Fatalf("visual sample marker missing: %v", tt.Texts())
	}
	if !tt.HasText("修复设置页保存错误") {
		t.Fatalf("workbench content missing: %v", tt.Texts())
	}
	if err := tt.Click("Diff"); err != nil {
		t.Fatalf("open diff panel: %v", err)
	}
	if tt.HasText("项目 Diff") {
		t.Fatalf("diff retained the redundant section header: %v", tt.Texts())
	}
	for _, text := range []string{"2 个文件", "+12", "−1"} {
		if !tt.HasText(text) {
			t.Fatalf("diff summary missing %q: %v", text, tt.Texts())
		}
	}
	assertPainted(t, tt.Image())

	tt.SetDark(true)
	tt.Frame()
	assertPainted(t, tt.Image())

	narrow := ui.NewTester(app.view, 860, 620)
	if !narrow.HasText("修复设置页保存错误") {
		t.Fatalf("narrow view lost conversation: %v", narrow.Texts())
	}
	assertPainted(t, narrow.Image())
	if err := tt.Click("设置"); err != nil {
		t.Fatalf("open settings: %v", err)
	}
	if !tt.HasText("模型服务") || !tt.HasText("发送方式") {
		t.Fatalf("settings content missing: %v", tt.Texts())
	}
	if err := tt.Click("返回工作台"); err != nil {
		t.Fatalf("close settings: %v", err)
	}
	app.Close()
}

func TestShortDiffCollapsePersistsAcrossFrames(t *testing.T) {
	app := NewVisualSample()
	t.Cleanup(func() { _ = app.Close() })
	tt := ui.NewTester(app.view, 1280, 800)
	if err := tt.Click("Diff"); err != nil {
		t.Fatalf("open diff panel: %v", err)
	}
	const path = "internal/settings/store.go"
	if err := tt.Click("Diff 文件 " + path); err != nil {
		t.Fatalf("collapse short diff: %v", err)
	}
	app.mu.RLock()
	open, remembered := app.diffExpanded[path]
	app.mu.RUnlock()
	if !remembered || open {
		t.Fatalf("short diff state = (%v, %v), want remembered false", open, remembered)
	}
	tt.Frame()
	app.mu.RLock()
	open, remembered = app.diffExpanded[path]
	app.mu.RUnlock()
	if !remembered || open {
		t.Fatalf("short diff reopened after redraw = (%v, %v)", open, remembered)
	}
}

func TestDiffLongLinesWrapWithinViewport(t *testing.T) {
	const tail = "TAIL_REACHABLE"
	lineText := strings.Repeat("value ", 70) + tail
	workspace := SampleWorkspaceSnapshot()
	workspace.Git.Changes = []appmodel.Change{{
		Path:   "internal/settings/long.go",
		Status: "M",
		Diff:   "@@ -1 +1 @@\n+" + lineText + "\n",
	}}
	app := Open(nil, OpenOptions{Initial: SampleSnapshot(), InitialWorkspace: workspace})
	app.right = rightDiff
	t.Cleanup(func() { _ = app.Close() })

	const windowWidth = 1280
	tt := ui.NewTester(app.view, windowWidth, 800)
	line, ok := tt.Find(lineText)
	if !ok {
		t.Fatalf("long diff line missing: %v", tt.Texts())
	}
	const rowWidth = float32(windowWidth - 240 - 590 - 24)
	if line.W > rowWidth+1 {
		t.Fatalf("long diff line overflowed its row: bounds=%#v rowWidth=%v", line, rowWidth)
	}
	if !tt.HasText(tail) {
		t.Fatalf("wrapped diff line lost its tail: %v", tt.Texts())
	}

	if line.H <= 24 {
		t.Fatalf("long diff line did not wrap: bounds=%#v", line)
	}
}

func TestShortDiffWrapUsesActualContentHeight(t *testing.T) {
	const tail = "TAIL_REACHABLE"
	lineText := strings.Repeat("value ", 70) + tail
	workspace := SampleWorkspaceSnapshot()
	workspace.Git.Changes = []appmodel.Change{
		{
			Path:   "internal/settings/long.go",
			Status: "M",
			Diff:   "@@ -1 +1 @@\n+" + lineText + "\n",
		},
		{
			Path:   "internal/settings/next.go",
			Status: "M",
			Diff:   "@@ -1 +1 @@\n+next\n",
		},
	}
	app := Open(nil, OpenOptions{Initial: SampleSnapshot(), InitialWorkspace: workspace})
	app.right = rightDiff
	t.Cleanup(func() { _ = app.Close() })

	tt := ui.NewTester(app.view, 1280, 800)
	line, ok := tt.Find(lineText)
	if !ok {
		t.Fatalf("short diff line missing: %v", tt.Texts())
	}
	if line.H <= 24 {
		t.Fatalf("short diff line did not wrap: bounds=%#v", line)
	}
	next, ok := tt.Find("Diff 文件 internal/settings/next.go")
	if !ok {
		t.Fatalf("second diff file missing: %v", tt.Texts())
	}
	if line.Y+line.H > next.Y {
		t.Fatalf("short diff line is clipped before the next file: line=%#v next=%#v", line, next)
	}
	if !tt.HasText(tail) {
		t.Fatalf("short diff tail is not reachable: %v", tt.Texts())
	}
}

func TestDiffHeaderSummaryPreservesGitStates(t *testing.T) {
	t.Run("non-git", func(t *testing.T) {
		app := Open(nil, OpenOptions{
			Initial:          SampleSnapshot(),
			InitialWorkspace: WorkspaceSnapshot{Git: appmodel.GitState{Branch: "non-git-marker"}},
		})
		app.right = rightDiff
		t.Cleanup(func() { _ = app.Close() })

		tt := ui.NewTester(app.view, 1280, 800)
		if !tt.HasText("不是 Git 项目") {
			t.Fatalf("non-Git state lost its empty view: %v", tt.Texts())
		}
		if tt.HasText("个文件") {
			t.Fatalf("non-Git state rendered a change summary: %v", tt.Texts())
		}
		if tt.HasText("non-git-marker") {
			t.Fatalf("non-Git branch leaked into the header: %v", tt.Texts())
		}
	})

	t.Run("clean-git", func(t *testing.T) {
		app := Open(nil, OpenOptions{
			Initial: SampleSnapshot(),
			InitialWorkspace: WorkspaceSnapshot{Git: appmodel.GitState{
				IsGit: true, Branch: "clean-git-marker",
			}},
		})
		app.right = rightDiff
		t.Cleanup(func() { _ = app.Close() })

		tt := ui.NewTester(app.view, 1280, 800)
		if !tt.HasText("工作区干净") {
			t.Fatalf("clean Git state lost its empty view: %v", tt.Texts())
		}
		if !tt.HasText("0 个文件") {
			t.Fatalf("clean Git state lost its zero-file summary: %v", tt.Texts())
		}
		if tt.HasText("clean-git-marker") {
			t.Fatalf("Git branch remained in the redundant header: %v", tt.Texts())
		}
	})
}

func TestDiffHeaderSummaryRefreshesChangedRaw(t *testing.T) {
	workspace := WorkspaceSnapshot{
		Git: appmodel.GitState{
			IsGit: true,
			Changes: []appmodel.Change{{
				Path:   "internal/settings/store.go",
				Status: "M",
				Diff:   "@@ -1 +1 @@\n-old\n+new\n",
			}},
		},
	}
	app := Open(nil, OpenOptions{Initial: SampleSnapshot(), InitialWorkspace: workspace})
	app.right = rightDiff
	t.Cleanup(func() { _ = app.Close() })

	tt := ui.NewTester(app.view, 1280, 800)
	if !tt.HasText("+1") || !tt.HasText("−1") {
		t.Fatalf("initial diff summary is wrong: %v", tt.Texts())
	}

	workspace.Git.Changes[0].Diff = "@@ -1 +1,2 @@\n-old\n+new\n+newer\n"
	app.UpdateWorkspace(workspace)
	tt.Frame()
	if !tt.HasText("+2") || !tt.HasText("−1") {
		t.Fatalf("updated diff summary is wrong: %v", tt.Texts())
	}
	app.mu.RLock()
	cached := app.diffCache["internal/settings/store.go"]
	app.mu.RUnlock()
	if cached == nil || cached.raw != workspace.Git.Changes[0].Diff || cached.added != 2 || cached.deleted != 1 {
		t.Fatalf("updated cached counts = %#v, want raw with +2/-1", cached)
	}
}

func TestLightCodePunctuationUsesReadableForeground(t *testing.T) {
	p := newPalette(false)
	for _, token := range []chroma.TokenType{chroma.Operator, chroma.Punctuation} {
		if got := syntaxColor(p, token); got != p.codeText {
			t.Fatalf("token %v color = %#v, want code foreground %#v", token, got, p.codeText)
		}
	}
	code := "func (a *App) saveSettings() error {\n\treturn store.Write(a.settings)\n}"
	spans, _ := highlightCodeSpans(p, "go", code)
	var rendered strings.Builder
	for _, span := range spans {
		rendered.WriteString(span.Text)
		if strings.ContainsAny(span.Text, "()*{}") && span.Color == p.code {
			t.Fatalf("punctuation span %q uses the code background color", span.Text)
		}
	}
	if rendered.String() != code {
		t.Fatalf("highlighting changed source text: %q", rendered.String())
	}
}

func TestToolStateLabelMapsRuntimeOKToCompleted(t *testing.T) {
	if got := stateLabelFromTool("ok"); got != "完成" {
		t.Fatalf("stateLabelFromTool(ok) = %q, want 完成", got)
	}
	if got := newPalette(false).success; got == newPalette(false).muted {
		t.Fatal("success tone must remain distinct from the waiting tone")
	}
}

func TestExecCommandArgumentsSummaryDecodesEscapedCommand(t *testing.T) {
	raw := `{"cmd":"printf '\u003e done\\n' && kill -s \u0003 $$","workdir":"/tmp/project","yield_time_ms":1000,"session_id":42}`
	got := toolArgumentsSummary(appmodel.ToolCall{Name: "运行测试", Arguments: raw})
	if !strings.Contains(got, "> done") {
		t.Fatalf("command summary did not decode JSON escapes: %q", got)
	}
	if !strings.Contains(got, "␃") {
		t.Fatalf("command summary hid the control character: %q", got)
	}
	if strings.Contains(got, "yield_time_ms") || strings.Contains(got, "session_id") {
		t.Fatalf("command summary exposed wire fields: %q", got)
	}
}

func TestExecCommandArgumentsCopyKeepsRawJSON(t *testing.T) {
	raw := `{"cmd":"printf '\u003e done\\n'","workdir":"/tmp/project","yield_time_ms":1000}`
	app := Open(nil, OpenOptions{Initial: appmodel.Snapshot{Sessions: []appmodel.Session{{
		ID: "tool-copy", Cwd: "/tmp/project", Messages: []appmodel.Message{{
			ID: "tool-message", Role: "assistant", Tools: []appmodel.ToolCall{{ID: "tool", Name: "运行测试", Arguments: raw, State: "ok"}},
		}},
	}}}})
	t.Cleanup(func() { _ = app.Close() })
	tt := ui.NewTester(app.view, 900, 640)
	if !tt.HasText("> done") {
		t.Fatalf("decoded command summary missing: %v", tt.Texts())
	}
	if err := tt.Click("复制原始参数"); err != nil {
		t.Fatalf("copy raw parameters: %v", err)
	}
	if got := tt.Clipboard(); got != raw {
		t.Fatalf("copied arguments = %q, want raw JSON %q", got, raw)
	}
}

func TestNarrowExpandedResourcePanelFitsAvailableWidth(t *testing.T) {
	if got, want := clampCenterWidth(1024, 244, 560), float32(488); got != want {
		t.Fatalf("clamped center width = %v, want %v", got, want)
	}
	app := NewVisualSample()
	t.Cleanup(func() { _ = app.Close() })
	tt := ui.NewTester(app.view, 1024, 700)
	if err := tt.Click("展开资源面板"); err != nil {
		t.Fatalf("expand resource panel: %v", err)
	}
	if err := tt.Click("文件"); err != nil {
		t.Fatalf("open files tab: %v", err)
	}
	panel, ok := tt.Find("工作区文件")
	if !ok {
		t.Fatalf("resource panel did not render: %v", tt.Texts())
	}
	if panel.X+panel.W > 1024 {
		t.Fatalf("resource panel overflows narrow window: %#v", panel)
	}
	pixel := tt.Image().At(1023, 250)
	r, g, b, _ := pixel.RGBA()
	want := newPalette(false).surfaceAlt
	wr, wg, wb := uint32(want.R)*257, uint32(want.G)*257, uint32(want.B)*257
	if r != wr || g != wg || b != wb {
		t.Fatalf("right edge is not filled by the resource panel: got %#v, want %#v", pixel, want)
	}
	if !tt.HasText("停止") {
		t.Fatalf("running composer lost its readable stop action: %v", tt.Texts())
	}
	if err := tt.Click("Diff"); err != nil {
		t.Fatalf("open diff tab in narrow panel: %v", err)
	}
	for _, text := range []string{"文件", "Diff", "终端", "收起资源面板"} {
		if !tt.HasText(text) {
			t.Fatalf("narrow diff header lost %q: %v", text, tt.Texts())
		}
	}
}

func TestNarrowSidebarKeepsBrandUnambiguous(t *testing.T) {
	app := NewVisualSample()
	t.Cleanup(func() { _ = app.Close() })
	tt := ui.NewTester(app.view, 1024, 700)
	if tt.HasText("GC...") {
		t.Fatalf("narrow sidebar ellipsized the brand: %v", tt.Texts())
	}
	if !tt.HasText("GCode") || !tt.HasText("G") {
		t.Fatalf("narrow sidebar lost the brand mark: %v", tt.Texts())
	}
}

func TestConversationVirtualizesLongSessions(t *testing.T) {
	const count = 5000
	app := Open(nil, OpenOptions{Initial: conversationTestSnapshot("long", count, 0)})
	t.Cleanup(func() { _ = app.Close() })

	tt := ui.NewTester(app.view, 900, 640)
	list := app.conversationListState("long")
	first, last := list.Visible()
	if last < first || last-first > 80 {
		t.Fatalf("long conversation built too many rows: %d..%d", first, last)
	}
	if first < count-80 {
		t.Fatalf("long conversation did not start at the end: first=%d count=%d", first, count)
	}
	if tt.HasText("消息 0000") {
		t.Fatal("the first message was built even though it is outside the viewport")
	}
	if !tt.HasText(fmt.Sprintf("消息 %04d", count-1)) {
		t.Fatal("the latest message was not built")
	}
}

func TestConversationFollowEndKeepsGrowingLastRowVisible(t *testing.T) {
	initial := conversationTestSnapshot("stream", 180, 0)
	app := Open(nil, OpenOptions{Initial: initial})
	t.Cleanup(func() { _ = app.Close() })
	tt := ui.NewTester(app.view, 900, 640)
	list := app.conversationListState("stream")
	if !list.AtEnd() {
		t.Fatal("conversation did not start following its end")
	}

	updated := cloneSnapshot(initial)
	updated.Sessions[0].Messages[len(updated.Sessions[0].Messages)-1].Text = strings.Repeat("流式输出继续增长。", 160)
	app.Update(updated)
	tt.Frame()
	if !list.AtEnd() {
		t.Fatal("streaming growth moved the list away from the end")
	}
}

func TestConversationScrollUpKeepsPositionWhenMessagesArrive(t *testing.T) {
	initial := conversationTestSnapshot("append", 220, 0)
	app := Open(nil, OpenOptions{Initial: initial})
	t.Cleanup(func() { _ = app.Close() })
	tt := ui.NewTester(app.view, 900, 640)
	list := app.conversationListState("append")
	tt.Move(420, 260)
	tt.Scroll(420, 260, 0, -700)
	beforeFirst, beforeLast := list.Visible()
	if list.AtEnd() {
		t.Fatal("scrolling up did not leave the conversation end")
	}

	updated := cloneSnapshot(initial)
	updated.Sessions[0].Messages = append(updated.Sessions[0].Messages,
		conversationTestMessages("append-new", 220, 4)...)
	app.Update(updated)
	tt.Frame()
	afterFirst, afterLast := list.Visible()
	if afterFirst != beforeFirst || afterLast != beforeLast {
		t.Fatalf("new messages stole the reading position: before=%d..%d after=%d..%d", beforeFirst, beforeLast, afterFirst, afterLast)
	}
	if list.AtEnd() {
		t.Fatal("new messages incorrectly restored end following after the user scrolled up")
	}
}

func TestConversationPositionsAreIndependentPerSession(t *testing.T) {
	firstSession := conversationTestSession("first", 220, 0)
	secondSession := conversationTestSession("second", 220, 0)
	app := Open(nil, OpenOptions{Initial: appmodel.Snapshot{Sessions: []appmodel.Session{firstSession, secondSession}}})
	t.Cleanup(func() { _ = app.Close() })
	tt := ui.NewTester(app.view, 900, 640)
	firstList := app.conversationListState("first")
	tt.Move(420, 260)
	tt.Scroll(420, 260, 0, -700)
	firstBefore, _ := firstList.Visible()
	if firstList.AtEnd() {
		t.Fatal("first session did not leave the end")
	}

	app.selectSession("second")
	tt.Frame()
	secondList := app.conversationListState("second")
	tt.Move(420, 260)
	tt.Scroll(420, 260, 0, -1100)
	secondBefore, _ := secondList.Visible()
	if secondList.AtEnd() {
		t.Fatal("second session did not leave the end")
	}

	app.selectSession("first")
	tt.Frame()
	firstAfter, _ := firstList.Visible()
	if firstAfter != firstBefore {
		t.Fatalf("switching sessions changed first reading position: before=%d after=%d", firstBefore, firstAfter)
	}
	app.selectSession("second")
	tt.Frame()
	secondAfter, _ := secondList.Visible()
	if secondAfter != secondBefore {
		t.Fatalf("switching sessions changed second reading position: before=%d after=%d", secondBefore, secondAfter)
	}
}

func TestConversationHistoryPrependKeepsReadingAnchor(t *testing.T) {
	initial := conversationTestSnapshot("history", 240, 0)
	app := Open(nil, OpenOptions{Initial: initial})
	t.Cleanup(func() { _ = app.Close() })
	tt := ui.NewTester(app.view, 900, 640)
	list := app.conversationListState("history")
	tt.Move(420, 260)
	tt.Scroll(420, 260, 0, -700)
	beforeFirst, _ := list.Visible()
	if list.AtEnd() {
		t.Fatal("history test did not leave the conversation end")
	}

	updated := cloneSnapshot(initial)
	older := conversationTestMessages("history-old", -20, 20)
	updated.Sessions[0].Messages = append(older, updated.Sessions[0].Messages...)
	app.Update(updated)
	tt.Frame()
	afterFirst, _ := list.Visible()
	if want := beforeFirst + len(older); afterFirst != want {
		t.Fatalf("history prepend moved the reading anchor: before=%d after=%d want=%d", beforeFirst, afterFirst, want)
	}
}

func conversationTestSnapshot(sessionID string, count, start int) appmodel.Snapshot {
	return appmodel.Snapshot{Sessions: []appmodel.Session{conversationTestSession(sessionID, count, start)}}
}

func conversationTestSession(sessionID string, count, start int) appmodel.Session {
	return appmodel.Session{ID: sessionID, Cwd: "/tmp/" + sessionID, Messages: conversationTestMessages(sessionID, start, count)}
}

func conversationTestMessages(prefix string, start, count int) []appmodel.Message {
	messages := make([]appmodel.Message, count)
	for i := range messages {
		index := start + i
		role := "assistant"
		if index%2 == 0 {
			role = "user"
		}
		messages[i] = appmodel.Message{
			ID:        fmt.Sprintf("%s-%05d", prefix, index),
			Role:      role,
			Text:      fmt.Sprintf("消息 %04d", index),
			CreatedAt: time.Unix(int64(index), 0),
		}
	}
	return messages
}

func TestNarrowWindowCollapsesResourcePanelUntilExplicitlyOpened(t *testing.T) {
	app := NewVisualSample()
	t.Cleanup(func() { _ = app.Close() })

	tt := ui.NewTester(app.view, 1024, 700)
	if tt.HasText("工作区文件") {
		t.Fatalf("narrow window rendered the resource panel: %v", tt.Texts())
	}
	if err := tt.Click("展开资源面板"); err != nil {
		t.Fatalf("expand resource panel: %v", err)
	}
	if err := tt.Click("文件"); err != nil {
		t.Fatalf("open files tab: %v", err)
	}
	if !tt.HasText("工作区文件") {
		t.Fatalf("explicit resource-panel toggle did not open the panel: %v", tt.Texts())
	}
	if err := tt.Click("刷新工作区"); err != nil {
		t.Fatalf("refresh workspace control missing: %v", err)
	}
}

func TestProviderSelectorUsesDisplayName(t *testing.T) {
	app := NewVisualSample()
	t.Cleanup(func() { _ = app.Close() })

	tt := ui.NewTester(app.view, 1280, 800)
	if !tt.HasText("OpenAI API") {
		t.Fatalf("provider selector did not show the provider name: %v", tt.Texts())
	}
	if tt.HasText("provider-openai") {
		t.Fatalf("provider selector exposed the internal provider ID: %v", tt.Texts())
	}
}

func TestNewSessionSelectsCreatedSessionAfterSuccess(t *testing.T) {
	project := appmodel.Project{ID: "project-1", Name: "GCode", Path: t.TempDir()}
	initial := appmodel.Snapshot{
		Projects: []appmodel.Project{project},
		Sessions: []appmodel.Session{{ID: "session-old", ProjectID: project.ID, Cwd: project.Path}},
	}
	created := appmodel.Session{ID: "session-new", ProjectID: project.ID, Cwd: project.Path}
	service := &desktopServiceStub{
		snapshotFn: func(context.Context) (appmodel.Snapshot, error) {
			snapshot := initial
			snapshot.Sessions = append(snapshot.Sessions, created)
			return snapshot, nil
		},
	}
	service.newSessionFn = func(_ context.Context, options appmodel.NewSessionOptions) error {
		if options.ProjectID != project.ID {
			t.Fatalf("new session project = %q, want %q", options.ProjectID, project.ID)
		}
		return nil
	}
	app := Open(service, OpenOptions{Initial: initial})
	t.Cleanup(func() { _ = app.Close() })

	app.newSession(project.ID)
	waitForDesktopState(t, "created session selection", func() bool {
		app.mu.RLock()
		defer app.mu.RUnlock()
		return app.activeSessionID == created.ID
	})
}

func TestSendKeepsInputWhenServiceRejects(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	service := &desktopServiceStub{
		sendFn: func(context.Context, string, string, appmodel.SendMode) error {
			close(started)
			<-release
			return errors.New("send failed")
		},
	}
	session := appmodel.Session{ID: "session-1", Cwd: t.TempDir()}
	app := Open(service, OpenOptions{Initial: appmodel.Snapshot{Sessions: []appmodel.Session{session}}})
	t.Cleanup(func() { _ = app.Close() })

	app.mu.Lock()
	app.input = "保留这条消息"
	app.mu.Unlock()
	app.send("保留这条消息", appmodel.Correction)
	<-started
	app.mu.RLock()
	if app.input != "保留这条消息" {
		app.mu.RUnlock()
		t.Fatalf("input was cleared before send completed: %q", app.input)
	}
	app.mu.RUnlock()
	close(release)
	waitForDesktopState(t, "send error", func() bool {
		app.mu.RLock()
		defer app.mu.RUnlock()
		return app.errorText != ""
	})
	app.mu.RLock()
	defer app.mu.RUnlock()
	if app.input != "保留这条消息" {
		t.Fatalf("failed send lost input: %q", app.input)
	}
}

func TestSaveSettingsUpdatesDirtyStateOnlyAfterMatchingSuccess(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	service := &desktopServiceStub{
		saveSettingsFn: func(context.Context, appmodel.Settings) error {
			close(started)
			<-release
			return nil
		},
	}
	app := Open(service, OpenOptions{Initial: appmodel.Snapshot{Settings: appmodel.Settings{Theme: "system"}}})
	t.Cleanup(func() { _ = app.Close() })

	app.updateSettingsDraft(func(draft *SettingsDraft) { draft.Theme = "dark" })
	app.saveSettings()
	<-started
	app.updateSettingsDraft(func(draft *SettingsDraft) { draft.Theme = "light" })
	close(release)
	waitForDesktopState(t, "stale settings save completion", func() bool {
		app.mu.RLock()
		defer app.mu.RUnlock()
		return app.data.Settings.Theme == "light"
	})
	app.mu.RLock()
	if !app.settingsDirty || app.settingsDraft.Theme != "light" {
		app.mu.RUnlock()
		t.Fatalf("newer settings draft was cleared: dirty=%v theme=%q", app.settingsDirty, app.settingsDraft.Theme)
	}
	app.mu.RUnlock()
}

func TestSaveSettingsKeepsDirtyStateAfterFailure(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	service := &desktopServiceStub{
		saveSettingsFn: func(context.Context, appmodel.Settings) error {
			close(started)
			<-release
			return errors.New("save failed")
		},
	}
	app := Open(service, OpenOptions{Initial: appmodel.Snapshot{Settings: appmodel.Settings{Theme: "system"}}})
	t.Cleanup(func() { _ = app.Close() })

	app.updateSettingsDraft(func(draft *SettingsDraft) { draft.Theme = "dark" })
	app.saveSettings()
	<-started
	app.mu.RLock()
	if !app.settingsDirty {
		app.mu.RUnlock()
		t.Fatal("settings became clean before save completed")
	}
	app.mu.RUnlock()
	close(release)
	waitForDesktopState(t, "settings save error", func() bool {
		app.mu.RLock()
		defer app.mu.RUnlock()
		return app.errorText != ""
	})
	app.mu.RLock()
	defer app.mu.RUnlock()
	if !app.settingsDirty || app.settingsDraft.Theme != "dark" {
		t.Fatalf("failed settings save discarded draft: dirty=%v theme=%q", app.settingsDirty, app.settingsDraft.Theme)
	}
}

func TestSaveProviderRefreshesKeyWithoutReplacingNewerDraft(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	initialProvider := appmodel.Provider{
		ID: "provider-1", Name: "保存前名称", Format: appmodel.ChatCompletions,
		BaseURL: "https://before.example/v1", Model: "before-model",
	}
	serverSnapshot := appmodel.Snapshot{Settings: appmodel.Settings{
		DefaultProviderID: "provider-1",
		Providers: []appmodel.Provider{{
			ID: "provider-1", Name: "服务器名称", Format: appmodel.ChatCompletions,
			BaseURL: "https://server.example/v1", Model: "server-model", HasAPIKey: true,
		}},
	}}
	var app *App
	var savedProvider appmodel.Provider
	var savedKey string
	service := &desktopServiceStub{}
	service.saveProviderFn = func(_ context.Context, provider appmodel.Provider, key string) error {
		savedProvider = provider
		savedKey = key
		close(started)
		<-release
		// SaveProvider publishes this controller snapshot after the durable save.
		app.Update(serverSnapshot)
		close(finished)
		return nil
	}
	app = Open(service, OpenOptions{Initial: appmodel.Snapshot{Settings: appmodel.Settings{
		DefaultProviderID: "provider-1", Theme: "system", DefaultSendMode: appmodel.Correction,
		Providers: []appmodel.Provider{initialProvider},
	}}})
	t.Cleanup(func() { _ = app.Close() })

	app.updateSettingsDraft(func(draft *SettingsDraft) {
		draft.Provider.Name = "保存时名称"
		draft.APIKey = "saved-key"
	})
	app.saveProvider()
	<-started
	app.updateSettingsDraft(func(draft *SettingsDraft) {
		draft.Provider.Name = "较新名称"
		draft.Provider.BaseURL = "https://newer.example/v1"
		draft.Provider.Model = "newer-model"
		draft.APIKey = "较新的输入"
		draft.Theme = "dark"
		draft.SendMode = appmodel.Queue
	})
	close(release)
	<-finished

	waitForDesktopState(t, "provider credential marker refresh", func() bool {
		app.mu.RLock()
		defer app.mu.RUnlock()
		return app.settingsDraft.Provider.HasAPIKey
	})
	app.mu.RLock()
	draft := app.settingsDraft
	data := app.data
	app.mu.RUnlock()
	if savedProvider.Name != "保存时名称" || savedKey != "saved-key" {
		t.Fatalf("save used unexpected draft: provider=%#v key=%q", savedProvider, savedKey)
	}
	if draft.Provider.Name != "较新名称" || draft.Provider.BaseURL != "https://newer.example/v1" || draft.Provider.Model != "newer-model" {
		t.Fatalf("newer provider draft was replaced: %#v", draft.Provider)
	}
	if draft.APIKey != "较新的输入" || draft.Theme != "dark" || draft.SendMode != appmodel.Queue {
		t.Fatalf("newer settings draft was replaced: key=%q theme=%q mode=%q", draft.APIKey, draft.Theme, draft.SendMode)
	}
	if !draft.Provider.HasAPIKey || !data.Settings.Providers[0].HasAPIKey {
		t.Fatalf("saved provider credential marker did not refresh: draft=%v data=%v", draft.Provider.HasAPIKey, data.Settings.Providers[0].HasAPIKey)
	}
}

func TestSaveProviderFailureKeepsDraftAndDoesNotRefreshSnapshot(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	snapshotCalled := make(chan struct{}, 1)
	service := &desktopServiceStub{
		snapshotFn: func(context.Context) (appmodel.Snapshot, error) {
			snapshotCalled <- struct{}{}
			return appmodel.Snapshot{}, nil
		},
	}
	service.saveProviderFn = func(context.Context, appmodel.Provider, string) error {
		close(started)
		<-release
		return errors.New("save provider failed")
	}
	app := Open(service, OpenOptions{Initial: appmodel.Snapshot{Settings: appmodel.Settings{
		DefaultProviderID: "provider-1", Providers: []appmodel.Provider{{ID: "provider-1", Name: "原名称"}},
	}}})
	t.Cleanup(func() { _ = app.Close() })

	app.updateSettingsDraft(func(draft *SettingsDraft) {
		draft.Provider.Name = "待保存名称"
		draft.APIKey = "待保存密钥"
	})
	app.saveProvider()
	<-started
	app.updateSettingsDraft(func(draft *SettingsDraft) { draft.Provider.Name = "失败期间的新名称" })
	close(release)
	waitForDesktopState(t, "provider save error", func() bool {
		app.mu.RLock()
		defer app.mu.RUnlock()
		return app.errorText != ""
	})
	select {
	case <-snapshotCalled:
		t.Fatal("failed provider save unexpectedly refreshed the snapshot")
	default:
	}
	app.mu.RLock()
	defer app.mu.RUnlock()
	if app.settingsDraft.Provider.Name != "失败期间的新名称" || app.settingsDraft.APIKey != "待保存密钥" {
		t.Fatalf("failed provider save discarded draft: %#v key=%q", app.settingsDraft.Provider, app.settingsDraft.APIKey)
	}
	if app.settingsDraft.Provider.HasAPIKey {
		t.Fatal("failed provider save reported a credential marker")
	}
}

func TestProviderSnapshotKeepsDifferentSelectedDraft(t *testing.T) {
	initial := appmodel.Snapshot{Settings: appmodel.Settings{
		DefaultProviderID: "provider-1",
		Providers:         []appmodel.Provider{{ID: "provider-1", Name: "服务一"}, {ID: "provider-2", Name: "服务二"}},
	}}
	app := Open(nil, OpenOptions{Initial: initial})
	t.Cleanup(func() { _ = app.Close() })
	app.selectProvider("provider-2")
	app.updateSettingsDraft(func(draft *SettingsDraft) {
		draft.Provider.Name = "服务二的新草稿"
		draft.APIKey = "服务二的新输入"
		draft.Theme = "dark"
		draft.SendMode = appmodel.Queue
	})
	app.Update(appmodel.Snapshot{Settings: appmodel.Settings{
		DefaultProviderID: "provider-1",
		Providers:         []appmodel.Provider{{ID: "provider-1", Name: "服务一", HasAPIKey: true}, {ID: "provider-2", Name: "服务二"}},
	}})
	app.mu.RLock()
	defer app.mu.RUnlock()
	if app.settingsDraft.ProviderID != "provider-2" || app.settingsDraft.Provider.Name != "服务二的新草稿" || app.settingsDraft.APIKey != "服务二的新输入" {
		t.Fatalf("different selected provider draft was replaced: %#v key=%q", app.settingsDraft.Provider, app.settingsDraft.APIKey)
	}
	if app.settingsDraft.Theme != "dark" || app.settingsDraft.SendMode != appmodel.Queue {
		t.Fatalf("different selected settings draft was replaced: theme=%q mode=%q", app.settingsDraft.Theme, app.settingsDraft.SendMode)
	}
	if !app.data.Settings.Providers[0].HasAPIKey {
		t.Fatal("matching provider marker did not refresh in the server data")
	}
}

func TestWorkspaceNavigationPreviewAndEditorStayWithinSessionRoot(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "src")
	probe := &workspaceProbe{
		snapshotCalls: make(chan string, 4),
		readCalls:     make(chan string, 2),
		editorCalls:   make(chan string, 2),
	}
	app := Open(nil, OpenOptions{
		Initial:   appmodel.Snapshot{Sessions: []appmodel.Session{{ID: "session-1", Cwd: root}}},
		Workspace: probe,
	})
	app.enterDirectory(nested)
	if got := <-probe.snapshotCalls; got != nested {
		t.Fatalf("snapshot cwd = %q, want %q", got, nested)
	}
	app.readWorkspaceFile("main.go")
	if got := <-probe.readCalls; got != filepath.Join(nested, "main.go") {
		t.Fatalf("read path = %q, want %q", got, filepath.Join(nested, "main.go"))
	}
	app.openWorkspaceFile("main.go")
	if got := <-probe.editorCalls; got != filepath.Join(nested, "main.go") {
		t.Fatalf("editor path = %q, want %q", got, filepath.Join(nested, "main.go"))
	}
	app.enterDirectory(filepath.Join(root, ".."))
	if got := app.errorText; got == "" {
		t.Fatal("expected path escape to report an error")
	}
	_ = app.Close()
}

func TestWorkspaceRefreshesAfterToolCompletionAndKeepsPreviewSelection(t *testing.T) {
	root := t.TempDir()
	probe := &workspaceProbe{
		snapshotCalls: make(chan string, 4),
		readCalls:     make(chan string, 4),
		editorCalls:   make(chan string, 1),
	}
	running := appmodel.Session{
		ID:    "session-1",
		Cwd:   root,
		State: appmodel.Running,
		Messages: []appmodel.Message{{
			ID: "assistant-1", Role: "assistant", Tools: []appmodel.ToolCall{{ID: "tool-1", State: "running"}},
		}},
	}
	initial := appmodel.Snapshot{Sessions: []appmodel.Session{running}}
	app := Open(nil, OpenOptions{
		Initial:   initial,
		Workspace: probe,
		InitialWorkspace: WorkspaceSnapshot{Preview: &appmodel.FilePreview{
			Path: "main.go", Text: "old contents",
		}},
	})
	t.Cleanup(func() { _ = app.Close() })
	app.mu.Lock()
	app.workspacePreviewPath = filepath.Join(root, "main.go")
	app.mu.Unlock()

	completed := cloneSnapshot(initial)
	completed.Sessions[0].Messages[0].Tools[0].State = "ok"
	completed.Sessions[0].Messages[0].Tools[0].Output = "updated"
	app.Update(completed)

	select {
	case got := <-probe.snapshotCalls:
		if got != root {
			t.Fatalf("workspace refresh cwd = %q, want %q", got, root)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("tool completion did not refresh the workspace")
	}
	select {
	case got := <-probe.readCalls:
		if got != filepath.Join(root, "main.go") {
			t.Fatalf("preview refresh path = %q, want %q", got, filepath.Join(root, "main.go"))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("workspace refresh dropped the selected preview")
	}

	app.mu.RLock()
	defer app.mu.RUnlock()
	if app.workspaceData.Preview == nil || app.workspaceData.Preview.Path != "main.go" {
		t.Fatalf("preview selection was lost during workspace refresh: %#v", app.workspaceData.Preview)
	}
}

func TestWorkspaceSnapshotRefreshSkipsStreamingText(t *testing.T) {
	previous := appmodel.Snapshot{Sessions: []appmodel.Session{{
		ID: "session-1", State: appmodel.Running,
		Messages: []appmodel.Message{{Tools: []appmodel.ToolCall{{ID: "tool-1", State: "running"}}}},
	}}}
	current := cloneSnapshot(previous)
	current.Sessions[0].Messages[0].Text = "token"
	if workspaceSnapshotNeedsRefresh(previous, current, "session-1") {
		t.Fatal("streaming assistant text should not trigger a workspace scan")
	}

	current = cloneSnapshot(previous)
	current.Sessions[0].Messages[0].Tools[0].State = "ok"
	if !workspaceSnapshotNeedsRefresh(previous, current, "session-1") {
		t.Fatal("completed tool result should trigger a workspace scan")
	}
}

func TestComposerSendModeOverrideSurvivesSnapshotAndResetsOnSessionChange(t *testing.T) {
	first := appmodel.Session{ID: "session-1", Cwd: t.TempDir()}
	second := appmodel.Session{ID: "session-2", Cwd: t.TempDir()}
	initial := appmodel.Snapshot{
		Settings: appmodel.Settings{DefaultSendMode: appmodel.Correction},
		Sessions: []appmodel.Session{first, second},
	}
	app := Open(nil, OpenOptions{Initial: initial})

	// This is the state transition performed by the composer segmented control.
	app.sendMode = appmodel.Queue
	app.sendModeDirty = true
	app.Update(appmodel.Snapshot{
		Settings: appmodel.Settings{DefaultSendMode: appmodel.Correction},
		Sessions: []appmodel.Session{first, second},
	})
	if app.sendMode != appmodel.Queue {
		t.Fatalf("snapshot reset transient send mode to %q", app.sendMode)
	}

	app.Update(appmodel.Snapshot{
		Settings: appmodel.Settings{DefaultSendMode: appmodel.Queue},
		Sessions: []appmodel.Session{first, second},
	})
	if app.sendMode != appmodel.Queue {
		t.Fatalf("global default changed transient mode: %q", app.sendMode)
	}
	app.selectSession(second.ID)
	if app.sendMode != appmodel.Queue || app.sendModeDirty {
		t.Fatalf("session switch did not reset to current default: mode=%q dirty=%v", app.sendMode, app.sendModeDirty)
	}

	app.sendModeDirty = false
	app.Update(appmodel.Snapshot{
		Settings: appmodel.Settings{DefaultSendMode: appmodel.Correction},
		Sessions: []appmodel.Session{first, second},
	})
	if app.sendMode != appmodel.Correction {
		t.Fatalf("non-overridden mode did not follow global default: %q", app.sendMode)
	}
	_ = app.Close()
}

func TestAPIKeyInputIsMaskedAndAbsentFromSnapshot(t *testing.T) {
	const secret = "sk-desktop-test-secret"
	snapshot := appmodel.Snapshot{
		Settings: appmodel.Settings{
			DefaultProviderID: "provider-1",
			DefaultSendMode:   appmodel.Correction,
			Providers:         []appmodel.Provider{{ID: "provider-1", Name: "Test", Model: "test-model"}},
		},
	}
	app := Open(nil, OpenOptions{Initial: snapshot})
	app.settingsDraft.APIKey = secret
	app.openSettings()
	tt := ui.NewTester(app.view, 1000, 780)
	texts := strings.Join(tt.Texts(), "\n")
	if strings.Contains(texts, secret) {
		t.Fatalf("API key appeared in rendered text: %q", texts)
	}
	if strings.Contains(strings.Join(tt.Texts(), ""), secret) {
		t.Fatalf("API key appeared in the UI text tree")
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	if strings.Contains(string(encoded), secret) {
		t.Fatal("API key appeared in appmodel snapshot")
	}
	_ = app.Close()
}

func assertPainted(t *testing.T, img *image.RGBA) {
	t.Helper()
	if img == nil || img.Bounds().Empty() {
		t.Fatal("empty rendered image")
	}
	var nonZero int
	for i := 3; i < len(img.Pix); i += 4 {
		alpha := img.Pix[i]
		if alpha != 0 {
			nonZero++
			if nonZero > 32 {
				return
			}
		}
	}
	t.Fatal("rendered image is blank")
}
