package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"gcode/internal/appmodel"
	"gcode/internal/sessions"
)

const sharedEditFixtureKey = "shared-edit-fixture-key"

type sharedEditWriteMode string

const (
	sharedEditReplace sharedEditWriteMode = "edit_file"
	sharedEditWrite   sharedEditWriteMode = "write_file"
)

// TestServiceSharedEditIntegrationS2 proves the application-level file
// coordination boundary through the real Service -> Controller -> Runtime ->
// Chat Completions adapter -> built-in tools path. The HTTP server only
// substitutes the model protocol; it never replaces the tool runner.
func TestServiceSharedEditIntegrationS2(t *testing.T) {
	for _, mode := range []sharedEditWriteMode{sharedEditReplace, sharedEditWrite} {
		mode := mode
		t.Run(string(mode)+"/same-file-old-read-collision", func(t *testing.T) {
			runSharedEditCollision(t, mode)
		})
		t.Run(string(mode)+"/external-edit-stale-rejection", func(t *testing.T) {
			runSharedEditExternalEdit(t, mode)
		})
		t.Run(string(mode)+"/different-files-concurrent", func(t *testing.T) {
			runSharedEditDifferentFiles(t, mode)
		})
	}
}

func runSharedEditCollision(t *testing.T, mode sharedEditWriteMode) {
	t.Helper()
	project := sharedEditGitProject(t, map[string]string{"shared.txt": "base\n"})
	paths := map[string]string{"shared-edit-A": "shared.txt", "shared-edit-B": "shared.txt"}
	values := map[string]string{"shared-edit-A": "from-A\n", "shared-edit-B": "from-B\n"}
	provider := newSharedEditProvider(t, mode, project, paths, values, 2, nil)
	service, sessions := newSharedEditService(t, provider, project, []string{"shared-edit-A", "shared-edit-B"})
	ctx := context.Background()
	defer service.Close(ctx)

	prompt, suffix := "读 shared.txt 后按要求完成编辑", "same-file"
	sharedEditSendConcurrently(t, service, sessions, prompt, suffix)
	provider.barrier.Wait(t)
	completed := sharedEditWaitForSessions(t, service, sessions)
	provider.assertNoErrors(t)

	successes := 0
	conflicts := 0
	winner := ""
	for marker, session := range completed {
		writeState := "either"
		evidence := sharedEditAssertSession(t, provider, session, marker, mode, "base\n", marker+" "+prompt+" ("+suffix+")", paths, values, writeState)
		call := evidence.write
		switch {
		case call.State == "ok":
			successes++
			winner = marker
		case call.State == "error" && strings.Contains(call.Output, "changed on disk since you last read it"):
			conflicts++
		default:
			t.Fatalf("%s %s result was incomplete or unexpected: state=%q output=%q", marker, mode, call.State, call.Output)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("same-file %s result counts = success:%d conflict:%d; want one each", mode, successes, conflicts)
	}
	content, err := os.ReadFile(filepath.Join(project, "shared.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(content), values[winner]; got != want {
		t.Fatalf("same-file %s final content = %q, want complete winner content %q", mode, got, want)
	}
	sharedEditAssertGitChanged(t, project, "shared.txt")
}

func runSharedEditExternalEdit(t *testing.T, mode sharedEditWriteMode) {
	project := sharedEditGitProject(t, map[string]string{"external.txt": "base\n"})
	path := filepath.Join(project, "external.txt")
	paths := map[string]string{"shared-edit-external": "external.txt"}
	values := map[string]string{"shared-edit-external": "agent-write\n"}
	external := func() error {
		return os.WriteFile(path, []byte("outside-editor\n"), 0o600)
	}
	provider := newSharedEditProvider(t, mode, project, paths, values, 1, external)
	service, sessions := newSharedEditService(t, provider, project, []string{"shared-edit-external"})
	ctx := context.Background()
	defer service.Close(ctx)

	prompt, suffix := "读取 external.txt 后编辑", "external-edit"
	sharedEditSendConcurrently(t, service, sessions, prompt, suffix)
	provider.barrier.Wait(t)
	completed := sharedEditWaitForSessions(t, service, sessions)
	provider.assertNoErrors(t)
	for marker, session := range completed {
		evidence := sharedEditAssertSession(t, provider, session, marker, mode, "base\n", marker+" "+prompt+" ("+suffix+")", paths, values, "error")
		call := evidence.write
		if call.State != "error" || !strings.Contains(call.Output, "changed on disk since you last read it") {
			t.Fatalf("external edit %s result = state:%q output:%q, want visible stale-content conflict", mode, call.State, call.Output)
		}
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(content); got != "outside-editor\n" {
		t.Fatalf("external edit %s final content = %q, want external content preserved", mode, got)
	}
	sharedEditAssertGitChanged(t, project, "external.txt")
}

func runSharedEditDifferentFiles(t *testing.T, mode sharedEditWriteMode) {
	project := sharedEditGitProject(t, map[string]string{"a.txt": "base-a\n", "b.txt": "base-b\n"})
	paths := map[string]string{"shared-edit-A": "a.txt", "shared-edit-B": "b.txt"}
	originals := map[string]string{"shared-edit-A": "base-a\n", "shared-edit-B": "base-b\n"}
	values := map[string]string{"shared-edit-A": "written-a\n", "shared-edit-B": "written-b\n"}
	provider := newSharedEditProvider(t, mode, project, paths, values, 2, nil)
	service, sessions := newSharedEditService(t, provider, project, []string{"shared-edit-A", "shared-edit-B"})
	ctx := context.Background()
	defer service.Close(ctx)

	prompt, suffix := "读取文件后分别完成编辑", "different-files"
	sharedEditSendConcurrently(t, service, sessions, prompt, suffix)
	provider.barrier.Wait(t)
	completed := sharedEditWaitForSessions(t, service, sessions)
	provider.assertNoErrors(t)
	for marker, session := range completed {
		evidence := sharedEditAssertSession(t, provider, session, marker, mode, originals[marker], marker+" "+prompt+" ("+suffix+")", paths, values, "ok")
		if evidence.write.State != "ok" {
			t.Fatalf("different files %s %s result = state:%q output:%q, want one successful write", marker, mode, evidence.write.State, evidence.write.Output)
		}
	}
	for marker, path := range paths {
		content, err := os.ReadFile(filepath.Join(project, path))
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(content), values[marker]; got != want {
			t.Fatalf("different files %s %s content = %q, want complete %q", marker, mode, got, want)
		}
		sharedEditAssertGitChanged(t, project, path)
	}
}

type sharedEditProvider struct {
	server          *httptest.Server
	dataDir         string
	project         string
	mode            sharedEditWriteMode
	paths           map[string]string
	values          map[string]string
	barrier         *sharedEditBarrier
	beforeWrite     func() error
	beforeWriteOnce sync.Once
	mu              sync.Mutex
	steps           map[string]int
	writeBodies     map[string]string
	protocolErrs    chan error
}

func newSharedEditProvider(t *testing.T, mode sharedEditWriteMode, project string, paths, values map[string]string, parties int, beforeWrite func() error) *sharedEditProvider {
	t.Helper()
	provider := &sharedEditProvider{
		project: project, mode: mode, paths: paths, values: values,
		barrier: newSharedEditBarrier(parties), beforeWrite: beforeWrite,
		steps: make(map[string]int), writeBodies: make(map[string]string),
		protocolErrs: make(chan error, 16),
	}
	provider.server = httptest.NewServer(http.HandlerFunc(provider.handle))
	t.Cleanup(func() { provider.server.Close() })
	return provider
}

func (provider *sharedEditProvider) handle(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path != "/v1/chat/completions" || request.Header.Get("Authorization") != "Bearer "+sharedEditFixtureKey {
		provider.recordError(errors.New("unexpected provider endpoint or authorization"))
		http.Error(writer, "unexpected provider request", http.StatusBadRequest)
		return
	}
	body, err := io.ReadAll(request.Body)
	if err != nil {
		provider.recordError(errors.New("unable to read provider request"))
		http.Error(writer, "invalid provider request", http.StatusBadRequest)
		return
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		provider.recordError(errors.New("invalid provider JSON"))
		http.Error(writer, "invalid provider request", http.StatusBadRequest)
		return
	}
	marker := sharedEditMarker(string(body), provider.paths)
	if marker == "" {
		provider.recordError(errors.New("provider request lost session marker"))
		http.Error(writer, "missing session marker", http.StatusBadRequest)
		return
	}
	provider.mu.Lock()
	provider.steps[marker]++
	step := provider.steps[marker]
	provider.mu.Unlock()

	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Cache-Control", "no-cache")
	switch step {
	case 1:
		writeToolStream(writer, "read-"+marker, "read_file", map[string]any{"path": provider.paths[marker]})
	case 2:
		if !strings.Contains(string(body), strings.TrimSpace(sharedEditExpectedRead(provider.paths[marker], provider.project))) {
			provider.recordError(errors.New("provider second turn did not retain the real read result"))
		}
		writeArguments := sharedEditWriteArguments(provider.mode, provider.project, provider.paths[marker], provider.values[marker])
		if err := provider.barrier.Arrive(request.Context(), marker); err != nil {
			provider.recordError(errors.New("read barrier timed out"))
			return
		}
		provider.beforeWriteOnce.Do(func() {
			if provider.beforeWrite != nil {
				if err := provider.beforeWrite(); err != nil {
					provider.recordError(errors.New("external edit fixture write failed"))
				}
			}
		})
		writeToolStream(writer, "write-"+marker, string(provider.mode), writeArguments)
	case 3:
		provider.mu.Lock()
		provider.writeBodies[marker] = string(body)
		provider.mu.Unlock()
		writeChatEvent(writer, map[string]any{"content": "fixture completed"}, "stop")
		_, _ = fmt.Fprint(writer, "data: [DONE]\n\n")
	default:
		writeChatEvent(writer, map[string]any{"content": "fixture completed"}, "stop")
		_, _ = fmt.Fprint(writer, "data: [DONE]\n\n")
	}
	_ = decoded
}

func (provider *sharedEditProvider) recordError(err error) {
	select {
	case provider.protocolErrs <- err:
	default:
	}
}

func (provider *sharedEditProvider) assertNoErrors(t *testing.T) {
	t.Helper()
	for {
		select {
		case err := <-provider.protocolErrs:
			t.Fatal(err)
		default:
			return
		}
	}
}

type sharedEditBarrier struct {
	mu       sync.Mutex
	expected int
	arrived  map[string]bool
	released chan struct{}
	once     sync.Once
}

func newSharedEditBarrier(expected int) *sharedEditBarrier {
	return &sharedEditBarrier{expected: expected, arrived: make(map[string]bool), released: make(chan struct{})}
}

func (barrier *sharedEditBarrier) Arrive(ctx context.Context, marker string) error {
	barrier.mu.Lock()
	if !barrier.arrived[marker] {
		barrier.arrived[marker] = true
		if len(barrier.arrived) >= barrier.expected {
			barrier.once.Do(func() { close(barrier.released) })
		}
	}
	released := barrier.released
	barrier.mu.Unlock()

	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case <-released:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return errors.New("shared edit read barrier timeout")
	}
}

func (barrier *sharedEditBarrier) Wait(t *testing.T) {
	t.Helper()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case <-barrier.released:
	case <-timer.C:
		t.Fatal("both sessions did not complete read_file before the write barrier deadline")
	}
}

func sharedEditSendConcurrently(t *testing.T, service *Service, sessions map[string]string, prompt, suffix string) {
	t.Helper()
	start := make(chan struct{})
	errs := make(chan error, len(sessions))
	var wait sync.WaitGroup
	for marker, id := range sessions {
		marker, id := marker, id
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			errs <- service.Send(context.Background(), id, marker+" "+prompt+" ("+suffix+")", appmodel.Correction)
		}()
	}
	close(start)
	done := make(chan struct{})
	go func() {
		wait.Wait()
		close(done)
	}()
	timer := time.NewTimer(20 * time.Second)
	select {
	case <-done:
		timer.Stop()
	case <-timer.C:
		t.Fatal("concurrent Service.Send calls did not return before the deadline")
	}
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func newSharedEditService(t *testing.T, providerFixture *sharedEditProvider, project string, markers []string) (*Service, map[string]string) {
	t.Helper()
	serviceData := t.TempDir()
	providerFixture.dataDir = serviceData
	service, err := New(serviceData)
	if err != nil {
		t.Fatal(err)
	}
	provider := appmodel.Provider{
		ID: "shared-edit-fixture", Name: "共享编辑协议替身", Format: appmodel.ChatCompletions,
		BaseURL: providerFixture.server.URL + "/v1", Model: "shared-edit-fixture-model",
	}
	ctx := context.Background()
	if err := service.SaveProvider(ctx, provider, sharedEditFixtureKey); err != nil {
		_ = service.Close(ctx)
		t.Fatal(err)
	}
	if err := service.AddProject(ctx, project); err != nil {
		_ = service.Close(ctx)
		t.Fatal(err)
	}
	snapshot, err := service.Snapshot(ctx)
	if err != nil || len(snapshot.Projects) != 1 {
		_ = service.Close(ctx)
		t.Fatalf("project setup failed: projects=%#v error=%v", snapshot.Projects, err)
	}
	ids := make(map[string]string, len(markers))
	for _, marker := range markers {
		if err := service.NewSession(ctx, appmodel.NewSessionOptions{
			ProjectID: snapshot.Projects[0].ID, ProviderID: provider.ID, FullAccess: true, Title: marker,
		}); err != nil {
			_ = service.Close(ctx)
			t.Fatal(err)
		}
	}
	snapshot, err = service.Snapshot(ctx)
	if err != nil {
		_ = service.Close(ctx)
		t.Fatal(err)
	}
	for _, session := range snapshot.Sessions {
		if _, ok := ids[session.Title]; !ok {
			for _, marker := range markers {
				if session.Title == marker {
					ids[marker] = session.ID
				}
			}
		}
	}
	for _, marker := range markers {
		if ids[marker] == "" {
			_ = service.Close(ctx)
			t.Fatalf("session %s missing from snapshot: %#v", marker, snapshot.Sessions)
		}
	}
	return service, ids
}

func sharedEditWaitForSessions(t *testing.T, service *Service, sessions map[string]string) map[string]appmodel.Session {
	t.Helper()
	completed := make(map[string]appmodel.Session, len(sessions))
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		snapshot, err := service.Snapshot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for marker, id := range sessions {
			if _, ok := completed[marker]; ok {
				continue
			}
			for _, session := range snapshot.Sessions {
				if session.ID != id {
					continue
				}
				if session.State == appmodel.Failed {
					t.Fatalf("session %s failed: %s", marker, session.Error)
				}
				if session.ActiveRun == nil && session.State == appmodel.Idle && len(session.Messages) > 0 {
					completed[marker] = session
				}
			}
		}
		if len(completed) == len(sessions) {
			return completed
		}
		select {
		case <-deadline.C:
			t.Fatalf("sessions did not become idle: completed=%#v snapshot=%#v", completed, snapshot.Sessions)
		case <-ticker.C:
		}
	}
}

