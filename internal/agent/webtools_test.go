package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gcode/internal/model"
)

func TestHTMLToTextKeepsProseDropsMarkup(t *testing.T) {
	text := htmlToText(`<html><head><title>T</title><style>body{color:red}</style></head><body><h1>Heading</h1><p>First para.</p><p>Second para.</p></body></html>`)
	for _, want := range []string{"Heading", "First para.", "Second para."} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %q", want, text)
		}
	}
	if strings.Contains(text, "color:red") || strings.Contains(text, "<p>") {
		t.Fatalf("markup leaked: %q", text)
	}
}

func TestHTMLToTextDropsScriptBodies(t *testing.T) {
	text := htmlToText(`<p>before</p><script>var x = '<p>fake</p>';</script><p>after</p>`)
	if !strings.Contains(text, "before") || !strings.Contains(text, "after") {
		t.Fatalf("prose lost: %q", text)
	}
	if strings.Contains(text, "fake") {
		t.Fatalf("script body leaked: %q", text)
	}
}

func TestHTMLToTextMixedCaseHiddenBlocksKeepUnicode(t *testing.T) {
	if got := htmlToText(`<p>前文</p><ScRiPt>ignored</sCrIpT><STYLE>ignored</STYLE><p>后文</p>`); got != "前文\n\n后文" {
		t.Fatalf("got %q", got)
	}
	// Many hidden blocks must not degrade the pass into losing prose.
	many := strings.Repeat("<STYLE>ignored</STYLE>", 2000) + "<p>kept</p>"
	if got := htmlToText(many); got != "kept" {
		t.Fatalf("got %q", got)
	}
}

func TestHTMLToTextDecodesEntitiesAndCollapsesBlankLines(t *testing.T) {
	if got := htmlToText(`<p>a &amp; b &lt;tag&gt; &mdash; done</p>`); !strings.Contains(got, "a & b <tag> — done") {
		t.Fatalf("entities: %q", got)
	}
	if got := htmlToText(`<p>one</p><br><br><br><p>two</p>`); strings.Contains(got, "\n\n\n") {
		t.Fatalf("blank runs not collapsed: %q", got)
	}
	if got := htmlToText(`<p>kept</p><p>unclosed`); !strings.Contains(got, "kept") || !strings.Contains(got, "unclosed") {
		t.Fatalf("tail lost: %q", got)
	}
}

func TestWebFetchRefusesNonHTTPSchemes(t *testing.T) {
	for _, url := range []string{"file:///etc/passwd", "ftp://example.test/x", "javascript:alert(1)"} {
		raw, _ := json.Marshal(map[string]string{"url": url})
		if _, err := (webFetchTool{}).Execute(context.Background(), "", raw); err == nil {
			t.Fatalf("%s was not refused", url)
		}
	}
	if _, err := (webFetchTool{}).Execute(context.Background(), "", json.RawMessage(`{}`)); err == nil {
		t.Fatal("missing url accepted")
	}
}

func TestWebFetchReducesHTMLAndReportsStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/page":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body><h1>Doc</h1><p>Body text here.</p><script>x()</script></body></html>`))
		case "/missing":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("no such page"))
		default:
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("plain body"))
		}
	}))
	defer server.Close()

	output, err := (webFetchTool{}).Execute(context.Background(), "", json.RawMessage(`{"url":"`+server.URL+`/page"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.Text, "Doc") || !strings.Contains(output.Text, "Body text here.") || strings.Contains(output.Text, "x()") {
		t.Fatalf("html not reduced: %q", output.Text)
	}

	// A non-HTML body comes back as it is.
	output, err = (webFetchTool{}).Execute(context.Background(), "", json.RawMessage(`{"url":"`+server.URL+`/plain.txt"}`))
	if err != nil || !strings.Contains(output.Text, "plain body") {
		t.Fatalf("plain body: %+v %v", output, err)
	}

	// A 4xx reports the status and the body that explains it.
	output, err = (webFetchTool{}).Execute(context.Background(), "", json.RawMessage(`{"url":"`+server.URL+`/missing"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.Text, "HTTP 404") || !strings.Contains(output.Text, "no such page") {
		t.Fatalf("status body: %q", output.Text)
	}
}

func TestWebFetchStopsAtTheByteLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		chunk := strings.Repeat("x", 64<<10)
		for written := 0; written <= webFetchMaxBytes; written += len(chunk) {
			if _, err := w.Write([]byte(chunk)); err != nil {
				return
			}
		}
	}))
	defer server.Close()
	start := time.Now()
	_, err := (webFetchTool{}).Execute(context.Background(), "", json.RawMessage(`{"url":"`+server.URL+`"}`))
	if err == nil || !strings.Contains(err.Error(), "byte limit") {
		t.Fatalf("oversized body accepted: %v", err)
	}
	if time.Since(start) > 10*time.Second {
		t.Fatal("byte limit waited for the body to end")
	}
}

