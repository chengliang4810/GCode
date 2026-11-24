package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"gcode/internal/contracts"
)

// CreateDirectory creates one folder on the host that owns the files and
// returns the new folder's listing, so the picker can enter it right away.
// The host owns the filesystem: the caller only names a parent folder and a
// single path segment.
func (e *Engine) CreateDirectory(ctx context.Context, request contracts.DirectoryCreateRequest) (contracts.DirectoryListing, error) {
	return hostDirectoryBrowser.createDirectory(ctx, request)
}

func (b *directoryBrowser) createDirectory(ctx context.Context, request contracts.DirectoryCreateRequest) (contracts.DirectoryListing, error) {
	return runDirectoryQuery(ctx, b, request.DeviceID, func(ctx context.Context) (contracts.DirectoryListing, error) {
		return b.createDirectoryAt(ctx, request)
	})
}

func (b *directoryBrowser) createDirectoryAt(ctx context.Context, request contracts.DirectoryCreateRequest) (contracts.DirectoryListing, error) {
	name, err := directoryName(request.Name)
	if err != nil {
		return contracts.DirectoryListing{}, err
	}
	parent, err := b.resolveDirectory(request.Parent)
	if err != nil {
		return contracts.DirectoryListing{}, err
	}
	target := filepath.Join(parent, name)
	switch _, err := b.stat(target); {
	case err == nil:
		return contracts.DirectoryListing{}, errors.New("a folder with this name already exists")
	case !errors.Is(err, os.ErrNotExist):
		return contracts.DirectoryListing{}, err
	}
	if err := b.mkdir(target); err != nil {
		return contracts.DirectoryListing{}, err
	}
	return b.readDirectories(ctx, target)
}

// resolveDirectory turns a browsed path into an existing folder. The home
// directory is the default parent, exactly like the picker.
func (b *directoryBrowser) resolveDirectory(path string) (string, error) {
	if path == "" || path == "~" {
		home, err := b.home()
		if err != nil {
			return "", err
		}
		if home == "" {
			return "", errors.New("the home folder is unavailable")
		}
		return filepath.Clean(home), nil
	}
	absolute, err := b.abs(path)
	if err != nil {
		return "", err
	}
	info, err := b.stat(absolute)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("the parent folder is not a folder")
	}
	return absolute, nil
}

// directoryName accepts one path segment only. A nested path would create
// folders the picker never listed and would escape the browsed parent.
func directoryName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	switch {
	case trimmed == "", trimmed == ".", trimmed == "..":
		return "", errors.New("enter a folder name")
	case strings.ContainsAny(trimmed, `/\`):
		return "", errors.New("a folder name cannot contain a path separator")
	case strings.ContainsRune(trimmed, 0):
		return "", errors.New("a folder name cannot contain a null byte")
	case len(trimmed) > 255:
		return "", errors.New("the folder name is too long")
	}
	return trimmed, nil
}
