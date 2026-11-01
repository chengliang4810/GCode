package plugins

import "strings"

// ComponentName applies the public identity rules to an extensionless path
// identity (or skill directory name). Namespace is empty for project resources.
// Command map keys are already path identities and ignore frontmatter name.
func ComponentName(kind, namespace, pathIdentity, frontmatterName string) string {
	name := strings.ReplaceAll(strings.ReplaceAll(pathIdentity, "\\", "/"), "/", ":")
	switch kind {
	case "commands":
	case "agents":
		if frontmatterName != "" {
			if at := strings.LastIndex(name, ":"); at >= 0 {
				name = name[:at+1] + frontmatterName
			} else {
				name = frontmatterName
			}
		}
	default:
		if frontmatterName != "" {
			name = frontmatterName
		}
	}
	if namespace != "" {
		return namespace + ":" + name
	}
	return name
}
