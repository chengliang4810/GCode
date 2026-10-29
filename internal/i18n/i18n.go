// Package i18n translates application interface text, never conversation or file content.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
)

const Chinese = "zh-CN"
const English = "en"

//go:embed zh-CN.json
var catalogs embed.FS

var chinese = loadCatalog()

func loadCatalog() map[string]string {
	data, err := catalogs.ReadFile("zh-CN.json")
	if err != nil {
		panic(err)
	}
	var messages map[string]string
	if err := json.Unmarshal(data, &messages); err != nil {
		panic(err)
	}
	return messages
}

// Normalize upgrades older settings without a language to the Chinese default.
func Normalize(language string) string {
	if strings.EqualFold(strings.TrimSpace(language), "en") || strings.HasPrefix(strings.ToLower(strings.TrimSpace(language)), "en-") {
		return English
	}
	return Chinese
}

func Native(language string) string {
	if Normalize(language) == English {
		return "en"
	}
	return "zh-Hans"
}

// In uses English source messages as stable keys and as the fallback for new text.
func In(language, message string, args ...any) string {
	if Normalize(language) == Chinese {
		if translated, ok := chinese[message]; ok {
			message = translated
		}
	}
	if len(args) > 0 {
		return fmt.Sprintf(message, args...)
	}
	return message
}
