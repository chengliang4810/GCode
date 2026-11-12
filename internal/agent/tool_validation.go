package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gcode/internal/model"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
)

// PrepareToolArguments validates before any tool effect. Only Agent's explicitly
// supported string booleans are normalized; other fields retain their JSON types.
func PrepareToolArguments(def model.Tool, raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("%s 参数必须是有效 JSON；请纠正后重试", def.Name)
	}
	normalized := false
	if def.Name == "Agent" {
		if fields, ok := value.(map[string]any); ok {
			if text, ok := fields["run_in_background"].(string); ok {
				switch text {
				case "true":
					fields["run_in_background"] = true
					normalized = true
				case "false":
					fields["run_in_background"] = false
					normalized = true
				}
			}
			if v, exists := fields["run_in_background"]; exists {
				if _, ok := v.(bool); !ok {
					return nil, errors.New("Agent.run_in_background 必须为布尔值 true/false（也接受字符串 \"true\"/\"false\"）；请纠正后重试")
				}
			}
		}
	}
	if len(def.Parameters) > 0 {
		// Definitions contain typed Go slices; round-trip to JSON values for
		// the schema compiler. External refs must never read host files or URLs.
		encoded, err := json.Marshal(def.Parameters)
		if err != nil {
			return nil, fmt.Errorf("%s 工具 Schema 无法编码", def.Name)
		}
		schemaValue, err := jsonschema.UnmarshalJSON(bytes.NewReader(encoded))
		if err != nil {
			return nil, fmt.Errorf("%s 工具 Schema 无法解析", def.Name)
		}
		compiler := jsonschema.NewCompiler()
		compiler.UseLoader(jsonschema.SchemeURLLoader{})
		const location = "urn:gcode:tool"
		if err := compiler.AddResource(location, schemaValue); err != nil {
			return nil, fmt.Errorf("%s 工具 Schema 无法加载", def.Name)
		}
		schema, err := compiler.Compile(location)
		if err != nil {
			return nil, fmt.Errorf("%s 工具 Schema 无效", def.Name)
		}
		if err := schema.Validate(value); err != nil {
			var validation *jsonschema.ValidationError
			if errors.As(err, &validation) {
				for len(validation.Causes) > 0 {
					validation = validation.Causes[0]
				}
				path := strings.Join(validation.InstanceLocation, ".")
				if expected, ok := validation.ErrorKind.(*kind.Type); ok {
					return nil, fmt.Errorf("%s.%s 参数类型错误，必须为 %s；请纠正后重试", def.Name, path, strings.Join(expected.Want, "/"))
				}
				return nil, fmt.Errorf("%s.%s 参数不符合 Schema 的 %s 约束；请纠正后重试",
					def.Name, path, strings.Join(validation.ErrorKind.KeywordPath(), "/"))
			}
			return nil, fmt.Errorf("%s 参数不符合工具 Schema；请纠正后重试", def.Name)
		}
	}
	// Preserve the original bytes unless the Agent compatibility rule applied.
	if normalized {
		return json.Marshal(value)
	}
	return raw, nil
}
