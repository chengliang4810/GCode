package plugins

import (
	"fmt"
	"slices"
	"strings"
)

func namespaceConflicts(selected map[string]Installation) []string {
	names := map[string]string{}
	var conflicts []string
	ids := make([]string, 0, len(selected))
	for id := range selected {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		installation := selected[id]
		if !installation.Enabled {
			continue
		}
		namespace := installation.Manifest.Name
		if old := names[namespace]; old != "" && old != id {
			conflicts = append(conflicts, fmt.Sprintf("namespace %q is claimed by %s and %s", namespace, old, id))
		} else {
			names[namespace] = id
		}
	}
	return conflicts
}

func namespaceState(data registry) map[string]bool {
	contexts := map[string]bool{"": true}
	for _, installation := range data.Installations {
		contexts[installation.ProjectPath] = true
	}
	result := map[string]bool{}
	for cwd := range contexts {
		for _, conflict := range namespaceConflicts(selectInstallations(data.Installations, cwd)) {
			if cwd != "" {
				conflict += " (" + cwd + ")"
			}
			result[conflict] = true
		}
	}
	return result
}

func validateNamespaceTransition(old, next registry) error {
	before := namespaceState(old)
	var added []string
	for conflict := range namespaceState(next) {
		if !before[conflict] {
			added = append(added, conflict)
		}
	}
	if len(added) == 0 {
		return nil
	}
	slices.Sort(added)
	return fmt.Errorf("plugin configuration conflict: %s", strings.Join(added, "; "))
}
