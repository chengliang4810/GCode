//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/storage"
)

// Git changes occur outside the running app in a private repository. The real
// local core and Read tool execute the send; only the model stream is synthetic.
func TestDesktopLocalRepositoryRefsRefresh(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			binary, artifacts := testPaths(t)
			repository := filepath.Join(artifacts, "repository")
			if err := os.MkdirAll(repository, 0700); err != nil {
				t.Fatal(err)
			}
			git := func(args ...string) string {
				t.Helper()
				command := exec.Command("git", append([]string{"-C", repository}, args...)...)
				configureProcess(command)
				output, err := command.CombinedOutput()
				if err != nil {
					t.Fatalf("fixture git %v: %s: %v", args, output, err)
				}
				return strings.TrimSpace(string(output))
			}
			git("init", "--initial-branch=main")
			if err := os.WriteFile(filepath.Join(repository, "marker.txt"), []byte("main marker"), 0600); err != nil {
				t.Fatal(err)
			}
			git("add", "marker.txt")
			git("-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "-m", "fixture")
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct{ Messages []struct{ Role string } }
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if len(request.Messages) > 0 && request.Messages[len(request.Messages)-1].Role == "tool" {
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Fresh checkout confirmed.\"},\"finish_reason\":\"stop\"}]}\n\n")
				} else {
					fmt.Fprint(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"read-marker","function":{"name":"Read","arguments":"{\"file_path\":\"marker.txt\"}"}}]},"finish_reason":"tool_calls"}]}`+"\n\n")
				}
				fmt.Fprint(w, "data: [DONE]\n\n")
			}))
			defer provider.Close()
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
			for _, command := range []contracts.Command{
				{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: model.Provider{ID: "refs", Name: "Refs fixture", Protocol: model.Chat, BaseURL: provider.URL, Models: []model.Model{{ID: "model"}}}}},
				{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: repository, Name: "Refs refresh"}},
			} {
				if err := core.Execute(t.Context(), command); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, err := core.Snapshot(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			project := snapshot.Projects[0]
			scope := contracts.RepoRefScope{ProjectID: project.ID, TargetID: "local", RepoPath: repository}
			if err := core.Execute(t.Context(), contracts.Command{Kind: "git.refs.load", Git: &contracts.GitRequest{Scope: scope}}); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(stateTimeout)
			for {
				snapshot, err = core.Snapshot(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				if snapshot.RepoRefs[scope.Key()].State == contracts.LoadReady {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("initial refs cache did not settle")
				}
				time.Sleep(10 * time.Millisecond)
			}
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}
			// The persisted Ready cache still says main when the UI starts.
			git("switch", "-c", "startup-fresh")
			app := startApp(t, binary, artifacts, "refs", data, "--local", "--appearance", appearance, "--language", "zh-CN", "--width", "900", "--height", "900")
			app.act("back-to-conversations", "press", "")
			awaitRef := func(want string) {
				t.Helper()
				app.await("footer ref "+want, func() error {
					ref := app.waitKey("Select ref")
					if ref.Value != want {
						return fmt.Errorf("ref=%q want=%q", ref.Value, want)
					}
					return nil
				})
			}
			closeMenu := func() {
				t.Helper()
				if err := app.client.call("input", map[string]any{"events": []map[string]any{{"type": "key", "key": "Escape"}}}, nil); err != nil {
					t.Fatal(err)
				}
			}
			awaitRef("startup-fresh")
			git("branch", "added-on-first-open")
			app.act("Select ref", "press", "")
			app.waitKey("startup-fresh") // Cached rows remain available as the refresh starts.
			app.waitKey("added-on-first-open")
			closeMenu()
			git("branch", "added-on-second-open")
			app.act("Select ref", "press", "")
			app.waitKey("added-on-second-open")
			if err := app.client.call("idle", map[string]int{"timeout": 2000}, nil); err != nil {
				t.Fatal(err)
			}
			app.capture("ref-menu-refreshed")
			if err := app.client.call("input", map[string]any{"events": []map[string]any{{"type": "key", "key": "Enter"}}}, nil); err != nil {
				t.Fatal(err)
			}
			app.await("Enter after refresh keeps the current branch", func() error {
				snapshot, err := readFixtureSnapshot(data)
				if err != nil {
					return err
				}
				refs := snapshot.RepoRefs[scope.Key()]
				if refs.Switching != "" || snapshot.Projects[0].Branch != "startup-fresh" {
					return fmt.Errorf("refresh moved keyboard selection: switching=%q branch=%q", refs.Switching, snapshot.Projects[0].Branch)
				}
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				for _, node := range tree.Nodes {
					if strings.Contains(node.Label, "搜索引用") {
						return fmt.Errorf("ref picker did not close")
					}
				}
				return nil
			})
			git("switch", "-c", "new-draft-fresh")
			app.act("sidebar-new-conversation", "press", "")
			awaitRef("new-draft-fresh")
			app.await("new draft refresh completes", func() error {
				snapshot, err := readFixtureSnapshot(data)
				if err != nil {
					return err
				}
				if refs := snapshot.RepoRefs[scope.Key()]; refs.State != contracts.LoadReady {
					return fmt.Errorf("refs state=%q", refs.State)
				}
				return nil
			})
			// Change HEAD again after refresh has completed. Following the
			// default current checkout must not submit the cached branch name.
			git("switch", "-c", "actual-at-send")
			if err := os.WriteFile(filepath.Join(repository, "marker.txt"), []byte("actual-at-send marker"), 0600); err != nil {
				t.Fatal(err)
			}
			awaitRef("new-draft-fresh")
			app.act("input", "setValue", "Read the marker from the current checkout")
			app.act("send", "press", "")
			app.await("send follows actual checkout despite cached label", func() error {
				snapshot, err := readFixtureSnapshot(data)
				if err != nil {
					return err
				}
				for _, session := range snapshot.Sessions {
					if session.ProjectID != project.ID || session.LastOutcome != "completed" {
						continue
					}
					if session.Branch != "actual-at-send" || session.SourceContext == nil ||
						session.SourceContext.Branch != "actual-at-send" || filepath.Clean(session.CWD) != filepath.Clean(repository) {
						return fmt.Errorf("session used cached branch: %+v", session)
					}
					for _, message := range snapshot.Messages[session.ID] {
						for _, block := range message.Blocks {
							if block.Tool != nil && block.Tool.Name == "Read" && !block.Tool.IsError &&
								strings.Contains(block.Tool.Output, "actual-at-send marker") {
								return nil
							}
						}
					}
				}
				return fmt.Errorf("completed real checkout read is missing")
			})
			if actual := git("branch", "--show-current"); actual != "actual-at-send" {
				t.Fatalf("default send switched the repository: %s", actual)
			}
			app.capture("actual-checkout-send")
		})
	}
}
