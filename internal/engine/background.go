package engine

import (
	"bytes"
	"context"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"path/filepath"
	"strings"

	"gcode/internal/attachmentio"
	"gcode/internal/contracts"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

func (e *Engine) updateBackground(ctx context.Context, command contracts.Command) error {
	var data []byte
	var format string
	if command.Kind == "background.install" {
		file, size, err := attachmentio.Open(attachmentio.File{Path: command.Path}, contracts.MaxAttachmentBytes)
		if err != nil {
			return err
		}
		data, err = attachmentio.Read(file, size, contracts.MaxAttachmentBytes)
		_ = file.Close()
		if err != nil {
			return err
		}
		config, kind, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || config.Width <= 0 || config.Height <= 0 || uint64(config.Width)*uint64(config.Height) > 64_000_000 {
			return errors.New("invalid or oversized background image; choose a PNG or JPEG")
		}
		if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
			return errors.New("damaged background image; choose a valid PNG or JPEG")
		}
		format = kind
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.checkLocked(ctx); err != nil {
		return err
	}
	a := &e.state.Settings.Appearance
	previous := *a
	defaultAdjustment := contracts.BackgroundAdjustment{FocalX: .5, FocalY: .5, Zoom: 1}
	switch command.Kind {
	case "background.install":
		rootID := e.storage.ActiveRootID()
		key := id("background") + "." + format
		directory, err := e.storage.Resolve(rootID)
		if err != nil {
			return err
		}
		uploads, err := managedUploadsRoot(directory)
		if err != nil {
			return err
		}
		defer uploads.Close()
		target, err := uploads.OpenFile(key, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, writeErr := target.Write(data)
		if err := errors.Join(writeErr, target.Sync(), target.Close()); err != nil {
			_ = uploads.Remove(key)
			return err
		}
		a.BackgroundAssetRef = managedResourceRef(rootID, key)
		a.Background = a.BackgroundAssetRef
		a.BackgroundName = filepath.Base(command.Path)
		a.BackgroundAdjustment = defaultAdjustment
	case "background.adjust":
		if a.Background == "" || command.Path != a.Background {
			return errors.New("background changed while adjustment was open")
		}
		adjust := *command.Adjustment
		a.BackgroundAdjustment = contracts.BackgroundAdjustment{
			FocalX: backgroundValue(adjust.FocalX, .5, 0, 1),
			FocalY: backgroundValue(adjust.FocalY, .5, 0, 1),
			Zoom:   backgroundValue(adjust.Zoom, 1, 1, 4),
		}
	case "background.remove":
		a.Background, a.BackgroundAssetRef, a.BackgroundName = "", "", ""
		a.BackgroundAdjustment = defaultAdjustment
	}
	if err := e.persistLocked(); err != nil {
		if command.Kind == "background.install" {
			e.removeBackgroundAssetLocked(a.BackgroundAssetRef)
		}
		*a = previous
		return err
	}
	if previous.BackgroundAssetRef != a.BackgroundAssetRef {
		e.removeBackgroundAssetLocked(previous.BackgroundAssetRef)
	}
	e.publishLocked(contracts.Event{Kind: command.Kind})
	return nil
}

func backgroundValue(value, fallback, lo, hi float32) float32 {
	if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
		return fallback
	}
	return min(max(value, lo), hi)
}

func (e *Engine) removeBackgroundAssetLocked(ref string) {
	if !strings.HasPrefix(ref, resourcePrefix) {
		return
	}
	rootID, key, ok := strings.Cut(strings.TrimPrefix(ref, resourcePrefix), "/")
	if !ok || !managedResourceComponent(rootID) || !managedResourceComponent(key) || !strings.HasPrefix(key, "background-") {
		return
	}
	directory, err := e.storage.Resolve(rootID)
	if err != nil {
		return
	}
	uploads, err := managedUploadsRoot(directory)
	if err == nil {
		defer uploads.Close()
		_ = uploads.Remove(key)
	}
}
