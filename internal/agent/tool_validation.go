package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"gcode/internal/model"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
)

const compiledToolSchemaCacheLimit = 64

var compiledToolSchemas = struct {
	sync.Mutex
	entries map[string]*jsonschema.Schema
	order   []string
}{entries: make(map[string]*jsonschema.Schema, compiledToolSchemaCacheLimit)}

// toolArgumentValidators hold the rules a tool explains in its own terms.
// They run before the schema check, so the model reads the specific reason
// instead of a generic schema violation; the schema still declares and
// enforces every constraint it advertises in the tool definition.
var toolArgumentValidators = map[string]func(json.RawMessage) error{
	"TodoWrite": validateTodoArguments,
}

func compileToolSchema(name string, parameters map[string]any) (*jsonschema.Schema, error) {
	// JSON encoding gives mutable definitions a content key, so a changed
	// definition cannot reuse a stale compiled schema. The small FIFO bound
	// keeps dynamically supplied tool definitions from growing this process
	// global without limit.
	encoded, err := json.Marshal(parameters)
	if err != nil {
		return nil, fmt.Errorf("%s tool schema cannot be encoded", name)
	}
	key := string(encoded)
	compiledToolSchemas.Lock()
	schema := compiledToolSchemas.entries[key]
	compiledToolSchemas.Unlock()
	if schema != nil {
		return schema, nil
	}

	schemaValue, err := jsonschema.UnmarshalJSON(bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("%s tool schema cannot be parsed", name)
	}
	compiler := jsonschema.NewCompiler()
	compiler.UseLoader(jsonschema.SchemeURLLoader{})
	const location = "urn:gcode:tool"
	if err := compiler.AddResource(location, schemaValue); err != nil {
		return nil, fmt.Errorf("%s tool schema cannot be loaded", name)
	}
	schema, err = compiler.Compile(location)
	if err != nil {
		return nil, fmt.Errorf("%s tool schema is invalid", name)
	}

	compiledToolSchemas.Lock()
	if existing := compiledToolSchemas.entries[key]; existing != nil {
		schema = existing
	} else {
		if len(compiledToolSchemas.order) >= compiledToolSchemaCacheLimit {
			oldest := compiledToolSchemas.order[0]
			delete(compiledToolSchemas.entries, oldest)
			compiledToolSchemas.order = compiledToolSchemas.order[1:]
		}
		compiledToolSchemas.entries[key] = schema
		compiledToolSchemas.order = append(compiledToolSchemas.order, key)
	}
	compiledToolSchemas.Unlock()
	return schema, nil
}

// PrepareToolArguments validates before any tool effect. Only Agent's explicitly
// supported string booleans are normalized; other fields retain their JSON types.
func PrepareToolArguments(def model.Tool, raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("%s arguments must be valid JSON; correct the call and retry", def.Name)
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
					return nil, errors.New("Agent.run_in_background must be a boolean true/false (the string \"true\"/\"false\" is also accepted); correct the call and retry")
				}
			}
		}
	}
	if validate, ok := toolArgumentValidators[def.Name]; ok {
		// A tool's own rules speak for themselves: they explain the exact
		// mistake before the schema reports a generic constraint violation.
		if err := validate(raw); err != nil {
			return nil, err
		}
	}
	if len(def.Parameters) > 0 {
		// Definitions contain typed Go slices; compile once per stable schema
		// definition. External refs must never read host files or URLs.
		schema, err := compileToolSchema(def.Name, def.Parameters)
		if err != nil {
			return nil, err
		}
		if err := schema.Validate(value); err != nil {
			var validation *jsonschema.ValidationError
			if errors.As(err, &validation) {
				for len(validation.Causes) > 0 {
					validation = validation.Causes[0]
				}
				path := strings.Join(validation.InstanceLocation, ".")
				if expected, ok := validation.ErrorKind.(*kind.Type); ok {
					return nil, fmt.Errorf("%s.%s argument type is wrong: expected %s; correct the call and retry", def.Name, path, strings.Join(expected.Want, "/"))
				}
				return nil, fmt.Errorf("%s.%s argument violates the %s schema constraint; correct the call and retry",
					def.Name, path, strings.Join(validation.ErrorKind.KeywordPath(), "/"))
			}
			return nil, fmt.Errorf("%s arguments do not match the tool schema; correct the call and retry", def.Name)
		}
	}
	// Preserve the original bytes unless the Agent compatibility rule applied.
	if normalized {
		return json.Marshal(value)
	}
	return raw, nil
}