func sharedEditToolCalls(session appmodel.Session, name string) []appmodel.ToolCall {
	var calls []appmodel.ToolCall
	for _, message := range session.Messages {
		for _, call := range message.Tools {
			if call.Name == name {
				calls = append(calls, call)
			}
		}
	}
	return calls
}

type sharedEditEvidence struct {
	read  appmodel.ToolCall
	write appmodel.ToolCall
}

type sharedEditHistoryToolCall struct {
	ID             string `json:"id"`
	ProviderCallID string `json:"providerCallId"`
	Name           string `json:"name"`
	Arguments      string `json:"arguments"`
}

type sharedEditHistoryToolResult struct {
	ID             string `json:"id"`
	ProviderCallID string `json:"providerCallId"`
	Name           string `json:"name"`
	Status         string `json:"status"`
	Output         string `json:"output"`
}

type sharedEditProviderToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type sharedEditProviderMessage struct {
	Role       string                       `json:"role"`
	Content    json.RawMessage              `json:"content"`
	ToolCalls  []sharedEditProviderToolCall `json:"tool_calls"`
	ToolCallID string                       `json:"tool_call_id"`
}

type sharedEditProviderRequest struct {
	Messages []sharedEditProviderMessage `json:"messages"`
}

func sharedEditAssertSession(t *testing.T, provider *sharedEditProvider, session appmodel.Session, marker string, mode sharedEditWriteMode, original, prompt string, paths, values map[string]string, expectedWriteState string) sharedEditEvidence {
	t.Helper()
	path := paths[marker]
	value := values[marker]
	if path == "" || value == "" {
		t.Fatalf("%s %s fixture expectation is incomplete: path=%q value=%q", marker, mode, path, value)
	}

	users := make([]string, 0, 1)
	allTools := make([]appmodel.ToolCall, 0, 2)
	for _, message := range session.Messages {
		if message.Role == "user" {
			users = append(users, message.Text)
		}
		allTools = append(allTools, message.Tools...)
	}
	if len(users) != 1 || users[0] != prompt {
		t.Fatalf("%s %s session user history = %#v, want exactly %q", marker, mode, users, prompt)
	}
	if len(allTools) != 2 {
		t.Fatalf("%s %s session tools = %#v, want exactly read_file and %s", marker, mode, allTools, mode)
	}
	readCalls := sharedEditToolCalls(session, "read_file")
	writeCalls := sharedEditToolCalls(session, string(mode))
	if len(readCalls) != 1 || len(writeCalls) != 1 {
		t.Fatalf("%s %s session tool partition = read:%#v write:%#v, want one of each", marker, mode, readCalls, writeCalls)
	}
	read, write := readCalls[0], writeCalls[0]
	if read.ID != "read-"+marker || write.ID != "write-"+marker {
		t.Fatalf("%s %s session tool IDs = read:%q write:%q, want read-%s/write-%s", marker, mode, read.ID, write.ID, marker, marker)
	}
	sharedEditAssertArguments(t, marker+" read_file", read.Arguments, map[string]any{"path": path})
	writeArguments := map[string]any{"path": path}
	if mode == sharedEditReplace {
		writeArguments["old_string"] = original
		writeArguments["new_string"] = value
	} else {
		writeArguments["content"] = value
		writeArguments["overwrite"] = true
	}
	sharedEditAssertArguments(t, marker+" "+string(mode), write.Arguments, writeArguments)

	wantReadOutput := sharedEditReadOutput(path, original)
	if read.State != "ok" || read.Output != wantReadOutput {
		t.Fatalf("%s %s read_file result = state:%q output:%q, want state:ok output:%q", marker, mode, read.State, read.Output, wantReadOutput)
	}
	if expectedWriteState != "" && expectedWriteState != "either" && write.State != expectedWriteState {
		t.Fatalf("%s %s write result state = %q, want %q", marker, mode, write.State, expectedWriteState)
	}
	if write.State != "ok" && !(write.State == "error" && strings.Contains(write.Output, "changed on disk since you last read it")) {
		t.Fatalf("%s %s write result = state:%q output:%q, want success or visible stale-content conflict", marker, mode, write.State, write.Output)
	}
	if write.Output == "" {
		t.Fatalf("%s %s write result is empty", marker, mode)
	}

	sharedEditAssertHistory(t, provider.dataDir, session.ID, marker, mode, prompt, path, original, read, write, paths, values)
	body := provider.writeBody(marker)
	if body == "" {
		t.Fatalf("%s %s provider final request body was not captured", marker, mode)
	}
	sharedEditAssertProviderBody(t, body, marker, mode, prompt, path, original, read, write, paths, values)
	return sharedEditEvidence{read: read, write: write}
}

