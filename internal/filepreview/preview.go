// Package filepreview identifies files the application renders itself and files
// that need a system application.
package filepreview

import (
	"bytes"
	"path"
	"strings"
	"unicode/utf8"
)

// Builtin reports whether the embedded browser renders the file without a
// system application. SVG stays on the native image view, which keeps zoom,
// pan and theme handling.
func Builtin(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".html", ".htm", ".xhtml", ".pdf":
		return true
	}
	return false
}

func External(name string, data []byte) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".doc", ".docx", ".docm", ".odt", ".rtf",
		".xls", ".xlsx", ".xlsm", ".xlsb", ".ods", ".numbers",
		".ppt", ".pptx", ".pptm", ".odp", ".key", ".pages",
		".zip", ".rar", ".7z", ".tar", ".gz", ".bz2", ".xz",
		".mp3", ".wav", ".flac", ".m4a", ".ogg", ".mp4", ".mov", ".avi", ".mkv",
		".aac", ".aiff", ".wma", ".opus", ".webm", ".m4v",
		".heic", ".heif", ".avif", ".ico", ".psd", ".ai", ".eps", ".ps",
		".ttf", ".otf", ".woff", ".woff2", ".eot",
		".db", ".sqlite", ".sqlite3", ".epub", ".mobi", ".azw", ".azw3",
		".dwg", ".dxf", ".stl", ".obj", ".fbx", ".blend", ".glb", ".gltf",
		".exe", ".dll", ".dmg", ".pkg", ".iso", ".bin":
		return true
	}
	if Builtin(name) {
		return false
	}
	return !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0
}
