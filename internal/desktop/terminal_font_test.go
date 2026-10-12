package desktop

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/go-text/typesetting/font"
)

// Check the actual embedded font data, rather than a font installed on the
// developer's machine. Native rendering is verified separately through the PTY.
func TestBundledTerminalFontCoversPromptSymbols(t *testing.T) {
	face, err := font.ParseTTF(bytes.NewReader(terminalSymbolsFont))
	if err != nil {
		t.Fatal(err)
	}
	for _, symbol := range []rune{'\ue0a0', '\ue0b0', '\uf07c', '\uf120'} {
		glyph, ok := face.NominalGlyph(symbol)
		if !ok || glyph == 0 {
			t.Errorf("bundled terminal font lacks U+%04X", symbol)
		}
	}
}

func TestNativeTerminalFactoryRejectsCanceledOpen(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	term, err := (NativeTerminalFactory{}).Open(ctx, t.TempDir())
	if term != nil {
		_ = term.Close()
		t.Fatal("canceled Open created a terminal")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Open error = %v, want context.Canceled", err)
	}
}
