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
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/storage"
)

// Real native UI, core, filesystem and process restart; the model is a local
// HTTP fixture. Every root and process belongs to this test.
func TestDesktopLocalStorageSwitchAndContinuation(t *testing.T) {
	binary, artifacts := testPaths(t)
	configRoot, nextRoot := filepath.Join(artifacts, "config"), filepath.Join(artifacts, "runtime")
	requests := make(chan string, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		requests <- string(body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"STORAGE_REPLY\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	store, err := storage.Open(storage.Options{Directory: configRoot})
	if err != nil {
		t.Fatal(err)
	}
	core, err := openExecutionFixture(t, engine.Options{Storage: store})
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	defer core.Close()
	provider := model.Provider{ID: "storage-test", Name: "Storage fixture", Protocol: model.Chat,
		BaseURL: server.URL, Models: []model.Model{{ID: "test"}}}
	for _, command := range []contracts.Command{
		{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider}},
		{Kind: "message.send", Send: &contracts.SendRequest{Text: "STORAGE_ORIGINAL_CONTEXT", ModelID: "storage-test/test"}},
	} {
		if err := core.Execute(context.Background(), command); err != nil {
			t.Fatal(err)
		}
	}
	var original contracts.Session
	deadline := time.Now().Add(stateTimeout)
	for {
		state, err := core.Snapshot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(state.Sessions) == 1 && state.Sessions[0].LastOutcome == "completed" {
			original = state.Sessions[0]
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("storage fixture did not finish")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	<-requests
	app := startApp(t, binary, artifacts, "select-runtime-root", configRoot, "--local")
	app.act("data-directory", "focus", "")
	app.act("data-directory", "setValue", nextRoot)
	app.act("data-directory-browse", "focus", "")
	app.await("new runtime root registered without moving historical files", func() error {
		data, err := os.ReadFile(filepath.Join(configRoot, "location.json"))
		if err != nil {
			return err
		}
		var location struct {
			Pending string `json:"pendingRootId"`
		}
		if err := json.Unmarshal(data, &location); err != nil {
			return err
		}
		if location.Pending == "" {
			return fmt.Errorf("directory change has not committed")
		}
		if _, err := os.Stat(filepath.Join(nextRoot, "sessions", original.ID)); !os.IsNotExist(err) {
			return fmt.Errorf("historical session appeared under the new root: %v", err)
		}
		return nil
	})
	app.capture("pending-runtime-root")
	app.stop()

	app = startApp(t, binary, artifacts, "continue-after-restart", configRoot, "--local")
	app.await("restarted process uses the selected root", func() error {
		tree, err := app.client.tree()
		if err != nil {
			return err
		}
		node, err := tree.findKey("data-directory")
		if err != nil {
			return err
		}
		if node.Value != nextRoot {
			return fmt.Errorf("current root = %q", node.Value)
		}
		return nil
	})
	app.act("back-to-conversations", "press", "")
	app.act("sidebar-session:"+original.ID, "press", "")
	app.waitKey("titlebar-session-info")
	app.act("input", "setValue", "STORAGE_CONTINUE_AT_ORIGIN")
	app.act("send", "press", "")
	var continuation string
	app.await("continued model request retains historical context", func() error {
		if continuation == "" {
			select {
			case continuation = <-requests:
			default:
				return fmt.Errorf("waiting for the continued model request")
			}
		}
		if !strings.Contains(continuation, "STORAGE_ORIGINAL_CONTEXT") || !strings.Contains(continuation, "STORAGE_CONTINUE_AT_ORIGIN") {
			return fmt.Errorf("continued request lacks original or new input")
		}
		return nil
	})
	app.await("continued conversation committed at its original root", func() error {
		state, records, err := readStorageNativeIndex(configRoot)
		if err != nil {
			return err
		}
		for _, session := range state.Sessions {
			if session.ID != original.ID {
				continue
			}
			if session.CWD != original.CWD || session.StorageRootID != original.StorageRootID || session.LastOutcome != "completed" {
				return fmt.Errorf("original conversation changed identity or is not finished")
			}
			body, err := os.ReadFile(filepath.Join(configRoot, "sessions", original.ID, records[original.ID]+".json"))
			if err != nil || !strings.Contains(string(body), "STORAGE_CONTINUE_AT_ORIGIN") {
				return fmt.Errorf("continuation has not committed at origin: %v", err)
			}
			return nil
		}
		return fmt.Errorf("original conversation disappeared")
	})
	app.capture("historical-continuation")
	app.act("sidebar-new-conversation", "press", "")
	app.waitKey("input")
	app.act("input", "setValue", "STORAGE_NEW_ROOT")
	app.act("send", "press", "")
	app.await("new independent conversation uses its own workspace in the new root", func() error {
		state, records, err := readStorageNativeIndex(configRoot)
		if err != nil {
			return err
		}
		for _, session := range state.Sessions {
			if session.ID == original.ID || session.LastOutcome != "completed" {
				continue
			}
			if session.StorageRootID == original.StorageRootID || session.CWD != filepath.Join(nextRoot, "workspaces", session.ID) {
				return fmt.Errorf("new conversation did not bind the active root")
			}
			if _, err := os.Stat(filepath.Join(nextRoot, "sessions", session.ID, records[session.ID]+".json")); err != nil {
				return err
			}
			return nil
		}
		return fmt.Errorf("waiting for new conversation")
	})
	app.capture("new-conversation-runtime-root")
	app.stop()
	if _, err := os.Stat(filepath.Join(nextRoot, "engine.json")); !os.IsNotExist(err) {
		t.Fatal("fixed configuration followed the runtime root", err)
	}
	if _, err := os.Stat(filepath.Join(nextRoot, "logs", "gcode.log")); err != nil {
		t.Fatal("native runtime log did not follow the selected root", err)
	}
}

func readStorageNativeIndex(directory string) (contracts.Snapshot, map[string]string, error) {
	data, err := os.ReadFile(filepath.Join(directory, "engine.json"))
	if err != nil {
		return contracts.Snapshot{}, nil, err
	}
	var index struct {
		Snapshot contracts.Snapshot `json:"snapshot"`
		Records  map[string]string  `json:"records"`
	}
	err = json.Unmarshal(data, &index)
	return index.Snapshot, index.Records, err
}
