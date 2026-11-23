package mock

import (
	"context"
	"errors"
	"path"
	"slices"
	"strings"

	"gcode/internal/contracts"
)

// SearchWorkspace uses the same mutable virtual files as the editor. It never
// reads the host filesystem or manufactures a file in response to a query.
func (s *Server) SearchWorkspace(ctx context.Context, request contracts.WorkspaceSearchRequest) (contracts.WorkspaceSearchResult, error) {
	if err := ctx.Err(); err != nil {
		return contracts.WorkspaceSearchResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return contracts.WorkspaceSearchResult{}, ErrClosed
	}
	result := contracts.WorkspaceSearchResult{Revision: s.state.Revision}
	if request.ProjectID == "" {
		return result, nil
	}
	query := strings.ToLower(strings.TrimSpace(request.Query))
	if !request.Recent && query == "" {
		return result, nil
	}
	files := slices.Clone(s.state.Files)
	slices.SortStableFunc(files, func(a, b contracts.File) int {
		return strings.Compare(strings.ToLower(a.Path), strings.ToLower(b.Path))
	})
	if request.Recent {
		changed := map[string]bool{}
		for _, change := range s.state.Changes {
			if change.ProjectID == request.ProjectID {
				changed[change.Path] = true
			}
		}
		slices.SortStableFunc(files, func(a, b contracts.File) int {
			if changed[a.Path] != changed[b.Path] {
				if changed[a.Path] {
					return -1
				}
				return 1
			}
			return 0
		})
	}
	fileLimit := request.FileLimit
	if fileLimit <= 0 {
		fileLimit = 50
	}
	fileLimit = min(fileLimit, 1000)
	contentLimit := request.ContentLimit
	if contentLimit <= 0 {
		contentLimit = 100
	}
	contentLimit = min(contentLimit, 1000)
	perFile := request.MatchesPerFile
	if perFile <= 0 {
		perFile = 3
	}
	perFile = min(perFile, 100)
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return contracts.WorkspaceSearchResult{}, err
		}
		if file.ProjectID != request.ProjectID {
			continue
		}
		if !request.IncludeHidden {
			hidden := false
			for _, part := range strings.Split(file.Path, "/") {
				if strings.HasPrefix(part, ".") && part != "." && part != ".." {
					hidden = true
					break
				}
			}
			if hidden {
				continue
			}
		}
		if (request.Recent && file.Kind != "directory") || (!request.Recent && searchWords(query, file.Path)) {
			if len(result.Files) < fileLimit {
				result.Files = append(result.Files, contracts.FileSearchMatch{FileID: file.ID, Path: file.Path, Kind: file.Kind})
			}
		}
		if request.Recent || request.NamesOnly || file.Kind == "directory" || file.Kind == "image" {
			continue
		}
		matches := 0
		for line, text := range strings.Split(file.Content, "\n") {
			if matches >= perFile || len(result.Content) >= contentLimit {
				break
			}
			if !searchWords(query, text) {
				continue
			}
			ranges := contracts.SearchRanges(text, query)
			column := 0
			if len(ranges) > 0 {
				column = ranges[0].Start
			}
			result.Content = append(result.Content, contracts.ContentSearchMatch{FileID: file.ID, Path: file.Path, Line: line + 1, Column: column, Text: text, Ranges: ranges})
			matches++
		}
	}
	return result, nil
}
func searchWords(query, text string) bool {
	text = strings.ToLower(text)
	for _, word := range strings.Fields(query) {
		if !strings.Contains(text, word) {
			return false
		}
	}
	return true
}

// directoryDeviceLocked resolves the requested device and rejects offline hosts
// before any virtual path is resolved.
func (s *Server) directoryDeviceLocked(deviceID string) (contracts.Device, error) {
	for _, device := range s.state.Devices {
		if device.ID != deviceID {
			continue
		}
		if device.Status != "online" && device.Status != "connected" && !device.Current {
			return contracts.Device{}, errors.New("device is offline")
		}
		return device, nil
	}
	return contracts.Device{}, errors.New("device not found")
}

