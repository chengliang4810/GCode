package engine

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/storage"
)

// Tool output belongs to the conversation's original root. It is not a global
// cache or temporary file and must survive restart and runtime-root switches.
func (e *Engine) storeToolOutput(ctx context.Context, session contracts.Session, text string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	results, directory, err := e.toolOutputDirectory(session)
	if err != nil {
		return "", err
	}
	defer results.Close()
	data := []byte(text)
	sum := sha256.Sum256(data)
	name := hex.EncodeToString(sum[:]) + ".txt"
	if err := storage.WriteBytesRoot(results, name, data); err != nil {
		return "", err
	}
	return filepath.Join(directory, name), nil
}

func (e *Engine) toolOutputDirectory(session contracts.Session) (*os.Root, string, error) {
	directory, err := e.storage.Resolve(session.StorageRootID)
	if err != nil {
		return nil, "", err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, "", err
	}
	defer root.Close()
	folder, err := openSessionDirectory(root, session.ID)
	if err != nil {
		return nil, "", err
	}
	defer folder.Close()
	if err := folder.Mkdir("tool-results", 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, "", err
	}
	results, err := openRealDirectory(folder, "tool-results")
	if err != nil {
		return nil, "", err
	}
	return results, filepath.Join(directory, "sessions", session.ID, "tool-results"), nil
}

func (e *Engine) openToolOutput(session contracts.Session) (io.WriteCloser, string, error) {
	directory, path, err := e.toolOutputDirectory(session)
	if err != nil {
		return nil, "", err
	}
	for {
		name := "stream-" + strings.ToLower(rand.Text()) + ".txt"
		file, err := directory.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			_ = directory.Close()
			return nil, "", err
		}
		return &durableToolOutput{File: file, directory: directory}, filepath.Join(path, name), nil
	}
}

type durableToolOutput struct {
	*os.File
	directory *os.Root
}

func (output *durableToolOutput) Close() error {
	err := errors.Join(output.File.Sync(), output.File.Close())
	if runtime.GOOS != "windows" {
		parent, openErr := output.directory.Open(".")
		if openErr == nil {
			openErr = errors.Join(parent.Sync(), parent.Close())
		}
		err = errors.Join(err, openErr)
	}
	return errors.Join(err, output.directory.Close())
}
