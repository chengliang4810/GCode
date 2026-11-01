package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"gcode/internal/contracts"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

var _ contracts.LinkedFileService = (*Engine)(nil)

const linkedTextLimit = 16 << 20

// The temporary bytes are written without this lock. Only publishing a complete
// immutable resource is serialized, independently from the engine state lock.
var linkedResourcePublish sync.Mutex

func (e *Engine) ReadLinkedFile(ctx context.Context, request contracts.LinkedFileRequest) (contracts.LinkedFileResult, error) {
	var result contracts.LinkedFileResult
	if request.Path == "" || len(request.Path) > 32<<10 || strings.ContainsRune(request.Path, 0) {
		return result, errors.New("invalid linked file path")
	}
	// Copy ownership metadata; filesystem reads and image validation happen
	// outside the core lock so opening a file does not stop model events.
	e.mu.Lock()
	if err := e.checkLocked(ctx); err != nil {
		e.mu.Unlock()
		return result, err
	}
	var session contracts.Session
	if request.SessionID != "" {
		index := e.sessionIndex(request.SessionID)
		if index < 0 {
			e.mu.Unlock()
			return result, errors.New("source session not found")
		}
		if err := e.ensureSessionDataLocked(request.SessionID); err != nil {
			e.mu.Unlock()
			return result, err
		}
		session = e.state.Sessions[index]
	}
	var project contracts.Project
	for _, candidate := range e.state.Projects {
		if candidate.ID == session.ProjectID {
			project = candidate
			break
		}
	}
	files := slices.Clone(e.state.Files)
	rootID := e.storage.ActiveRootID()
	if request.SessionID != "" {
		rootID = session.StorageRootID
	}
	dataRoot, err := e.storage.Resolve(rootID)
	e.mu.Unlock()
	if err != nil {
		return result, err
	}

	path := filepath.FromSlash(request.Path)
	relative := !filepath.IsAbs(path)
	if relative {
		if session.CWD == "" || !filepath.IsLocal(path) {
			return result, errors.New("relative file path must stay within the source session working directory")
		}
		path = filepath.Join(session.CWD, path)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return result, err
	}
	absolute, err = filepath.EvalSymlinks(absolute)
	if err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return result, err
	}
	if !info.Mode().IsRegular() {
		return result, errors.New("linked file must be a regular file")
	}
	extension := strings.ToLower(filepath.Ext(absolute))
	imageFormat := linkedImageFormat(extension)
	limit := int64(linkedTextLimit)
	if imageFormat != "" {
		limit = contracts.MaxAttachmentBytes
	}
	if info.Size() > limit {
		return result, fmt.Errorf("linked file exceeds %d MiB", limit>>20)
	}
	var handle *os.File
	if relative {
		root, openErr := os.OpenRoot(session.CWD)
		if openErr != nil {
			return result, openErr
		}
		// Open the original relative path under os.Root, which also rejects
		// symlink escapes even if the directory changes during the read.
		handle, err = root.Open(filepath.FromSlash(request.Path))
		_ = root.Close()
	} else {
		handle, err = os.Open(absolute)
	}
	if err != nil {
		return result, err
	}
	defer handle.Close()
	openedInfo, err := handle.Stat()
	if err != nil {
		return result, err
	}
	if !openedInfo.Mode().IsRegular() {
		return result, errors.New("linked file must be a regular file")
	}
	data, err := io.ReadAll(io.LimitReader(linkedContextReader{ctx: ctx, reader: handle}, limit+1))
	if err != nil {
		return result, err
	}
	if int64(len(data)) > limit {
		return result, fmt.Errorf("linked file exceeds %d MiB", limit>>20)
	}
	file := contracts.File{
		ID: linkedFileID(dataRoot, request.SessionID, absolute), Path: filepath.ToSlash(absolute),
		Name: filepath.Base(absolute), Kind: "file", Loaded: true, ReadOnly: true, Version: fileVersion(data),
	}
	// Ownership comes from the registered project and source session, not the
	// asynchronously populated file index. Other checkouts/CWDs remain read-only.
	if project.ID != "" && linkedWithin(session.CWD, absolute) && linkedWithin(project.Path, absolute) {
		projectRoot, _ := filepath.EvalSymlinks(project.Path)
		relative, relErr := filepath.Rel(projectRoot, absolute)
		if relErr != nil || !filepath.IsLocal(relative) {
			return result, errors.New("linked file is outside its project")
		}
		file.Path = filepath.ToSlash(relative)
		file.ID, file.ProjectID, file.ReadOnly = project.ID+":"+file.Path, project.ID, false
		if parent := filepath.ToSlash(filepath.Dir(relative)); parent != "." {
			file.ParentID = project.ID + ":" + parent
		}
		for _, candidate := range files {
			if candidate.ProjectID == project.ID && candidate.Kind != "directory" &&
				sameLinkedPath(filepath.Join(projectRoot, filepath.FromSlash(candidate.Path)), absolute) {
				file.ID, file.ProjectID, file.Path, file.ParentID = candidate.ID, candidate.ProjectID, candidate.Path, candidate.ParentID
				file.ReadOnly = candidate.ReadOnly
				break
			}
		}
	}
	if imageFormat == "" {
		if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
			return result, errors.New("linked file is not UTF-8 text")
		}
		file.Content = string(data)
	} else {
		if err := validateLinkedImage(ctx, data, imageFormat); err != nil {
			return result, err
		}
		file.Kind = "image"
		key, err := cacheLinkedImage(ctx, dataRoot, extension, data)
		if err != nil {
			return result, err
		}
		file.AssetRef = managedResourceRef(rootID, key)
	}
	e.mu.Lock()
	err = e.checkLocked(ctx)
	if err == nil && file.ProjectID != "" {
		sessionIndex := e.sessionIndex(request.SessionID)
		projectIndex := slices.IndexFunc(e.state.Projects, func(candidate contracts.Project) bool {
			return candidate.ID == project.ID && candidate.Path == project.Path
		})
		index := slices.IndexFunc(e.state.Files, func(candidate contracts.File) bool {
			return candidate.ID == file.ID && candidate.ProjectID == file.ProjectID && candidate.Path == file.Path
		})
		if projectIndex < 0 || sessionIndex < 0 || e.state.Sessions[sessionIndex].CWD != session.CWD ||
			e.state.Sessions[sessionIndex].ProjectID != session.ProjectID {
			err = errors.New("linked file project changed while reading")
		} else if index < 0 || e.state.Files[index] != file {
			if index < 0 {
				e.state.Files = append(e.state.Files, file)
			} else {
				e.state.Files[index] = file
			}
			// Publish the authoritative loaded buffer before returning it.
			// Later message deltas use this snapshot's Files projection, so
			// they cannot restore an older unloaded/empty file in the client.
			e.publishLocked(contracts.Event{Kind: "file.read", SessionID: request.SessionID})
		}
	}
	e.mu.Unlock()
	if err != nil {
		return result, err
	}
	return contracts.LinkedFileResult{File: file, AbsolutePath: filepath.ToSlash(absolute)}, nil
}

