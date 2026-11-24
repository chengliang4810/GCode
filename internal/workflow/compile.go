package workflow

// 工作流脚本的编译入口。脚本是程序，图只是它的投影：编译负责把脚本文本
// 变成核心能执行的阶段程序，并在同一次调用里给出诊断。诊断与程序一起
// 返回，调用方既知道能不能跑，也知道跑起来是什么形状。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"gcode/internal/contracts"
)

// ScriptIDPattern 约束脚本里声明的身份。身份会进入事实路径、稳定键和
// 文件名，因此只允许字母、数字、下划线和连字符。
var ScriptIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// ArgumentTypes 是脚本可以声明的参数类型。
var ArgumentTypes = []string{"string", "number", "boolean", "json"}

// EdgeKinds 是阶段边允许的走向。next 是默认值。
var EdgeKinds = []string{"next", "branch", "loop"}

// Compiled 是脚本编译的结果。Program 只在没有错误诊断时可用。
type Compiled struct {
	Name        string
	Description string
	Program     contracts.WorkflowProgram
	Arguments   []contracts.WorkflowArgument
	Diagnostics []contracts.WorkflowDiagnostic
}

// Failed 报告编译是否不可用。警告不阻止运行。
func (c Compiled) Failed() bool {
	return slices.ContainsFunc(c.Diagnostics, func(diagnostic contracts.WorkflowDiagnostic) bool {
		return diagnostic.Severity == contracts.WorkflowDiagnosticError
	})
}

// ErrorText 返回第一条错误诊断；没有错误时返回空串。
func (c Compiled) ErrorText() string {
	for _, diagnostic := range c.Diagnostics {
		if diagnostic.Severity == contracts.WorkflowDiagnosticError {
			return diagnostic.Message
		}
	}
	return ""
}

// Report 把诊断拼成给模型看的多行文本。没有诊断时返回空串。
func (c Compiled) Report() string {
	if len(c.Diagnostics) == 0 {
		return ""
	}
	lines := make([]string, 0, len(c.Diagnostics))
	for _, diagnostic := range c.Diagnostics {
		line := diagnostic.Severity + ": " + diagnostic.Message
		if diagnostic.Path != "" {
			line = diagnostic.Severity + " at " + diagnostic.Path + ": " + diagnostic.Message
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// PhaseCount 是脚本声明了多少个阶段。诊断阶段也用它说明规模。
func (c Compiled) PhaseCount() int {
	return len(c.Program.Phases)
}

// ActorCount 是脚本声明的实例总数。
func (c Compiled) ActorCount() int {
	count := 0
	for _, phase := range c.Program.Phases {
		count += len(phase.Instances)
	}
	return count
}

// CompileScript 把脚本文本编译成阶段程序。一个脚本只有一份阶段声明，
// 阶段按声明顺序执行，边只描述走向与条件。
func CompileScript(source string) Compiled {
	var compiled Compiled
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return compiled.fail("empty_script", "", "The workflow script is empty.")
	}
	var document map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader([]byte(trimmed)))
	decoder.UseNumber()
	if err := decoder.Decode(&document); err != nil {
		return compiled.fail("invalid_json", "", "The workflow script must be a JSON object: "+err.Error())
	}
	if err := ensureEOF(decoder); err != nil {
		return compiled.fail("invalid_json", "", err.Error())
	}
	warnUnknownKeys(&compiled, document, "", []string{"name", "description", "arguments", "phases"})
	compiled.Name, _ = stringField(document, "name")
	compiled.Description, _ = stringField(document, "description")
	compiled.Arguments = compileArguments(&compiled, document["arguments"])
	compiled.Program = compilePhases(&compiled, document["phases"])
	return compiled
}

// fail 记录一条错误诊断。编译遇到无法继续的问题时先返回再继续收集。
func (c Compiled) fail(code, path, message string) Compiled {
	c.Diagnostics = append(c.Diagnostics, contracts.WorkflowDiagnostic{
		Severity: contracts.WorkflowDiagnosticError, Code: code, Path: path, Message: message,
	})
	return c
}

func (c *Compiled) add(severity, code, path, message string) {
	c.Diagnostics = append(c.Diagnostics, contracts.WorkflowDiagnostic{Severity: severity, Code: code, Path: path, Message: message})
}

func (c *Compiled) errorf(code, path, format string, args ...any) {
	c.add(contracts.WorkflowDiagnosticError, code, path, fmt.Sprintf(format, args...))
}

func (c *Compiled) warnf(code, path, format string, args ...any) {
	c.add(contracts.WorkflowDiagnosticWarning, code, path, fmt.Sprintf(format, args...))
}

// ensureEOF 拒绝脚本后面跟着的第二份 JSON 文档，避免静默丢内容。
func ensureEOF(decoder *json.Decoder) error {
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err == nil {
		return fmt.Errorf("the workflow script has more than one JSON document")
	}
	return nil
}

// warnUnknownKeys 把不认识的字段报成警告。写错字段名通常意味着意思落空。
func warnUnknownKeys(compiled *Compiled, document map[string]json.RawMessage, prefix string, known []string) {
	for key := range document {
		if slices.Contains(known, key) {
			continue
		}
		compiled.warnf("unknown_field", joinPath(prefix, key), "Unknown field %q is ignored.", key)
	}
}

// joinPath 拼接诊断位置。
func joinPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

// stringField 读取可选字符串字段。类型不对时留空并由调用方报告。
func stringField(document map[string]json.RawMessage, key string) (string, bool) {
	raw, ok := document[key]
	if !ok || len(raw) == 0 || string(raw) == "null" {
		return "", false
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	return value, true
}

// compileArguments 编译参数声明。类型与名称在这里一次性校验。
func compileArguments(compiled *Compiled, raw json.RawMessage) []contracts.WorkflowArgument {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		compiled.errorf("invalid_arguments", "arguments", "The arguments must be a list.")
		return nil
	}
	arguments := make([]contracts.WorkflowArgument, 0, len(entries))
	seen := map[string]bool{}
	for index, entry := range entries {
		path := fmt.Sprintf("arguments[%d]", index)
		document, err := objectValue(entry)
		if err != nil {
			compiled.errorf("invalid_argument", path, "%s", err.Error())
			continue
		}
		warnUnknownKeys(compiled, document, path, []string{"name", "type", "description", "required", "default"})
		name, _ := stringField(document, "name")
		if name == "" {
			compiled.errorf("argument_name_missing", path, "Every argument needs a name.")
			continue
		}
		if !ScriptIDPattern.MatchString(name) {
			compiled.errorf("argument_name_invalid", joinPath(path, "name"), "Argument names use 1 to 64 letters, digits, underscores or dashes.")
			continue
		}
		if seen[name] {
			compiled.errorf("argument_name_duplicate", joinPath(path, "name"), "Argument %q is declared twice.", name)
			continue
		}
		seen[name] = true
		kind := "string"
		if value, ok := stringField(document, "type"); ok {
			kind = value
		}
		if !slices.Contains(ArgumentTypes, kind) {
			compiled.errorf("argument_type_invalid", joinPath(path, "type"), "Argument %q has type %q; use string, number, boolean or json.", name, kind)
			continue
		}
		argument := contracts.WorkflowArgument{Name: name, Type: kind}
		argument.Description, _ = stringField(document, "description")
		_ = json.Unmarshal(document["required"], &argument.Required)
		if defaultValue, ok := stringField(document, "default"); ok {
			argument.Default, argument.HasDefault = defaultValue, true
		}
		arguments = append(arguments, argument)
	}
	return arguments
}

// objectValue 把一段 JSON 解成对象。数组或标量在这里就是写错了。
func objectValue(raw json.RawMessage) (map[string]json.RawMessage, error) {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("expected an object: %w", err)
	}
	if document == nil {
		return nil, fmt.Errorf("expected an object, found null")
	}
	return document, nil
}

