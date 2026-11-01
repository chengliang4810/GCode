package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gcode/internal/processutil"
)

// Git identifies ignored directories without walking their build/cache contents.
// No Git repository/tool is required to register or browse an ordinary directory.
func ignoredProjectEntries(ctx context.Context, root string) (map[string]bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	probe, cancel := context.WithCancel(ctx)
	defer cancel()
	command := exec.CommandContext(probe, "git", "-c", "core.hooksPath=", "-C", root,
		"ls-files", "--others", "--ignored", "--exclude-standard", "--directory", "-z", "--", ".")
	processutil.HideConsole(command)
	command.WaitDelay = 2 * time.Second
	stream, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := command.Start(); err != nil {
		var missing *exec.Error
		if errors.As(err, &missing) {
			return nil, nil
		}
		return nil, fmt.Errorf("cannot read project ignore rules: %w", err)
	}
	const limit = 4 << 20
	data, readErr := io.ReadAll(io.LimitReader(stream, limit+1))
	if readErr != nil || len(data) > limit {
		cancel()
	}
	waitErr := command.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if readErr != nil {
		return nil, readErr
	}
	if len(data) > limit {
		return nil, errors.New("project ignore listing exceeds 4 MiB")
	}
	if waitErr != nil {
		// Git may be absent for this folder (including an uninitialized project).
		// The regular filesystem collector remains authoritative in that case.
		return nil, nil
	}
	ignored := map[string]bool{}
	for _, item := range bytes.Split(data, []byte{0}) {
		if len(item) == 0 {
			continue
		}
		name := filepath.Clean(filepath.FromSlash(strings.TrimSuffix(string(item), "/")))
		if filepath.IsLocal(name) && name != "." {
			ignored[name] = true
		}
	}
	return ignored, nil
}
