package mock

import (
	_ "embed"
	"encoding/json"
	"strings"
	"sync"

	"gcode/internal/contracts"
)

//go:embed artifacts.json
var artifactData []byte

// ReferenceArtifact returns owned metadata for vector paths exported by the
// pinned reference renderers. Missing resources are reported, never fabricated.
func ReferenceArtifact(id string) (contracts.Artifact, bool) {
	v, ok := artifactCatalog()[id]
	return v, ok
}

var artifactCatalog = sync.OnceValue(func() map[string]contracts.Artifact {
	var rows []struct {
		ID string `json:"id"`
		contracts.Artifact
	}
	if err := json.Unmarshal(artifactData, &rows); err != nil {
		panic(err)
	}
	result := make(map[string]contracts.Artifact, len(rows))
	for _, row := range rows {
		result[row.ID] = row.Artifact
	}
	return result
})

func artifactFor(kind string, settings contracts.AppearanceSettings) *contracts.Artifact {
	theme := settings.ThemeID
	if theme == "" {
		if strings.EqualFold(settings.Mode, "light") {
			theme = settings.LightThemeID
			if theme == "" {
				theme = "zeron-light"
			}
		} else {
			theme = settings.DarkThemeID
			if theme == "" {
				theme = "zeron-dark"
			}
		}
	}
	face := map[string]string{"Fira Math": "fira", "STIX Two": "stixTwo", "Computer Modern": "computerModern", "Libertinus": "libertinus", "Palatino": "pagella", "Euler": "euler"}[settings.MathFont]
	if face == "" {
		face = "fira"
	}
	id := "diagram-" + theme
	if kind == "math" {
		id = "math-" + face + "-" + theme
	}
	if v, ok := ReferenceArtifact(id); ok {
		return &v
	}
	return nil
}

func (s *Server) rethemeArtifactsLocked() {
	for sid, ms := range s.state.Messages {
		for mi := range ms {
			for bi := range ms[mi].Blocks {
				b := &ms[mi].Blocks[bi]
				if b.Kind == "math" || b.Kind == "diagram" {
					b.Artifact = artifactFor(b.Kind, s.state.Settings.Appearance)
				}
			}
		}
		s.state.Messages[sid] = ms
	}
}
