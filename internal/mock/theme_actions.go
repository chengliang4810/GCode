package mock

import (
	"errors"
	"path"
	"strings"

	"gcode/internal/contracts"
)

// Theme imports model the source workflow with predetermined compiled palettes.
// They never read arbitrary files or execute extension packages.
func (s *Server) themeMutationLocked(c contracts.Command) error {
	switch c.Kind {
	case "theme.analyze":
		p := strings.TrimSpace(c.Path)
		mode := c.Name
		if mode == "" {
			mode = "imported"
		}
		if mode != "imported" && mode != "linked" && mode != "editable" {
			return errors.New("unknown theme source mode")
		}
		preview := &contracts.ThemeImportPreview{Path: p, Mode: mode}
		s.state.ThemeImport = preview
		if p == "" {
			preview.Error = "Choose a theme file, extension manifest, or folder."
			return nil
		}
		if strings.Contains(strings.ToLower(p), "broken") || strings.Contains(strings.ToLower(p), "invalid") {
			preview.Error = "The theme could not be compiled. Check the JSONC syntax and color definitions."
			return nil
		}
		preview.Variants = []contracts.ThemeImportVariant{{ID: "github-light", Name: "GitHub Light", Appearance: "light", Selected: true}, {ID: "github-dark", Name: "GitHub Dark", Appearance: "dark", Selected: true}}
		preview.Warnings = []string{"Opaque surfaces are recommended for imported themes."}
		preview.Ready = true
		return nil
	case "theme.select":
		if s.state.ThemeImport == nil || !s.state.ThemeImport.Ready {
			return errors.New("analyze a theme before selecting variants")
		}
		for i, v := range s.state.ThemeImport.Variants {
			if v.ID == c.ID {
				s.state.ThemeImport.Variants[i].Selected = c.Enabled
				return nil
			}
		}
		return ErrNotFound
	case "theme.import":
		p := s.state.ThemeImport
		if p == nil || !p.Ready || p.Error != "" {
			return errors.New("no valid theme preview to import")
		}
		ids := []string{}
		for _, v := range p.Variants {
			if v.Selected {
				ids = append(ids, v.ID)
			}
		}
		if len(ids) == 0 {
			return errors.New("select at least one theme variant")
		}
		mode := p.Mode
		if c.Name != "" {
			mode = c.Name
		}
		if mode != "imported" && mode != "linked" && mode != "editable" {
			return errors.New("unknown theme source mode")
		}
		name := path.Base(strings.ReplaceAll(p.Path, "\\", "/"))
		if name == "package.json" {
			name = path.Base(path.Dir(strings.ReplaceAll(p.Path, "\\", "/")))
		}
		name = strings.TrimSuffix(strings.TrimSuffix(name, ".jsonc"), ".json")
		s.state.ThemeLibrary = append(s.state.ThemeLibrary, contracts.ThemeLibraryEntry{ID: s.idLocked("theme"), Name: name, SourceKind: mode, Path: p.Path, VariantIDs: ids})
		s.state.ThemeImport = nil
		return nil
	case "theme.remove":
		for i, v := range s.state.ThemeLibrary {
			if v.ID == c.ID {
				s.state.ThemeLibrary = append(s.state.ThemeLibrary[:i], s.state.ThemeLibrary[i+1:]...)
				return nil
			}
		}
		return ErrNotFound
	case "theme.reload":
		for i, v := range s.state.ThemeLibrary {
			if v.ID == c.ID {
				if strings.Contains(strings.ToLower(v.Path), "missing") || strings.Contains(strings.ToLower(v.Path), "unavailable") {
					s.state.ThemeLibrary[i].Warning = "The source could not be read. The last successfully compiled theme is still active."
				} else {
					s.state.ThemeLibrary[i].Warning = ""
				}
				return nil
			}
		}
		return ErrNotFound
	case "theme.duplicate":
		name := c.ID
		ids := []string{c.ID}
		for _, v := range s.state.ThemeLibrary {
			if v.ID == c.ID {
				name = v.Name
				ids = append([]string(nil), v.VariantIDs...)
				break
			}
		}
		valid := false
		for _, id := range ids {
			if _, ok := ReferenceArtifact("diagram-" + id); ok {
				valid = true
			}
		}
		if !valid {
			return ErrNotFound
		}
		id := s.idLocked("theme")
		s.state.ThemeLibrary = append(s.state.ThemeLibrary, contracts.ThemeLibraryEntry{ID: id, Name: name + " copy", SourceKind: "editable", Path: "themes/" + id + ".json", VariantIDs: ids})
		return nil
	}
	return errors.New("unsupported theme operation")
}
