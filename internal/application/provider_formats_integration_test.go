package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gcode/internal/appmodel"
)

const (
	providerFormatsFixtureKey     = "provider-formats-fixture-secret"
	providerFormatsFixturePath    = "fixture.txt"
	providerFormatsFixtureContent = "fixture-file-content"
)

type providerFormatsCase struct {
	name           string
	format         appmodel.APIFormat
	primaryModel   string
	secondaryModel string
	authHeader     string
}

func providerFormatsCases() []providerFormatsCase {
	return []providerFormatsCase{
		{
			name:           "responses",
			format:         appmodel.Responses,
			primaryModel:   "responses-fixture-primary",
			secondaryModel: "responses-fixture-secondary",
			authHeader:     "Authorization",
		},
		{
			name:           "chat-completions",
			format:         appmodel.ChatCompletions,
			primaryModel:   "chat-fixture-primary",
			secondaryModel: "chat-fixture-secondary",
			authHeader:     "Authorization",
		},
		{
			name:           "anthropic-messages",
			format:         appmodel.Anthropic,
			primaryModel:   "anthropic-fixture-primary",
			secondaryModel: "anthropic-fixture-secondary",
			authHeader:     "x-api-key",
		},
		{
			name:           "gemini-generate-content",
			format:         appmodel.Gemini,
			primaryModel:   "gemini-fixture-primary",
			secondaryModel: "gemini-fixture-secondary",
			authHeader:     "x-goog-api-key",
		},
	}
}

// TestServiceProviderFormatsFlow proves one application-level contract for all
// four wire formats. The HTTP server replaces only the model service. The
// application, controller, runtime, provider adapter, file tracker and actual
// read_file implementation remain production code.
func TestServiceProviderFormatsFlow(t *testing.T) {
	for _, testCase := range providerFormatsCases() {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			runProviderFormatsFlow(t, testCase)
		})
	}
}

