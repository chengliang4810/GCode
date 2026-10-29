package mock

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gcode/internal/contracts"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

const managedResourcePrefix = "resource://managed/"
const maxBackgroundBytes = 50 << 20

var _ contracts.ResourceService = (*Server)(nil)

func normalizeAdjustment(v contracts.BackgroundAdjustment) contracts.BackgroundAdjustment {
	if math.IsNaN(float64(v.FocalX)) || math.IsInf(float64(v.FocalX), 0) {
		v.FocalX = .5
	}
	if math.IsNaN(float64(v.FocalY)) || math.IsInf(float64(v.FocalY), 0) {
		v.FocalY = .5
	}
	if math.IsNaN(float64(v.Zoom)) || math.IsInf(float64(v.Zoom), 0) {
		v.Zoom = 1
	}
	v.FocalX = float32(clamp(float64(v.FocalX), 0, 1))
	v.FocalY = float32(clamp(float64(v.FocalY), 0, 1))
	v.Zoom = float32(clamp(float64(v.Zoom), 1, 4))
	return v
}

func readBackground(path string) ([]byte, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", fmt.Errorf("open background image: %w", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxBackgroundBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("read background image: %w", err)
	}
	if len(data) > maxBackgroundBytes {
		return nil, "", errors.New("background image exceeds 50 MiB")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 || uint64(config.Width)*uint64(config.Height) > 64_000_000 {
		return nil, "", errors.New("this background image is unsupported or damaged; choose a valid image such as PNG or JPEG")
	}
	if _, _, err = image.Decode(bytes.NewReader(data)); err != nil {
		return nil, "", errors.New("this background image is unsupported or damaged; choose a valid image such as PNG or JPEG")
	}
	return data, format, nil
}

// updateBackground commits the image pointer to disk before publishing it or
// retiring a previous managed image. Failed commits preserve the active image.
func (s *Server) updateBackground(ctx context.Context, c contracts.Command) error {
	var data []byte
	var format string
	var err error
	if c.Kind == "background.install" {
		data, format, err = readBackground(c.Path)
		if err != nil {
			return err
		}
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	next := cloneSnapshot(s.state)
	a := &next.Settings.Appearance
	previous := a.BackgroundAssetRef
	var prepared string
	switch c.Kind {
	case "background.install":
		name := s.idLocked("background") + "." + format
		ref := managedResourcePrefix + name
		if s.opts.StatePath != "" {
			prepared = filepath.Join(filepath.Dir(s.opts.StatePath), "backgrounds", name)
			if err = writeManagedImage(prepared, data); err != nil {
				return err
			}
		}
		a.Background = ref
		a.BackgroundAssetRef = ref
		a.BackgroundName = filepath.Base(c.Path)
		a.BackgroundAdjustment = contracts.BackgroundAdjustment{FocalX: .5, FocalY: .5, Zoom: 1}
	case "background.adjust":
		if a.Background == "" || c.Path != a.Background {
			return fmt.Errorf("background changed while adjustment was open: %w", ErrConflict)
		}
		a.BackgroundAdjustment = normalizeAdjustment(*c.Adjustment)
	case "background.remove":
		a.Background, a.BackgroundAssetRef, a.BackgroundName = "", "", ""
		a.BackgroundAdjustment = contracts.BackgroundAdjustment{FocalX: .5, FocalY: .5, Zoom: 1}
	default:
		return errors.New("unsupported background command")
	}
	if err = ctx.Err(); err == nil {
		next.Revision++
		err = s.savePersisted(s.withContextSessionsLocked(persisted{Snapshot: next, Next: s.next}))
	}
	if err != nil {
		if prepared != "" {
			_ = os.Remove(prepared)
		}
		return fmt.Errorf("save background image: %w", err)
	}
	if c.Kind == "background.install" && s.opts.StatePath == "" {
		s.resources[a.BackgroundAssetRef] = slices.Clone(data)
	}
	s.state = next
	s.deliverLocked(c.Kind, "", "", "", "")
	if previous != "" && previous != a.BackgroundAssetRef {
		delete(s.resources, previous)
		if path, ok := s.managedResourcePath(previous); ok {
			_ = os.Remove(path)
		}
	}
	return nil
}

func writeManagedImage(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create managed resource directory: %w", err)
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".resource-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(f.Name(), path)
	}
	return err
}

func (s *Server) managedResourcePath(ref string) (string, bool) {
	if s.opts.StatePath == "" || !strings.HasPrefix(ref, managedResourcePrefix) {
		return "", false
	}
	name := strings.TrimPrefix(ref, managedResourcePrefix)
	if name != filepath.Base(name) || strings.ContainsAny(name, "/\\:") || name == "" {
		return "", false
	}
	directory := "backgrounds"
	if strings.HasPrefix(name, "attachment-") {
		directory = "attachments"
	} else if !strings.HasPrefix(name, "background-") {
		return "", false
	}
	return filepath.Join(filepath.Dir(s.opts.StatePath), directory, name), true
}

func (s *Server) ReadResource(ctx context.Context, ref string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrClosed
	}
	known := ref != "" && ref == s.state.Settings.Appearance.BackgroundAssetRef
	limit := maxBackgroundBytes
	if !known {
		for _, attachment := range s.state.Attachments {
			if attachment.AssetRef == ref && ref != "" {
				known = true
				limit = contracts.MaxAttachmentBytes
				break
			}
		}
	}
	if !known {
		return nil, ErrNotFound
	}
	if data, ok := s.resources[ref]; ok {
		return slices.Clone(data), nil
	}
	path, ok := s.managedResourcePath(ref)
	if !ok {
		return nil, ErrNotFound
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read managed image: %w", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	if err == nil && len(data) > limit {
		return nil, errors.New("managed resource exceeds its size limit")
	}
	if err == nil {
		err = ctx.Err()
	}
	return data, err
}
