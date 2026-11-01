//go:build e2e && windows

package e2e

import (
	"fmt"
	"path/filepath"
	"testing"
)

// The title bar draws the window's caption buttons itself, from the shared
// SVG glyphs, because the window hides the platform's. This checks the real
// native window: the controls exist, share the Files/pane slots' size and
// center line, and maximizing swaps the middle button for restore.
func TestDesktopWindowCaptionControls(t *testing.T) {
	binary, artifacts := testPaths(t)
	app := startApp(t, binary, artifacts, "titlebar-caption", filepath.Join(artifacts, "data"), "--scenario", "default")

	app.waitKey("window-minimize")
	app.waitKey("window-maximize")
	app.waitKey("window-close")
	// The Files and pane toggles sit beside the caption cluster only in a
	// session's chat route; the app starts on Settings.
	app.act("back-to-conversations", "press", "")
	app.waitKey("sidebar-session:chat-0")
	app.act("sidebar-session:chat-0", "press", "")
	app.waitKey("titlebar-files")
	app.waitKey("titlebar-right-pane")

	tree, err := app.client.tree()
	if err != nil {
		t.Fatal(err)
	}
	node := func(key string) accessNode {
		t.Helper()
		n, err := tree.findKey(key)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	minimize, maximize, closeButton := node("window-minimize"), node("window-maximize"), node("window-close")
	files, pane := node("titlebar-files"), node("titlebar-right-pane")
	for _, test := range []struct {
		key  string
		node accessNode
	}{{"Minimize window", minimize}, {"Maximize window", maximize}, {"Close window", closeButton}} {
		b := test.node.Bounds
		if b.Width != 28 || b.Height != 28 {
			t.Fatalf("%s = %gx%g, want the shared 28x28 titlebar target", test.key, b.Width, b.Height)
		}
		if b.Y != files.Bounds.Y || b.Y+b.Height != files.Bounds.Y+files.Bounds.Height {
			t.Fatalf("%s y=%g..%g, adjacent Files toggle y=%g..%g", test.key, b.Y, b.Y+b.Height, files.Bounds.Y, files.Bounds.Y+files.Bounds.Height)
		}
	}
	gap := func(left, right accessNode) float64 {
		return right.Bounds.X - (left.Bounds.X + left.Bounds.Width)
	}
	if g := gap(files, pane); g != 4 {
		t.Fatalf("Files/pane gap = %g, want 4", g)
	}
	if g := gap(pane, minimize); g != 4 {
		t.Fatalf("pane/caption gap = %g, want 4", g)
	}
	if g := gap(minimize, maximize); g != 4 {
		t.Fatalf("minimize/maximize gap = %g, want 4", g)
	}
	if g := gap(maximize, closeButton); g != 4 {
		t.Fatalf("maximize/close gap = %g, want 4", g)
	}
	app.capture("caption-controls")

	app.act("window-maximize", "press", "")
	app.await("restore glyph after maximizing", func() error {
		tree, err := app.client.tree()
		if err != nil {
			return err
		}
		n, err := tree.findKey("window-maximize")
		if err != nil {
			return err
		}
		if n.Label != "Restore window" {
			return fmt.Errorf("maximize button label = %q, want the restore glyph", n.Label)
		}
		return nil
	})
	app.capture("caption-controls-maximized")

	app.act("window-maximize", "press", "")
	app.await("maximize glyph after restoring", func() error {
		tree, err := app.client.tree()
		if err != nil {
			return err
		}
		n, err := tree.findKey("window-maximize")
		if err != nil {
			return err
		}
		if n.Label != "Maximize window" {
			return fmt.Errorf("maximize button label = %q, want the maximize glyph", n.Label)
		}
		return nil
	})
}
