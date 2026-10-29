package mock

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"io"
	"math"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"gcode/internal/contracts"
)

var _ contracts.AttachmentImporter = (*Server)(nil)

type stagedAttachment struct {
	attachment contracts.Attachment
	data       []byte
}

func (s *Server) ImportAttachments(ctx context.Context, request contracts.AttachmentImportRequest) ([]contracts.Attachment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	count := len(request.Paths)
	if request.Clipboard != nil {
		count++
	}
	if count == 0 || count > contracts.MaxAttachmentCount {
		return nil, fmt.Errorf("select between 1 and %d attachments", contracts.MaxAttachmentCount)
	}
	if request.Clipboard != nil && len(request.Clipboard.Data) > contracts.MaxAttachmentBytes {
		return nil, errors.New("attachment exceeds 24 MiB")
	}
	request = request.Clone()
	staged := make([]stagedAttachment, 0, count)
	var total int64
	for _, path := range request.Paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		data, err := readAttachmentFile(path)
		if err != nil {
			return nil, err
		}
		total += int64(len(data))
		if total > contracts.MaxAttachmentBatchBytes {
			return nil, errors.New("attachment batch exceeds 96 MiB")
		}
		item, err := stageAttachment(filepath.Base(path), "", data)
		if err != nil {
			return nil, err
		}
		staged = append(staged, item)
	}
	if clipboard := request.Clipboard; clipboard != nil {
		if len(clipboard.Data) == 0 {
			return nil, errors.New("clipboard attachment is empty")
		}
		total += int64(len(clipboard.Data))
		if total > contracts.MaxAttachmentBatchBytes {
			return nil, errors.New("attachment batch exceeds 96 MiB")
		}
		item, err := stageAttachment(clipboard.Name, clipboard.MIME, clipboard.Data)
		if err != nil {
			return nil, err
		}
		staged = append(staged, item)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrClosed
	}
	next := cloneSnapshot(s.state)
	result := make([]contracts.Attachment, 0, len(staged))
	prepared := make([]string, 0, len(staged))
	cleanup := func() {
		for _, path := range prepared {
			_ = os.Remove(path)
		}
	}
	for i := range staged {
		item := &staged[i]
		item.attachment.ID = s.idLocked("attachment")
		item.attachment.AssetRef = managedResourcePrefix + item.attachment.ID + attachmentExtension(item.attachment.MIME)
		item.attachment.Status = "ready"
		if request.Transfer {
			item.attachment.Status = "transferring"
		}
		if s.opts.StatePath != "" {
			path, _ := s.managedResourcePath(item.attachment.AssetRef)
			if err := writeManagedImage(path, item.data); err != nil {
				cleanup()
				return nil, fmt.Errorf("store attachment: %w", err)
			}
			prepared = append(prepared, path)
		}
		next.Attachments = append(next.Attachments, item.attachment)
		result = append(result, item.attachment)
	}
	if err := ctx.Err(); err != nil {
		cleanup()
		return nil, err
	}
	next.Revision++
	if err := s.savePersisted(s.withContextSessionsLocked(persisted{Snapshot: next, Next: s.next})); err != nil {
		cleanup()
		return nil, fmt.Errorf("save attachment catalog: %w", err)
	}
	if s.opts.StatePath == "" {
		for _, item := range staged {
			s.resources[item.attachment.AssetRef] = slices.Clone(item.data)
		}
	}
	s.state = next
	s.deliverLocked("attachments.imported", "", "", "", "")
	if request.Transfer {
		s.startAttachmentTransferLocked(ctx, result)
	}
	return result, nil
}

func readAttachmentFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open attachment: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("choose a regular file for the attachment")
	}
	if info.Size() > contracts.MaxAttachmentBytes {
		return nil, errors.New("attachment exceeds 24 MiB")
	}
	data, err := io.ReadAll(io.LimitReader(f, contracts.MaxAttachmentBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > contracts.MaxAttachmentBytes {
		return nil, errors.New("attachment exceeds 24 MiB")
	}
	return data, nil
}

func stageAttachment(name, hintedMIME string, data []byte) (stagedAttachment, error) {
	if len(data) > contracts.MaxAttachmentBytes {
		return stagedAttachment{}, errors.New("attachment exceeds 24 MiB")
	}
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.TrimSpace(strings.NewReplacer("\r", "_", "\n", "_", "\x00", "_").Replace(name))
	if name == "" || name == "." || name == "/" {
		name = "clipboard"
	}
	hint := ""
	if hintedMIME != "" {
		var err error
		hint, _, err = mime.ParseMediaType(hintedMIME)
		if err != nil {
			return stagedAttachment{}, errors.New("invalid clipboard MIME type")
		}
		hint = strings.ToLower(hint)
		if hint == "image/jpg" {
			hint = "image/jpeg"
		}
		if hint == "image/x-png" {
			hint = "image/png"
		}
		if hint == "image/x-ms-bmp" {
			hint = "image/bmp"
		}
	}
	kind, _, _ := mime.ParseMediaType(http.DetectContentType(data))
	width, height := 0, 0
	if strings.EqualFold(filepath.Ext(name), ".svg") || hint == "image/svg+xml" {
		var err error
		width, height, err = svgAttachmentSize(data)
		if err != nil {
			return stagedAttachment{}, err
		}
		kind = "image/svg+xml"
	} else if config, format, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
		width, height = config.Width, config.Height
		if err = validateAttachmentDimensions(width, height); err != nil {
			return stagedAttachment{}, err
		}
		if _, _, err = image.Decode(bytes.NewReader(data)); err != nil {
			return stagedAttachment{}, errors.New("attachment image is damaged")
		}
		kind = "image/" + format
	} else if strings.HasPrefix(hint, "image/") || imageAttachmentExtension(name) {
		return stagedAttachment{}, errors.New("attachment image is unsupported or damaged")
	}
	if strings.HasPrefix(hint, "image/") && hint != kind {
		return stagedAttachment{}, errors.New("clipboard image MIME type does not match its bytes")
	}
	if hint != "" && !strings.HasPrefix(kind, "image/") {
		kind = hint
	}
	if filepath.Ext(name) == "" && strings.HasPrefix(kind, "image/") {
		name += attachmentExtension(kind)
	}
	return stagedAttachment{attachment: contracts.Attachment{Name: name, MIME: kind, Size: int64(len(data)), Width: width, Height: height}, data: data}, nil
}

