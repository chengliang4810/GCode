package engine

import (
	"context"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"gcode/internal/contracts"
)

// ListDirectoryLocations returns the picker's starting points for a device:
// the home directory first, then the mounted roots/volumes. Roots come from the
// host filesystem at call time; nothing is cached in the snapshot.
func (e *Engine) ListDirectoryLocations(ctx context.Context, request contracts.DirectoryRequest) (contracts.DirectoryLocations, error) {
	return hostDirectoryBrowser.listDirectoryLocations(ctx, request)
}

func (b *directoryBrowser) listDirectoryLocations(ctx context.Context, request contracts.DirectoryRequest) (contracts.DirectoryLocations, error) {
	return runDirectoryQuery(ctx, b, request.DeviceID, b.readDirectoryLocations)
}

func (b *directoryBrowser) readDirectoryLocations(ctx context.Context) (contracts.DirectoryLocations, error) {
	home, err := b.home()
	if err != nil {
		return contracts.DirectoryLocations{}, err
	}
	result := contracts.DirectoryLocations{DeviceID: "local"}
	if home != "" {
		result.Locations = append(result.Locations, contracts.DirectoryLocation{Name: "Home", Path: filepath.ToSlash(home), Kind: contracts.LocationHome})
	}
	if err := ctx.Err(); err != nil {
		return contracts.DirectoryLocations{}, err
	}
	for _, root := range b.roots() {
		if err := ctx.Err(); err != nil {
			return contracts.DirectoryLocations{}, err
		}
		if root == filepath.ToSlash(home) {
			continue
		}
		if len(result.Locations) == contracts.DirectoryResultLimit {
			result.Truncated = true
			break
		}
		result.Locations = append(result.Locations, contracts.DirectoryLocation{Name: volumeName(root), Path: filepath.ToSlash(root), Kind: contracts.LocationVolume})
	}
	return result, nil
}

// volumeName is the label the picker shows for a root. A Windows drive reads as
// "C:"; other systems show the mount point.
func volumeName(root string) string {
	if len(root) == 3 && root[1] == ':' && (root[2] == '/' || root[2] == '\\') {
		return root[:2]
	}
	trimmed := strings.TrimSuffix(filepath.ToSlash(root), "/")
	if trimmed == "" {
		return "/"
	}
	return trimmed
}

// volumeRoots never returns duplicates and stays in a stable order so repeated
// picker opens list the same volumes.
func volumeRoots() []string {
	roots := platformVolumeRoots()
	seen := make(map[string]bool, len(roots))
	unique := roots[:0]
	for _, root := range roots {
		if root == "" || seen[root] {
			continue
		}
		seen[root] = true
		unique = append(unique, root)
	}
	sort.Strings(unique)
	return unique
}

func platformVolumeRoots() []string {
	if runtime.GOOS == "windows" {
		return windowsVolumeRoots()
	}
	// POSIX has a single filesystem root; removable media mounts appear as
	// ordinary folders under it and are reached by browsing.
	return []string{"/"}
}