func runProviderFormatsFlow(t *testing.T, testCase providerFormatsCase) {
	t.Helper()
	fixture := newProviderFormatsFixture(t, testCase, providerFormatsFlow)
	service, ctx, dataDir, sessionID := newProviderFormatsService(t, testCase, fixture)

	readPrompt := "provider-format/" + testCase.name + "/read-fixture"
	continuePrompt := "provider-format/" + testCase.name + "/continue-after-read"
	switchPrompt := "provider-format/" + testCase.name + "/new-task-after-model-switch"

	if err := service.Send(ctx, sessionID, readPrompt, appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	first := waitProviderFormatsSession(t, service, sessionID, func(session appmodel.Session) bool {
		return session.State == appmodel.Idle && session.ActiveRun == nil &&
			providerFormatsHasTool(session, "read_file")
	})
	providerFormatsAssertReadTool(t, first)

	if err := service.Send(ctx, sessionID, continuePrompt, appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	continued := waitProviderFormatsSession(t, service, sessionID, func(session appmodel.Session) bool {
		return session.State == appmodel.Idle && session.ActiveRun == nil &&
			providerFormatsMessageContains(session, "fixture continuation complete")
	})
	if !providerFormatsMessageContains(continued, "fixture continuation complete") {
		t.Fatalf("continuation response missing from session: %#v", continued.Messages)
	}

	secondaryID := "provider-format-" + testCase.name + "-secondary"
	if err := service.SetProvider(ctx, sessionID, secondaryID); err != nil {
		t.Fatal(err)
	}
	selected := providerFormatsSession(t, service, sessionID)
	if selected.ProviderID != secondaryID {
		t.Fatalf("session provider = %q, want %q after model switch", selected.ProviderID, secondaryID)
	}
	if err := service.Send(ctx, sessionID, switchPrompt, appmodel.Correction); err != nil {
		t.Fatal(err)
	}
	switched := waitProviderFormatsSession(t, service, sessionID, func(session appmodel.Session) bool {
		return session.State == appmodel.Idle && session.ActiveRun == nil &&
			providerFormatsMessageContains(session, "fixture model switch complete")
	})
	if !providerFormatsMessageContains(switched, "fixture model switch complete") {
		t.Fatalf("model-switch response missing from session: %#v", switched.Messages)
	}

	requests := fixture.requestsSnapshot()
	if len(requests) != 4 {
		t.Fatalf("provider requests = %d, want read/tool, continuation, and switched-model turns", len(requests))
	}
	for index, request := range requests {
		step := index + 1
		wantModel := testCase.primaryModel
		if step == 4 {
			wantModel = testCase.secondaryModel
		}
		if err := providerFormatsAssertRequest(t, testCase, request, step, wantModel); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(request.body), providerFormatsFixtureKey) {
			t.Fatalf("request %d body leaked fixture key", step)
		}
	}
	fixture.assertNoErrors(t)
	providerFormatsAssertNoSecretOnDisk(t, dataDir, providerFormatsFixtureKey)
}

// TestServiceProviderFormatsCancellation proves that Stop cancels the active
// request through Service -> Controller -> Runtime -> provider adapter for all
// formats. The fixture waits on request.Context().Done(), rather than using a
// sleep as a cancellation oracle.
func TestServiceProviderFormatsCancellation(t *testing.T) {
	for _, testCase := range providerFormatsCases() {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newProviderFormatsFixture(t, testCase, providerFormatsCancellation)
			service, ctx, dataDir, sessionID := newProviderFormatsService(t, testCase, fixture)
			if err := service.Send(ctx, sessionID, "provider-format/"+testCase.name+"/cancel", appmodel.Correction); err != nil {
				t.Fatal(err)
			}
			select {
			case <-fixture.started:
			case <-time.After(5 * time.Second):
				t.Fatal("provider request did not start")
			}
			if err := service.Stop(ctx, sessionID); err != nil {
				t.Fatal(err)
			}
			select {
			case <-fixture.cancelled:
			case <-time.After(5 * time.Second):
				t.Fatal("provider request did not observe context cancellation")
			}
			session := waitProviderFormatsSession(t, service, sessionID, func(session appmodel.Session) bool {
				return session.ActiveRun == nil && session.State == appmodel.Interrupted
			})
			if session.Error != "" {
				t.Fatalf("cancelled session has error: %q", session.Error)
			}
			fixture.assertNoErrors(t)
			providerFormatsAssertNoSecretOnDisk(t, dataDir, providerFormatsFixtureKey)
		})
	}
}

// TestServiceProviderFormatsHTTPError proves that each adapter turns a
// non-success response into an application failure and that the key remains
// absent from the session snapshot and persisted events.
func TestServiceProviderFormatsHTTPError(t *testing.T) {
	for _, testCase := range providerFormatsCases() {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newProviderFormatsFixture(t, testCase, providerFormatsHTTPError)
			service, ctx, dataDir, sessionID := newProviderFormatsService(t, testCase, fixture)
			if err := service.Send(ctx, sessionID, "provider-format/"+testCase.name+"/http-error", appmodel.Correction); err != nil {
				t.Fatal(err)
			}
			session := waitProviderFormatsSession(t, service, sessionID, func(session appmodel.Session) bool {
				return session.ActiveRun == nil && session.State == appmodel.Failed
			})
			if session.Error == "" {
				t.Fatal("HTTP error did not reach the application session")
			}
			if strings.Contains(session.Error, providerFormatsFixtureKey) {
				t.Fatalf("session error leaked fixture key: %q", session.Error)
			}
			providerFormatsAssertNoSecretInSnapshot(t, service, ctx, providerFormatsFixtureKey)
			fixture.assertNoErrors(t)
			providerFormatsAssertNoSecretOnDisk(t, dataDir, providerFormatsFixtureKey)
		})
	}
}

type providerFormatsFixtureMode string

const (
	providerFormatsFlow         providerFormatsFixtureMode = "flow"
	providerFormatsCancellation providerFormatsFixtureMode = "cancellation"
	providerFormatsHTTPError    providerFormatsFixtureMode = "http-error"
)

