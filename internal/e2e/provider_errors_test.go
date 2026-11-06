//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/storage"
)

// The two endpoints deliberately expose the same model name. Only the native
// pointer path selects the second provider; accessibility actions bypass it.
func TestDesktopLocalSameNameProviderSwitch(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			testDesktopSameNameProviderSwitch(t, appearance)
		})
	}
}

func testDesktopSameNameProviderSwitch(t *testing.T, appearance string) {
	t.Helper()
	binary, artifacts := testPaths(t)
	var secondCalls atomic.Int32
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, `{"code":503,"message":"**渠道凭证不可用** fixture-secret","stdCode":50301}`)
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Model != "same-name" {
			t.Errorf("second provider received wrong model: %+v err=%v", request, err)
		}
		secondCalls.Add(1)
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"SECOND_PROVIDER_OK\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer second.Close()
	data := filepath.Join(artifacts, "data")
	store, err := storage.Open(storage.Options{Directory: data})
	if err != nil {
		t.Fatal(err)
	}
	core, err := openExecutionFixture(t, engine.Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	for i, endpoint := range []string{first.URL, second.URL} {
		id := fmt.Sprintf("provider-%d", i)
		err := core.Execute(context.Background(), contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{
			Provider: model.Provider{ID: id, Name: id, Protocol: model.Chat, BaseURL: endpoint,
				APIKey: "fixture-secret", Models: []model.Model{{ID: "same-name", ReasoningLevels: []string{"low", "high", "max"}}}},
		}})
		if err != nil {
			t.Fatal(err)
		}
	}
	requestID := "provider-failure"
	if err := core.Execute(context.Background(), contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
		RequestID: requestID, ModelID: "provider-0/same-name", Text: "First provider failure",
	}}); err != nil {
		t.Fatal(err)
	}
	state, err := core.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Sessions) != 1 {
		t.Fatalf("provider fixture created %d sessions, want 1", len(state.Sessions))
	}
	sessionID := state.Sessions[0].ID
	deadline := time.Now().Add(stateTimeout)
	for {
		state, err = core.Snapshot(context.Background(), sessionID)
		if err != nil {
			t.Fatal(err)
		}
		var session contracts.Session
		for _, candidate := range state.Sessions {
			if candidate.ID == sessionID {
				session = candidate
				break
			}
		}
		turnFailed := false
		for _, run := range state.Runs {
			if run.SessionID == sessionID && run.RequestID == requestID {
				turnFailed = run.Status == "failed"
				break
			}
		}
		if session.Status == contracts.Errored && session.LastOutcome == "failed" && turnFailed {
			messages := state.Messages[sessionID]
			if last := messages[len(messages)-1]; len(last.Blocks) != 1 || last.Blocks[0].Kind != "error" {
				t.Fatalf("provider error entered the markdown text path: %+v", last)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("first provider did not fail: session=%+v runs=%+v", session, state.Runs)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	app := startApp(t, binary, artifacts, "switch", data, "--local", "--appearance", appearance, "--language", "zh-CN", "--width", "900", "--height", "800")
	capture := func(name string) {
		t.Helper()
		if err := app.client.call("idle", map[string]int{"timeout": 2000}, nil); err != nil {
			t.Fatal(err)
		}
		app.capture(name)
	}
	app.act("back-to-conversations", "press", "")
	app.act("sidebar-session:"+sessionID, "press", "")
	app.await("provider error body is visible and redacted after restart", func() error {
		tree, err := app.client.tree()
		if err != nil {
			return err
		}
		for _, want := range []string{"HTTP 503", `"code":503`, `"stdCode":50301`, "**渠道凭证不可用** [redacted]"} {
			if !activityTreeContains(tree, want) {
				return fmt.Errorf("original provider error missing: %s", want)
			}
		}
		if activityTreeContains(tree, "fixture-secret") {
			return fmt.Errorf("provider credential exposed in error UI")
		}
		return nil
	})
	capture("original-provider-error")
	pointerClick := func(key string) {
		t.Helper()
		node := app.waitKey(key)
		x, y := node.Bounds.X+node.Bounds.Width/2, node.Bounds.Y+node.Bounds.Height/2
		if err := app.client.call("input", map[string]any{"events": []map[string]any{
			{"type": "pointerMove", "x": x, "y": y},
			{"type": "pointerDown", "x": x, "y": y},
			{"type": "pointerUp", "x": x, "y": y},
		}}, nil); err != nil {
			t.Fatal(err)
		}
	}
	pointerClick("model-trigger")
	capture("current-model-without-hover")
	pointerClick("model-provider:provider-1")
	capture("provider-filter-without-model-hover")
	movePointer := func(key string) {
		t.Helper()
		node := app.waitKey(key)
		if err := app.client.call("input", map[string]any{"events": []map[string]any{{
			"type": "pointerMove", "x": node.Bounds.X + node.Bounds.Width/2, "y": node.Bounds.Y + node.Bounds.Height/2,
		}}}, nil); err != nil {
			t.Fatal(err)
		}
	}
	movePointer("model-choice:provider-1/same-name")
	capture("model-hover")
	movePointer("model-provider:provider-1")
	capture("model-hover-cleared")
	pointerClick("model-choice:provider-1/same-name")
	app.await("same-name model switches provider and closes picker", func() error {
		tree, err := app.client.tree()
		if err != nil {
			return err
		}
		if _, err := tree.findKey("model-list-menu"); err == nil {
			return fmt.Errorf("model picker remains open after single click")
		}
		state, err := readFixtureSnapshot(data)
		if err != nil {
			return err
		}
		for _, session := range state.Sessions {
			if session.ID == sessionID && session.ModelID == "provider-1/same-name" {
				return nil
			}
		}
		return fmt.Errorf("second provider not persisted")
	})
	if identity := app.waitKey("model-trigger").Value; identity != "provider-1 · same-name" {
		t.Fatalf("same-name models lack visible provider identity: %s", identity)
	}
	// Hover the button to verify the identity is available to sighted users too.
	node := app.waitKey("model-trigger")
	if err := app.client.call("input", map[string]any{"events": []map[string]any{{
		"type": "pointerMove", "x": node.Bounds.X + 2, "y": node.Bounds.Y + 2,
	}}}, nil); err != nil {
		t.Fatal(err)
	}
	app.await("selected provider and model appear on hover", func() error {
		tree, err := app.client.tree()
		if err != nil {
			return err
		}
		for _, candidate := range tree.Nodes {
			if candidate.Role == "tooltip" && candidate.Label == "provider-1 · same-name" {
				return nil
			}
		}
		return fmt.Errorf("selected provider tooltip missing")
	})
	capture("selected-provider-tooltip")
	app.act("input", "setValue", "Use the second provider")
	app.act("send", "press", "")
	app.await("actual request reaches second provider", func() error {
		tree, err := app.client.tree()
		if err != nil {
			return err
		}
		if secondCalls.Load() != 1 || !activityTreeContains(tree, "SECOND_PROVIDER_OK") {
			return fmt.Errorf("second provider calls=%d", secondCalls.Load())
		}
		return nil
	})
	capture("second-provider-reply")
	app.stop()
	// Both the transcript and every failed attempt retain the same original
	// body through the production trajectory route, without provider secrets.
	app = startAppReady(t, binary, artifacts, "trajectory", data, "trajectory-pane:"+sessionID,
		"--local", "--appearance", appearance, "--route", "trajectory/"+sessionID, "--language", "zh-CN")
	app.act("trajectory-search", "setValue", "50301")
	app.await("all failed attempts retain raw details in trajectory", func() error {
		tree, err := app.client.tree()
		if err != nil {
			return err
		}
		if !activityTreeContains(tree, `"stdCode":50301`) || !activityTreeContains(tree, "**渠道凭证不可用** [redacted]") {
			return fmt.Errorf("original provider details missing from trajectory")
		}
		if activityTreeContains(tree, "fixture-secret") {
			return fmt.Errorf("provider credential exposed in trajectory")
		}
		state, err := readFixtureSnapshot(data)
		if err != nil {
			return err
		}
		for _, session := range state.Sessions {
			if session.ID == sessionID && strings.HasPrefix(session.ModelID, "provider-1/") {
				return nil
			}
		}
		return fmt.Errorf("selected provider lost on restart")
	})
	capture("original-provider-error-trajectory")
}