func TestWebSearchSendsTavilyRequestAndRendersResults(t *testing.T) {
	var gotAuth, gotPath string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"title":"Alpha","url":"https://a.test/1","content":"first snippet"},{"title":"Beta","url":"https://b.test/2","content":"second snippet"}]}`))
	}))
	defer server.Close()

	tool := NewWebSearchTool(&WebSearchConfig{Endpoint: server.URL, Token: "secret"})
	output, err := tool.Execute(context.Background(), "", json.RawMessage(`{"query":"go generics","num_results":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/search" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAuth != "Bearer secret" {
		t.Fatalf("authorization = %q", gotAuth)
	}
	if gotBody["query"] != "go generics" || gotBody["max_results"] != float64(2) {
		t.Fatalf("body = %+v", gotBody)
	}
	for _, want := range []string{"2 result(s)", "Alpha", "https://a.test/1", "first snippet", "Beta"} {
		if !strings.Contains(output.Text, want) {
			t.Fatalf("missing %q in %q", want, output.Text)
		}
	}
}

func TestWebSearchWithoutTokenSendsNoAuthorization(t *testing.T) {
	sawAuth := "unset"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if value, ok := r.Header["Authorization"]; ok {
			sawAuth = strings.Join(value, ",")
		} else {
			sawAuth = ""
		}
		_, _ = w.Write([]byte(`{"results":[{"title":"t","url":"https://x.test"}]}`))
	}))
	defer server.Close()
	if _, err := NewWebSearchTool(&WebSearchConfig{Endpoint: server.URL}).Execute(context.Background(), "", json.RawMessage(`{"query":"q"}`)); err != nil {
		t.Fatal(err)
	}
	if sawAuth != "" {
		t.Fatalf("authorization sent without a token: %q", sawAuth)
	}
}

func TestWebSearchValidationAndDisabled(t *testing.T) {
	// A nil config is an explicit disable, not an empty configuration.
	output, err := NewWebSearchTool(nil).Execute(context.Background(), "", json.RawMessage(`{"query":"q"}`))
	if err != nil || !strings.Contains(output.Text, "turned off") {
		t.Fatalf("disabled search: %+v %v", output, err)
	}
	for _, input := range []string{`{}`, `{"query":"  "}`, `{"query":"q","num_results":0}`, `{"query":"q","num_results":21}`} {
		if _, err := NewWebSearchTool(&WebSearchConfig{}).Execute(context.Background(), "", json.RawMessage(input)); err == nil {
			t.Fatalf("invalid input accepted: %s", input)
		}
	}
	// Endpoints with embedded credentials or paths are refused outright.
	for _, endpoint := range []string{"ftp://x.test", "https://user:pw@x.test", "https://x.test/base"} {
		_, err := NewWebSearchTool(&WebSearchConfig{Endpoint: endpoint}).Execute(context.Background(), "", json.RawMessage(`{"query":"q"}`))
		if err == nil {
			t.Fatalf("endpoint %q accepted", endpoint)
		}
	}
}