// compilePhases 编译阶段、实例与边。程序至少要有一个阶段。
func compilePhases(compiled *Compiled, raw json.RawMessage) contracts.WorkflowProgram {
	if len(raw) == 0 || string(raw) == "null" {
		compiled.errorf("phases_missing", "phases", "A workflow needs at least one phase.")
		return contracts.WorkflowProgram{}
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		compiled.errorf("invalid_phases", "phases", "The phases must be a list.")
		return contracts.WorkflowProgram{}
	}
	if len(entries) == 0 {
		compiled.errorf("phases_missing", "phases", "A workflow needs at least one phase.")
		return contracts.WorkflowProgram{}
	}
	phases := make([]contracts.WorkflowPhaseSpec, 0, len(entries))
	seen := map[string]bool{}
	for index, entry := range entries {
		path := fmt.Sprintf("phases[%d]", index)
		document, err := objectValue(entry)
		if err != nil {
			compiled.errorf("invalid_phase", path, "%s", err.Error())
			continue
		}
		warnUnknownKeys(compiled, document, path, []string{"id", "name", "instances", "edges"})
		id, _ := stringField(document, "id")
		if id == "" {
			compiled.errorf("phase_id_missing", path, "Every phase needs an id.")
			continue
		}
		if !ScriptIDPattern.MatchString(id) {
			compiled.errorf("phase_id_invalid", joinPath(path, "id"), "Phase ids use 1 to 64 letters, digits, underscores or dashes.")
			continue
		}
		if seen[id] {
			compiled.errorf("phase_id_duplicate", joinPath(path, "id"), "Phase %q is declared twice.", id)
			continue
		}
		seen[id] = true
		phase := contracts.WorkflowPhaseSpec{ID: id}
		// 名字保持脚本写下的样子：没写就是空，界面用阶段序号称呼它，
		// 不拿内部 id 顶替名字。
		phase.Name, _ = stringField(document, "name")
		phase.Instances = compileInstances(compiled, document["instances"], id, joinPath(path, "instances"))
		phase.Edges = compileEdges(compiled, document["edges"], id, joinPath(path, "edges"))
		phases = append(phases, phase)
	}
	// 边的目标可能在后面才声明，因此等所有阶段都收集完再校验一次。
	validateEdgeTargets(compiled, phases)
	return contracts.WorkflowProgram{Phases: phases}
}