type providerFormatsFixture struct {
	testCase   providerFormatsCase
	mode       providerFormatsFixtureMode
	key        string
	server     *httptest.Server
	started    chan struct{}
	cancelled  chan struct{}
	startOnce  sync.Once
	cancelOnce sync.Once

	mu       sync.Mutex
	requests []providerFormatsRequest
	errors   []error
}

type providerFormatsRequest struct {
	step    int
	path    string
	query   string
	headers http.Header
	body    []byte
}

func newProviderFormatsFixture(t *testing.T, testCase providerFormatsCase, mode providerFormatsFixtureMode) *providerFormatsFixture {
	t.Helper()
	fixture := &providerFormatsFixture{
		testCase:  testCase,
		mode:      mode,
		key:       providerFormatsFixtureKey,
		started:   make(chan struct{}),
		cancelled: make(chan struct{}),
	}
	fixture.server = httptest.NewServer(http.HandlerFunc(fixture.handle))
	t.Cleanup(func() { fixture.server.Close() })
	return fixture
}

func (fixture *providerFormatsFixture) handle(writer http.ResponseWriter, request *http.Request) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		fixture.recordError(fmt.Errorf("read request body: %w", err))
		http.Error(writer, "invalid request body", http.StatusBadRequest)
		return
	}
	step := fixture.recordRequest(request, body)
	if err := fixture.validateRequest(step, request, body); err != nil {
		fixture.recordError(err)
		http.Error(writer, "fixture request mismatch", http.StatusBadRequest)
		return
	}

	switch fixture.mode {
	case providerFormatsCancellation:
		fixture.startOnce.Do(func() { close(fixture.started) })
		select {
		case <-request.Context().Done():
			fixture.cancelOnce.Do(func() { close(fixture.cancelled) })
		case <-time.After(30 * time.Second):
			fixture.recordError(errors.New("cancellation fixture timed out"))
		}
	case providerFormatsHTTPError:
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(writer, `{"error":{"message":"fixture rejected %s"}}`, fixture.key)
	case providerFormatsFlow:
		fixture.writeFlowResponse(writer, step)
	default:
		fixture.recordError(fmt.Errorf("unknown fixture mode %q", fixture.mode))
		http.Error(writer, "unknown fixture mode", http.StatusInternalServerError)
	}
}

func (fixture *providerFormatsFixture) recordRequest(request *http.Request, body []byte) int {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	step := len(fixture.requests) + 1
	fixture.requests = append(fixture.requests, providerFormatsRequest{
		step:    step,
		path:    request.URL.Path,
		query:   request.URL.RawQuery,
		headers: request.Header.Clone(),
		body:    append([]byte(nil), body...),
	})
	return step
}

