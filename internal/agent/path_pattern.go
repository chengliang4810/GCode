package agent

import (
	"errors"
	"path"
	"strings"
)

// pathPattern uses segment matching so * never crosses a slash. A whole **
// segment consumes zero or more directories. Bare names match at any depth.
type pathPattern struct {
	alternatives [][]string
}

func compilePathPattern(pattern string) (*pathPattern, error) {
	if pattern == "" || len(pattern) > 4096 {
		return nil, errors.New("path pattern must contain 1..4096 bytes")
	}
	pattern = strings.ReplaceAll(pattern, `\`, "/")
	pattern = strings.TrimPrefix(pattern, "./")
	if strings.HasPrefix(pattern, "/") || strings.Contains(pattern, ":") {
		return nil, errors.New("pattern must be relative; use path to choose the search directory")
	}
	expanded, err := expandPathAlternatives(pattern)
	if err != nil {
		return nil, err
	}
	compiled := &pathPattern{alternatives: make([][]string, 0, len(expanded))}
	for _, alternative := range expanded {
		if !strings.Contains(alternative, "/") {
			alternative = "**/" + alternative
		}
		segments := strings.Split(alternative, "/")
		for _, segment := range segments {
			if segment == "" || segment == "." || segment == ".." {
				return nil, errors.New("pattern contains an empty or relative directory segment")
			}
			if strings.Contains(segment, "**") && segment != "**" {
				return nil, errors.New("** must occupy a complete path segment")
			}
			if segment != "**" {
				if _, err := path.Match(segment, ""); err != nil {
					return nil, errors.New("invalid path pattern character class")
				}
			}
		}
		compiled.alternatives = append(compiled.alternatives, segments)
	}
	return compiled, nil
}

func (p *pathPattern) matches(name string) bool {
	parts := strings.Split(strings.ReplaceAll(name, `\`, "/"), "/")
	for _, pattern := range p.alternatives {
		reachable := make([]bool, len(parts)+1)
		reachable[0] = true
		for _, segment := range pattern {
			next := make([]bool, len(parts)+1)
			if segment == "**" {
				for i := range next {
					next[i] = reachable[i] || (i > 0 && next[i-1])
				}
			} else {
				for i, part := range parts {
					if reachable[i] {
						next[i+1], _ = path.Match(segment, part)
					}
				}
			}
			reachable = next
		}
		if reachable[len(parts)] {
			return true
		}
	}
	return false
}

func expandPathAlternatives(pattern string) ([]string, error) {
	results := []string{}
	var expand func(string, int) error
	expand = func(current string, depth int) error {
		if depth > 16 || len(results) >= 64 {
			return errors.New("path pattern has too many alternatives")
		}
		open, close, nesting := -1, -1, 0
		inClass := false
		start := 0
		choices := []string{}
		for i, ch := range current {
			if ch == '[' {
				inClass = true
			} else if ch == ']' {
				inClass = false
			}
			if inClass {
				continue
			}
			switch ch {
			case '{':
				if nesting == 0 {
					open, start = i, i+1
				}
				nesting++
			case ',':
				if nesting == 1 {
					choices = append(choices, current[start:i])
					start = i + 1
				}
			case '}':
				if nesting == 0 {
					return errors.New("unmatched closing brace in path pattern")
				}
				nesting--
				if nesting == 0 {
					close = i
					choices = append(choices, current[start:i])
				}
			}
			if close >= 0 {
				break
			}
		}
		if open < 0 {
			results = append(results, current)
			return nil
		}
		if close < 0 || len(choices) < 2 {
			return errors.New("path alternatives require balanced braces and a comma")
		}
		for _, choice := range choices {
			if err := expand(current[:open]+choice+current[close+1:], depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := expand(pattern, 0); err != nil {
		return nil, err
	}
	return results, nil
}
