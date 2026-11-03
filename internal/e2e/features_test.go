//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/processutil"
	"gcode/internal/storage"
)

// This uses a real desktop and core, a temporary Git repository and a local
// HTTP model fixture. It never uses an external provider or the user's data.
func TestDesktopLocalNamingAndWorktree(t *testing.T) {
	binary, artifacts := testPaths(t)
	repository := filepath.Join(artifacts, "repository")
	if err := os.MkdirAll(repository, 0700); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) {
		t.Helper()
		command := exec.Command("git", append([]string{"-C", repository}, args...)...)
		processutil.HideConsole(command)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("temporary git repository: %v: %s", err, output)
		}
	}
	git("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(repository, "README.md"), []byte("Native worktree test\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "--", "README.md")
	git("-c", "user.name=GCode test", "-c", "user.email=rcode-test@example.invalid", "commit", "-m", "Initial fixture")

	var namingRequests, chatRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		text := "The checkout is ready.\n\n$$\\frac{a}{b}+\\sqrt{x^2+y^2}$$"
		if strings.Contains(string(body), "Create a concise conversation title") {
			namingRequests.Add(1)
			text = "Native worktree conversation"
		} else {
			chatRequests.Add(1)
		}
		payload, _ := json.Marshal(map[string]any{
			"choices": []any{map[string]any{
				"delta": map[string]any{"content": text}, "finish_reason": "stop",
			}},
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", payload)
	}))
	defer server.Close()

	data := filepath.Join(artifacts, "data")
	store, err := storage.Open(storage.Options{Directory: data})
	if err != nil {
		t.Fatal(err)
	}
	core, err := engine.Open(engine.Options{Storage: store})
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	ctx := context.Background()
	config := model.Provider{
		ID: "native-test", Name: "Native local HTTP fixture", Protocol: model.Chat,
		BaseURL: server.URL, Models: []model.Model{{ID: "test-model", MaxOutput: 512}},
	}
	for _, command := range []contracts.Command{
		{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: config}},
		{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: repository}},
	} {
		if err := core.Execute(ctx, command); err != nil {
			_ = core.Close()
			t.Fatal(err)
		}
	}
	snapshot, err := core.Snapshot(ctx)
	if err != nil {
		_ = core.Close()
		t.Fatal(err)
	}
	settings := snapshot.Settings
	settings.General.ThreadNamingModel = "native-test/test-model"
	worktrees := filepath.Join(data, "worktrees")
	// Test delivery separately through injected sinks; avoid OS notifications
	// or sounds from this real-window fixture.
	settings.Notifications = contracts.NotificationSettings{}
	if err := core.Execute(ctx, contracts.Command{Kind: "settings.update", Settings: &settings}); err != nil {
		_ = core.Close()
		t.Fatal(err)
	}
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}

	app := startApp(t, binary, artifacts, "conversation", data, "--local")
	app.act("back-to-conversations", "press", "")
	app.act("Select checkout", "press", "")
	app.act("checkout-newWorktree", "press", "")
	app.act("input", "setValue", "Prepare this conversation in its own checkout")
	app.act("send", "press", "")

	var accepted contracts.Session
	app.await("named conversation accepted in a real data-root worktree", func() error {
		payload, err := os.ReadFile(filepath.Join(data, "engine.json"))
		if err != nil {
			return err
		}
		var disk struct {
			Snapshot contracts.Snapshot `json:"snapshot"`
		}
		if err := json.Unmarshal(payload, &disk); err != nil {
			return err
		}
		for _, session := range disk.Snapshot.Sessions {
			if session.Title != "Native worktree conversation" {
				continue
			}
			relative, err := filepath.Rel(worktrees, session.CWD)
			if err != nil || relative == "." || !filepath.IsLocal(relative) || session.WorktreeID == "" {
				return fmt.Errorf("accepted cwd %q is not a new worktree inside %q", session.CWD, worktrees)
			}
			if session.Status != contracts.Idle || session.LastOutcome != "completed" {
				return fmt.Errorf("conversation is not complete: %s %s", session.Status, session.LastOutcome)
			}
			accepted = session
			return nil
		}
		return fmt.Errorf("generated title not persisted")
	})
	app.waitKey("sidebar-session:" + accepted.ID)
	app.capture("named-worktree-with-math")
	app.stop()
	if namingRequests.Load() != 1 || chatRequests.Load() != 1 {
		t.Fatalf("model requests: naming=%d chat=%d, want one each", namingRequests.Load(), chatRequests.Load())
	}
	command := exec.Command("git", "-C", accepted.CWD, "rev-parse", "--show-toplevel")
	processutil.HideConsole(command)
	if output, err := command.CombinedOutput(); err != nil ||
		!strings.EqualFold(filepath.Clean(strings.TrimSpace(string(output))), filepath.Clean(accepted.CWD)) {
		t.Fatalf("created checkout is not registered with Git: %s, %v", output, err)
	}
}
