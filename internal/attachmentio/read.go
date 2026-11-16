// Package attachmentio validates attachment batches before reading their bodies.
package attachmentio

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	"gcode/internal/contracts"
)

var ErrLimit = errors.New("attachment exceeds its size limit")

type File struct {
	Path, Name string
}

// Plan checks the entire batch using metadata, without opening or retaining
// file bodies. Clipboard bytes reserve their share of the batch first.
func Plan(request contracts.AttachmentImportRequest) ([]File, error) {
	count := len(request.Paths)
	var total int64
	if request.Clipboard != nil {
		count++
		total = int64(len(request.Clipboard.Data))
		if total == 0 || total > contracts.MaxAttachmentBytes {
			return nil, errors.New("invalid clipboard attachment size")
		}
	}
	if count < 1 || count > contracts.MaxAttachmentCount {
		return nil, errors.New("select between 1 and 20 attachments")
	}
	files := make([]File, 0, len(request.Paths))
	for _, path := range request.Paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, errors.New("attachment must be a regular file")
		}
		if info.Size() > contracts.MaxAttachmentBytes {
			return nil, ErrLimit
		}
		total += info.Size()
		if total > contracts.MaxAttachmentBatchBytes {
			return nil, errors.New("attachment batch exceeds 96 MiB")
		}
		files = append(files, File{Path: path, Name: filepath.Base(path)})
	}
	return files, nil
}

// Open rechecks the remaining allowance in case the source changed after Plan.
func Open(file File, remaining int64) (*os.File, int64, error) {
	info, err := os.Stat(file.Path)
	if err != nil {
		return nil, 0, err
	}
	if !info.Mode().IsRegular() {
		return nil, 0, errors.New("attachment must be a regular file")
	}
	f, err := os.Open(file.Path)
	if err != nil {
		return nil, 0, err
	}
	info, err = f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = f.Close()
		return nil, 0, errors.New("attachment must be a regular file")
	}
	if info.Size() > min(int64(contracts.MaxAttachmentBytes), remaining) {
		_ = f.Close()
		return nil, 0, ErrLimit
	}
	return f, info.Size(), nil
}

// Read allocates once for a known size, plus one byte to detect growth. Unknown
// length streams remain bounded; no JSON/base64 representation is materialized.
func Read(reader io.Reader, size, limit int64) ([]byte, error) {
	if limit < 0 || size > limit {
		return nil, ErrLimit
	}
	if size >= 0 {
		data := make([]byte, int(size)+1)
		n, err := io.ReadFull(reader, data)
		if n > int(size) {
			if int64(n) > limit {
				return nil, ErrLimit
			}
			return nil, errors.New("attachment changed size while reading")
		}
		if n == int(size) && (errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)) {
			return data[:n], nil
		}
		if err == nil {
			return nil, errors.New("attachment changed size while reading")
		}
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err == nil && int64(len(data)) > limit {
		return nil, ErrLimit
	}
	return data, err
}