func imageAttachmentExtension(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".webp", ".tiff", ".tif":
		return true
	}
	return false
}

func attachmentExtension(kind string) string {
	switch kind {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/bmp":
		return ".bmp"
	case "image/tiff":
		return ".tiff"
	case "image/svg+xml":
		return ".svg"
	}
	return ".bin"
}

func validateAttachmentDimensions(width, height int) error {
	if width <= 0 || height <= 0 || width > contracts.MaxAttachmentImageDimension || height > contracts.MaxAttachmentImageDimension || uint64(width)*uint64(height) > contracts.MaxAttachmentImagePixels {
		return errors.New("attachment image exceeds 16384 pixels per side or 32 million pixels")
	}
	return nil
}

func svgAttachmentSize(data []byte) (int, int, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	var root *xml.StartElement
	depth, tokens := 0, 0
	for {
		token, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return 0, 0, errors.New("attachment SVG is damaged")
		}
		tokens++
		if tokens > 100_000 {
			return 0, 0, errors.New("attachment SVG is too complex")
		}
		switch v := token.(type) {
		case xml.StartElement:
			depth++
			if depth > 128 {
				return 0, 0, errors.New("attachment SVG is too deeply nested")
			}
			if root == nil {
				if v.Name.Local != "svg" {
					return 0, 0, errors.New("attachment is not an SVG image")
				}
				element := v.Copy()
				root = &element
			} else if depth == 1 {
				return 0, 0, errors.New("attachment SVG has multiple roots")
			}
		case xml.EndElement:
			depth--
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(v)) != "" {
				return 0, 0, errors.New("attachment SVG has text outside its root")
			}
		}
	}
	if root == nil || depth != 0 {
		return 0, 0, errors.New("attachment SVG is incomplete")
	}
	w, h := float64(0), float64(0)
	var view []string
	for _, attr := range root.Attr {
		switch attr.Name.Local {
		case "width":
			w, _ = strconv.ParseFloat(strings.TrimSuffix(attr.Value, "px"), 64)
		case "height":
			h, _ = strconv.ParseFloat(strings.TrimSuffix(attr.Value, "px"), 64)
		case "viewBox":
			view = strings.Fields(strings.ReplaceAll(attr.Value, ",", " "))
		}
	}
	if len(view) == 4 {
		if w <= 0 {
			w, _ = strconv.ParseFloat(view[2], 64)
		}
		if h <= 0 {
			h, _ = strconv.ParseFloat(view[3], 64)
		}
	}
	if w == 0 {
		w = 300
	}
	if h == 0 {
		h = 150
	}
	if math.IsNaN(w) || math.IsInf(w, 0) || math.IsNaN(h) || math.IsInf(h, 0) || w < 0 || h < 0 || w > contracts.MaxAttachmentImageDimension || h > contracts.MaxAttachmentImageDimension {
		return 0, 0, errors.New("invalid SVG image dimensions")
	}
	width, height := int(math.Ceil(w)), int(math.Ceil(h))
	return width, height, validateAttachmentDimensions(width, height)
}

func (s *Server) startAttachmentTransferLocked(ctx context.Context, attachments []contracts.Attachment) {
	ids := make(map[string]struct{}, len(attachments))
	for _, attachment := range attachments {
		ids[attachment.ID] = struct{}{}
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		status := "ready"
		if !s.opts.Instant {
			timer := time.NewTimer(s.opts.TokenInterval)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				status = "cancelled"
			case <-s.done:
				return
			case <-timer.C:
			}
		}
		if ctx.Err() != nil {
			status = "cancelled"
		}
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return
		}
		for i := range s.state.Attachments {
			if _, ok := ids[s.state.Attachments[i].ID]; ok && s.state.Attachments[i].Status == "transferring" {
				s.state.Attachments[i].Status = status
			}
		}
		s.publishLocked("attachments."+status, "", "", "", "")
		s.mu.Unlock()
		_ = s.persist()
	}()
}
