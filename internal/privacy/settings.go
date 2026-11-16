// Package privacy owns opt-in masking at the outbound model boundary.
package privacy

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"

	"gcode/third_party/magpie/pii"
	"gcode/third_party/magpie/redact"
)

type Settings struct {
	Enabled      bool               `json:"enabled"`
	Secrets      bool               `json:"secrets"`
	Personal     bool               `json:"personal"`
	Shape        bool               `json:"shape"`
	Notify       bool               `json:"notify"`
	Words        []string           `json:"words"`
	WordsAnyCase bool               `json:"wordsAnyCase"`
	WordsWhole   bool               `json:"wordsWhole"`
	Kinds        map[string]bool    `json:"kinds"`
	Rules        []redact.Rule      `json:"rules"`
	Actions      map[string]string  `json:"actions"`
	Allow        []string           `json:"allow"`
	LocalModel   bool               `json:"localModel"`
	ModelName    string             `json:"modelName"`
	ModelScores  map[string]float64 `json:"modelScores"`
}

func DefaultSettings() Settings { return Settings{Secrets: true} }

func (s Settings) Clone() Settings {
	s.Words, s.Allow = slices.Clone(s.Words), slices.Clone(s.Allow)
	s.Rules = slices.Clone(s.Rules)
	s.Kinds, s.Actions = maps.Clone(s.Kinds), maps.Clone(s.Actions)
	s.ModelScores = maps.Clone(s.ModelScores)
	return s
}

func (s Settings) Validate() error {
	if s.ModelName != "" && s.ModelName != pii.AstrLinkGuard && s.ModelName != pii.PrivacyFilter {
		return errors.New("unknown local privacy model")
	}
	for id, score := range s.ModelScores {
		if (id != pii.AstrLinkGuard && id != pii.PrivacyFilter) || math.IsNaN(score) || math.IsInf(score, 0) || score < pii.Floor || score >= 1 {
			return errors.New("local privacy model confidence must be at least 0.5 and less than 1")
		}
	}
	if _, err := redact.CheckRules(s.Rules); err != nil {
		return err
	}
	for id := range s.Kinds {
		if !slices.ContainsFunc(redact.Categories, func(c redact.Category) bool { return c.ID == id }) {
			return fmt.Errorf("unknown personal data category %q", id)
		}
	}
	for kind, action := range s.Actions {
		if !redact.KindName(kind) || !slices.Contains([]string{"mask", redact.Warn, redact.Block}, action) {
			return errors.New("masking actions require an uppercase kind and mask, warn or block")
		}
	}
	for _, list := range []struct {
		values      []string
		max, length int
	}{{s.Words, 1000, 256}, {s.Allow, 200, 256}} {
		if len(list.values) > list.max {
			return fmt.Errorf("masking list exceeds %d values", list.max)
		}
		for _, value := range list.values {
			if strings.TrimSpace(value) == "" || len(value) > list.length {
				return fmt.Errorf("masking values must contain 1 to %d bytes", list.length)
			}
		}
	}
	return nil
}

func (s Settings) Options(route string) redact.Options {
	rules, _ := redact.CheckRules(s.Rules) // validated at both save and send boundaries
	return redact.Options{
		Secrets: s.Secrets, Personal: s.Personal, Shape: s.Shape,
		Words: s.Words, WordsAnyCase: s.WordsAnyCase, WordsWhole: s.WordsWhole,
		Kinds: s.Kinds, Rules: rules, Actions: s.Actions, Allow: s.Allow, Route: route,
	}
}