func (fixture *providerFormatsFixture) validateRequest(step int, request *http.Request, body []byte) error {
	if request.Method != http.MethodPost {
		return fmt.Errorf("request %d method = %q, want POST", step, request.Method)
	}
	wantModel := fixture.testCase.primaryModel
	if fixture.mode == providerFormatsFlow && step == 4 {
		wantModel = fixture.testCase.secondaryModel
	}
	wantPath := providerFormatsPath(fixture.testCase, request.URL, wantModel)
	if request.URL.Path != wantPath {
		return fmt.Errorf("request %d path = %q, want %q", step, request.URL.Path, wantPath)
	}
	if fixture.testCase.format == appmodel.Gemini && request.URL.RawQuery != "alt=sse" {
		return fmt.Errorf("request %d Gemini query = %q, want alt=sse", step, request.URL.RawQuery)
	}
	if fixture.testCase.format != appmodel.Gemini && request.URL.RawQuery != "" {
		return fmt.Errorf("request %d unexpected query %q", step, request.URL.RawQuery)
	}
	wantAuth := fixture.key
	if fixture.testCase.authHeader == "Authorization" {
		wantAuth = "Bearer " + fixture.key
	}
	if got := request.Header.Get(fixture.testCase.authHeader); got != wantAuth {
		return fmt.Errorf("request %d %s = %q, want %q", step, fixture.testCase.authHeader, got, wantAuth)
	}
	for _, header := range []string{"Authorization", "x-api-key", "x-goog-api-key"} {
		if header == fixture.testCase.authHeader {
			continue
		}
		if got := request.Header.Get(header); strings.Contains(got, fixture.key) {
			return fmt.Errorf("request %d unexpected %s carries fixture key", step, header)
		}
	}
	if got := request.Header.Get("Cookie"); got != "" {
		return fmt.Errorf("request %d unexpectedly sent Cookie header", step)
	}
	if fixture.testCase.format == appmodel.Anthropic && request.Header.Get("anthropic-version") == "" {
		return fmt.Errorf("request %d missing anthropic-version", step)
	}
	if strings.Contains(string(body), fixture.key) {
		return fmt.Errorf("request %d body contains fixture key", step)
	}
	if fixture.testCase.format != appmodel.Gemini {
		var envelope map[string]any
		if err := json.Unmarshal(body, &envelope); err != nil {
			return fmt.Errorf("request %d invalid JSON: %w", step, err)
		}
		if got, _ := envelope["model"].(string); got != wantModel {
			return fmt.Errorf("request %d model = %q, want %q", step, got, wantModel)
		}
	}
	if fixture.mode != providerFormatsFlow {
		return nil
	}
	if prompt := providerFormatsPrompt(fixture.testCase, step); prompt != "" && !strings.Contains(string(body), prompt) {
		return fmt.Errorf("request %d lost prompt marker %q", step, prompt)
	}
	if step >= 2 {
		if err := providerFormatsAssertHistory(fixture.testCase.format, body); err != nil {
			return fmt.Errorf("request %d history: %w", step, err)
		}
	}
	return nil
}

func (fixture *providerFormatsFixture) writeFlowResponse(writer http.ResponseWriter, step int) {
	if step == 1 {
		writeProviderFormatsToolStream(writer, fixture.testCase.format, providerFormatsCallID(fixture.testCase.format))
		return
	}
	text := "fixture continuation complete"
	if step == 4 {
		text = "fixture model switch complete"
	}
	if step > 4 {
		fixture.recordError(fmt.Errorf("unexpected flow request step %d", step))
		http.Error(writer, "unexpected fixture step", http.StatusBadRequest)
		return
	}
	writeProviderFormatsTextStream(writer, fixture.testCase.format, text)
}

func (fixture *providerFormatsFixture) recordError(err error) {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	fixture.errors = append(fixture.errors, err)
}

func (fixture *providerFormatsFixture) requestsSnapshot() []providerFormatsRequest {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	result := make([]providerFormatsRequest, len(fixture.requests))
	copy(result, fixture.requests)
	return result
}

func (fixture *providerFormatsFixture) assertNoErrors(t *testing.T) {
	t.Helper()
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if len(fixture.errors) > 0 {
		t.Fatalf("provider fixture errors: %v", fixture.errors)
	}
}

