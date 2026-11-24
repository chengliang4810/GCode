package workflow

// 全局模式与会话激活的纯规则。模式是默认值，不是上限：
// 用户可以在会话里把按需模式打开一次并一直保留。

import "gcode/internal/contracts"

// NormalizeMode 把设置里读到的值收敛成三个合法模式之一。
// 空值和未知值都读成按需模式，因为那是最保守的可用默认。
func NormalizeMode(value string) string {
	switch value {
	case contracts.WorkflowModeDisabled:
		return contracts.WorkflowModeDisabled
	case contracts.WorkflowModeAlwaysOn:
		return contracts.WorkflowModeAlwaysOn
	default:
		return contracts.WorkflowModeOnDemand
	}
}

// Available 报告会话里能不能用工作流能力。按需模式要等用户先激活；
// 激活之后一直有效，压缩上下文不会撤销它。
func Available(mode string, activated bool) bool {
	switch NormalizeMode(mode) {
	case contracts.WorkflowModeDisabled:
		return false
	case contracts.WorkflowModeAlwaysOn:
		return true
	default:
		return activated
	}
}

// ActivatesOnCommand 报告一个动作是否会让按需模式在本会话生效。
// 触发激活的是用户动作：执行 /workflow、再次运行、恢复运行、改设置。
func ActivatesOnCommand() bool { return true }
