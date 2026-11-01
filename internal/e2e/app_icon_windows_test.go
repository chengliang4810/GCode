//go:build e2e && windows

package e2e

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"unsafe"

	"gcode/internal/assets"

	"golang.org/x/sys/windows"
)

// The E2E driver builds with plain go build, without PE icon resources.
// Query the OS window to ensure runtime branding also covers that path.
func TestDesktopApplicationIcon(t *testing.T) {
	binary, artifacts := testPaths(t)
	app := startApp(t, binary, artifacts, "icon", filepath.Join(artifacts, "data"), "--scenario", "default")
	user32 := windows.NewLazySystemDLL("user32.dll")
	enumWindows := user32.NewProc("EnumWindows")
	getPID := user32.NewProc("GetWindowThreadProcessId")
	getText := user32.NewProc("GetWindowTextW")
	getIcon := user32.NewProc("SendMessageW")
	isVisible := user32.NewProc("IsWindowVisible")
	var nativeIcons [2]uintptr
	app.await("native GCode window has small and large icons", func() error {
		var found bool
		var iconError error
		callback := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
			var pid uint32
			getPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
			if pid != uint32(app.cmd.Process.Pid) {
				return 1
			}
			visible, _, _ := isVisible.Call(hwnd)
			if visible == 0 {
				return 1
			}
			var title [256]uint16
			getText.Call(hwnd, uintptr(unsafe.Pointer(&title[0])), uintptr(len(title)))
			if windows.UTF16ToString(title[:]) != "GCode" {
				return 1
			}
			found = true
			const wmGetIcon = 0x007f
			for _, kind := range []uintptr{0, 1} {
				icon, _, _ := getIcon.Call(hwnd, wmGetIcon, kind, 0)
				if icon == 0 {
					iconError = fmt.Errorf("missing native icon kind %d", kind)
				}
				nativeIcons[kind] = icon
			}
			return 0
		})
		enumWindows.Call(callback, 0)
		if !found {
			return fmt.Errorf("no visible GCode window for process %d", app.cmd.Process.Pid)
		}
		return iconError
	})
	for kind, handle := range nativeIcons {
		actual, err := readNativeIcon(handle)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(app.artifacts, fmt.Sprintf("native-icon-%d-%dpx.png", kind, actual.Bounds().Dx()))
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		encodeErr := png.Encode(file, actual)
		closeErr := file.Close()
		if encodeErr != nil || closeErr != nil {
			t.Fatalf("save icon: %v, %v", encodeErr, closeErr)
		}
		source, err := assets.AppIconForSize(actual.Bounds().Dx())
		if err != nil {
			t.Fatal(err)
		}
		expected, err := png.Decode(bytes.NewReader(source))
		if err != nil {
			t.Fatal(err)
		}
		for y := 0; y < actual.Bounds().Dy(); y++ {
			for x := 0; x < actual.Bounds().Dx(); x++ {
				ar, ag, ab, aa := actual.At(x, y).RGBA()
				er, eg, eb, ea := expected.At(x, y).RGBA()
				for channel, pair := range [][2]uint32{{ar, er}, {ag, eg}, {ab, eb}, {aa, ea}} {
					if max(pair[0], pair[1])-min(pair[0], pair[1]) > 257 {
						t.Fatalf("native %dpx icon differs from dedicated artwork at (%d,%d) channel %d: %v",
							actual.Bounds().Dx(), x, y, channel, pair)
					}
				}
			}
		}
		t.Logf("native %dpx icon matches dedicated artwork: %s", actual.Bounds().Dx(), path)
	}
	app.capture("application-icon")
}

func readNativeIcon(handle uintptr) (*image.NRGBA, error) {
	user32 := windows.NewLazySystemDLL("user32.dll")
	gdi32 := windows.NewLazySystemDLL("gdi32.dll")
	var info struct {
		Icon        int32
		HotspotX    uint32
		HotspotY    uint32
		Mask, Color uintptr
	}
	ok, _, err := user32.NewProc("GetIconInfo").Call(handle, uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		return nil, err
	}
	defer gdi32.NewProc("DeleteObject").Call(info.Mask)
	defer gdi32.NewProc("DeleteObject").Call(info.Color)
	var bitmap struct {
		Type, Width, Height, WidthBytes int32
		Planes, BitsPerPixel            uint16
		Bits                            uintptr
	}
	ok, _, err = gdi32.NewProc("GetObjectW").Call(
		info.Color, unsafe.Sizeof(bitmap), uintptr(unsafe.Pointer(&bitmap)),
	)
	if ok == 0 {
		return nil, err
	}
	var header struct {
		Size            uint32
		Width, Height   int32
		Planes, Bits    uint16
		Compression     uint32
		ImageSize       uint32
		XRes, YRes      int32
		Used, Important uint32
	}
	header.Size = uint32(unsafe.Sizeof(header))
	header.Width, header.Height = bitmap.Width, -bitmap.Height
	header.Planes, header.Bits = 1, 32
	dc, _, err := gdi32.NewProc("CreateCompatibleDC").Call(0)
	if dc == 0 {
		return nil, err
	}
	defer gdi32.NewProc("DeleteDC").Call(dc)
	pixels := make([]byte, int(bitmap.Width*bitmap.Height)*4)
	ok, _, err = gdi32.NewProc("GetDIBits").Call(
		dc, info.Color, 0, uintptr(bitmap.Height), uintptr(unsafe.Pointer(&pixels[0])),
		uintptr(unsafe.Pointer(&header)), 0,
	)
	if ok == 0 {
		return nil, err
	}
	// GetDIBits exposes the PNG icon's straight BGRA channels.
	for offset := 0; offset < len(pixels); offset += 4 {
		pixels[offset], pixels[offset+2] = pixels[offset+2], pixels[offset]
	}
	return &image.NRGBA{
		Pix: pixels, Stride: int(bitmap.Width) * 4,
		Rect: image.Rect(0, 0, int(bitmap.Width), int(bitmap.Height)),
	}, nil
}
