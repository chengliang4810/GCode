// Package linktarget parses link destinations without accessing files or the network.
// File paths are interpreted from their own syntax, independently of the UI host OS.
package linktarget

import (
	"errors"
	"net"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Kind string

const (
	Web    Kind = "web"
	File   Kind = "file"
	Anchor Kind = "anchor"
	Mail   Kind = "mail"
)

// Target keeps link line and character columns one-based. A zero value means
// the destination did not specify that position. Paths use forward slashes.
type Target struct {
	Kind         Kind
	Path         string
	URL          string
	Anchor       string
	Line, Column int
}

var (
	colonPosition = regexp.MustCompile(`^(.+?):([0-9]+)(?::([0-9]+))?(?:-([0-9]+)(?::([0-9]+))?)?$`)
	hashPosition  = regexp.MustCompile(`^(.+)#L([0-9]+)(?:C([0-9]+))?(?:-L?([0-9]+)(?:C([0-9]+))?)?$`)
	schemePrefix  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)
)

// Parse accepts explicit HTTP(S) links, file links and local document anchors.
// Chat file paths cannot contain dot segments, queries, UNC shares or ambiguous
// drive-relative paths. Invalid destinations must not be sent to a system opener.
func Parse(raw string) (Target, error) {
	return parse(raw, false)
}

func parse(raw string, preview bool) (Target, error) {
	if raw == "" || strings.TrimSpace(raw) != raw || hasControl(raw) {
		return Target{}, errors.New("empty or invalid link destination")
	}
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "http:") || strings.HasPrefix(lower, "https:") {
		return parseWeb(raw)
	}
	if strings.HasPrefix(raw, "#") {
		if len(raw) == 1 || hasWhitespace(raw) {
			return Target{}, errors.New("invalid document anchor")
		}
		return Target{Kind: Anchor, Anchor: raw[1:]}, nil
	}
	if strings.HasPrefix(lower, "mailto:") {
		if !preview {
			return Target{}, errors.New("mail links are only supported in document previews")
		}
		if hasWhitespace(raw) || strings.Contains(raw, `\`) {
			return Target{}, errors.New("invalid mail link")
		}
		u, err := url.Parse(raw)
		if err != nil || u.Opaque == "" || strings.ContainsAny(u.Opaque, "\r\n") {
			return Target{}, errors.New("invalid mail link")
		}
		return Target{Kind: Mail, URL: raw}, nil
	}
	if isDrive(raw) && (len(raw) < 3 || raw[2] != '/' && raw[2] != '\\') {
		return Target{}, errors.New("drive-relative paths are unsupported")
	}

	raw, line, column, err := splitPosition(raw)
	if err != nil {
		return Target{}, err
	}
	if strings.HasPrefix(strings.ToLower(raw), "file:") {
		if preview {
			return Target{}, errors.New("absolute file links are not supported in document previews")
		}
		u, err := url.Parse(raw)
		if err != nil || u.Opaque != "" || u.Host != "" || u.RawQuery != "" || u.Fragment != "" {
			return Target{}, errors.New("invalid local file URL")
		}
		raw = u.Path
		if strings.HasPrefix(raw, "/") && isDrive(raw[1:]) {
			raw = raw[1:]
		}
		if !isAbsolute(raw) {
			return Target{}, errors.New("file URL requires an absolute path")
		}
	}
	anchor := ""
	if preview {
		if base, fragment, found := strings.Cut(raw, "#"); found {
			raw, anchor = base, fragment
			if anchor == "" || hasWhitespace(anchor) {
				return Target{}, errors.New("invalid document anchor")
			}
		}
	}
	normalized, err := validatePath(raw, preview)
	if err != nil {
		return Target{}, err
	}
	if preview && isAbsolute(normalized) {
		return Target{}, errors.New("absolute file links are not supported in document previews")
	}
	return Target{Kind: File, Path: normalized, Anchor: anchor, Line: line, Column: column}, nil
}

func parseWeb(raw string) (Target, error) {
	if hasWhitespace(raw) || strings.Contains(raw, `\`) {
		return Target{}, errors.New("web links cannot contain whitespace or backslashes")
	}
	if _, err := url.PathUnescape(raw); err != nil {
		return Target{}, errors.New("web link contains an invalid percent escape")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Opaque != "" || u.User != nil || u.Host == "" ||
		!strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https") {
		return Target{}, errors.New("invalid explicit HTTP(S) URL")
	}
	host := u.Hostname()
	if host == "" || strings.ContainsAny(host, `/%?#@`) {
		return Target{}, errors.New("invalid web host")
	}
	if strings.Contains(host, ":") && net.ParseIP(host) == nil {
		return Target{}, errors.New("invalid IPv6 host")
	}
	if port := u.Port(); port != "" {
		value, err := strconv.Atoi(port)
		if err != nil || value < 1 || value > 65535 {
			return Target{}, errors.New("invalid web port")
		}
	} else if strings.HasSuffix(u.Host, ":") {
		return Target{}, errors.New("empty web port")
	}
	return Target{Kind: Web, URL: raw}, nil
}

