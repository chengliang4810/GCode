// WebSearch queries a Tavily-compatible search endpoint and hands the model
// ranked results. The endpoint and optional bearer token come from a search
// configuration of their own: a search credential is never inherited from the
// model provider, and the empty configuration uses the built-in free endpoint
// without writing an endpoint or credential into saved settings.
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"gcode/internal/model"
)

const (
	// webSearchDefaultEndpoint is the free fallback endpoint. No availability
	// is promised for a third-party service.
	webSearchDefaultEndpoint = "https://tavily.claude-code-best.win"
	webSearchDefaultResults  = 10
	webSearchMaxResults      = 20
	webSearchMaxQueryChars   = 2_000
	webSearchMaxTitleChars   = 300
	webSearchMaxURLChars     = 2_048
	webSearchMaxSnippetChars = 500
	// webSearchMaxResponseBytes is checked while streaming the response, not
	// after the whole payload arrives.
	webSearchMaxResponseBytes = 1 << 20
	webSearchTimeout          = 20 * time.Second
)

// WebSearchConfig is the search backend, independent of model providers. An
// empty endpoint means the free fallback; an empty token sends no
// authorization header.
type WebSearchConfig struct {
	Endpoint string `json:"endpoint,omitempty"`
	Token    string `json:"token,omitempty"`
}

func (c WebSearchConfig) resolvedEndpoint() string {
	trimmed := strings.TrimRight(strings.TrimSpace(c.Endpoint), "/")
	if trimmed == "" {
		return webSearchDefaultEndpoint
	}
	return trimmed
}

type webSearchTool struct {
	config *WebSearchConfig
}

// NewWebSearchTool builds the search tool for the given configuration. A nil
// configuration disables search; an empty one uses the free endpoint.
func NewWebSearchTool(config *WebSearchConfig) Tool {
	return webSearchTool{config: config}
}

func (webSearchTool) Definition() model.Tool {
	return model.Tool{
		Name: "WebSearch",
		Description: "Search the web for current information. Use it when the answer depends " +
			"on something after your training data — a release, an error message, a " +
			"version's behaviour. Prefer WebFetch when you already have the URL.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "minLength": 1, "maxLength": webSearchMaxQueryChars, "description": "What to search for."},
				"num_results": map[string]any{
					"type": "integer", "minimum": 1, "maximum": webSearchMaxResults, "default": webSearchDefaultResults,
					"description": "Number of results to return (1–20, default 10).",
				},
			},
			"required":             []string{"query"},
			"additionalProperties": false,
		},
	}
}

func (webSearchTool) Concurrent() bool { return true }

func (t webSearchTool) Execute(ctx context.Context, _ string, raw json.RawMessage) (model.ToolOutput, error) {
	var input struct {
		Query      string `json:"query"`
		NumResults *int   `json:"num_results"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return model.ToolOutput{}, fmt.Errorf("invalid WebSearch input: %w", err)
	}
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return model.ToolOutput{}, fmt.Errorf("`query` is required")
	}
	if utf8.RuneCountInString(query) > webSearchMaxQueryChars {
		return model.ToolOutput{}, fmt.Errorf("`query` exceeds %d characters", webSearchMaxQueryChars)
	}
	numResults := webSearchDefaultResults
	if input.NumResults != nil {
		if *input.NumResults < 1 || *input.NumResults > webSearchMaxResults {
			return model.ToolOutput{}, fmt.Errorf("`num_results` must be an integer from 1 to %d", webSearchMaxResults)
		}
		numResults = *input.NumResults
	}
	if t.config == nil {
		return model.ToolOutput{Text: "Web search is turned off on this device. Answer without searching, or ask the user to enable it."}, nil
	}

	endpoint, err := url.Parse(t.config.resolvedEndpoint())
	if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return model.ToolOutput{}, fmt.Errorf("search endpoint must be an http(s) base URL")
	}
	if endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || endpoint.Path != "" {
		return model.ToolOutput{}, fmt.Errorf("search endpoint must be an http(s) base URL without credentials, path, query or fragment")
	}
	body, err := json.Marshal(map[string]any{"query": query, "max_results": numResults})
	if err != nil {
		return model.ToolOutput{}, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, webSearchTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint.String()+"/search", bytes.NewReader(body))
	if err != nil {
		return model.ToolOutput{}, fmt.Errorf("search failed: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "gcode-agent/1.0")
	if token := strings.TrimSpace(t.config.Token); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{
		Timeout: webSearchTimeout,
		// Do not follow redirects: a query or search credential must not be
		// sent to another host.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	response, err := client.Do(request)
	if err != nil {
		return model.ToolOutput{}, fmt.Errorf("search failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		// The error body is not read: it may wait indefinitely or echo
		// credentials.
		return model.ToolOutput{Text: fmt.Sprintf("The search endpoint returned HTTP %d.", response.StatusCode)}, nil
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, webSearchMaxResponseBytes+1))
	if err != nil {
		return model.ToolOutput{}, fmt.Errorf("search failed: %w", err)
	}
	if len(payload) > webSearchMaxResponseBytes {
		return model.ToolOutput{}, fmt.Errorf("search response exceeds the %d-byte limit", webSearchMaxResponseBytes)
	}
	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		return model.ToolOutput{}, fmt.Errorf("the search endpoint did not return JSON: %w", err)
	}
	return renderSearchResults(value, query, numResults)
}

// searchResultItems finds the result array in a Tavily-compatible response.
// The field tolerance covers the shapes the endpoint has used; it does not
// make the request side compatible with other search services.
func searchResultItems(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	case map[string]any:
		for _, key := range []string{"results", "items", "data"} {
			if items, ok := typed[key].([]any); ok {
				return items
			}
		}
	}
	return nil
}

type searchResult struct{ title, url, snippet string }

func extractSearchResults(value any, numResults int) []searchResult {
	items := searchResultItems(value)
	results := make([]searchResult, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		resultURL, _ := entry["url"].(string)
		if strings.TrimSpace(resultURL) == "" {
			continue
		}
		title, _ := entry["title"].(string)
		if title == "" {
			title = resultURL
		}
		snippet := ""
		for _, key := range []string{"snippet", "content", "description"} {
			if text, ok := entry[key].(string); ok {
				snippet = text
				break
			}
		}
		results = append(results, searchResult{
			title:   truncateChars(title, webSearchMaxTitleChars),
			url:     truncateChars(resultURL, webSearchMaxURLChars),
			snippet: truncateChars(snippet, webSearchMaxSnippetChars),
		})
		if len(results) >= min(numResults, webSearchMaxResults) {
			break
		}
	}
	return results
}

func renderSearchResults(value any, query string, numResults int) (model.ToolOutput, error) {
	items := searchResultItems(value)
	if items == nil {
		return model.ToolOutput{}, fmt.Errorf("the search endpoint returned no results array; expected Tavily-compatible JSON")
	}
	results := extractSearchResults(value, numResults)
	if len(results) == 0 {
		if len(items) > 0 {
			return model.ToolOutput{}, fmt.Errorf("the search endpoint returned no readable result URLs")
		}
		return model.ToolOutput{Text: fmt.Sprintf("No results for `%s`.", query)}, nil
	}
	entries := make([]string, 0, len(results))
	for index, result := range results {
		entry := fmt.Sprintf("%d. %s\n   %s", index+1, result.title, result.url)
		if result.snippet != "" {
			entry += "\n   " + result.snippet
		}
		entries = append(entries, entry)
	}
	return model.ToolOutput{Text: fmt.Sprintf("%d result(s) for `%s`:\n\n%s", len(results), query, strings.Join(entries, "\n\n"))}, nil
}
