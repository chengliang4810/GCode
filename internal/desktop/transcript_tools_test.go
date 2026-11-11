package desktop

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/design"
	localize "gcode/internal/i18n/view"
	"github.com/egoist/mygo/ui"
)

func TestSemanticToolsRenderReadableActivity(t *testing.T) {
	budget := int64(1000)
	for _, dark := range []bool{false, true} {
		for _, language := range []string{"en", "zh-CN"} {
			for _, width := range []int{420, 900} {
				name := fmt.Sprintf("%s-dark-%t-%d", language, dark, width)
				t.Run(name, func(t *testing.T) {
					a := testApp(t, "default")
					a.palette = design.Palette("rcode", dark)
					a.state.Settings.General.Language = language
					cases := []struct {
						label, chinese, want string
						tool                 contracts.ToolCall
					}{
						{"Stop task", "停止任务", "go test ./...", contracts.ToolCall{
							Kind: contracts.ToolTaskStop, Name: "TaskStop", Summary: "后台校验",
							TaskID: "task-check", TaskStatus: "killed", Command: "go test ./...", Args: `{"task_id":"task-check"}`,
						}},
						{"Stop task", "停止任务", "", contracts.ToolCall{
							Kind: contracts.ToolTaskStop, Name: "TaskStop", Summary: "子任务",
							TaskID: "child-check", TaskStatus: "stopping", Args: `{"task_id":"child-check"}`,
						}},
						{"Use skill", "使用技能", "Review src", contracts.ToolCall{
							Kind: contracts.ToolSkill, Name: "Skill", Summary: "review-code", Command: "Review src",
							Args: `{"skill":"review-code","args":"Review src"}`, Output: "Skill instructions",
						}},
						{"Read goal", "读取目标", "核对工具展示", contracts.ToolCall{
							Kind: contracts.ToolGoalRead, Name: "goal_read", Summary: "核对工具展示", Args: `{}`,
							GoalSnapshot: &contracts.SessionGoal{
								Objective: "核对工具展示", Status: contracts.GoalActive, TokensUsed: 25,
								TokenBudget: &budget, NextAction: "核对窄窗",
							},
						}},
						{"List directory", "列出目录", "README.md", contracts.ToolCall{
							Kind: contracts.ToolDirectory, Name: "ListDirectory", Summary: "src", Path: "/workspace/src",
							Args: `{"path":"src"}`, Output: "README.md\nmain.go",
						}},
						{"Read web page", "读取网页", "Documentation contents", contracts.ToolCall{
							Kind: contracts.ToolWebFetch, Name: "WebFetch", Summary: "https://example.test/docs",
							Args: `{"url":"https://example.test/docs"}`, Output: "Documentation contents",
						}},
						{"Search web", "搜索网页", "Documentation result", contracts.ToolCall{
							Kind: contracts.ToolWebSearch, Name: "WebSearch", Summary: "Go documentation",
							Args: `{"query":"Go documentation"}`, Output: "Documentation result",
						}},
						{"Call tool", "调用工具", "File contents", contracts.ToolCall{
							Kind: contracts.ToolMCP, Name: "mcp__files__read_text", Summary: "文件服务 · 读取文本",
							Args: `{"path":"README.md"}`, Output: "File contents",
						}},
						{"Start agent", "启动智能体", "provider is unavailable", contracts.ToolCall{
							Kind: contracts.ToolAgent, Name: "Agent", Summary: "调查界面",
							Args:    `{"description":"调查界面","prompt":"Inspect controls"}`,
							IsError: true, Output: "provider is unavailable",
						}},
					}
					tt := ui.NewTester(func(c *ui.Context) {
						localize.Set(c, language)
						c.SetTheme(a.palette.UI())
						c.DisableAnimations()
						c.Root().Background(a.palette.Background)
						ui.Column(c).Fill().Padding(24).Children(func() {
							for i, test := range cases {
								tool := test.tool
								tool.Status = "completed"
								id := fmt.Sprintf("semantic-%d", i)
								a.activityItem(c, id, contracts.Block{ID: id, Kind: "tool", Tool: &tool}, true, false, activityRowMotion{settled: true})
							}
						})
					}, width, 1400)
					for _, test := range cases {
						label := test.label
						if language == "zh-CN" {
							label = test.chinese
						}
						if err := tt.Click(label + ": " + test.tool.Summary); err != nil {
							t.Fatal(err)
						}
						if test.want != "" && !tt.HasText(test.want) {
							t.Fatalf("lost readable detail %q: %v", test.want, tt.Texts())
						}
					}
					stopped, requested := "Task status: Stopped", "Task status: Stop requested"
					if language == "zh-CN" {
						stopped, requested = "任务状态：已停止", "任务状态：已请求停止"
					}
					if !tt.HasText(stopped) || !tt.HasText(requested) {
						t.Fatal("stop request was confused with completed termination")
					}
					for _, text := range tt.Texts() {
						if strings.Contains(text, "mcp__") || strings.Contains(text, `"task_id"`) || strings.Contains(text, `"query"`) {
							t.Fatalf("raw tool identity or known parameters leaked: %q", text)
						}
					}
					if directory := os.Getenv("RCODE_TOOL_LABEL_EVIDENCE"); directory != "" {
						if err := os.MkdirAll(directory, 0755); err != nil {
							t.Fatal(err)
						}
						file, err := os.Create(filepath.Join(directory, name+".png"))
						if err != nil {
							t.Fatal(err)
						}
						if err := png.Encode(file, tt.Image()); err != nil {
							file.Close()
							t.Fatal(err)
						}
						if err := file.Close(); err != nil {
							t.Fatal(err)
						}
					}
				})
			}
		}
	}
}

func TestGoalDetailShowsEmptyStateAndPreservesReadErrors(t *testing.T) {
	tool := contracts.ToolCall{Kind: contracts.ToolGoalRead, Name: "goal_read", Status: "completed", Args: `{}`}
	var source activityDetailSource
	ui.NewTester(func(c *ui.Context) {
		source = readableDetailSource(c, contracts.Block{Tool: &tool}, false)
	}, 1, 1)
	if source.command != "No active goal" || source.args != "" {
		t.Fatal("no-goal result still looks like protocol data")
	}
	tool.IsError, tool.Output = true, "cannot read goal"
	ui.NewTester(func(c *ui.Context) {
		source = readableDetailSource(c, contracts.Block{Tool: &tool}, false)
	}, 1, 1)
	if source.output != "cannot read goal" || source.args != "{}" {
		t.Fatal("failed read concealed its error or original parameters")
	}
}
