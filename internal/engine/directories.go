package engine

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"gcode/internal/contracts"
)

func (e *Engine) ListDirectories(ctx context.Context, request contracts.DirectoryRequest) (contracts.DirectoryListing, error) {
	return hostDirectoryBrowser.listDirectories(ctx, request)
}

func (b *directoryBrowser) listDirectories(ctx context.Context, request contracts.DirectoryRequest) (contracts.DirectoryListing, error) {
	return runDirectoryQuery(ctx, b, request.DeviceID, func(ctx context.Context) (contracts.DirectoryListing, error) {
		return b.readDirectories(ctx, request.Path)
	})
}

func (b *directoryBrowser) readDirectories(ctx context.Context, path string) (contracts.DirectoryListing, error) {
	home, err := b.home()
	if err != nil {
		return contracts.DirectoryListing{}, err
	}
	if err := ctx.Err(); err != nil {
		return contracts.DirectoryListing{}, err
	}
	if path == "" || path == "~" {
		path = home
	}
	path, err = b.abs(path)
	if err != nil {
		return contracts.DirectoryListing{}, err
	}
	if err := ctx.Err(); err != nil {
		return contracts.DirectoryListing{}, err
	}
	dir, err := b.open(path)
	if err != nil {
		return contracts.DirectoryListing{}, err
	}
	defer dir.Close()
	result := contracts.DirectoryListing{DeviceID: "local", Path: filepath.ToSlash(path), Parent: filepath.ToSlash(filepath.Dir(path)), Home: filepath.ToSlash(home)}
	for scanned := 0; scanned < contracts.DirectoryScanLimit; {
		if err := ctx.Err(); err != nil {
			return contracts.DirectoryListing{}, err
		}
		entries, readErr := dir.ReadDir(min(directoryReadBatch, contracts.DirectoryScanLimit-scanned))
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return contracts.DirectoryListing{}, err
			}
			scanned++
			if !entry.IsDir() {
				continue
			}
			if len(result.Entries) == contracts.DirectoryResultLimit {
				result.Truncated = true
				break
			}
			result.Entries = append(result.Entries, contracts.DirectoryEntry{Name: entry.Name(), Path: filepath.ToSlash(filepath.Join(path, entry.Name())), Hidden: strings.HasPrefix(entry.Name(), ".")})
		}
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return contracts.DirectoryListing{}, readErr
		}
		if result.Truncated || errors.Is(readErr, io.EOF) {
			break
		}
		// Do not read another batch just to prove EOF at the scan boundary.
		// Truncated conservatively means completeness is not guaranteed.
		if scanned == contracts.DirectoryScanLimit {
			result.Truncated = true
		}
	}
	// ReadDir(n) follows filesystem order. Keep the returned subset in stable
	// path order without ever materializing an unbounded directory listing.
	slices.SortFunc(result.Entries, func(a, b contracts.DirectoryEntry) int {
		return strings.Compare(a.Path, b.Path)
	})
	return result, nil
}