func newProviderFormatsService(t *testing.T, testCase providerFormatsCase, fixture *providerFormatsFixture) (*Service, context.Context, string, string) {
	t.Helper()
	project := t.TempDir()
	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, providerFormatsFixturePath), []byte(providerFormatsFixtureContent+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	service, err := New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	t.Cleanup(func() { _ = service.Close(ctx) })
	baseURL := providerFormatsBaseURL(testCase, fixture.server.URL)
	primary := appmodel.Provider{
		ID:      "provider-format-" + testCase.name + "-primary",
		Name:    "provider format " + testCase.name + " primary",
		Format:  testCase.format,
		BaseURL: baseURL,
		Model:   testCase.primaryModel,
	}
	secondary := appmodel.Provider{
		ID:      "provider-format-" + testCase.name + "-secondary",
		Name:    "provider format " + testCase.name + " secondary",
		Format:  testCase.format,
		BaseURL: baseURL,
		Model:   testCase.secondaryModel,
	}
	if err := service.SaveProvider(ctx, primary, fixture.key); err != nil {
		t.Fatal(err)
	}
	if err := service.SaveProvider(ctx, secondary, fixture.key); err != nil {
		t.Fatal(err)
	}
	if err := service.AddProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.Snapshot(ctx)
	if err != nil || len(snapshot.Projects) != 1 {
		t.Fatalf("project snapshot = %#v, error = %v", snapshot.Projects, err)
	}
	if err := service.NewSession(ctx, appmodel.NewSessionOptions{
		ProjectID:  snapshot.Projects[0].ID,
		ProviderID: primary.ID,
		FullAccess: true,
		Title:      "provider format " + testCase.name,
	}); err != nil {
		t.Fatal(err)
	}
	snapshot, err = service.Snapshot(ctx)
	if err != nil || len(snapshot.Sessions) != 1 {
		t.Fatalf("session snapshot = %#v, error = %v", snapshot.Sessions, err)
	}
	return service, ctx, dataDir, snapshot.Sessions[0].ID
}

func waitProviderFormatsSession(t *testing.T, service *Service, sessionID string, predicate func(appmodel.Session) bool) appmodel.Session {
	t.Helper()
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var latest appmodel.Session
	for {
		snapshot, err := service.Snapshot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, session := range snapshot.Sessions {
			if session.ID != sessionID {
				continue
			}
			latest = session
			matched := predicate(session)
			if matched {
				return session
			}
			if session.State == appmodel.Failed {
				t.Fatalf("session %s reached failed state: state=%q error=%q active=%#v messages=%#v", sessionID, session.State, session.Error, session.ActiveRun, session.Messages)
			}
		}
		select {
		case <-deadline.C:
			t.Fatalf("session %s did not reach expected state: state=%q error=%q active=%#v messages=%#v", sessionID, latest.State, latest.Error, latest.ActiveRun, latest.Messages)
		case <-ticker.C:
		}
	}
}

func providerFormatsSession(t *testing.T, service *Service, sessionID string) appmodel.Session {
	t.Helper()
	snapshot, err := service.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, session := range snapshot.Sessions {
		if session.ID == sessionID {
			return session
		}
	}
	t.Fatalf("session %s not found", sessionID)
	return appmodel.Session{}
}

func providerFormatsMessageContains(session appmodel.Session, want string) bool {
	for _, message := range session.Messages {
		if strings.Contains(message.Text, want) {
			return true
		}
	}
	return false
}

func providerFormatsHasTool(session appmodel.Session, name string) bool {
	for _, message := range session.Messages {
		for _, tool := range message.Tools {
			if tool.Name == name {
				return true
			}
		}
	}
	return false
}

func providerFormatsAssertReadTool(t *testing.T, session appmodel.Session) {
	t.Helper()
	for _, message := range session.Messages {
		for _, tool := range message.Tools {
			if tool.Name != "read_file" {
				continue
			}
			var arguments map[string]any
			if err := json.Unmarshal([]byte(tool.Arguments), &arguments); err != nil {
				t.Fatalf("read_file arguments = %q: %v", tool.Arguments, err)
			}
			if arguments["path"] != providerFormatsFixturePath {
				t.Fatalf("read_file path = %#v, want %q", arguments["path"], providerFormatsFixturePath)
			}
			if tool.State != "ok" || !strings.Contains(tool.Output, providerFormatsFixtureContent) {
				t.Fatalf("read_file result = %#v, want successful real file result", tool)
			}
			return
		}
	}
	t.Fatalf("session has no read_file tool result: %#v", session.Messages)
}