func splitPosition(raw string) (string, int, int, error) {
	matches := hashPosition.FindStringSubmatch(raw)
	if matches == nil {
		matches = colonPosition.FindStringSubmatch(raw)
	}
	if matches == nil {
		return raw, 0, 0, nil
	}
	values := [4]int{}
	for i, value := range matches[2:] {
		if value == "" {
			continue
		}
		number, err := strconv.Atoi(value)
		if err != nil || number < 1 {
			return "", 0, 0, errors.New("file positions must be positive integers")
		}
		values[i] = number
	}
	if values[2] != 0 && (values[2] < values[0] ||
		values[2] == values[0] && values[3] != 0 && values[1] > values[3]) {
		return "", 0, 0, errors.New("file range ends before it starts")
	}
	return matches[1], values[0], values[1], nil
}

func validatePath(raw string, allowDots bool) (string, error) {
	normalized := strings.ReplaceAll(raw, `\`, "/")
	if normalized == "" || hasControl(normalized) || strings.HasPrefix(normalized, "//") ||
		strings.ContainsAny(normalized, `?#<>"|*`) {
		return "", errors.New("invalid or unsupported file path")
	}
	remainder := normalized
	if isDrive(normalized) {
		if len(normalized) < 3 || normalized[2] != '/' {
			return "", errors.New("drive-relative paths are unsupported")
		}
		remainder = normalized[2:]
	} else if schemePrefix.MatchString(normalized) {
		return "", errors.New("unsupported link scheme")
	}
	if strings.Contains(remainder, ":") {
		return "", errors.New("invalid file path or position")
	}
	for _, component := range strings.Split(remainder, "/") {
		if component == "." || component == ".." {
			if !allowDots {
				return "", errors.New("dot segments are unsupported in chat file links")
			}
		}
	}
	if strings.TrimSpace(normalized) != normalized {
		return "", errors.New("file path has surrounding whitespace")
	}
	return normalized, nil
}

func isDrive(value string) bool {
	return len(value) >= 2 && (value[0] >= 'A' && value[0] <= 'Z' ||
		value[0] >= 'a' && value[0] <= 'z') && value[1] == ':'
}

func isAbsolute(value string) bool {
	value = strings.ReplaceAll(value, `\`, "/")
	return strings.HasPrefix(value, "/") || isDrive(value) && len(value) >= 3 && value[2] == '/'
}

func hasWhitespace(value string) bool {
	return strings.ContainsFunc(value, unicode.IsSpace)
}

func hasControl(value string) bool {
	return strings.ContainsFunc(value, unicode.IsControl) || !utf8.ValidString(value)
}

// ResolveFile resolves a path lexically using the root's path syntax. It does not
// consult the local OS or filesystem. Outside files have an empty relative path.
func ResolveFile(filePath, root string) (absolute, relative string, inside bool, err error) {
	root, err = validatePath(root, true)
	if err != nil || !isAbsolute(root) {
		return "", "", false, errors.New("file root must be an absolute local path")
	}
	filePath, err = validatePath(filePath, true)
	if err != nil {
		return "", "", false, err
	}
	root = cleanPath(root)
	windows := isDrive(root)
	if isAbsolute(filePath) {
		if isDrive(filePath) != windows {
			return "", "", false, errors.New("file path and root use different path syntax")
		}
		absolute = cleanPath(filePath)
	} else {
		absolute = cleanPath(root + "/" + filePath)
	}
	rootParts := strings.Split(strings.Trim(root, "/"), "/")
	absoluteParts := strings.Split(strings.Trim(absolute, "/"), "/")
	if root == "/" {
		rootParts = nil
	}
	if absolute == "/" {
		absoluteParts = nil
	}
	if len(absoluteParts) < len(rootParts) {
		return absolute, "", false, nil
	}
	for i, component := range rootParts {
		same := component == absoluteParts[i]
		if windows {
			same = strings.EqualFold(component, absoluteParts[i])
		}
		if !same {
			return absolute, "", false, nil
		}
	}
	relative = strings.Join(absoluteParts[len(rootParts):], "/")
	if relative == "" {
		relative = "."
	}
	return absolute, relative, true, nil
}

func cleanPath(value string) string {
	if isDrive(value) {
		return value[:2] + path.Clean(value[2:])
	}
	return path.Clean(value)
}

// CharacterColumn converts a one-based Unicode character column into a zero-based
// UTF-8 byte column. Positions outside a line are clamped to its end.
func CharacterColumn(text string, line, column int) int {
	if line < 1 || column < 1 {
		return 0
	}
	current := text
	for i := 1; i < line; i++ {
		_, rest, found := strings.Cut(current, "\n")
		if !found {
			return 0
		}
		current = rest
	}
	current, _, _ = strings.Cut(current, "\n")
	current = strings.TrimSuffix(current, "\r")
	characters := 1
	for offset := range current {
		if characters >= column {
			return offset
		}
		characters++
	}
	return len(current)
}

// ParsePreview resolves document-relative destinations against the current
// document's directory. Absolute file destinations are intentionally unsupported.
func ParsePreview(raw, documentPath string) (Target, error) {
	target, err := parse(raw, true)
	if err != nil || target.Kind != File {
		return target, err
	}
	documentPath, err = validatePath(documentPath, true)
	if err != nil || !isAbsolute(documentPath) {
		return Target{}, errors.New("preview document must have an absolute local path")
	}
	directory := path.Dir(documentPath)
	if isDrive(directory) && len(directory) == 2 {
		directory += "/"
	}
	target.Path, _, _, err = ResolveFile(target.Path, directory)
	return target, err
}
