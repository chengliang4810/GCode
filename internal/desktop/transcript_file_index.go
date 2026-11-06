package desktop

import (
	"path"
	"strings"
	"unicode"

	"gcode/internal/contracts"
	"gcode/internal/linktarget"
)

// File metadata belongs to an accepted snapshot, not a scroll frame. Resolve
// each project path once; inline code must not resolve the entire file tree
// again for every candidate link on every build.
type transcriptFileIndex struct {
	known  map[string]bool
	names  map[string][]string
	labels map[string]string
}

func (a *App) transcriptFiles(project contracts.Project) *transcriptFileIndex {
	if index := a.linkFiles[project.ID]; index != nil {
		return index
	}
	index := &transcriptFileIndex{
		known: make(map[string]bool), names: make(map[string][]string), labels: make(map[string]string),
	}
	for _, file := range a.state.Files {
		if file.ProjectID != project.ID || file.Kind == "directory" {
			continue
		}
		absolute, _, _, err := linktarget.ResolveFile(file.Path, project.Path)
		if err != nil {
			continue
		}
		index.known[linkPathKey(absolute)] = true
		name := foldLinkName(path.Base(absolute))
		index.names[name] = append(index.names[name], absolute)
	}
	if a.linkFiles == nil {
		a.linkFiles = make(map[string]*transcriptFileIndex)
	}
	a.linkFiles[project.ID] = index
	return index
}

func (index *transcriptFileIndex) lookup(absolute string) (known bool, label string) {
	label, found := index.labels[absolute]
	if !found {
		label = disambiguatedLinkLabel(absolute, index.names[foldLinkName(path.Base(absolute))])
		index.labels[absolute] = label
	}
	return index.known[linkPathKey(absolute)], label
}

func linkPathKey(absolute string) string {
	if len(absolute) > 1 && absolute[1] == ':' {
		return foldLinkName(absolute)
	}
	return absolute
}

// Use the same Unicode equivalence as EqualFold, including sigma and Kelvin K.
// Path identity on POSIX remains case sensitive; basename disambiguation and
// Windows path identity retain their existing case insensitive behavior.
func foldLinkName(name string) string {
	return strings.Map(func(r rune) rune {
		first := r
		for next := unicode.SimpleFold(r); next != first; next = unicode.SimpleFold(next) {
			r = min(r, next)
		}
		return r
	}, name)
}
