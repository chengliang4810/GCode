//go:build ignore

// A real headless-host smoke test. Build for Linux and run without a display.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

func main() {
	binary := flag.String("server", "", "Linux gcode-server executable")
	evidence := flag.String("evidence", "", "Evidence JSON path")
	flag.Parse()
	if err := run(*binary, *evidence); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(binary, evidence string) error {
	if runtime.GOOS != "linux" || binary == "" || evidence == "" {
		return errors.New("requires Linux, --server and --evidence")
	}
	root, err := os.MkdirTemp("", "gcode-linux-plugin-smoke-")
	if err != nil {
		return err
	}
	// Read only the token created by this owned host, never a user's profile.
	var token string
	profile := filepath.Join(root, "profile")
	if err := os.Mkdir(profile, 0700); err != nil {
		return err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	address := listener.Addr().String()
	_ = listener.Close()
	log, err := os.OpenFile(evidence+".log", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer log.Close()
	command := exec.Command(binary, "--data-dir", profile, "--listen", address)
	command.Dir, command.Stdout, command.Stderr = root, log, log
	command.Env = []string{"HOME=" + root, "PATH=/usr/local/bin:/usr/bin:/bin", "LANG=C.UTF-8"}
	if err := command.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	defer func() {
		_ = command.Process.Signal(syscall.SIGTERM)
		select {
		case <-done:
		case <-time.After(12 * time.Second):
			_ = command.Process.Kill()
			<-done
		}
	}()
	client := &http.Client{Timeout: 3 * time.Second}
	var state struct {
		Plugins struct {
			Marketplaces []map[string]any                    `json:"marketplaces"`
			Installed    []map[string]any                    `json:"installed"`
			Operations   []struct{ ID, State, Error string } `json:"operations"`
		} `json:"plugins"`
	}
	request := func(method, path string, body any, authenticate bool, output any) (int, error) {
		var reader io.Reader
		if body != nil {
			data, err := json.Marshal(body)
			if err != nil {
				return 0, err
			}
			reader = bytes.NewReader(data)
		}
		req, err := http.NewRequest(method, "http://"+address+path, reader)
		if err != nil {
			return 0, err
		}
		req.Header.Set("Content-Type", "application/json")
		if authenticate {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response, err := client.Do(req)
		if err != nil {
			return 0, err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return response.StatusCode, nil
		}
		if output != nil {
			return response.StatusCode, json.NewDecoder(response.Body).Decode(output)
		}
		return response.StatusCode, nil
	}
	ready := false
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); {
		if token == "" {
			if data, err := os.ReadFile(filepath.Join(profile, "server.json")); err == nil {
				var config struct{ Token string }
				if json.Unmarshal(data, &config) == nil {
					token = config.Token
				}
			}
		}
		if status, err := request("GET", "/v1/snapshot", nil, true, &state); err == nil && status == 200 {
			ready = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !ready {
		return fmt.Errorf("headless host not ready; see %s.log", evidence)
	}
	unauthorized, err := request("GET", "/v1/snapshot", nil, false, nil)
	if err != nil || unauthorized != http.StatusUnauthorized {
		return fmt.Errorf("authentication boundary: status=%d err=%v", unauthorized, err)
	}
	builtin := map[string]bool{}
	for _, source := range state.Plugins.Marketplaces {
		if id, ok := source["id"].(string); ok && source["builtin"] == true {
			builtin[id] = true
		}
	}
	if !builtin["official"] || !builtin["community"] {
		return errors.New("default plugin sources missing from real remote snapshot")
	}
	market := filepath.Join(root, "market")
	for path, contents := range map[string]string{
		".claude-plugin/marketplace.json":   `{"name":"linux-fixture","owner":{"name":"smoke"},"plugins":[{"name":"sample","source":"./sample"}]}`,
		"sample/.claude-plugin/plugin.json": `{"name":"sample","version":"1.0.0"}`,
		"sample/skills/check/SKILL.md":      "---\ndescription: Linux smoke skill\n---\nRead the fixture.\n",
	} {
		target := filepath.Join(market, path)
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		if err := os.WriteFile(target, []byte(contents), 0600); err != nil {
			return err
		}
	}
	execute := func(input map[string]any, id string) error {
		input["requestId"] = id
		status, err := request("POST", "/v1/commands", input, true, nil)
		if err != nil || status != 200 {
			return fmt.Errorf("%s admission: status=%d err=%v", id, status, err)
		}
		for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); {
			if _, err := request("GET", "/v1/snapshot", nil, true, &state); err != nil {
				return err
			}
			for _, operation := range state.Plugins.Operations {
				if operation.ID == id && operation.State != "running" {
					if operation.State != "completed" {
						return fmt.Errorf("%s: %s", id, operation.Error)
					}
					return nil
				}
			}
			time.Sleep(25 * time.Millisecond)
		}
		return fmt.Errorf("%s did not complete", id)
	}
	steps := []map[string]any{
		{"kind": "plugins.source.save", "plugin": map[string]any{"marketplace": map[string]any{"id": "fixture", "name": "linux-fixture", "kind": "directory", "source": market}}},
		{"kind": "plugins.source.refresh", "id": "fixture"},
		{"kind": "plugins.install", "plugin": map[string]any{"install": map[string]any{"marketplace": "linux-fixture", "name": "sample", "scope": "user"}}},
		{"kind": "plugins.disable", "id": "sample@linux-fixture"},
	}
	for index, input := range steps {
		if err := execute(input, fmt.Sprintf("linux-smoke-%d", index)); err != nil {
			return err
		}
	}
	if len(state.Plugins.Installed) != 1 || state.Plugins.Installed[0]["enabled"] != false {
		return errors.New("actual installation/disable state differs")
	}
	if err := execute(map[string]any{"kind": "plugins.uninstall", "id": "sample@linux-fixture"}, "linux-smoke-remove"); err != nil {
		return err
	}
	if len(state.Plugins.Installed) != 0 {
		return errors.New("actual uninstall did not remove registration")
	}
	data, _ := json.MarshalIndent(map[string]any{
		"platform": runtime.GOOS + "/" + runtime.GOARCH, "passed": true,
		"headless": true, "displayEnvironment": false, "unauthenticatedStatus": unauthorized,
		"defaultSources": []string{"official", "community"},
		"operations":     []string{"source.save", "source.refresh", "install", "disable", "uninstall"},
		"profile":        profile, "officialRuntimePasses": 0,
	}, "", "  ")
	if err := os.WriteFile(evidence, data, 0600); err != nil {
		return err
	}
	fmt.Println(strings.TrimSpace(string(data)))
	return nil
}
