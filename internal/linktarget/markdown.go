package linktarget

import (
	"regexp"
	"strings"
)

var markdownTitle = regexp.MustCompile(`(?s)^(.*?)([ \t]+(?:"[^"]*"|'[^']*'|\([^()]*\)))$`)

// NormalizeMarkdown protects Windows drive destinations from Markdown escape
// processing and space termination. Link labels and fenced/inline code retain
// their original bytes; this function does not make invalid links clickable.
func NormalizeMarkdown(src string) string {
	var out strings.Builder
	out.Grow(len(src))
	fence := byte(0)
	fenceLength := 0
	inlineLength := 0
	for start := 0; start < len(src); {
		end := strings.IndexByte(src[start:], '\n')
		if end < 0 {
			end = len(src)
		} else {
			end += start + 1
		}
		line := src[start:end]
		marker, length, closing := fenceMarker(line)
		if fence != 0 {
			out.WriteString(line)
			if marker == fence && length >= fenceLength && closing {
				fence, fenceLength = 0, 0
			}
		} else if marker != 0 && inlineLength == 0 {
			out.WriteString(line)
			fence, fenceLength = marker, length
		} else {
			normalizeLine(&out, line, &inlineLength)
		}
		start = end
	}
	return out.String()
}

func fenceMarker(line string) (byte, int, bool) {
	i := 0
	for i < len(line) && i < 4 && line[i] == ' ' {
		i++
	}
	if i > 3 || i == len(line) || line[i] != '`' && line[i] != '~' {
		return 0, 0, false
	}
	marker, start := line[i], i
	for i < len(line) && line[i] == marker {
		i++
	}
	if i-start < 3 {
		return 0, 0, false
	}
	return marker, i - start, strings.TrimSpace(line[i:]) == ""
}

func normalizeLine(out *strings.Builder, line string, inlineLength *int) {
	for i := 0; i < len(line); {
		if line[i] == '`' && !escaped(line, i) {
			end := i + 1
			for end < len(line) && line[end] == '`' {
				end++
			}
			run := end - i
			if *inlineLength == 0 {
				*inlineLength = run
			} else if *inlineLength == run {
				*inlineLength = 0
			}
			out.WriteString(line[i:end])
			i = end
			continue
		}
		if *inlineLength == 0 && line[i] == ']' && i+1 < len(line) && line[i+1] == '(' && !escaped(line, i) {
			end := destinationEnd(line, i+2)
			if end >= 0 {
				out.WriteString("](")
				out.WriteString(normalizeDestination(line[i+2 : end]))
				out.WriteByte(')')
				i = end + 1
				continue
			}
		}
		out.WriteByte(line[i])
		i++
	}
}

func escaped(value string, offset int) bool {
	count := 0
	for i := offset - 1; i >= 0 && value[i] == '\\'; i-- {
		count++
	}
	return count%2 != 0
}

func destinationEnd(line string, start int) int {
	depth := 0
	angle := false
	for i := start; i < len(line); i++ {
		switch line[i] {
		case '<':
			angle = true
		case '>':
			angle = false
		case '(':
			if !angle {
				depth++
			}
		case ')':
			if angle {
				continue
			}
			if depth == 0 {
				return i
			}
			depth--
		}
	}
	return -1
}

func normalizeDestination(value string) string {
	trimmed := strings.TrimSpace(value)
	prefix := value[:len(value)-len(strings.TrimLeft(value, " \t\r\n"))]
	suffix := value[len(strings.TrimRight(value, " \t\r\n")):]
	destination, title := trimmed, ""
	if strings.HasPrefix(destination, "<") {
		if close := strings.IndexByte(destination, '>'); close >= 0 {
			title, destination = destination[close+1:], destination[1:close]
		}
	} else if matches := markdownTitle.FindStringSubmatch(destination); matches != nil {
		destination, title = matches[1], matches[2]
	}
	normalized := strings.ReplaceAll(destination, `\`, "/")
	if !isDrive(normalized) || len(normalized) < 3 || normalized[2] != '/' {
		return value
	}
	return prefix + "<" + normalized + ">" + title + suffix
}