// ListDirectoryLocations offers the virtual workspace home and the virtual
// roots that hold configured projects. It never enumerates the real machine.
func (s *Server) ListDirectoryLocations(ctx context.Context, request contracts.DirectoryRequest) (contracts.DirectoryLocations, error) {
	if err := ctx.Err(); err != nil {
		return contracts.DirectoryLocations{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return contracts.DirectoryLocations{}, ErrClosed
	}
	device, err := s.directoryDeviceLocked(request.DeviceID)
	if err != nil {
		return contracts.DirectoryLocations{}, err
	}
	home := "~/projects"
	roots := map[string]bool{}
	truncated := false
	for i, project := range s.state.Projects {
		if err := ctx.Err(); err != nil {
			return contracts.DirectoryLocations{}, err
		}
		if i == contracts.DirectoryScanLimit {
			truncated = true
			break
		}
		if project.DeviceID != device.ID {
			continue
		}
		projectPath := path.Clean(strings.ReplaceAll(project.Path, "\\", "/"))
		home = path.Dir(projectPath)
		if volume := virtualVolume(projectPath); volume != "" {
			if !roots[volume] && len(roots) == contracts.DirectoryResultLimit-1 {
				truncated = true
				break
			}
			roots[volume] = true
		}
	}
	names := make([]string, 0, len(roots))
	for root := range roots {
		names = append(names, root)
	}
	slices.Sort(names)
	result := contracts.DirectoryLocations{DeviceID: device.ID, Truncated: truncated || len(s.state.Projects) >= contracts.DirectoryScanLimit}
	result.Locations = append(result.Locations, contracts.DirectoryLocation{Name: "Home", Path: home, Kind: contracts.LocationHome})
	for _, root := range names {
		result.Locations = append(result.Locations, contracts.DirectoryLocation{Name: root, Path: root, Kind: contracts.LocationVolume})
	}
	return result, nil
}

// virtualVolume returns the drive letter or leading POSIX root of a virtual
// project path, so the picker can offer the same roots the projects live under.
func virtualVolume(projectPath string) string {
	if len(projectPath) >= 2 && projectPath[1] == ':' {
		return projectPath[:2]
	}
	if strings.HasPrefix(projectPath, "/") {
		return "/"
	}
	return ""
}

// ListDirectories exposes directories derived from virtual project paths. The
// picker never offers access to the real local or remote filesystem.
func (s *Server) ListDirectories(ctx context.Context, request contracts.DirectoryRequest) (contracts.DirectoryListing, error) {
	if err := ctx.Err(); err != nil {
		return contracts.DirectoryListing{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return contracts.DirectoryListing{}, ErrClosed
	}
	if _, err := s.directoryDeviceLocked(request.DeviceID); err != nil {
		return contracts.DirectoryListing{}, err
	}
	home := "~/projects"
	projects := map[string]string{}
	projectPaths := []string{}
	scanned := 0
	truncated := false
	// Build a bounded device-scoped index once, rather than scanning all files
	// again for every project. Mock browsing never touches the host filesystem.
	for _, project := range s.state.Projects {
		if err := ctx.Err(); err != nil {
			return contracts.DirectoryListing{}, err
		}
		if scanned == contracts.DirectoryScanLimit {
			truncated = true
			break
		}
		scanned++
		if project.DeviceID != request.DeviceID {
			continue
		}
		projectPath := path.Clean(strings.ReplaceAll(project.Path, "\\", "/"))
		if len(projectPaths) == 0 {
			home = path.Dir(projectPath)
		}
		projects[project.ID] = projectPath
		projectPaths = append(projectPaths, projectPath)
	}
	current := strings.ReplaceAll(strings.TrimSpace(request.Path), "\\", "/")
	if current == "" || current == "~" {
		current = home
	}
	current = path.Clean(current)
	entries := map[string]contracts.DirectoryEntry{}
	known := current == home || current == "/" || strings.HasSuffix(current, ":")
	add := func(full, projectPath string) bool {
		if full == current {
			known = true
		}
		prefix := strings.TrimSuffix(current, "/") + "/"
		if !strings.HasPrefix(full, prefix) {
			return true
		}
		known = true
		child, _, _ := strings.Cut(strings.TrimPrefix(full, prefix), "/")
		if child == "" || child == "." {
			return true
		}
		childPath := path.Join(current, child)
		previous, exists := entries[childPath]
		if !exists && len(entries) == contracts.DirectoryResultLimit {
			truncated = true
			return false
		}
		entries[childPath] = contracts.DirectoryEntry{Name: child, Path: childPath, Hidden: strings.HasPrefix(child, "."), Project: previous.Project || childPath == projectPath}
		return true
	}
	resultLimit := false
	for _, projectPath := range projectPaths {
		if err := ctx.Err(); err != nil {
			return contracts.DirectoryListing{}, err
		}
		if !add(projectPath, projectPath) {
			resultLimit = true
			break
		}
	}
	if !resultLimit {
		for _, file := range s.state.Files {
			if err := ctx.Err(); err != nil {
				return contracts.DirectoryListing{}, err
			}
			if scanned == contracts.DirectoryScanLimit {
				truncated = true
				break
			}
			scanned++
			projectPath, exists := projects[file.ProjectID]
			if !exists {
				continue
			}
			relative := strings.ReplaceAll(file.Path, "\\", "/")
			if file.Kind != "directory" {
				relative = path.Dir(relative)
			}
			if !add(path.Join(projectPath, relative), projectPath) {
				break
			}
		}
	}
	if !known && !truncated && scanned < contracts.DirectoryScanLimit {
		return contracts.DirectoryListing{}, errors.New("folder not found")
	}
	listing := contracts.DirectoryListing{DeviceID: request.DeviceID, Path: current, Home: home, Revision: s.state.Revision, Truncated: truncated || scanned == contracts.DirectoryScanLimit}
	if parent := path.Dir(current); parent != current && parent != "." {
		listing.Parent = parent
	}
	for _, entry := range entries {
		listing.Entries = append(listing.Entries, entry)
	}
	slices.SortFunc(listing.Entries, func(a, b contracts.DirectoryEntry) int {
		return strings.Compare(a.Path, b.Path)
	})
	return listing, nil
}
