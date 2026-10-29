package view

import (
	"gcode/internal/i18n"
	_ "github.com/egoist/mygo/locales/zh-Hans"
	"github.com/egoist/mygo/ui"
	"time"
)

type scopeKey struct{}

// Set stores only the language string in the root's frame-safe widget state.
// It keeps render helpers independent of global language and of saved UI contexts.
func Set(c *ui.Context, language string) {
	*ui.Local(c.Root(), scopeKey{}, func() string { return i18n.English }) = i18n.Normalize(language)
	c.Root().Lang(i18n.Native(language))
}

// T is for explicitly identified interface text. Standalone widget fixtures keep
// English until their owner supplies settings with Set.
func T(c *ui.Context, message string, args ...any) string {
	language := *ui.Local(c.Root(), scopeKey{}, func() string { return i18n.English })
	return i18n.In(language, message, args...)
}

func Date(c *ui.Context, at time.Time, layout string) string {
	language := *ui.Local(c.Root(), scopeKey{}, func() string { return i18n.English })
	return i18n.Date(language, at, layout)
}
