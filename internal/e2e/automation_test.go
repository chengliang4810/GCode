//go:build e2e

// Package e2e exercises a separately built, real native desktop through MyGo's
// automation plugin. These tests require a graphical desktop and never use
// --headless or the ui.Tester renderer.
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/png"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

const stateTimeout = 20 * time.Second

type automationClient struct {
	socket string
}

type automationError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *automationError) Error() string { return e.Code + ": " + e.Message }

// call uses a fresh connection so a timed out request cannot leave a stale
// response queued ahead of the next request. Every RPC has its own deadline.
func (c automationClient) call(method string, params, result any) error {
	conn, err := net.DialTimeout("unix", c.socket, 2*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		return err
	}
	req := struct {
		ID     int    `json:"id"`
		Method string `json:"method"`
		Params any    `json:"params"`
	}{1, method, params}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return err
	}
	var response struct {
		ID     int              `json:"id"`
		Result json.RawMessage  `json:"result"`
		Error  *automationError `json:"error"`
	}
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return err
	}
	if response.ID != req.ID {
		return fmt.Errorf("response ID = %d, want %d", response.ID, req.ID)
	}
	if response.Error != nil {
		return response.Error
	}
	if result != nil {
		return json.Unmarshal(response.Result, result)
	}
	return nil
}

type accessNode struct {
	Bounds struct {
		X, Y, Width, Height float64
	} `json:"bounds"`
	ID      string          `json:"id"`
	Key     json.RawMessage `json:"key,omitempty"`
	Role    string          `json:"role"`
	Label   string          `json:"label,omitempty"`
	Value   string          `json:"value,omitempty"`
	States  []string        `json:"states,omitempty"`
	Actions []string        `json:"actions,omitempty"`
}

func (n accessNode) checked() bool { return slices.Contains(n.States, "checked") }

type accessTree struct {
	Window int          `json:"window"`
	Focus  string       `json:"focus,omitempty"`
	Nodes  []accessNode `json:"nodes"`
}

func (c automationClient) tree() (accessTree, error) {
	var tree accessTree
	err := c.call("tree", nil, &tree)
	return tree, err
}

func (tree accessTree) findKey(key string) (accessNode, error) {
	var found accessNode
	count := 0
	for _, node := range tree.Nodes {
		var value string
		if json.Unmarshal(node.Key, &value) == nil && value == key {
			found = node
			count++
		}
	}
	if count != 1 {
		return accessNode{}, fmt.Errorf("key %q: found %d nodes, want exactly one", key, count)
	}
	return found, nil
}

type nativeApp struct {
	t          *testing.T
	client     automationClient
	cmd        *exec.Cmd
	done       chan struct{}
	waitErr    error
	log        *os.File
	socketRoot string
	artifacts  string
	stopped    bool
}

func testPaths(t *testing.T) (binary, artifacts string) {
	t.Helper()
	binary = os.Getenv("RCODE_E2E_BINARY")
	artifacts = os.Getenv("RCODE_E2E_ARTIFACTS")
	if binary == "" || artifacts == "" {
		t.Fatal("native E2E requires RCODE_E2E_BINARY and RCODE_E2E_ARTIFACTS; run go run ./scripts/desktop-e2e (a graphical desktop is required)")
	}
	if !filepath.IsAbs(binary) || !filepath.IsAbs(artifacts) {
		t.Fatal("RCODE_E2E_BINARY and RCODE_E2E_ARTIFACTS must be absolute paths")
	}
	if info, err := os.Stat(binary); err != nil || info.IsDir() {
		t.Fatalf("native E2E binary %q is not a file: %v", binary, err)
	}
	if err := os.MkdirAll(artifacts, 0700); err != nil {
		t.Fatal(err)
	}
	artifacts, err := os.MkdirTemp(artifacts, strings.ReplaceAll(t.Name(), "/", "-")+"-")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("native artifacts: %s", artifacts)
	return binary, artifacts
}

