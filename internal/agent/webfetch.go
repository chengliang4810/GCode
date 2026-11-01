// WebFetch retrieves a URL and hands the model readable text. HTML is reduced
// to prose rather than returned raw: a page's markup is mostly navigation and
// styling, and pasting it into the next request costs tokens while telling the
// model nothing. The conversion is deliberately crude — a real reader-mode
// extractor is a project of its own — but it removes tags, scripts and styles,
// which is where the waste is.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"gcode/internal/model"
)

const (
	// webFetchMaxBytes bounds the response read; a page larger than this is
	// not something the model should be reading through a tool call.
	webFetchMaxBytes = 2 << 20
	// webFetchMaxChars bounds the text returned to the model.
	webFetchMaxChars = 100_000
	webFetchTimeout  = 30 * time.Second
	// webErrorBodyChars bounds the body echoed for a 4xx/5xx.
	webErrorBodyChars = 4_000
)

type webFetchTool struct{}

func (webFetchTool) Definition() model.Tool {
	text := map[string]any{"type": "string"}
	return model.Tool{
		Name: "WebFetch",
		Description: "Fetch a URL and return its text. Use WebFetch to read a page you have " +
			"a reason to trust — documentation, a changelog, an issue. HTML is " +
			"reduced to text; very long pages are truncated.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url":    text,
				"prompt": map[string]any{"type": "string", "description": "What you are looking for, so the result can be summarised around it."},
			},
			"required":             []string{"url"},
			"additionalProperties": false,
		},
	}
}

func (webFetchTool) Concurrent() bool { return true }

func (webFetchTool) Execute(ctx context.Context, _ string, raw json.RawMessage) (model.ToolOutput, error) {
	var input struct {
		URL    string `json:"url"`
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return model.ToolOutput{}, fmt.Errorf("invalid WebFetch input: %w", err)
	}
	url := strings.TrimSpace(input.URL)
	if url == "" {
		return model.ToolOutput{}, fmt.Errorf("`url` is required")
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return model.ToolOutput{}, fmt.Errorf("`%s` is not an http(s) URL", url)
	}

	requestCtx, cancel := context.WithTimeout(ctx, webFetchTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, url, nil)
	if err != nil {
		return model.ToolOutput{}, fmt.Errorf("cannot fetch %s: %w", url, err)
	}
	request.Header.Set("User-Agent", "gcode-agent/1.0")
	client := &http.Client{
		Timeout: webFetchTimeout,
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			return nil
		},
	}
	response, err := client.Do(request)
	if err != nil {
		return model.ToolOutput{}, fmt.Errorf("cannot fetch %s: %w", url, err)
	}
	defer response.Body.Close()

	contentType := response.Header.Get("Content-Type")
	// Read one byte past the limit so an oversized body is refused before its
	// end, rather than after downloading all of it.
	body, readErr := io.ReadAll(io.LimitReader(response.Body, webFetchMaxBytes+1))
	if readErr != nil {
		return model.ToolOutput{}, fmt.Errorf("cannot read %s: %w", url, readErr)
	}
	if len(body) > webFetchMaxBytes {
		return model.ToolOutput{}, fmt.Errorf("%s exceeds the %d-byte limit", url, webFetchMaxBytes)
	}

	text := string(body)
	if strings.Contains(strings.ToLower(contentType), "html") {
		text = htmlToText(text)
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		// The body often explains a 4xx better than the code does, so it is
		// reported rather than replaced by the status alone.
		return model.ToolOutput{Text: fmt.Sprintf("%s returned HTTP %d\n\n%s", url, response.StatusCode, truncateChars(text, webErrorBodyChars))}, nil
	}
	truncated := false
	if utf8.RuneCountInString(text) > webFetchMaxChars {
		text = truncateChars(text, webFetchMaxChars)
		truncated = true
	}
	note := ""
	if truncated {
		note = "\n\n… truncated"
	}
	return model.ToolOutput{Text: url + "\n\n" + text + note}, nil
}