func providerFormatsAssertRequest(t *testing.T, testCase providerFormatsCase, request providerFormatsRequest, step int, wantModel string) error {
	t.Helper()
	wantPath := providerFormatsPath(testCase, nil, wantModel)
	if request.path != wantPath {
		return fmt.Errorf("request %d path = %q, want %q", step, request.path, wantPath)
	}
	if testCase.format == appmodel.Gemini && request.query != "alt=sse" {
		return fmt.Errorf("request %d query = %q, want alt=sse", step, request.query)
	}
	if testCase.format != appmodel.Gemini && request.query != "" {
		return fmt.Errorf("request %d unexpected query %q", step, request.query)
	}
	if testCase.authHeader == "Authorization" {
		if got := request.headers.Get("Authorization"); got != "Bearer "+providerFormatsFixtureKey {
			return fmt.Errorf("request %d Authorization = %q", step, got)
		}
	} else if got := request.headers.Get(testCase.authHeader); got != providerFormatsFixtureKey {
		return fmt.Errorf("request %d %s = %q", step, testCase.authHeader, got)
	}
	if testCase.format != appmodel.Gemini {
		var envelope map[string]any
		if err := json.Unmarshal(request.body, &envelope); err != nil {
			return fmt.Errorf("request %d invalid JSON: %w", step, err)
		}
		if got, _ := envelope["model"].(string); got != wantModel {
			return fmt.Errorf("request %d model = %q, want %q", step, got, wantModel)
		}
	}
	return nil
}

func providerFormatsBaseURL(testCase providerFormatsCase, serverURL string) string {
	if testCase.format == appmodel.Responses || testCase.format == appmodel.ChatCompletions {
		return serverURL + "/v1"
	}
	return serverURL
}

func providerFormatsPath(testCase providerFormatsCase, requestURL *url.URL, model string) string {
	switch testCase.format {
	case appmodel.Responses:
		return "/v1/responses"
	case appmodel.ChatCompletions:
		return "/v1/chat/completions"
	case appmodel.Anthropic:
		return "/v1/messages"
	case appmodel.Gemini:
		return "/v1beta/models/" + url.PathEscape(model) + ":streamGenerateContent"
	default:
		if requestURL != nil {
			return requestURL.Path
		}
		return ""
	}
}

func providerFormatsPrompt(testCase providerFormatsCase, step int) string {
	switch step {
	case 1, 2:
		return "provider-format/" + testCase.name + "/read-fixture"
	case 3:
		return "provider-format/" + testCase.name + "/continue-after-read"
	case 4:
		return "provider-format/" + testCase.name + "/new-task-after-model-switch"
	default:
		return ""
	}
}

func providerFormatsCallID(format appmodel.APIFormat) string {
	switch format {
	case appmodel.Responses:
		return "responses-read-call"
	case appmodel.ChatCompletions:
		return "chat-read-call"
	case appmodel.Anthropic:
		return "anthropic-read-call"
	case appmodel.Gemini:
		return "gemini-read-call"
	default:
		return "read-call"
	}
}