func startApp(t *testing.T, binary, artifacts, phase, data string, extra ...string) *nativeApp {
	t.Helper()
	phaseDir := filepath.Join(artifacts, phase)
	if err := os.MkdirAll(phaseDir, 0700); err != nil {
		t.Fatal(err)
	}
	// Windows AF_UNIX connections can fail under redirected system TEMP even
	// when Listen succeeds. A private directory in the user's profile also keeps
	// the socket within sockaddr_un's short path limit.
	socketParent, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	socketRoot, err := os.MkdirTemp(socketParent, ".rcode-e2e-")
	if err != nil {
		t.Fatal(err)
	}
	log, err := os.Create(filepath.Join(phaseDir, "app.log"))
	if err != nil {
		_ = os.RemoveAll(socketRoot)
		t.Fatal(err)
	}
	args := []string{"--data-dir", data, "--route", "settings/general", "--language", "en", "--appearance", "light", "--width", "1320", "--height", "880"}
	args = append(args, extra...)
	app := &nativeApp{t: t, client: automationClient{socket: filepath.Join(socketRoot, "app.sock")}, log: log, socketRoot: socketRoot, artifacts: phaseDir, done: make(chan struct{})}
	app.cmd = exec.Command(binary, args...)
	app.cmd.Stdout, app.cmd.Stderr = log, log
	app.cmd.Env = append(withoutEnv(os.Environ(), "MYGO_AUTOMATION"), "MYGO_AUTOMATION="+app.client.socket)
	configureProcess(app.cmd)
	if err := app.cmd.Start(); err != nil {
		_ = log.Close()
		_ = os.RemoveAll(socketRoot)
		t.Fatal(err)
	}
	go func() {
		app.waitErr = app.cmd.Wait()
		close(app.done)
	}()
	t.Cleanup(func() {
		if !app.stopped {
			app.capture("final")
			app.stop()
		}
	})
	app.await("GCode native window and General controls", func() error {
		var result struct {
			Windows []struct {
				Title   string `json:"title"`
				Kind    string `json:"kind"`
				Visible bool   `json:"visible"`
			} `json:"windows"`
		}
		if err := app.client.call("windows", nil, &result); err != nil {
			return err
		}
		found := false
		for _, window := range result.Windows {
			found = found || window.Title == "GCode" && window.Kind == "content" && window.Visible
		}
		if !found {
			return fmt.Errorf("no visible GCode content window: %+v", result.Windows)
		}
		tree, err := app.client.tree()
		if err != nil {
			return err
		}
		_, err = tree.findKey("Compact mode")
		return err
	})
	return app
}

func withoutEnv(env []string, name string) []string {
	result := make([]string, 0, len(env))
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.EqualFold(key, name) {
			result = append(result, entry)
		}
	}
	return result
}

func (app *nativeApp) await(description string, condition func() error) {
	app.t.Helper()
	deadline := time.Now().Add(stateTimeout)
	var last error
	for {
		select {
		case <-app.done:
			app.t.Fatalf("waiting for %s: app exited: %v (log: %s)", description, app.waitErr, filepath.Join(app.artifacts, "app.log"))
		default:
		}
		last = condition()
		if last == nil {
			return
		}
		if time.Now().After(deadline) {
			app.t.Fatalf("waiting for %s: %v", description, last)
		}
		timer := time.NewTimer(80 * time.Millisecond)
		select {
		case <-timer.C:
		case <-app.done:
			timer.Stop()
		}
	}
}

func (app *nativeApp) waitKey(key string) accessNode {
	app.t.Helper()
	var node accessNode
	app.await("key "+key, func() error {
		tree, err := app.client.tree()
		if err != nil {
			return err
		}
		node, err = tree.findKey(key)
		return err
	})
	return node
}