// truncateChars cuts text to at most limit runes without splitting a rune.
func truncateChars(text string, limit int) string {
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	count := 0
	for index := range text {
		if count == limit {
			return text[:index]
		}
		count++
	}
	return text
}

// htmlToText reduces HTML to readable text. It removes script/style/noscript/
// template bodies and comments, turns block-level tags into newlines so
// paragraphs do not run together, then drops the remaining tags and decodes
// the entities that actually appear in prose.
func htmlToText(html string) string {
	var out strings.Builder
	out.Grow(len(html) / 2)
	// ASCII folding preserves byte offsets. Compute it once rather than
	// copying the remaining document for every script or style block; non-ASCII
	// bytes are unchanged by ToLower, so offsets stay aligned.
	lower := strings.ToLower(html)
	rest := html

	for {
		start := strings.IndexByte(rest, '<')
		if start < 0 {
			break
		}
		out.WriteString(rest[:start])
		end := strings.IndexByte(rest[start:], '>')
		if end < 0 {
			// An unterminated tag: the rest is text as far as we can tell.
			out.WriteString(rest[start:])
			return finishHTMLText(out.String())
		}
		tag := rest[start+1 : start+end]
		name := tagName(tag)

		// Skip the contents of these entirely — they are not prose.
		if name == "script" || name == "style" || name == "noscript" || name == "template" {
			closing := "</" + name
			consumed := len(html) - len(rest)
			offset := strings.Index(lower[consumed+start:], closing)
			if offset < 0 {
				return finishHTMLText(out.String())
			}
			after := start + offset
			closeEnd := strings.IndexByte(rest[after:], '>')
			if closeEnd < 0 {
				return finishHTMLText(out.String())
			}
			rest = rest[after+closeEnd+1:]
			continue
		}

		if isBlockTag(name) {
			out.WriteByte('\n')
		}
		rest = rest[start+end+1:]
	}
	out.WriteString(rest)
	return finishHTMLText(out.String())
}

// tagName reads the element name from a raw tag body such as "/p" or
// "a href=\"x\"".
func tagName(tag string) string {
	tag = strings.TrimPrefix(tag, "/")
	if index := strings.IndexFunc(tag, func(r rune) bool {
		return r == '/' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	}); index >= 0 {
		tag = tag[:index]
	}
	return strings.ToLower(tag)
}

// isBlockTag reports tags whose boundaries separate text.
func isBlockTag(name string) bool {
	switch name {
	case "p", "div", "br", "li", "tr", "td", "th", "h1", "h2", "h3", "h4", "h5", "h6",
		"section", "article", "header", "footer", "blockquote", "pre", "table", "ul", "ol":
		return true
	}
	return false
}

// finishHTMLText decodes entities and collapses blank-line runs.
func finishHTMLText(text string) string {
	return collapseBlankLines(decodeEntities(text))
}

// decodeEntities covers the entities that appear in ordinary prose. A full
// table would be longer than it is worth; anything unrecognised is left as
// written, which is visible and honest rather than silently wrong.
func decodeEntities(text string) string {
	replacer := strings.NewReplacer(
		"&nbsp;", " ",
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", "\"",
		"&#39;", "'",
		"&apos;", "'",
		"&mdash;", "—",
		"&ndash;", "–",
		"&hellip;", "…",
		"&rsquo;", "\u2019",
		"&lsquo;", "\u2018",
		"&ldquo;", "\u201C",
		"&rdquo;", "\u201D",
	)
	return replacer.Replace(text)
}

// collapseBlankLines collapses runs of blank lines and strips trailing
// whitespace per line.
func collapseBlankLines(text string) string {
	var out strings.Builder
	out.Grow(len(text))
	blankRun := 0
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			blankRun++
			if blankRun > 1 {
				continue
			}
		} else {
			blankRun = 0
		}
		out.WriteString(trimmed)
		out.WriteByte('\n')
	}
	return strings.TrimSpace(out.String())
}