func TestWebSearchReportsHttpErrorWithoutReadingBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(strings.Repeat("noise", 1000)))
	}))
	defer server.Close()
	output, err := NewWebSearchTool(&WebSearchConfig{Endpoint: server.URL}).Execute(context.Background(), "", json.RawMessage(`{"query":"q"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.Text, "HTTP 429") || strings.Contains(output.Text, "noise") {
		t.Fatalf("error body echoed: %q", output.Text)
	}
}

// summaryModel records the request it answered and replies with a fixed
// extraction.
type summaryModel struct {
	requests []model.Request
	reply    string
	err      error
	calls    []model.Call
}

func (m *summaryModel) Stream(_ context.Context, request model.Request, _ func(model.Delta) error) (model.Result, error) {
	m.requests = append(m.requests, request)
	if m.err != nil {
		return model.Result{}, m.err
	}
	return model.Result{Message: model.Message{Role: "assistant", Text: m.reply, Calls: m.calls}, Stop: model.StopComplete}, nil
}

func TestWebFetchSummarisesThePageAroundThePrompt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("release 4.2 fixed the crash\nrelease 4.3 added tracing"))
	}))
	defer server.Close()

	bound := &summaryModel{reply: "4.3"}
	tool := webFetchTool{}.withModel(bound)
	output, err := tool.Execute(context.Background(), "", json.RawMessage(`{"url":"`+server.URL+`","prompt":"which release added tracing"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(bound.requests) != 1 {
		t.Fatalf("summary requests = %d", len(bound.requests))
	}
	request := bound.requests[0]
	if len(request.History) != 1 || !strings.Contains(request.History[0].Text, "which release added tracing") {
		t.Fatalf("prompt missing from request: %+v", request.History)
	}
	if !strings.Contains(request.History[0].Text, "release 4.2 fixed the crash") {
		t.Fatalf("page missing from request: %+v", request.History)
	}
	if !strings.Contains(output.Text, "4.3") || strings.Contains(output.Text, "release 4.2 fixed the crash") {
		t.Fatalf("summary did not replace the page: %q", output.Text)
	}
	if !strings.Contains(output.Text, "summarised from") {
		t.Fatalf("summary origin missing: %q", output.Text)
	}
}

func TestWebFetchWithoutPromptKeepsTheWholePage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("whole page"))
	}))
	defer server.Close()

	bound := &summaryModel{reply: "summary"}
	tool := webFetchTool{}.withModel(bound)
	output, err := tool.Execute(context.Background(), "", json.RawMessage(`{"url":"`+server.URL+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(bound.requests) != 0 {
		t.Fatalf("model called without a prompt: %+v", bound.requests)
	}
	if !strings.Contains(output.Text, "whole page") {
		t.Fatalf("page lost: %q", output.Text)
	}
}

func TestWebFetchKeepsThePageWhenTheSummaryFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("whole page"))
	}))
	defer server.Close()

	bound := &summaryModel{err: errors.New("provider unavailable")}
	tool := webFetchTool{}.withModel(bound)
	output, err := tool.Execute(context.Background(), "", json.RawMessage(`{"url":"`+server.URL+`","prompt":"anything"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.Text, "whole page") || !strings.Contains(output.Text, "the prompt was not applied") {
		t.Fatalf("failure was not reported with the page: %q", output.Text)
	}
	if !strings.Contains(output.Text, "provider unavailable") {
		t.Fatalf("failure reason missing: %q", output.Text)
	}
}

func TestWebFetchWithoutABoundModelReportsTheUnusedPrompt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("whole page"))
	}))
	defer server.Close()

	output, err := (webFetchTool{}).Execute(context.Background(), "", json.RawMessage(`{"url":"`+server.URL+`","prompt":"anything"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.Text, "whole page") || !strings.Contains(output.Text, "the prompt was not applied") {
		t.Fatalf("unused prompt was not reported: %q", output.Text)
	}
}

func TestWebFetchDoesNotSummariseAnErrorPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("no such page"))
	}))
	defer server.Close()

	bound := &summaryModel{reply: "summary"}
	tool := webFetchTool{}.withModel(bound)
	output, err := tool.Execute(context.Background(), "", json.RawMessage(`{"url":"`+server.URL+`","prompt":"anything"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(bound.requests) != 0 {
		t.Fatalf("model called for an error page: %+v", bound.requests)
	}
	if !strings.Contains(output.Text, "HTTP 404") {
		t.Fatalf("status lost: %q", output.Text)
	}
}

func TestToolLoopBindsTheTurnModelToToolsThatNeedIt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("whole page"))
	}))
	defer server.Close()

	bound := &summaryModel{reply: "extracted"}
	var tool Tool = webFetchTool{}
	if _, ok := tool.(modelTool); !ok {
		t.Fatal("web fetch does not declare the model dependency")
	}
	var recorded model.Message
	err := executeTools(context.Background(), Options{Model: bound, Emit: func(Event) error { return nil }},
		map[string]Tool{"WebFetch": tool},
		[]model.Call{{ID: "call", Name: "WebFetch", Arguments: `{"url":"` + server.URL + `","prompt":"anything"}`}},
		func(message model.Message) error { recorded = message; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(bound.requests) != 1 {
		t.Fatalf("model was not bound to the tool: %+v", bound.requests)
	}
	if !strings.Contains(recorded.Text, "extracted") {
		t.Fatalf("summary was not returned to the model: %q", recorded.Text)
	}
}
