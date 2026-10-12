package desktop

import (
	"strings"
	"testing"

	"gcode/internal/appmodel"
	"github.com/egoist/mygo/ui"
)

func TestPermissionCardShowsCompleteDecodedCommand(t *testing.T) {
	command := "printf '> 已批准\\n' > result.txt\n" + strings.Repeat("echo '继续检查中文参数'\n", 120)
	raw := `{"cmd":"printf '\u003e 已批准\\n' \u003e result.txt\n` + strings.Repeat(`echo '继续检查中文参数'\n`, 120) + `","workdir":"/tmp/编码项目","sandbox_permissions":"require_escalated","yield_time_ms":1000}`
	app := Open(nil, OpenOptions{Initial: appmodel.Snapshot{Sessions: []appmodel.Session{{
		ID: "permission-readable", Cwd: "/tmp/编码项目", State: appmodel.WaitingPermission,
		Permissions: []appmodel.Permission{{ID: "permission", Tool: "exec_command", Arguments: raw,
			Options: []appmodel.PermissionOption{{ID: "allow", Label: "允许本次"}, {ID: "deny", Label: "拒绝"}}}},
	}}}})
	t.Cleanup(func() { _ = app.Close() })
	tt := ui.NewTester(app.view, 1000, 700)
	if !tt.HasText(command + "\n工作目录：/tmp/编码项目") {
		t.Fatalf("complete decoded command or directory missing: %v", tt.Texts())
	}
	if tt.HasText("sandbox_permissions") || tt.HasText("yield_time_ms") || tt.HasText(`\u003e`) {
		t.Fatalf("permission card exposed protocol JSON: %v", tt.Texts())
	}
	if !tt.HasText("允许本次") || !tt.HasText("拒绝") {
		t.Fatalf("permission choices missing: %v", tt.Texts())
	}
	if button, ok := tt.Find("允许本次"); !ok || button.Y+button.H > 700 {
		t.Fatalf("long command moved permission choice outside viewport: %#v", button)
	}
	app.mu.RLock()
	got := app.data.Sessions[0].Permissions[0].Arguments
	app.mu.RUnlock()
	if got != raw {
		t.Fatalf("presentation changed executable parameters: %q", got)
	}
}

func TestInterruptedToolPresentationKeepsFailureAndDiagnosticDistinct(t *testing.T) {
	for _, tc := range []struct {
		name        string
		interrupted bool
		output      string
		wantFailure bool
	}{
		{"expected cancellation", true, "Error: Permission denied for exec_command: context canceled", false},
		{"ordinary failure", false, "Error: command exited with code 1", true},
		{"failure before interruption", true, "Error: command exited with code 1", true},
		{"unexpected cancellation", false, "Error: context canceled", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := Open(nil, OpenOptions{Initial: appmodel.Snapshot{Sessions: []appmodel.Session{{
				ID: "tool-presentation", Messages: []appmodel.Message{{
					ID: "message", Role: "assistant", Interrupted: tc.interrupted,
					Tools: []appmodel.ToolCall{{ID: "tool", Name: "exec_command", State: "error", Output: tc.output}},
				}},
			}}}})
			t.Cleanup(func() { _ = app.Close() })
			tt := ui.NewTester(app.view, 1000, 700)
			if got := tt.HasText("失败"); got != tc.wantFailure {
				t.Fatalf("failure label = %v, want %v: %v", got, tc.wantFailure, tt.Texts())
			}
			if !tc.wantFailure {
				if !tt.HasText("已中断") || tt.HasText(tc.output) {
					t.Fatalf("expected interruption should be neutral and initially folded: %v", tt.Texts())
				}
				if err := tt.Click("工具 exec_command"); err != nil {
					t.Fatal(err)
				}
				if !tt.HasText(tc.output) {
					t.Fatalf("original cancellation diagnostic not available after expansion: %v", tt.Texts())
				}
			} else if !tt.HasText(tc.output) {
				t.Fatalf("real failure should remain expanded: %v", tt.Texts())
			}
		})
	}
}
