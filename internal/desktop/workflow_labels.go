package desktop

import (
	"fmt"
	"strings"

	"gcode/internal/workflow"
	"github.com/egoist/mygo/ui"
)

// workflowPhaseLabel 是阶段在界面上的名字：脚本写了名字就用它，没写就用
// 阶段在脚本里的序号。内部 id 不当作名字。
func workflowPhaseLabel(c *ui.Context, phase workflow.RunPhaseView) string {
	if name := strings.TrimSpace(phase.Name); name != "" {
		return name
	}
	return fmt.Sprintf(tr(c, "Phase %d"), phase.Index+1)
}

// workflowStationLabel 是时间线上一站的名字。留白没写标题时用所属阶段的
// 序号，让人知道它长在哪一段；连所属阶段都找不到时就说它是一处留白。
func workflowStationLabel(c *ui.Context, station workflow.TimelineStation) string {
	if name := strings.TrimSpace(station.Name); name != "" {
		return name
	}
	if station.PhaseOrdinal > 0 {
		return fmt.Sprintf(tr(c, "Phase %d"), station.PhaseOrdinal)
	}
	return tr(c, "Hole")
}

// workflowActorLabel 是实例的显示名：脚本没写名字就用它在阶段里的序号。
func workflowActorLabel(c *ui.Context, order int, name string) string {
	if name = strings.TrimSpace(name); name != "" {
		return name
	}
	return fmt.Sprintf(tr(c, "Agent %d"), order)
}

// workflowRunLabel 是一次运行在界面上的名字：没有名字时说它的状态。
func (a *App) workflowRunLabel(c *ui.Context, view workflow.RunView) string {
	if name := strings.TrimSpace(view.Name); name != "" {
		return name
	}
	return a.workflowRunKindWord(c, view)
}
