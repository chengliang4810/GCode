package engine

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/model"
)

const resourcePrefix = "resource://managed/"

func (e *Engine) ImportAttachments(ctx context.Context, request contracts.AttachmentImportRequest) ([]contracts.Attachment, error) {
	request = request.Clone()
	count := len(request.Paths)
	if request.Clipboard != nil {
		count++
	}
	if count < 1 || count > contracts.MaxAttachmentCount {
		return nil, errors.New("select between 1 and 20 attachments")
	}
	var staged []struct {
		name string
		data []byte
	}
	var total int
	for _, path := range request.Paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		info, statErr := f.Stat()
		if statErr != nil || !info.Mode().IsRegular() {
			_ = f.Close()
			return nil, errors.New("attachment must be a regular file")
		}
		data, readErr := io.ReadAll(io.LimitReader(f, contracts.MaxAttachmentBytes+1))
		_ = f.Close()
		if readErr != nil {
			return nil, readErr
		}
		if len(data) > contracts.MaxAttachmentBytes {
			return nil, errors.New("attachment exceeds 24 MiB")
		}
		total += len(data)
		staged = append(staged, struct {
			name string
			data []byte
		}{filepath.Base(path), data})
	}
	if request.Clipboard != nil {
		data := request.Clipboard.Data
		if len(data) == 0 || len(data) > contracts.MaxAttachmentBytes {
			return nil, errors.New("invalid clipboard attachment size")
		}
		total += len(data)
		staged = append(staged, struct {
			name string
			data []byte
		}{request.Clipboard.Name, data})
	}
	if total > contracts.MaxAttachmentBatchBytes {
		return nil, errors.New("attachment batch exceeds 96 MiB")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	defer e.evictIdleSessionsLocked()
	if err := e.checkLocked(ctx); err != nil {
		return nil, err
	}
	rootID := e.storage.ActiveRootID()
	if request.SessionID != "" {
		index := e.sessionIndex(request.SessionID)
		if index < 0 {
			return nil, errors.New("source session not found")
		}
		if err := e.ensureSessionDataLocked(request.SessionID); err != nil {
			return nil, err
		}
		session := e.state.Sessions[index]
		rootID = session.StorageRootID
	}
	directory, err := e.storage.Resolve(rootID)
	if err != nil {
		return nil, err
	}
	uploads, err := managedUploadsRoot(directory)
	if err != nil {
		return nil, err
	}
	defer uploads.Close()
	var attachments []contracts.Attachment
	var paths []string
	cleanup := func() {
		for _, path := range paths {
			_ = uploads.Remove(path)
		}
	}
	for _, file := range staged {
		key := id("attachment")
		// Keep common file extensions so agent tools can select the right reader.
		if ext := filepath.Ext(file.name); managedResourceComponent(ext) {
			key += ext
		}
		mime := http.DetectContentType(file.data)
		item := contracts.Attachment{ID: key, StorageRootID: rootID, Name: file.name, MIME: mime, Size: int64(len(file.data)), Status: "ready", AssetRef: managedResourceRef(rootID, key)}
		if strings.HasPrefix(mime, "image/") {
			config, _, err := image.DecodeConfig(bytes.NewReader(file.data))
			if err != nil {
				cleanup()
				return nil, err
			}
			if config.Width > contracts.MaxAttachmentImageDimension || config.Height > contracts.MaxAttachmentImageDimension || config.Width*config.Height > contracts.MaxAttachmentImagePixels {
				cleanup()
				return nil, errors.New("image dimensions are too large")
			}
			item.Width, item.Height = config.Width, config.Height
		}
		target, err := uploads.OpenFile(key, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			cleanup()
			return nil, err
		}
		paths = append(paths, key)
		_, writeErr := target.Write(file.data)
		closeErr := target.Close()
		if err := errors.Join(writeErr, closeErr); err != nil {
			cleanup()
			return nil, err
		}
		attachments = append(attachments, item)
	}
	before := len(e.state.Attachments)
	e.state.Attachments = append(e.state.Attachments, attachments...)
	if err := e.persistLocked(); err != nil {
		e.state.Attachments = e.state.Attachments[:before]
		cleanup()
		return nil, err
	}
	e.publishLocked(contracts.Event{Kind: "attachments.imported"})
	return attachments, nil
}

func (e *Engine) ReadResource(ctx context.Context, ref string) ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.checkLocked(ctx); err != nil {
		return nil, err
	}
	return e.resourceLocked(ref)
}
func (e *Engine) resourceLocked(ref string) ([]byte, error) {
	file, err := e.openResourceLocked(ref)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, contracts.MaxAttachmentBytes+1))
	if len(data) > contracts.MaxAttachmentBytes {
		return nil, errors.New("managed resource exceeds size limit")
	}
	return data, err
}

// Open through os.Root before exposing a host path, preserving the same
// registered-root and symlink boundary used by resource reads.
func (e *Engine) openResourceLocked(ref string) (*os.File, error) {
	if !strings.HasPrefix(ref, resourcePrefix) {
		return nil, errors.New("unknown resource")
	}
	parts := strings.Split(strings.TrimPrefix(ref, resourcePrefix), "/")
	if len(parts) != 2 || !managedResourceComponent(parts[0]) || !managedResourceComponent(parts[1]) {
		return nil, errors.New("invalid managed resource")
	}
	directory, err := e.storage.Resolve(parts[0])
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	file, err := root.Open(filepath.Join("uploads", parts[1]))
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, errors.New("managed resource must be a regular file")
	}
	if info.Size() > contracts.MaxAttachmentBytes {
		_ = file.Close()
		return nil, errors.New("managed resource exceeds size limit")
	}
	return file, nil
}

func managedResourceRef(rootID, key string) string { return resourcePrefix + rootID + "/" + key }

func managedResourceComponent(value string) bool {
	if value == "" || value == "." || value == ".." {
		return false
	}
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-' || char == '_' || char == '.' {
			continue
		}
		return false
	}
	return true
}

func managedUploadsRoot(directory string) (*os.Root, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if err := root.MkdirAll("uploads", 0700); err != nil {
		return nil, err
	}
	return root.OpenRoot("uploads")
}

func (e *Engine) inputMessageLocked(text string, attachments []contracts.Attachment) (model.Message, error) {
	message := model.Message{Role: "user", Text: text, UserInput: true, InputIDs: []string{id("input")}}
	if len(attachments) > contracts.MaxAttachmentCount {
		return message, errors.New("too many attachments")
	}
	for _, input := range attachments {
		var stored *contracts.Attachment
		for i := range e.state.Attachments {
			if e.state.Attachments[i].ID == input.ID {
				stored = &e.state.Attachments[i]
				break
			}
		}
		if stored == nil {
			return message, errors.New("attachment not found")
		}
		if strings.HasPrefix(stored.MIME, "image/") {
			data, err := e.resourceLocked(stored.AssetRef)
			if err != nil {
				return message, err
			}
			message.Images = append(message.Images, model.Image{
				MIME: stored.MIME, Data: base64.StdEncoding.EncodeToString(data),
				Width: stored.Width, Height: stored.Height,
			})
		} else {
			file, err := e.openResourceLocked(stored.AssetRef)
			if err != nil {
				return message, err
			}
			path := file.Name()
			if err := file.Close(); err != nil {
				return message, err
			}
			message.Text += fmt.Sprintf("\n\n<attachment name=%q path=%q />", stored.Name, path)
		}
	}
	return message, nil
}