func cacheLinkedImage(ctx context.Context, directory, extension string, data []byte) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	key := "linked-" + hex.EncodeToString(sum[:]) + extension
	root, err := managedUploadsRoot(directory)
	if err != nil {
		return "", err
	}
	defer root.Close()
	exists := func() (bool, error) {
		info, err := root.Lstat(key)
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if !info.Mode().IsRegular() || info.Size() != int64(len(data)) {
			return false, errors.New("invalid cached linked image")
		}
		return true, nil
	}
	if present, err := exists(); present || err != nil {
		return key, err
	}
	tempName := id("linked-image")
	temp, err := root.OpenFile(tempName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	defer root.Remove(tempName)
	if _, err := io.Copy(temp, linkedContextReader{ctx: ctx, reader: bytes.NewReader(data)}); err != nil {
		_ = temp.Close()
		return "", err
	}
	if err := temp.Close(); err != nil {
		return "", err
	}
	linkedResourcePublish.Lock()
	defer linkedResourcePublish.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	// A concurrent caller may already have published the same complete bytes.
	// Never replace or remove that caller's resource, including on failure.
	if present, err := exists(); present || err != nil {
		return key, err
	}
	if err := root.Rename(tempName, key); err != nil {
		return "", err
	}
	return key, nil
}

func linkedFileID(dataRoot, sessionID, path string) string {
	hostname, _ := os.Hostname()
	sum := sha256.Sum256([]byte(hostname + "\x00" + dataRoot + "\x00" + sessionID + "\x00" + path))
	return "linked:" + hex.EncodeToString(sum[:])
}

func linkedWithin(root, path string) bool {
	if root == "" {
		return false
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(canonical, path)
	return err == nil && filepath.IsLocal(relative)
}

func sameLinkedPath(a, b string) bool {
	relative, err := filepath.Rel(a, b)
	return err == nil && relative == "."
}

func linkedImageFormat(extension string) string {
	switch extension {
	case ".png":
		return "png"
	case ".jpg", ".jpeg":
		return "jpeg"
	case ".gif":
		return "gif"
	case ".webp":
		return "webp"
	case ".bmp":
		return "bmp"
	case ".tif", ".tiff":
		return "tiff"
	case ".svg":
		return "svg"
	}
	return ""
}

func validateLinkedImage(ctx context.Context, data []byte, expected string) error {
	if expected == "svg" {
		decoder := xml.NewDecoder(linkedContextReader{ctx: ctx, reader: bytes.NewReader(data)})
		root, depth := false, 0
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				if root && depth == 0 {
					return nil
				}
				return errors.New("invalid SVG image")
			}
			if err != nil {
				return fmt.Errorf("invalid SVG image: %w", err)
			}
			switch token := token.(type) {
			case xml.StartElement:
				if depth == 0 {
					if root || token.Name.Local != "svg" || token.Name.Space != "" && token.Name.Space != "http://www.w3.org/2000/svg" {
						return errors.New("invalid SVG root")
					}
					root = true
				}
				depth++
			case xml.EndElement:
				depth--
			case xml.Directive:
				return errors.New("SVG directives are not supported")
			case xml.CharData:
				if depth == 0 && len(bytes.TrimSpace(token)) != 0 {
					return errors.New("invalid text outside SVG root")
				}
			}
		}
	}
	config, actual, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("invalid linked image: %w", err)
	}
	if actual != expected {
		return errors.New("linked image format does not match its extension")
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > contracts.MaxAttachmentImageDimension ||
		config.Height > contracts.MaxAttachmentImageDimension || int64(config.Width)*int64(config.Height) > contracts.MaxAttachmentImagePixels {
		return errors.New("linked image dimensions are too large")
	}
	if _, _, err := image.Decode(linkedContextReader{ctx: ctx, reader: bytes.NewReader(data)}); err != nil {
		return fmt.Errorf("invalid linked image: %w", err)
	}
	return ctx.Err()
}

type linkedContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r linkedContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
