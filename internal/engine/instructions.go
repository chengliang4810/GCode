package engine

import (
	"context"
	"errors"
	"io"
	"os"
	"unicode/utf8"

	"gcode/internal/contracts"
	"gcode/internal/storage"
)

var _ contracts.GlobalInstructionsService = (*Engine)(nil)

func (e *Engine) instructionsRoot(ctx context.Context) (*os.Root, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.checkLocked(ctx); err != nil {
		return nil, err
	}
	return os.OpenRoot(e.storage.ConfigDirectory())
}

func (e *Engine) ReadGlobalInstructions(ctx context.Context) (string, error) {
	root, err := e.instructionsRoot(ctx)
	if err != nil {
		return "", err
	}
	defer root.Close()
	info, err := root.Lstat("AGENTS.md")
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("global instructions must be a regular file")
	}
	file, err := root.Open("AGENTS.md")
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !os.SameFile(info, opened) {
		return "", errors.New("global instructions changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(file, contracts.MaxGlobalInstructionsBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > contracts.MaxGlobalInstructionsBytes || !utf8.Valid(data) {
		return "", errors.New("global instructions must be UTF-8 text no larger than 256 KB")
	}
	return string(data), ctx.Err()
}

func (e *Engine) WriteGlobalInstructions(ctx context.Context, text string) error {
	if len(text) > contracts.MaxGlobalInstructionsBytes || !utf8.ValidString(text) {
		return errors.New("global instructions must be UTF-8 text no larger than 256 KB")
	}
	e.instructionsMu.Lock()
	defer e.instructionsMu.Unlock()
	root, err := e.instructionsRoot(ctx)
	if err != nil {
		return err
	}
	defer root.Close()
	return storage.WriteBytesRoot(root, "AGENTS.md", []byte(text))
}
