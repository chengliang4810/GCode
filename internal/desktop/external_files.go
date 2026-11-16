package desktop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"gcode/internal/contracts"
	"github.com/egoist/mygo"
)

func (a *App) openExternalWorkspaceFile(sessionID string, result contracts.LinkedFileResult) {
	config := a.linkConfig(sessionID, "", a.services)
	destination, ok := config.Resolve(result.AbsolutePath)
	if !ok {
		a.errorText = "Cannot resolve file"
		return
	}
	a.errorText = ""
	a.backgroundTask(func() func() {
		resolved := result
		var err error
		if !destination.Local && resolved.File.AssetRef == "" {
			reader, available := a.service.(contracts.LinkedFileService)
			if !available {
				err = errors.New("file download is unavailable")
			} else {
				resolved, err = reader.ReadLinkedFile(a.ctx, contracts.LinkedFileRequest{SessionID: sessionID, Path: result.AbsolutePath})
			}
		}
		var path string
		if err == nil {
			path, err = externalFilePath(a.ctx, a.service, resolved, destination.Local)
		}
		if err == nil {
			err = mygo.Shell.OpenPath(path)
		}
		return func() {
			if err != nil {
				a.errorText = err.Error()
			}
		}
	})
}

// Local files open in place. A remote host's path is never opened on the client;
// its immutable resource is copied to a private temporary directory instead.
func externalFilePath(ctx context.Context, service contracts.Service, result contracts.LinkedFileResult, local bool) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if local {
		return filepath.FromSlash(result.AbsolutePath), nil
	}
	reader, ok := service.(contracts.ResourceService)
	if !ok || result.File.AssetRef == "" {
		return "", errors.New("file download is unavailable")
	}
	name := result.File.Name
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") {
		return "", errors.New("invalid external file name")
	}
	data, err := reader.ReadResource(ctx, result.File.AssetRef)
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	directory, err := os.MkdirTemp("", "gcode-external-")
	if err != nil {
		return "", err
	}
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, data, 0600); err != nil {
		_ = os.RemoveAll(directory)
		return "", err
	}
	return path, nil
}
