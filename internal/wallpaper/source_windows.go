package wallpaper

import (
	"context"
	"errors"
	"syscall"
	"unsafe"
)

var systemParametersInfo = syscall.NewLazyDLL("user32.dll").NewProc("SystemParametersInfoW")

func currentPath(ctx context.Context, _ bool) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	// SPI_GETDESKWALLPAPER writes the active desktop wallpaper path. UTF-16
	// storage stays alive for this synchronous Windows call.
	buffer := make([]uint16, 32768)
	ok, _, err := systemParametersInfo.Call(0x0073, uintptr(len(buffer)), uintptr(unsafe.Pointer(&buffer[0])), 0)
	if ok == 0 {
		return "", err
	}
	path := syscall.UTF16ToString(buffer)
	if path == "" {
		return "", errors.New("desktop uses a solid color or has no wallpaper")
	}
	return path, nil
}
