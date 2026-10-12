package desktop

import (
	"context"
	_ "embed"
	"fmt"
	"sync"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

//go:embed fonts/SymbolsNerdFontMono-Regular.ttf
var terminalSymbolsFont []byte

const terminalSymbolsFamily = "GCode Terminal Symbols"

var registerTerminalSymbols = sync.OnceValue(func() error {
	return ui.RegisterFont(terminalSymbolsFont, terminalSymbolsFamily)
})

type NativeTerminalFactory struct{}

func (NativeTerminalFactory) Open(ctx context.Context, cwd string) (TerminalSurface, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := registerTerminalSymbols(); err != nil {
		return nil, fmt.Errorf("register terminal symbols font: %w", err)
	}
	value, err := terminal.New(terminal.Options{
		Dir: cwd, Scrollback: 10 << 20,
		// CoreText shapes private-use icons only when their font is primary.
		// ASCII and CJK use the system fallback; terminal cells retain monospace widths.
		Font: terminal.Font{Family: terminalSymbolsFamily + ", monospace"},
	})
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		_ = value.Close()
		return nil, err
	}
	return nativeTerminal{value: value}, nil
}

type nativeTerminal struct {
	value *terminal.Terminal
}

func (t nativeTerminal) View(c *ui.Context) ui.Element {
	return terminal.View(c, t.value).Fill().AutoFocus()
}

func (t nativeTerminal) Close() error {
	if t.value == nil {
		return nil
	}
	return t.value.Close()
}
