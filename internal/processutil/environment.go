package processutil

import (
	"runtime"
	"slices"
	"strings"
)

// Environment merges inherited variables and child-only layers in priority order.
// Windows variable names are case insensitive; a later layer wins.
func Environment(inherited []string, layers ...map[string]string) []string {
	type variable struct{ name, value string }
	values := map[string]variable{}
	key := func(name string) string {
		if runtime.GOOS == "windows" {
			return strings.ToUpper(name)
		}
		return name
	}
	for _, entry := range inherited {
		at := strings.IndexByte(entry, '=')
		// Windows 还可能携带 =C: 形式的盘符工作目录环境项。
		if at == 0 {
			if next := strings.IndexByte(entry[1:], '='); next >= 0 {
				at = next + 1
			}
		}
		if at > 0 {
			name := entry[:at]
			values[key(name)] = variable{name, entry[at+1:]}
		}
	}
	for _, layer := range layers {
		names := make([]string, 0, len(layer))
		for name := range layer {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			values[key(name)] = variable{name, layer[name]}
		}
	}
	keys := make([]string, 0, len(values))
	for name := range values {
		keys = append(keys, name)
	}
	slices.Sort(keys)
	result := make([]string, 0, len(keys))
	for _, name := range keys {
		value := values[name]
		result = append(result, value.name+"="+value.value)
	}
	return result
}

// EnvironmentMap merges explicit layers without adding the parent environment.
func EnvironmentMap(layers ...map[string]string) map[string]string {
	result := map[string]string{}
	for _, entry := range Environment(nil, layers...) {
		name, value, _ := strings.Cut(entry, "=")
		result[name] = value
	}
	return result
}
