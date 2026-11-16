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

	"gcode/internal/attachmentio"
	"gcode/internal/contracts"
	"gcode/internal/model"
)

const resourcePrefix = "resource://managed/"

func (e *Engine) ImportAttachments(ctx context.Context, request contracts.AttachmentImportRequest) ([]contracts.Attachment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	files, err := attachmentio.Plan(request)
	if err != nil {
		return nil, err
	}
	request = request.Clone()
	var staged []struct {
		name string
		data []byte
	}
	remaining := int64(contracts.MaxAttachmentBatchBytes)
	if request.Clipboard != nil {
		remaining -= int64(len(request.Clipboard.Data))
	}
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		f, size, err := attachmentio.Open(file, remaining)
		if err != nil {
			return nil, err
		}
		data, readErr := attachmentio.Read(f, size, min(remaining, int64(contracts.MaxAttachmentBytes)))
		_ = f.Close()
		if readErr != nil {
			return nil, readErr
		}
		remaining -= int64(len(data))
		staged = append(staged, struct {
			name string
			data []byte
		}{file.Name, data})
	}
	if request.Clipboard != nil {
		data := request.Clipboard.Data
		staged = append(staged, struct {
			name string
			data []byte
		}{request.Clipboard.Name, data})
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.checkLocked(ctx); err != nil {
		return nil, err
	}
	rootID := e.storage.ActiveRootID()
	if request.SessionID != "" {
		index := e.sessionIndex(request.SessionID)
		if index < 0 {
			return nil, errors.New("source session not found")
		}
		session := e.state.Sessions[index]
		rootID = session.StorageRootID
	}
	var uploads *os.Root
	var attachments []contracts.Attachment
	var paths []string
	cleanup := func() {
		if uploads != nil {
			for _, path := range paths {
				_ = uploads.Remove(path)
			}
		}
	}
	err = e.withStateUnlocked(true, func() error {
		directory, err := e.storage.Resolve(rootID)
		if err != nil {
			return err
		}
		uploads, err = managedUploadsRoot(directory)
		if err != nil {
			return err
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
					return err
				}
				if config.Width > contracts.MaxAttachmentImageDimension || config.Height > contracts.MaxAttachmentImageDimension || config.Width*config.Height > contracts.MaxAttachmentImagePixels {
					cleanup()
					return errors.New("image dimensions are too large")
				}
				item.Width, item.Height = config.Width, config.Height
			}
			target, err := uploads.OpenFile(key, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				cleanup()
				return err
			}
			paths = append(paths, key)
			_, writeErr := target.Write(file.data)
			closeErr := target.Close()
			if err := errors.Join(writeErr, closeErr); err != nil {
				cleanup()
				return err
			}
			attachments = append(attachments, item)
		}
		return nil
	})
	if uploads != nil {
		defer uploads.Close()
	}
	if err != nil {
		return nil, err
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
	var data []byte
	err := e.withStateUnlocked(true, func() error {
		var err error
		data, err = e.resourceLocked(ref)
		return err
	})
	return data, err
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
	var message model.Message
	err := e.withStateUnlocked(false, func() error {
		var err error
		message, err = e.inputMessage(text, attachments)
		return err
	})
	return message, err
}

// The transaction gate keeps the attachment catalog stable during resource I/O.
func (e *Engine) inputMessage(text string, attachments []contracts.Attachment) (model.Message, error) {
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