func providerFormatsAssertHistory(format appmodel.APIFormat, body []byte) error {
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("decode request: %w", err)
	}
	wantID := providerFormatsCallID(format)
	switch format {
	case appmodel.Responses:
		items, ok := envelope["input"].([]any)
		if !ok {
			return fmt.Errorf("input is not an array")
		}
		call, output := false, false
		for _, raw := range items {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if item["type"] == "function_call" && item["name"] == "read_file" && item["call_id"] == wantID {
				call = true
			}
			if item["type"] == "function_call_output" && item["call_id"] == wantID && strings.Contains(fmt.Sprint(item["output"]), providerFormatsFixtureContent) {
				output = true
			}
		}
		if !call || !output {
			return fmt.Errorf("Responses history missing function_call/function_call_output: call=%v output=%v", call, output)
		}
	case appmodel.ChatCompletions:
		messages, ok := envelope["messages"].([]any)
		if !ok {
			return fmt.Errorf("messages is not an array")
		}
		call, output := false, false
		for _, raw := range messages {
			message, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			switch message["role"] {
			case "assistant":
				calls, _ := message["tool_calls"].([]any)
				for _, rawCall := range calls {
					toolCall, _ := rawCall.(map[string]any)
					function, _ := toolCall["function"].(map[string]any)
					if toolCall["id"] == wantID && function["name"] == "read_file" {
						call = true
					}
				}
			case "tool":
				if message["tool_call_id"] == wantID && strings.Contains(fmt.Sprint(message["content"]), providerFormatsFixtureContent) {
					output = true
				}
			}
		}
		if !call || !output {
			return fmt.Errorf("Chat history missing assistant tool call/tool result: call=%v output=%v", call, output)
		}
	case appmodel.Anthropic:
		messages, ok := envelope["messages"].([]any)
		if !ok {
			return fmt.Errorf("messages is not an array")
		}
		call, output := false, false
		for _, raw := range messages {
			message, _ := raw.(map[string]any)
			blocks, _ := message["content"].([]any)
			for _, rawBlock := range blocks {
				block, _ := rawBlock.(map[string]any)
				switch block["type"] {
				case "tool_use":
					if block["id"] == wantID && block["name"] == "read_file" {
						call = true
					}
				case "tool_result":
					if block["tool_use_id"] == wantID && strings.Contains(fmt.Sprint(block["content"]), providerFormatsFixtureContent) {
						output = true
					}
				}
			}
		}
		if !call || !output {
			return fmt.Errorf("Anthropic history missing tool_use/tool_result: call=%v output=%v", call, output)
		}
	case appmodel.Gemini:
		contents, ok := envelope["contents"].([]any)
		if !ok {
			return fmt.Errorf("contents is not an array")
		}
		call, output := false, false
		for _, rawContent := range contents {
			content, _ := rawContent.(map[string]any)
			parts, _ := content["parts"].([]any)
			for _, rawPart := range parts {
				part, _ := rawPart.(map[string]any)
				if functionCall, ok := part["functionCall"].(map[string]any); ok && functionCall["id"] == wantID && functionCall["name"] == "read_file" {
					call = true
				}
				if functionResponse, ok := part["functionResponse"].(map[string]any); ok && functionResponse["id"] == wantID && strings.Contains(fmt.Sprint(functionResponse["response"]), providerFormatsFixtureContent) {
					output = true
				}
			}
		}
		if !call || !output {
			return fmt.Errorf("Gemini history missing functionCall/functionResponse: call=%v output=%v", call, output)
		}
	default:
		return fmt.Errorf("unsupported format %q", format)
	}
	return nil
}

