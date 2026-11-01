package engine

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"gcode/internal/mcp"
	"gcode/internal/processutil"
)

// processEnvironment routes child scratch to its owning data root.
// Process-owned directories are never swept while background work can own them.
func (e *Engine) processEnvironment(rootID, ownerID string) (map[string]string, error) {
	if ownerID == "" || len(ownerID) > 100 || strings.Trim(ownerID, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-") != "" {
		return nil, errors.New("invalid process temporary directory owner")
	}
	directory, err := e.storage.Resolve(rootID)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	relative := ""
	for _, component := range []string{"tmp", "processes", ownerID} {
		relative = filepath.Join(relative, component)
		if err := root.Mkdir(relative, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		info, err := root.Lstat(relative)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("process temporary directory must not use symbolic links")
		}
	}
	// 正斜杠同时适用于 Windows 本机进程与 Git Bash，不改变宿主全局环境。
	path := filepath.ToSlash(filepath.Join(directory, relative))
	return map[string]string{"TEMP": path, "TMP": path, "TMPDIR": path}, nil
}

func mcpProcessEnvironment(config mcp.Config, defaults map[string]string) mcp.Config {
	// 宿主临时目录是进程元数据，不应作为凭据红化返回的产物路径。
	// 显式 MCP env 仍由用户控制，启动子进程时优先于默认值。
	config.ProcessEnv = processutil.EnvironmentMap(defaults, config.ProcessEnv)
	config.Env = maps.Clone(config.Env)
	return config
}
