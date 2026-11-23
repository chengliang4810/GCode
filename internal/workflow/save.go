package workflow

import (
	"strings"

	"gcode/internal/contracts"
)

// SavedNameMaxChars 是保存下来的工作流名字的长度上限。名字就是落点的文件名，
// 所以它与文件名规则是同一套：字符集越界就是一次路径穿越。
const SavedNameMaxChars = 64

// SavedScopeProject 与 SavedScopeGlobal 是保存的两个落点。项目档写进本项目，
// 全局档写进本机的家目录——远端项目因此不提供全局档。
const (
	SavedScopeProject = "project"
	SavedScopeGlobal  = "global"
)

// ValidSavedName 报告一个名字能不能当保存的工作流名字。它只做前置回声：
// 真正的裁决仍在核心，这里只负责在用户按下按钮之前就把话说清楚。
func ValidSavedName(name string) bool {
	if name == "" || len(name) > SavedNameMaxChars {
		return false
	}
	dots := 0
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_', r == '-':
		case r == '.':
			dots++
		default:
			return false
		}
	}
	// 全是点的名字在文件系统里是 "." 与 ".."。
	return dots != len(name)
}

// SavedPathPreview 是保存落点的预览。项目档写相对路径，全局档写家目录路径：
// 用户要能一眼看出这份定义会落在哪儿。
func SavedPathPreview(scope, name string) string {
	if scope == SavedScopeGlobal {
		return "~/.gcode/workflows/" + name + ".json"
	}
	return ".gcode/workflows/" + name + ".json"
}

// SaveDraft 是保存表单在打开那一刻的草稿。
type SaveDraft struct {
	Name        string
	Scope       string
	Description string
}

// InitialSaveDraft 给保存表单一个起点。名字只在运行名本身合规则时才预填：
// 运行名是模型用用户的语言起的标签，塞进文件名框只会换来一次必然报错。
// 说明则反过来预填运行名——它正是「这个工作流是做什么的」那一句。
func InitialSaveDraft(runName, scope string) SaveDraft {
	name := strings.TrimSpace(runName)
	draft := SaveDraft{
		Scope:       SavedScopeProject,
		Description: name,
	}
	if ValidSavedName(name) {
		draft.Name = name
	}
	if scope == SavedScopeGlobal {
		draft.Scope = SavedScopeGlobal
	}
	return draft
}

// SavedDefinition 是一次运行保存下来的定义：它记着这次运行实际执行的那一份脚本。
// 直接保存读的是这份脚本，不经过模型回合。
func SavedDefinition(run contracts.WorkflowRun, draft SaveDraft) contracts.WorkflowDefinition {
	name := strings.TrimSpace(draft.Name)
	description := strings.TrimSpace(draft.Description)
	if description == "" {
		description = run.Name
	}
	return contracts.WorkflowDefinition{
		Name:        name,
		Description: description,
		Scope:       draft.Scope,
		ProjectID:   run.ProjectID,
		HostID:      run.HostID,
		Script:      run.Script,
		Program:     programOfRun(run),
	}
}

// programOfRun 把一次运行的阶段事实折回程序声明。保存写的是这次运行实际
// 执行的结构：阶段顺序、阶段内实例与阶段边。
func programOfRun(run contracts.WorkflowRun) contracts.WorkflowProgram {
	program := contracts.WorkflowProgram{}
	for _, phase := range run.Phases {
		spec := contracts.WorkflowPhaseSpec{ID: phase.ID, Name: phase.Name}
		for _, actor := range run.Actors {
			if actor.PhaseID != phase.ID {
				continue
			}
			spec.Instances = append(spec.Instances, contracts.WorkflowActorSpec{
				ID: actor.ID, Name: actor.Name, ModelID: actor.ModelID,
			})
		}
		for _, edge := range run.Edges {
			if edge.From != phase.ID {
				continue
			}
			spec.Edges = append(spec.Edges, edge)
		}
		program.Phases = append(program.Phases, spec)
	}
	return program
}

// LaunchRequest 是从一条已保存的定义再次运行时的请求。它不是 Resume：
// 新会话、新运行，只有定义身份与上一次的运行实参被沿用。
func LaunchRequest(run contracts.WorkflowRun, definition contracts.WorkflowDefinition, sessionID string) contracts.WorkflowRunRequest {
	return contracts.WorkflowRunRequest{
		DefinitionID: definition.ID,
		Name:         definition.Name,
		SessionID:    sessionID,
		ProjectID:    definition.ProjectID,
		HostID:       definition.HostID,
		Args:         append([]contracts.WorkflowArgValue(nil), run.Args...),
		Origin:       contracts.WorkflowOriginRerun,
	}
}

// SavedByScript 报告这次运行的脚本是否与某个已保存定义逐字相同。项目优先，
// 每个作用域最多看 limit 份：这是定位已保存事实的最后一层推断，不是全盘扫描。
func SavedByScript(run contracts.WorkflowRun, definitions []contracts.WorkflowDefinition, limit int) (contracts.WorkflowDefinition, bool) {
	if strings.TrimSpace(run.Script) == "" {
		return contracts.WorkflowDefinition{}, false
	}
	for _, scope := range []string{SavedScopeProject, SavedScopeGlobal} {
		scanned := 0
		for _, definition := range definitions {
			if definition.Scope != scope {
				continue
			}
			if scanned >= limit {
				break
			}
			scanned++
			if definition.Script == run.Script {
				return definition, true
			}
		}
	}
	return contracts.WorkflowDefinition{}, false
}
