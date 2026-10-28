package workbench

import (
	"encoding/json"
	"path"
	"strings"
	"sync"

	"gcode/internal/assets"
	"gcode/internal/contracts"

	"github.com/egoist/mygo/ui"
)

type fileIconTheme struct {
	IconDefinitions map[string]struct {
		IconPath string `json:"iconPath"`
	} `json:"iconDefinitions"`
	File                string            `json:"file"`
	Folder              string            `json:"folder"`
	FolderExpanded      string            `json:"folderExpanded"`
	FileNames           map[string]string `json:"fileNames"`
	FileExtensions      map[string]string `json:"fileExtensions"`
	FolderNames         map[string]string `json:"folderNames"`
	FolderNamesExpanded map[string]string `json:"folderNamesExpanded"`
	LanguageIDs         map[string]string `json:"languageIds"`
}

var fileThemeOnce sync.Once
var fileTheme fileIconTheme
var fileSVGs sync.Map

// FileIcon returns the reference Symbols icon for a file without opening it.
func FileIcon(f contracts.File, dark bool) *ui.SVG { return sourceFileIcon(f, false, dark) }

func normalizeIconNames(names map[string]string) map[string]string {
	result := make(map[string]string, len(names))
	for name, value := range names {
		result[strings.ToLower(name)] = value
	}
	return result
}
func sourceFileIcon(f contracts.File, expanded, dark bool) *ui.SVG {
	fileThemeOnce.Do(func() {
		data, err := assets.Read("file-icons.json")
		if err == nil {
			_ = json.Unmarshal(data, &fileTheme)
		}
		fileTheme.FileNames = normalizeIconNames(fileTheme.FileNames)
		fileTheme.FolderNames = normalizeIconNames(fileTheme.FolderNames)
		fileTheme.FolderNamesExpanded = normalizeIconNames(fileTheme.FolderNamesExpanded)
	})
	name := strings.ToLower(path.Base(strings.ReplaceAll(f.Name, "\\", "/")))
	id := ""
	if f.Kind == "directory" {
		if expanded {
			id = fileTheme.FolderNamesExpanded[name]
			if id == "" {
				id = fileTheme.FolderExpanded
			}
		}
		if id == "" {
			id = fileTheme.FolderNames[name]
		}
		if id == "" {
			id = fileTheme.Folder
		}
	} else {
		id = fileTheme.FileNames[name]
		if id == "" {
			for at := strings.IndexByte(name, '.'); at >= 0 && at < len(name)-1; {
				if candidate := fileTheme.FileExtensions[name[at+1:]]; candidate != "" {
					id = candidate
					break
				}
				next := strings.IndexByte(name[at+1:], '.')
				if next < 0 {
					break
				}
				at += next + 1
			}
		}
		if id == "" {
			id = fileTheme.LanguageIDs[f.Language]
		}
		if id == "" {
			id = fileTheme.File
		}
	}
	definition, ok := fileTheme.IconDefinitions[id]
	if !ok {
		if f.Kind == "directory" {
			return assets.Icon("folder")
		}
		return assets.Icon("document")
	}
	assetPath := "file-icons/" + strings.TrimPrefix(strings.TrimPrefix(definition.IconPath, "./"), "icons/")
	key := assetPath
	if dark {
		key = "dark:" + key
	}
	if cached, ok := fileSVGs.Load(key); ok {
		return cached.(*ui.SVG)
	}
	data, err := assets.Read(assetPath)
	if err != nil {
		return assets.Icon("document")
	}
	if dark {
		svg := string(data)
		for _, pair := range [][2]string{{"#64748B", "#CBD5E1"}, {"#71717A", "#D4D4D8"}, {"#2563EB", "#60A5FA"}, {"#EA580C", "#FB923C"}, {"#16A34A", "#4ADE80"}, {"#8B5CF6", "#A78BFA"}, {"#A855F7", "#C084FC"}} {
			svg = strings.ReplaceAll(svg, pair[0], pair[1])
			svg = strings.ReplaceAll(svg, strings.ToLower(pair[0]), pair[1])
		}
		data = []byte(svg)
	}
	icon, err := ui.ParseSVG(data)
	if err != nil {
		return assets.Icon("document")
	}
	actual, _ := fileSVGs.LoadOrStore(key, icon)
	return actual.(*ui.SVG)
}
