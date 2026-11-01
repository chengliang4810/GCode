package desktop

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

type settingsImportState struct {
	open      bool
	kind      string
	json      string
	id        string
	pendingID string
	errorText string
}

func (a *App) openSettingsImport(kind string) {
	var sample any
	switch kind {
	case "mcp":
		sample = contracts.MCPServerInput{Transport: "stdio", Enabled: true}
	case "command":
		sample = contracts.UserCommand{Enabled: true}
	default:
		return
	}
	data, _ := json.MarshalIndent(sample, "", "  ")
	a.settingsImport = settingsImportState{
		open: true,
		kind: kind,
		id:   kind + "-" + rand.Text(),
		json: string(data),
	}
}

func decodeSettingsImport(text string, value any) error {
	if !strings.HasPrefix(strings.TrimSpace(text), "{") {
		return errors.New("a JSON object is required")
	}
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("exactly one JSON object is required")
	}
	return nil
}

func (s *settingsImportState) command() (contracts.Command, error) {
	switch s.kind {
	case "mcp":
		var value contracts.MCPServerInput
		if err := decodeSettingsImport(s.json, &value); err != nil {
			return contracts.Command{}, err
		}
		value.ID = s.id
		value.Name = strings.TrimSpace(value.Name)
		if value.Name == "" {
			return contracts.Command{}, errors.New("MCP server name is required")
		}
		switch value.Transport {
		case "stdio":
			if strings.TrimSpace(value.Command) == "" || value.URL != "" {
				return contracts.Command{}, errors.New("stdio requires an executable and no URL")
			}
		case "http":
			if strings.TrimSpace(value.URL) == "" || value.Command != "" {
				return contracts.Command{}, errors.New("HTTP requires a URL and no executable")
			}
		default:
			return contracts.Command{}, errors.New("transport must be stdio or http")
		}
		return contracts.Command{
			Kind:     "resource.mcp.save",
			Resource: &contracts.ResourceInput{MCP: &value},
		}, nil
	case "command":
		var value contracts.UserCommand
		if err := decodeSettingsImport(s.json, &value); err != nil {
			return contracts.Command{}, err
		}
		value.ID = s.id
		value.Name = strings.TrimSpace(value.Name)
		if value.Name == "" || strings.TrimSpace(value.Content) == "" {
			return contracts.Command{}, errors.New("command name and prompt are required")
		}
		return contracts.Command{
			Kind:     "resource.command.save",
			Resource: &contracts.ResourceInput{Command: &value},
		}, nil
	default:
		return contracts.Command{}, fmt.Errorf("unsupported import category %q", s.kind)
	}
}

func (a *App) saveSettingsImport() {
	s := &a.settingsImport
	if !s.open || s.pendingID != "" {
		return
	}
	command, err := s.command()
	if err != nil {
		s.errorText = a.t("Invalid configuration") + ": " + err.Error()
		return
	}
	s.errorText = ""
	s.pendingID = s.id
	a.dispatch(command)
}

func (a *App) settingsImportCommandFinished(command contracts.Command, err error) {
	s := &a.settingsImport
	if !s.open || s.pendingID == "" || command.Resource == nil {
		return
	}
	id := ""
	switch command.Kind {
	case "resource.mcp.save":
		if s.kind == "mcp" && command.Resource.MCP != nil {
			id = command.Resource.MCP.ID
		}
	case "resource.command.save":
		if s.kind == "command" && command.Resource.Command != nil {
			id = command.Resource.Command.ID
		}
	}
	if id != s.pendingID {
		return
	}
	s.pendingID = ""
	if err != nil {
		s.errorText = err.Error()
		return
	}
	a.settingsImport = settingsImportState{}
}

func (a *App) settingsImportDialog(c *ui.Context) {
	s := &a.settingsImport
	title := "Import command"
	if s.kind == "mcp" {
		title = "Import MCP server"
	}
	a.settingsDialog(c, "settings-resource-import", title, &s.open, 680, func() {
		ui.Column(c.Key("settings-import-fields")).FillWidth().Gap(12).Children(func() {
			controlTextArea(c.Key("settings-import-json"), &s.json).
				FillWidth().Height(320).Label(tr(c, "Configuration JSON")).
				Disabled(s.pendingID != "")
			if s.errorText != "" {
				ui.Text(c, s.errorText).TextColor(a.palette.Danger)
			}
			ui.Row(c.Key("settings-import-actions")).FillWidth().Justify(ui.End).Gap(8).Children(func() {
				textButton(c, tr(c, "Cancel")).OnClick(func() {
					a.settingsImport = settingsImportState{}
				})
				primaryButton(c, tr(c, "Import")).
					Disabled(s.pendingID != "").OnClick(a.saveSettingsImport)
			})
		})
	})
	if !s.open {
		// An overlay/escape dismissal must not retain pasted credentials.
		a.settingsImport = settingsImportState{}
	}
}