// awaitKeyPrefix returns the single key starting with prefix. Volume rows carry
// the host's drive or mount path, so a native run never guesses a key.
func (app *nativeApp) awaitKeyPrefix(prefix string) string {
	app.t.Helper()
	found := ""
	app.await("key prefix "+prefix, func() error {
		tree, err := app.client.tree()
		if err != nil {
			return err
		}
		matches := make([]string, 0, 2)
		for _, node := range tree.Nodes {
			var value string
			if json.Unmarshal(node.Key, &value) == nil && strings.HasPrefix(value, prefix) {
				matches = append(matches, value)
			}
		}
		if len(matches) == 0 {
			return fmt.Errorf("no key starts with %q", prefix)
		}
		found = matches[0]
		return nil
	})
	return found
}

func (app *nativeApp) act(key, action, text string) {
	app.t.Helper()
	app.await(fmt.Sprintf("%s action on %q", action, key), func() error {
		tree, err := app.client.tree()
		if err != nil {
			return err
		}
		node, err := tree.findKey(key)
		if err != nil {
			return err
		}
		if !slices.Contains(node.Actions, action) {
			return fmt.Errorf("key %q does not advertise action %q: %v", key, action, node.Actions)
		}
		return nil
	})
	params := map[string]any{"key": key, "action": action}
	if action == "setValue" || action == "insertText" {
		params["text"] = text
	}
	if err := app.client.call("act", params, nil); err != nil {
		app.t.Fatalf("%s %q: %v", action, key, err)
	}
}

func (app *nativeApp) waitChecked(key string, want bool) {
	app.t.Helper()
	app.await(fmt.Sprintf("%s checked=%t", key, want), func() error {
		tree, err := app.client.tree()
		if err != nil {
			return err
		}
		node, err := tree.findKey(key)
		if err != nil {
			return err
		}
		if node.checked() != want {
			return fmt.Errorf("checked=%t, want %t", node.checked(), want)
		}
		return nil
	})
}

func (app *nativeApp) capture(name string) {
	app.t.Helper()
	// Preserve the complete protocol tree, including geometry and announcements.
	var tree json.RawMessage
	if err := app.client.call("tree", nil, &tree); err != nil {
		app.t.Errorf("capture %s tree: %v", name, err)
	} else if err := os.WriteFile(filepath.Join(app.artifacts, name+"-tree.json"), tree, 0600); err != nil {
		app.t.Error(err)
	}
	var capture struct {
		PNG    []byte `json:"png"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
	}
	if err := app.client.call("capture", nil, &capture); err != nil {
		app.t.Errorf("capture %s image: %v", name, err)
		return
	}
	config, err := png.DecodeConfig(bytes.NewReader(capture.PNG))
	if err != nil || config.Width <= 0 || config.Height <= 0 || config.Width != capture.Width || config.Height != capture.Height {
		app.t.Errorf("capture %s invalid PNG geometry: %+v, protocol %dx%d, err=%v", name, config, capture.Width, capture.Height, err)
		return
	}
	if err := os.WriteFile(filepath.Join(app.artifacts, name+".png"), capture.PNG, 0600); err != nil {
		app.t.Error(err)
	}
}

func (app *nativeApp) stop() {
	app.t.Helper()
	if app.stopped {
		return
	}
	app.stopped = true
	select {
	case <-app.done:
	default:
		// Only the process created by this test is terminated. No process-name
		// searches, user app shutdowns or shared-profile cleanup are involved.
		if err := app.cmd.Process.Kill(); err != nil {
			app.t.Errorf("stop owned desktop: %v", err)
		}
		select {
		case <-app.done:
		case <-time.After(5 * time.Second):
			app.t.Error("owned desktop did not exit after termination")
		}
	}
	if err := app.log.Close(); err != nil {
		app.t.Error(err)
	}
	if err := os.RemoveAll(app.socketRoot); err != nil {
		app.t.Errorf("remove owned automation socket directory: %v", err)
	}
}
