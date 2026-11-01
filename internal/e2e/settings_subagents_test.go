//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"gcode/internal/contracts"
)

// This exercises only custom profiles through a native window and the real
// local core. It owns its data and processes and makes no model requests.
func TestDesktopLocalCustomSubagentSettings(t *testing.T) {
	binary, artifacts := testPaths(t)
	data := filepath.Join(artifacts, "data")
	const name = "Native custom investigator"
	const description = "Investigate the current checkout"
	const prompt = "Read the relevant source and report verified findings."
	waitSavedFields := func(app *nativeApp) {
		app.t.Helper()
		app.await("saved custom profile in the editor", func() error {
			tree, err := app.client.tree()
			if err != nil {
				return err
			}
			for _, field := range []struct{ key, value string }{
				{"subagent-name", name},
				{"subagent-description", description},
				{"subagent-prompt", prompt},
			} {
				node, err := tree.findKey(field.key)
				if err != nil {
					return err
				}
				if node.Value != field.value {
					return fmt.Errorf("%s=%q, want %q", field.key, node.Value, field.value)
				}
			}
			return nil
		})
	}
	readProfiles := func() ([]contracts.SubagentProfile, error) {
		payload, err := os.ReadFile(filepath.Join(data, "engine.json"))
		if err != nil {
			return nil, err
		}
		var disk struct {
			Snapshot contracts.Snapshot `json:"snapshot"`
		}
		if err := json.Unmarshal(payload, &disk); err != nil {
			return nil, err
		}
		return disk.Snapshot.Settings.Subagents, nil
	}
	app := startApp(t, binary, artifacts, "create", data, "--local")
	app.act("settings-nav-Subagents", "press", "")
	app.act("add-subagent", "press", "")
	app.act("subagent-name", "setValue", name)
	app.act("subagent-description", "setValue", description)
	app.act("subagent-prompt", "setValue", prompt)
	waitSavedFields(app)
	// Public values can update before the requested paint. This synchronizes
	// the capture only; persisted business state is awaited separately below.
	if err := app.client.call("idle", map[string]int{"timeout": 2000}, nil); err != nil {
		t.Fatal(err)
	}
	app.capture("custom-form")
	app.act("save-subagent", "press", "")

	var saved contracts.SubagentProfile
	app.await("custom subagent persisted by the local core", func() error {
		profiles, err := readProfiles()
		if err != nil {
			return err
		}
		index := slices.IndexFunc(profiles, func(profile contracts.SubagentProfile) bool {
			return profile.Name == name
		})
		if index < 0 {
			return fmt.Errorf("custom subagent has not been persisted")
		}
		saved = profiles[index]
		if saved.ID == "" || contracts.IsBuiltinSubagent(saved.ID) ||
			saved.Description != description || saved.Prompt != prompt ||
			saved.Disabled || saved.Model != nil || len(saved.Tools) != 0 {
			return fmt.Errorf("unexpected persisted custom profile: %+v", saved)
		}
		return nil
	})
	waitList := func(app *nativeApp) {
		app.t.Helper()
		app.waitKey("add-subagent")
		app.waitKey("subagent-row-" + saved.ID)
		app.await("editor closed after a successful action", func() error {
			tree, err := app.client.tree()
			if err != nil {
				return err
			}
			if _, err := tree.findKey("save-subagent"); err == nil {
				return fmt.Errorf("custom subagent editor is still open")
			}
			return nil
		})
	}
	waitList(app)
	app.capture("saved-list")
	app.act("subagent-edit-"+saved.ID, "press", "")
	waitSavedFields(app)
	app.act("subagent-name", "setValue", "Unsaved draft")
	app.act("subagent-description", "setValue", "Unsaved description")
	app.act("subagent-prompt", "setValue", "Unsaved prompt")
	app.act("cancel-subagent", "press", "")
	waitList(app)
	app.act("subagent-edit-"+saved.ID, "press", "")
	waitSavedFields(app)
	app.act("cancel-subagent", "press", "")
	waitList(app)
	app.act("subagent-enabled-"+saved.ID, "press", "")
	app.waitChecked("subagent-enabled-"+saved.ID, false)
	app.await("disabled custom profile persisted without unsaved edits", func() error {
		profiles, err := readProfiles()
		if err != nil {
			return err
		}
		for _, profile := range profiles {
			if profile.ID == saved.ID && profile.Name == name &&
				profile.Description == description && profile.Prompt == prompt && profile.Disabled {
				return nil
			}
		}
		return fmt.Errorf("saved custom profile was changed by cancel or its disabled state is not persisted")
	})
	app.capture("disabled-list")
	app.stop()

	restarted := startApp(t, binary, artifacts, "restart", data, "--local")
	restarted.act("settings-nav-Subagents", "press", "")
	waitList(restarted)
	restarted.waitChecked("subagent-enabled-"+saved.ID, false)
	restarted.act("subagent-edit-"+saved.ID, "press", "")
	waitSavedFields(restarted)
	restarted.capture("restored-editor")
	restarted.act("cancel-subagent", "press", "")

	restarted.act("subagent-delete-"+saved.ID, "press", "")
	restarted.waitKey("settings-remove-confirm-action")
	restarted.act("settings-remove-cancel", "press", "")
	waitList(restarted)
	restarted.act("subagent-edit-"+saved.ID, "press", "")
	restarted.act("delete-subagent", "press", "")
	restarted.act("settings-remove-confirm-action", "press", "")
	restarted.await("confirmed custom profile removal persisted", func() error {
		profiles, err := readProfiles()
		if err != nil {
			return err
		}
		if slices.ContainsFunc(profiles, func(profile contracts.SubagentProfile) bool {
			return profile.ID == saved.ID
		}) {
			return fmt.Errorf("removed custom profile is still persisted")
		}
		return nil
	})
	restarted.waitKey("add-subagent")
	restarted.await("removed custom row is absent", func() error {
		tree, err := restarted.client.tree()
		if err != nil {
			return err
		}
		if _, err := tree.findKey("subagent-row-" + saved.ID); err == nil {
			return fmt.Errorf("removed custom profile is still listed")
		}
		return nil
	})
	restarted.capture("after-delete")
}
