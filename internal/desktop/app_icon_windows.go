package desktop

import (
	"fmt"
	"log"
	"unsafe"

	"gcode/internal/assets"

	"github.com/egoist/mygo"
	"golang.org/x/sys/windows"
)

var iconUser32 = windows.NewLazySystemDLL("user32.dll")
var iconCreate = iconUser32.NewProc("CreateIconFromResourceEx")
var iconDestroy = iconUser32.NewProc("DestroyIcon")
var iconSend = iconUser32.NewProc("SendMessageW")
var iconDPI = iconUser32.NewProc("GetDpiForWindow")
var iconMetrics = iconUser32.NewProc("GetSystemMetricsForDpi")

func setApplicationIcon(win *mygo.Window) error {
	hwnd := win.NativeHandle()
	var handles [2]uintptr
	var previousDPI uintptr
	release := func(icons [2]uintptr) {
		for _, handle := range icons {
			if handle != 0 {
				iconDestroy.Call(handle)
			}
		}
	}
	update := func() error {
		dpi, _, _ := iconDPI.Call(hwnd)
		if dpi == previousDPI {
			return nil
		}
		var next [2]uintptr
		// Windows 标题栏和任务栏各取自身 DPI 的原生尺寸。直接加载专用小图，
		// 避免把 1254px 展示稿交给 CreateIconFromResourceEx 再次缩放。
		for kind, metric := range []uintptr{49, 11} {
			size, _, _ := iconMetrics.Call(metric, dpi)
			if size == 0 {
				release(next)
				return fmt.Errorf("read native icon size at DPI %d", dpi)
			}
			data, err := assets.AppIconForSize(int(size))
			if err != nil {
				release(next)
				return err
			}
			handle, _, callErr := iconCreate.Call(
				uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)),
				1, 0x00030000, size, size, 0,
			)
			if handle == 0 {
				release(next)
				return fmt.Errorf("create native icon %dpx: %w", size, callErr)
			}
			next[kind] = handle
		}
		for kind, handle := range next {
			iconSend.Call(hwnd, 0x0080, uintptr(kind), handle)
		}
		release(handles)
		handles = next
		previousDPI = dpi
		return nil
	}
	if err := update(); err != nil {
		return err
	}
	off := win.OnResize(func() {
		if err := update(); err != nil {
			// A failed DPI refresh retains the already installed valid icons.
			log.Printf("refresh application icon: %v", err)
		}
	})
	win.OnClosed(func() {
		off()
		release(handles)
	})
	return nil
}