func writeProviderFormatsToolStream(writer http.ResponseWriter, format appmodel.APIFormat, callID string) {
	arguments := `{"path":"` + providerFormatsFixturePath + `"}`
	switch format {
	case appmodel.Responses:
		writeProviderFormatsSSE(writer, `{"type":"response.output_item.added","item":{"type":"function_call","id":"`+callID+`","call_id":"`+callID+`","name":"read_file"}}`)
		writeProviderFormatsSSE(writer, `{"type":"response.function_call_arguments.delta","item_id":"`+callID+`","delta":"`+strings.ReplaceAll(arguments, `"`, `\"`)+`"}`)
		writeProviderFormatsSSE(writer, `{"type":"response.output_item.done","item":{"type":"function_call","id":"`+callID+`","call_id":"`+callID+`","name":"read_file","arguments":"`+strings.ReplaceAll(arguments, `"`, `\"`)+`"}}`)
		writeProviderFormatsSSE(writer, `{"type":"response.completed","response":{"id":"`+callID+`-response","status":"completed"}}`)
	case appmodel.ChatCompletions:
		payload := map[string]any{
			"choices": []any{
				map[string]any{
					"delta": map[string]any{
						"tool_calls": []any{
							map[string]any{
								"index": 0,
								"id":    callID,
								"type":  "function",
								"function": map[string]any{
									"name":      "read_file",
									"arguments": arguments,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
		}
		writeProviderFormatsJSONSSE(writer, payload)
		_, _ = fmt.Fprint(writer, "data: [DONE]\n\n")
	case appmodel.Anthropic:
		writeProviderFormatsSSE(writer, `{"type":"message_start","message":{"id":"`+callID+`-message","usage":{"input_tokens":1}}}`)
		writeProviderFormatsSSE(writer, `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"`+callID+`","name":"read_file","input":{}}}`)
		writeProviderFormatsSSE(writer, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"`+strings.ReplaceAll(arguments, `"`, `\"`)+`"}}`)
		writeProviderFormatsSSE(writer, `{"type":"content_block_stop","index":0}`)
		writeProviderFormatsSSE(writer, `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":1}}`)
		writeProviderFormatsSSE(writer, `{"type":"message_stop"}`)
	case appmodel.Gemini:
		writeProviderFormatsSSE(writer, `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"id":"`+callID+`","name":"read_file","args":{"path":"`+providerFormatsFixturePath+`"}}}]},"finishReason":"STOP"}]}`)
	}
}

func writeProviderFormatsTextStream(writer http.ResponseWriter, format appmodel.APIFormat, text string) {
	first, second := text[:len(text)/2], text[len(text)/2:]
	switch format {
	case appmodel.Responses:
		writeProviderFormatsSSE(writer, `{"type":"response.created","response":{"id":"text-response"}}`)
		writeProviderFormatsSSE(writer, `{"type":"response.output_text.delta","delta":"`+first+`"}`)
		writeProviderFormatsSSE(writer, `{"type":"response.output_text.delta","delta":"`+second+`"}`)
		writeProviderFormatsSSE(writer, `{"type":"response.completed","response":{"id":"text-response","status":"completed"}}`)
	case appmodel.ChatCompletions:
		writeProviderFormatsJSONSSE(writer, map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"content": first}}}})
		writeProviderFormatsJSONSSE(writer, map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"content": second}, "finish_reason": "stop"}}})
		_, _ = fmt.Fprint(writer, "data: [DONE]\n\n")
	case appmodel.Anthropic:
		writeProviderFormatsSSE(writer, `{"type":"message_start","message":{"id":"text-message","usage":{"input_tokens":1}}}`)
		writeProviderFormatsSSE(writer, `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		writeProviderFormatsSSE(writer, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"`+first+`"}}`)
		writeProviderFormatsSSE(writer, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"`+second+`"}}`)
		writeProviderFormatsSSE(writer, `{"type":"content_block_stop","index":0}`)
		writeProviderFormatsSSE(writer, `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`)
		writeProviderFormatsSSE(writer, `{"type":"message_stop"}`)
	case appmodel.Gemini:
		writeProviderFormatsSSE(writer, `{"candidates":[{"content":{"role":"model","parts":[{"text":"`+first+`"}]}}]}`)
		writeProviderFormatsSSE(writer, `{"candidates":[{"content":{"role":"model","parts":[{"text":"`+second+`"}]},"finishReason":"STOP"}]}`)
	}
}

func writeProviderFormatsJSONSSE(writer http.ResponseWriter, value any) {
	encoded, _ := json.Marshal(value)
	writeProviderFormatsSSE(writer, string(encoded))
}

func writeProviderFormatsSSE(writer http.ResponseWriter, payload string) {
	writer.Header().Set("Content-Type", "text/event-stream")
	_, _ = fmt.Fprintf(writer, "data: %s\n\n", payload)
	if flusher, ok := writer.(http.Flusher); ok {
		flusher.Flush()
	}
}

func providerFormatsAssertNoSecretOnDisk(t *testing.T, dataDir, secret string) {
	t.Helper()
	err := filepath.Walk(dataDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), secret) {
			return fmt.Errorf("secret appears in %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func providerFormatsAssertNoSecretInSnapshot(t *testing.T, service *Service, ctx context.Context, secret string) {
	t.Helper()
	snapshot, err := service.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal session snapshot: %v", err)
	}
	if strings.Contains(string(encoded), secret) {
		t.Fatalf("fixture key appears in serialized session snapshot")
	}
}
