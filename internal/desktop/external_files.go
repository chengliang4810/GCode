package desktop

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gcode/internal/contracts"
	"github.com/egoist/mygo"
)

func (a *App) localProjectFolder(project contracts.Project) bool {
	return project.DeviceID == "local" && (a.state.Runtime == nil || !a.state.Runtime.Remote) &&
		filepath.IsAbs(filepath.FromSlash(project.Path))
}

func (a *App) openProjectFolder(project contracts.Project) {
	if !a.localProjectFolder(project) {
		return
	}
	notFolder := a.t("Project path is not a folder")
	a.backgroundTask(func() func() {
		path := filepath.FromSlash(project.Path)
		info, err := os.Stat(path)
		if err == nil {
			if !info.IsDir() {
				err = errors.New(notFolder)
			} else {
				err = mygo.Shell.OpenPath(path)
			}
		}
		return func() {
			if err != nil {
				a.errorText = err.Error()
			}
		}
	})
}

func (a *App) openExternalWorkspaceFile(sessionID string, result contracts.LinkedFileResult) {
	config := a.linkConfig(sessionID, "", a.services)
	destination, ok := config.Resolve(result.AbsolutePath)
	if !ok {
		a.errorText = "Cannot resolve file"
		return
	}
	a.errorText = ""
	a.backgroundTask(func() func() {
		var path string
		path, err := resolveWorkspaceFile(a.ctx, a.service, sessionID, result, destination.Local)
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

// openPreviewWorkspaceFile opens a renderable file, such as HTML or PDF, in the
// embedded browser. The preview never claims a remote host path is local: its
// resource is downloaded first.
func (a *App) openPreviewWorkspaceFile(sessionID string, result contracts.LinkedFileResult) {
	config := a.linkConfig(sessionID, "", a.services)
	destination, ok := config.Resolve(result.AbsolutePath)
	if !ok {
		a.errorText = "Cannot resolve file"
		return
	}
	name := result.File.Name
	if name == "" {
		name = path.Base(result.AbsolutePath)
	}
	a.errorText = ""
	a.backgroundTask(func() func() {
		path, err := resolveWorkspaceFile(a.ctx, a.service, sessionID, result, destination.Local)
		return func() {
			if err == nil {
				err = a.workspace.OpenFilePreview(path, name)
			}
			if err != nil {
				a.errorText = err.Error()
			}
		}
	})
}

// resolveWorkspaceFile returns a path this machine can read. It downloads the
// host's resource when the earlier read carried no bytes.
func resolveWorkspaceFile(ctx context.Context, service contracts.Service, sessionID string, result contracts.LinkedFileResult, local bool) (string, error) {
	if local || result.File.AssetRef != "" {
		return externalFilePath(ctx, service, result, local)
	}
	reader, available := service.(contracts.LinkedFileService)
	if !available {
		return "", errors.New("file download is unavailable")
	}
	resolved, err := reader.ReadLinkedFile(ctx, contracts.LinkedFileRequest{SessionID: sessionID, Path: result.AbsolutePath})
	if err != nil {
		return "", err
	}
	return externalFilePath(ctx, service, resolved, local)
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
