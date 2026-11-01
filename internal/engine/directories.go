package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"gcode/internal/contracts"
)

func (e *Engine) ListDirectories(ctx context.Context, request contracts.DirectoryRequest) (contracts.DirectoryListing, error) {
	if err := ctx.Err(); err != nil {
		return contracts.DirectoryListing{}, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return contracts.DirectoryListing{}, err
	}
	path := request.Path
	if path == "" || path == "~" {
		path = home
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return contracts.DirectoryListing{}, err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return contracts.DirectoryListing{}, err
	}
	result := contracts.DirectoryListing{DeviceID: "local", Path: filepath.ToSlash(path), Parent: filepath.ToSlash(filepath.Dir(path)), Home: filepath.ToSlash(home)}
	for _, entry := range entries {
		if entry.IsDir() {
			result.Entries = append(result.Entries, contracts.DirectoryEntry{Name: entry.Name(), Path: filepath.ToSlash(filepath.Join(path, entry.Name())), Hidden: strings.HasPrefix(entry.Name(), ".")})
		}
	}
	return result, nil
}