// compileInstances 编译阶段内的实例。实例身份只在阶段内唯一。
func compileInstances(compiled *Compiled, raw json.RawMessage, phaseID, path string) []contracts.WorkflowActorSpec {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		compiled.errorf("invalid_instances", path, "The instances of phase %q must be a list.", phaseID)
		return nil
	}
	instances := make([]contracts.WorkflowActorSpec, 0, len(entries))
	seen := map[string]bool{}
	for index, entry := range entries {
		entryPath := fmt.Sprintf("%s[%d]", path, index)
		document, err := objectValue(entry)
		if err != nil {
			compiled.errorf("invalid_instance", entryPath, "%s", err.Error())
			continue
		}
		warnUnknownKeys(compiled, document, entryPath, []string{"id", "name", "prompt", "modelId", "static"})
		id, _ := stringField(document, "id")
		if id == "" {
			compiled.errorf("instance_id_missing", entryPath, "Every instance needs an id.")
			continue
		}
		if !ScriptIDPattern.MatchString(id) {
			compiled.errorf("instance_id_invalid", joinPath(entryPath, "id"), "Instance ids use 1 to 64 letters, digits, underscores or dashes.")
			continue
		}
		if seen[id] {
			compiled.errorf("instance_id_duplicate", joinPath(entryPath, "id"), "Instance %q is declared twice in phase %q.", id, phaseID)
			continue
		}
		seen[id] = true
		instance := contracts.WorkflowActorSpec{ID: id}
		// 同上：没写名字就留空，界面用实例序号称呼它。
		instance.Name, _ = stringField(document, "name")
		instance.Prompt, _ = stringField(document, "prompt")
		instance.ModelID, _ = stringField(document, "modelId")
		instance.Static, _ = stringField(document, "static")
		instances = append(instances, instance)
	}
	return instances
}

// compileEdges 编译阶段边。From 省略时就是声明它的阶段。
func compileEdges(compiled *Compiled, raw json.RawMessage, phaseID, path string) []contracts.WorkflowEdge {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		compiled.errorf("invalid_edges", path, "The edges of phase %q must be a list.", phaseID)
		return nil
	}
	edges := make([]contracts.WorkflowEdge, 0, len(entries))
	for index, entry := range entries {
		entryPath := fmt.Sprintf("%s[%d]", path, index)
		document, err := objectValue(entry)
		if err != nil {
			compiled.errorf("invalid_edge", entryPath, "%s", err.Error())
			continue
		}
		warnUnknownKeys(compiled, document, entryPath, []string{"from", "to", "kind", "label", "when"})
		edge := contracts.WorkflowEdge{From: phaseID, Kind: "next"}
		if from, ok := stringField(document, "from"); ok && from != "" {
			edge.From = from
		}
		to, _ := stringField(document, "to")
		if to == "" {
			compiled.errorf("edge_target_missing", joinPath(entryPath, "to"), "Every edge needs a target phase.")
			continue
		}
		edge.To = to
		if kind, ok := stringField(document, "kind"); ok && kind != "" {
			edge.Kind = kind
		}
		if !slices.Contains(EdgeKinds, edge.Kind) {
			compiled.errorf("edge_kind_invalid", joinPath(entryPath, "kind"), "Edge kind %q is unknown; use next, branch or loop.", edge.Kind)
			continue
		}
		edge.Label, _ = stringField(document, "label")
		if when, ok := stringField(document, "when"); ok && strings.TrimSpace(when) != "" {
			condition, err := ParseCondition(when)
			if err != nil {
				compiled.errorf("edge_condition_invalid", joinPath(entryPath, "when"), "%s", err.Error())
				continue
			}
			edge.When = condition.Text()
		}
		edges = append(edges, edge)
	}
	return edges
}

// validateEdgeTargets 校验边的两端都是已经声明的阶段。
func validateEdgeTargets(compiled *Compiled, phases []contracts.WorkflowPhaseSpec) {
	known := map[string]bool{}
	for _, phase := range phases {
		known[phase.ID] = true
	}
	for index, phase := range phases {
		for edgeIndex, edge := range phase.Edges {
			path := fmt.Sprintf("phases[%d].edges[%d]", index, edgeIndex)
			if !known[edge.From] {
				compiled.errorf("edge_source_unknown", joinPath(path, "from"), "Edge %q starts at unknown phase %q.", edge.From, edge.To)
			}
			if !known[edge.To] {
				compiled.errorf("edge_target_unknown", joinPath(path, "to"), "Edge %q points at unknown phase %q.", edge.From, edge.To)
			}
			if edge.From == edge.To && edge.Kind != "loop" {
				compiled.warnf("edge_self", path, "Edge from %q to itself is only meaningful as a loop.", edge.From)
			}
		}
	}
}