func sharedEditAssertArguments(t *testing.T, label, raw string, want map[string]any) {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("%s arguments %q are invalid JSON: %v", label, raw, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s arguments = %#v, want exactly %#v", label, got, want)
	}
}

func sharedEditAssertHistory(t *testing.T, dataDir, sessionID, marker string, mode sharedEditWriteMode, prompt, path, original string, read, write appmodel.ToolCall, paths, values map[string]string) {
	t.Helper()
	store := sessions.NewStore(sessions.StoreOptions{RootDir: filepath.Join(dataDir, "sessions")})
	events, err := store.ReadEvents(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatalf("%s %s history is empty", marker, mode)
	}
	historyParts := make([]string, 0, len(events))
	userContents := make([]string, 0, 1)
	assistantCalls := make([]sharedEditHistoryToolCall, 0, 2)
	eventCalls := make([]sharedEditHistoryToolCall, 0, 2)
	results := make([]sharedEditHistoryToolResult, 0, 2)
	for _, event := range events {
		if event.SessionID != sessionID {
			t.Fatalf("%s %s event %s has sessionId %q, want %q", marker, mode, event.Type, event.SessionID, sessionID)
		}
		encoded, marshalErr := json.Marshal(event)
		if marshalErr != nil {
			t.Fatalf("%s %s marshal event %s: %v", marker, mode, event.Type, marshalErr)
		}
		historyParts = append(historyParts, string(encoded))
		switch event.Type {
		case sessions.EventMessage:
			var payload struct {
				Role      string                      `json:"role"`
				Content   string                      `json:"content"`
				ToolCalls []sharedEditHistoryToolCall `json:"toolCalls"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("%s %s decode message event: %v", marker, mode, err)
			}
			if payload.Role == "user" {
				userContents = append(userContents, payload.Content)
			}
			if payload.Role == "assistant" {
				assistantCalls = append(assistantCalls, payload.ToolCalls...)
			}
		case sessions.EventToolCall:
			var payload sharedEditHistoryToolCall
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("%s %s decode tool call event: %v", marker, mode, err)
			}
			eventCalls = append(eventCalls, payload)
		case sessions.EventToolResult:
			var payload sharedEditHistoryToolResult
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("%s %s decode tool result event: %v", marker, mode, err)
			}
			results = append(results, payload)
		}
	}
	if len(userContents) != 1 || userContents[0] != prompt {
		t.Fatalf("%s %s history user messages = %#v, want exactly %q", marker, mode, userContents, prompt)
	}
	wantCalls := []sharedEditHistoryToolCall{
		{ID: read.ID, Name: "read_file", Arguments: read.Arguments},
		{ID: write.ID, Name: string(mode), Arguments: write.Arguments},
	}
	sharedEditAssertHistoryCalls(t, marker, mode, "assistant toolCalls", assistantCalls, wantCalls)
	sharedEditAssertHistoryCalls(t, marker, mode, "tool_call events", eventCalls, wantCalls)
	if len(results) != 2 {
		t.Fatalf("%s %s tool_result events = %#v, want exactly two", marker, mode, results)
	}
	wantReadOutput := sharedEditReadOutput(path, original)
	wantResults := map[string]sharedEditHistoryToolResult{
		read.ID:  {ID: read.ID, Name: "read_file", Status: "ok", Output: wantReadOutput},
		write.ID: {ID: write.ID, Name: string(mode), Status: write.State, Output: write.Output},
	}
	seenResults := make(map[string]bool, len(results))
	for _, result := range results {
		want, ok := wantResults[result.ID]
		if !ok || seenResults[result.ID] {
			t.Fatalf("%s %s unexpected or duplicate tool_result = %#v", marker, mode, result)
		}
		seenResults[result.ID] = true
		if result.Name != want.Name || result.Status != want.Status || result.Output != want.Output {
			t.Fatalf("%s %s tool_result %q = %#v, want name:%q status:%q output:%q", marker, mode, result.ID, result, want.Name, want.Status, want.Output)
		}
	}
	if len(seenResults) != len(wantResults) {
		t.Fatalf("%s %s tool_result IDs = %#v, want %#v", marker, mode, seenResults, wantResults)
	}
	sharedEditAssertNoOtherSessionFields(t, marker, mode, strings.Join(historyParts, "\n"), paths, values)
}

func sharedEditAssertHistoryCalls(t *testing.T, marker string, mode sharedEditWriteMode, label string, got, want []sharedEditHistoryToolCall) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s %s %s = %#v, want %#v", marker, mode, label, got, want)
	}
	byID := make(map[string]sharedEditHistoryToolCall, len(got))
	for _, call := range got {
		if _, exists := byID[call.ID]; exists {
			t.Fatalf("%s %s %s contains duplicate call ID %q", marker, mode, label, call.ID)
		}
		byID[call.ID] = call
	}
	for _, expected := range want {
		actual, ok := byID[expected.ID]
		if !ok || actual.Name != expected.Name || actual.Arguments != expected.Arguments {
			t.Fatalf("%s %s %s call %q = %#v, want name:%q arguments:%q", marker, mode, label, expected.ID, actual, expected.Name, expected.Arguments)
		}
	}
}

func sharedEditAssertProviderBody(t *testing.T, body, marker string, mode sharedEditWriteMode, prompt, path, original string, read, write appmodel.ToolCall, paths, values map[string]string) {
	t.Helper()
	var request sharedEditProviderRequest
	if err := json.Unmarshal([]byte(body), &request); err != nil {
		t.Fatalf("%s %s provider final request is invalid JSON: %v", marker, mode, err)
	}
	users := make([]string, 0, 1)
	calls := make([]sharedEditProviderToolCall, 0, 2)
	results := make([]struct {
		ID      string
		Content string
	}, 0, 2)
	for _, message := range request.Messages {
		switch message.Role {
		case "user":
			content, err := sharedEditProviderContent(message.Content)
			if err != nil {
				t.Fatalf("%s %s provider user content: %v", marker, mode, err)
			}
			users = append(users, content)
		case "assistant":
			calls = append(calls, message.ToolCalls...)
		case "tool":
			content, err := sharedEditProviderContent(message.Content)
			if err != nil {
				t.Fatalf("%s %s provider tool result content: %v", marker, mode, err)
			}
			results = append(results, struct {
				ID      string
				Content string
			}{ID: message.ToolCallID, Content: content})
		}
	}
	if len(users) != 1 || users[0] != prompt {
		t.Fatalf("%s %s provider user messages = %#v, want exactly %q", marker, mode, users, prompt)
	}
	if len(calls) != 2 {
		t.Fatalf("%s %s provider assistant tool calls = %#v, want exactly two", marker, mode, calls)
	}
	wantCalls := []struct {
		id   string
		name string
		args string
	}{
		{id: read.ID, name: "read_file", args: read.Arguments},
		{id: write.ID, name: string(mode), args: write.Arguments},
	}
	seenCalls := make(map[string]bool, len(calls))
	for _, call := range calls {
		if call.Type != "function" {
			t.Fatalf("%s %s provider tool call %q type = %q, want function", marker, mode, call.ID, call.Type)
		}
		if seenCalls[call.ID] {
			t.Fatalf("%s %s provider tool calls contain duplicate ID %q", marker, mode, call.ID)
		}
		seenCalls[call.ID] = true
		var want struct {
			id   string
			name string
			args string
		}
		for _, candidate := range wantCalls {
			if candidate.id == call.ID {
				want = candidate
				break
			}
		}
		if want.id == "" || call.Function.Name != want.name || call.Function.Arguments != want.args {
			t.Fatalf("%s %s provider tool call %q = name:%q arguments:%q, want %#v", marker, mode, call.ID, call.Function.Name, call.Function.Arguments, want)
		}
	}
	if len(seenCalls) != len(wantCalls) {
		t.Fatalf("%s %s provider tool call IDs = %#v, want %#v", marker, mode, seenCalls, wantCalls)
	}
	if len(results) != 2 {
		t.Fatalf("%s %s provider tool results = %#v, want exactly two", marker, mode, results)
	}
	wantResults := map[string]string{read.ID: sharedEditReadOutput(path, original), write.ID: write.Output}
	seenResults := make(map[string]bool, len(results))
	for _, result := range results {
		want, ok := wantResults[result.ID]
		if !ok || seenResults[result.ID] {
			t.Fatalf("%s %s provider tool result = %#v, want known unique call ID", marker, mode, result)
		}
		seenResults[result.ID] = true
		if result.Content != want {
			t.Fatalf("%s %s provider tool result %q content = %q, want %q", marker, mode, result.ID, result.Content, want)
		}
	}
	if len(seenResults) != len(wantResults) {
		t.Fatalf("%s %s provider tool result IDs = %#v, want %#v", marker, mode, seenResults, wantResults)
	}
	sharedEditAssertNoOtherSessionFields(t, marker, mode, body, paths, values)
}

func sharedEditProviderContent(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", errors.New("content field is missing")
	}
	var content string
	if err := json.Unmarshal(raw, &content); err != nil {
		return "", fmt.Errorf("content is not a string: %w", err)
	}
	return content, nil
}

func sharedEditAssertNoOtherSessionFields(t *testing.T, marker string, mode sharedEditWriteMode, text string, paths, values map[string]string) {
	t.Helper()
	for otherMarker := range paths {
		if otherMarker != marker {
			sharedEditAssertAbsentJSONScalar(t, marker, mode, text, "other session marker", otherMarker)
		}
	}
	for otherMarker, otherPath := range paths {
		if otherMarker != marker && otherPath != paths[marker] {
			sharedEditAssertAbsentJSONScalar(t, marker, mode, text, "other session path", otherPath)
		}
	}
	for otherMarker, otherValue := range values {
		if otherMarker != marker && otherValue != values[marker] {
			sharedEditAssertAbsentJSONScalar(t, marker, mode, text, "other session value", otherValue)
		}
	}
}

func sharedEditAssertAbsentJSONScalar(t *testing.T, marker string, mode sharedEditWriteMode, text, label, value string) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("%s %s encode %s %q: %v", marker, mode, label, value, err)
	}
	variants := []string{string(encoded)}
	// Arguments are themselves JSON strings inside both events.jsonl and the
	// Chat Completions request. Search their escaped scalar form as well so a
	// foreign path/value cannot hide behind the outer JSON encoding.
	nested, nestedErr := json.Marshal(string(encoded))
	if nestedErr != nil {
		t.Fatalf("%s %s encode nested %s %q: %v", marker, mode, label, value, nestedErr)
	}
	if len(nested) >= 2 {
		variants = append(variants, string(nested[1:len(nested)-1]))
	}
	for _, variant := range variants {
		if strings.Contains(text, variant) {
			t.Fatalf("%s %s contains %s %q", marker, mode, label, value)
		}
	}
}

func sharedEditReadOutput(path, content string) string {
	content = strings.TrimSuffix(content, "\n")
	lines := strings.Split(content, "\n")
	var output strings.Builder
	fmt.Fprintf(&output, "File: %s (%d lines)\n\n", path, len(lines))
	for index, line := range lines {
		if index > 0 {
			output.WriteByte('\n')
		}
		fmt.Fprintf(&output, "%d→%s", index+1, line)
	}
	return output.String()
}

func (provider *sharedEditProvider) writeBody(marker string) string {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return provider.writeBodies[marker]
}

func sharedEditAssertGitChanged(t *testing.T, project, path string) {
	t.Helper()
	command := exec.Command("git", "-C", project, "status", "--porcelain", "--", path)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git status %s: %v", path, err)
	}
	if !strings.Contains(string(output), path) {
		t.Fatalf("git status did not report %s: %q", path, output)
	}
}

func sharedEditGitProject(t *testing.T, files map[string]string) string {
	t.Helper()
	project := t.TempDir()
	for path, content := range files {
		fullPath := filepath.Join(project, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sharedEditRunGit(t, project, "init", "-q")
	sharedEditRunGit(t, project, "config", "user.email", "shared-edit@example.invalid")
	sharedEditRunGit(t, project, "config", "user.name", "GCode shared edit fixture")
	sharedEditRunGit(t, project, "add", ".")
	sharedEditRunGit(t, project, "commit", "-qm", "initial fixture")
	return project
}

func sharedEditRunGit(t *testing.T, project string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", project}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
}

func sharedEditMarker(body string, paths map[string]string) string {
	for marker := range paths {
		if strings.Contains(body, marker) {
			return marker
		}
	}
	return ""
}

func sharedEditExpectedRead(relativePath, project string) string {
	content, err := os.ReadFile(filepath.Join(project, relativePath))
	if err != nil {
		return ""
	}
	return string(content)
}

func sharedEditWriteArguments(mode sharedEditWriteMode, project, path, value string) map[string]any {
	if mode == sharedEditWrite {
		return map[string]any{"path": path, "content": value, "overwrite": true}
	}
	original, err := os.ReadFile(filepath.Join(project, path))
	if err != nil {
		return map[string]any{"path": path, "old_string": "base\n", "new_string": value}
	}
	return map[string]any{"path": path, "old_string": string(original), "new_string": value}
}
