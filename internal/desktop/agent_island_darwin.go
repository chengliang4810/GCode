//go:build darwin

package desktop

import (
	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
)

// The companion shares the menu bar's level and follows the user across Spaces.
// NativeHandle is NSWindow*; calls execute on MyGo's UI thread.
func configureIslandWindow(window *mygo.Window) {
	win := objc.ID(window.NativeHandle())
	win.Send(objc.RegisterName("setStyleMask:"), uint64(128))
	win.Send(objc.RegisterName("setLevel:"), int64(27))
	win.Send(objc.RegisterName("setCollectionBehavior:"), uint64(1|256))
	win.Send(objc.RegisterName("setHidesOnDeactivate:"), false)
	win.Send(objc.RegisterName("setMovable:"), false)
}
