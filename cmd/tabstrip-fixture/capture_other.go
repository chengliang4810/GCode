//go:build uifixture && !windows

package main

import (
	"context"
	"fmt"
)

func captureNativeWindow(ctx context.Context, hwnd uintptr, bridgePath, outputPath string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("fixture WGC capture is supported only on Windows")
}
