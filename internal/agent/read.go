package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"gcode/internal/model"
)

const (
	maxReadTextBytes  = 16 << 20
	maxReadTextOutput = 64000
)

type readTool struct{}

func (readTool) Definition() model.Tool {
	return model.Tool{
		Name: "Read",
		Description: "Read UTF-8 text or a PNG, JPEG, GIF, WebP or BMP image from a relative or absolute path. " +
			"Text is numbered and bounded; use the reported next offset to continue. " +
			"Images are returned as visual attachments, automatically oriented, resized and compressed, " +
			"with original/display dimensions; GIF uses its first frame. " +
			"offset and limit apply only to text. SVG is read as text; PDF is not supported.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{"type": "string", "description": "File path"},
				"offset":    map[string]any{"type": "integer", "minimum": 1, "description": "First text line (1-based)"},
				"limit":     map[string]any{"type": "integer", "minimum": 1, "maximum": 2000, "description": "Text lines, default 2000"},
			},
			"required": []string{"file_path"}, "additionalProperties": false,
		},
	}
}

func (readTool) Concurrent() bool { return true }

func (readTool) Execute(ctx context.Context, cwd string, raw json.RawMessage) (model.ToolOutput, error) {
	var args struct {
		Path   string `json:"file_path"`
		Offset *int   `json:"offset"`
		Limit  *int   `json:"limit"`
	}
	if err := decodeSearchInput(raw, &args); err != nil {
		return model.ToolOutput{}, err
	}
	if strings.TrimSpace(args.Path) == "" {
		return model.ToolOutput{}, errors.New("file_path is required")
	}
	offset, limit := 1, 2000
	if args.Offset != nil {
		offset = *args.Offset
	}
	if args.Limit != nil {
		limit = *args.Limit
	}
	if offset < 1 || limit < 1 || limit > 2000 {
		return model.ToolOutput{}, errors.New("offset must be positive; limit must be 1..2000")
	}
	if err := ctx.Err(); err != nil {
		return model.ToolOutput{}, err
	}
	path := args.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return model.ToolOutput{}, err
	}
	if !info.Mode().IsRegular() {
		return model.ToolOutput{}, errors.New("read requires a regular file; use ListDirectory for directories")
	}
	file, err := os.Open(path)
	if err != nil {
		return model.ToolOutput{}, err
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil {
		return model.ToolOutput{}, err
	}
	if !info.Mode().IsRegular() {
		return model.ToolOutput{}, errors.New("read requires a regular file")
	}
	header := make([]byte, 512)
	n, err := file.Read(header)
	if err != nil && !errors.Is(err, io.EOF) {
		return model.ToolOutput{}, err
	}
	header = header[:n]
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return model.ToolOutput{}, err
	}
	mime := http.DetectContentType(header)
	if strings.HasPrefix(mime, "image/") || hasImageExtension(path) {
		if args.Offset != nil || args.Limit != nil {
			return model.ToolOutput{}, errors.New("offset and limit apply to text, not images")
		}
		return readImage(ctx, file, info.Size(), path)
	}
	if bytes.HasPrefix(header, []byte("%PDF-")) || bytes.IndexByte(header, 0) >= 0 {
		return model.ToolOutput{}, errors.New("unsupported binary file; read accepts UTF-8 text or supported images")
	}
	if info.Size() > maxReadTextBytes {
		return model.ToolOutput{}, errors.New("text file exceeds 16 MiB; use grep or another file-processing tool")
	}
	output, err := readText(ctx, file, offset, limit)
	if data, ok := output.Data.(map[string]any); ok {
		data["file"].(map[string]any)["filePath"] = path
	}
	return output, err
}

func hasImageExtension(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".tif", ".tiff", ".heic", ".avif", ".ico":
		return true
	default:
		return false
	}
}

func readText(ctx context.Context, reader io.Reader, offset, limit int) (model.ToolOutput, error) {
	scanner := bufio.NewScanner(io.LimitReader(reader, maxReadTextBytes+1))
	scanner.Buffer(make([]byte, 4096), 2<<20)
	var out, content strings.Builder
	stopped := false
	returned := 0
	line := 0
	for scanner.Scan() {
		line++
		if err := ctx.Err(); err != nil {
			return model.ToolOutput{}, err
		}
		if !utf8.Valid(scanner.Bytes()) || bytes.IndexByte(scanner.Bytes(), 0) >= 0 {
			return model.ToolOutput{}, errors.New("file is not UTF-8 text")
		}
		if stopped || line < offset {
			continue
		}
		if line-offset >= limit {
			fmt.Fprintf(&out, "\n[output truncated; continue with offset=%d]", line)
			stopped = true
			continue
		}
		numbered := fmt.Sprintf("%d: %s\n", line, scanner.Text())
		if out.Len()+len(numbered) > maxReadTextOutput-160 {
			if out.Len() == 0 {
				end := min(len(numbered), maxReadTextOutput-160)
				for !utf8.ValidString(numbered[:end]) {
					end--
				}
				out.WriteString(numbered[:end])
				fmt.Fprintf(&out, "\n[line %d truncated; next line offset=%d]", line, line+1)
			} else {
				fmt.Fprintf(&out, "\n[output truncated; continue with offset=%d]", line)
			}
			stopped = true
			continue
		}
		out.WriteString(numbered)
		content.WriteString(scanner.Text())
		content.WriteByte('\n')
		returned++
	}
	if err := scanner.Err(); err != nil {
		return model.ToolOutput{}, fmt.Errorf("read text: %w", err)
	}
	if line == 0 && offset == 1 {
		out.WriteString("[empty file]")
	}
	if offset > line && !(line == 0 && offset == 1) {
		return model.ToolOutput{}, fmt.Errorf("offset %d exceeds file length (%d lines)", offset, line)
	}
	return model.ToolOutput{Text: out.String(), Data: map[string]any{"type": "text", "file": map[string]any{"filePath": "", "content": content.String(), "numLines": returned, "startLine": offset, "totalLines": line}}}, nil
}
