package desktop

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// A provider configuration travels as one JSON object: the exported provider
// itself. Importing it always mints a new provider ID, so a file can be
// imported twice without overwriting the configuration it came from.
const providerJSONSample = `{
  "name": "Example provider",
  "protocol": "openai-chat",
  "baseUrl": "https://api.example.com/v1",
  "apiKey": "",
  "models": [
    { "id": "example-model" }
  ]
}`

// The exported JSON is a small text file; refuse absurd imports instead of
// copying an unbounded document into the dialog.
const providerJSONMaxBytes = 1 << 20

type providerImportState struct {
	open      bool
	text      string
	id        string
	pendingID string
	errorText string
	loading   bool
}

type providerExportState struct {
	open       bool
	providerID string
	json       string
	errorText  string
	loading    bool
	request    uint64
}

func (a *App) openProviderImport() {
	a.providerImport = providerImportState{
		open: true,
		id:   "provider-" + rand.Text(),
		text: providerJSONSample,
	}
}

func (a *App) dismissProviderImport() {
	a.providerImport = providerImportState{}
}

// command validates the pasted JSON as a provider configuration. The ID in the
// document is ignored: the import owns the new identity.
func (s *providerImportState) command() (contracts.Command, error) {
	var provider model.Provider
	if err := decodeSettingsImport(s.text, &provider); err != nil {
		return contracts.Command{}, err
	}
	provider = model.CloneProvider(provider)
	provider.ID = s.id
	provider.Name = strings.TrimSpace(provider.Name)
	provider.BaseURL = strings.TrimSpace(provider.BaseURL)
	provider.APIKey = strings.TrimSpace(provider.APIKey)
	if provider.Protocol == "" {
		provider.Protocol = model.Chat
	}
	if provider.Name == "" {
		return contracts.Command{}, errors.New("provider name is required")
	}
	if err := provider.Validate(); err != nil {
		return contracts.Command{}, err
	}
	return contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider}}, nil
}

func (a *App) saveProviderImport() {
	s := &a.providerImport
	if !s.open || s.loading || s.pendingID != "" {
		return
	}
	command, err := s.command()
	if err != nil {
		s.errorText = a.t("Invalid configuration") + ": " + err.Error()
		return
	}
	s.errorText = ""
	s.pendingID = s.id
	command.ID = "provider-import-" + rand.Text()
	a.dispatch(command)
}

func (a *App) providerImportCommandFinished(command contracts.Command, err error) {
	s := &a.providerImport
	if command.Kind != "provider.save" || command.Provider == nil || s.pendingID == "" ||
		command.Provider.Provider.ID != s.pendingID {
		return
	}
	s.pendingID = ""
	if err != nil {
		s.errorText = err.Error()
		return
	}
	a.modelsUI.selected = s.id
	a.dismissProviderImport()
	a.notification(toastSuccess, a.t("Provider imported"))
}

// chooseProviderJSON reads a JSON document into the import dialog, keeping the
// paste path and the file path on the same validation.
func (a *App) chooseProviderJSON() {
	s := &a.providerImport
	if !s.open || s.loading {
		return
	}
	s.loading, s.errorText = true, ""
	window, choose, title := a.win, a.chooseProviderImportPath, a.t("Choose provider JSON")
	a.backgroundTask(func() func() {
		var data []byte
		var err error
		if choose != nil {
			var path string
			if path, err = choose(); err == nil && path != "" {
				data, err = os.ReadFile(path)
			}
		} else if window != nil {
			paths, openErr := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: window, Title: title,
				Filters: []mygo.FileFilter{{Name: "JSON", Extensions: []string{"json"}}}})
			if err = openErr; err == nil && len(paths) > 0 {
				data, err = os.ReadFile(paths[0])
			}
		} else {
			err = errors.New("no file dialog is available")
		}
		return func() {
			s.loading = false
			if err != nil {
				s.errorText = err.Error()
				return
			}
			if len(data) > providerJSONMaxBytes {
				s.errorText = a.t("The file is too large for a provider configuration")
				return
			}
			if len(data) > 0 {
				s.text = string(data)
			}
		}
	})
}

func (a *App) providerImportDialog(c *ui.Context) {
	s := &a.providerImport
	a.settingsDialog(c, "settings-provider-import", "Import provider", &s.open, 680, func() {
		ui.Column(c.Key("provider-import-fields")).FillWidth().Gap(12).Children(func() {
			controlTextArea(c.Key("provider-import-json"), &s.text).
				FillWidth().Height(320).Label(tr(c, "Provider JSON")).
				Disabled(s.loading || s.pendingID != "")
			ui.Text(c, tr(c, "A new provider ID is assigned on import; the API key is stored encrypted.")).
				TextColor(a.palette.Muted)
			if s.errorText != "" {
				ui.Text(c, s.errorText).TextColor(a.palette.Readable(a.palette.Danger))
			}
			ui.Row(c.Key("provider-import-actions")).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
				buttonWithIcon(c.Key("provider-import-file"), tr(c, "Choose file"), "document", buttonSecondary).
					Disabled(s.loading || s.pendingID != "").OnClick(a.chooseProviderJSON)
				ui.Spacer(c).Grow(1)
				textButton(c, tr(c, "Cancel")).OnClick(a.dismissProviderImport)
				primaryButton(c, tr(c, "Import")).
					Disabled(s.loading || s.pendingID != "").OnClick(a.saveProviderImport)
			})
		})
	})
	if !s.open {
		// An overlay/escape dismissal must not retain pasted credentials.
		a.dismissProviderImport()
	}
}

func providerExportJSON(provider model.Provider) (string, error) {
	data, err := json.MarshalIndent(model.CloneProvider(provider), "", "  ")
	if err != nil {
		return "", err
	}
	return string(data) + "\n", nil
}

// exportProvider reveals the saved credential for this explicit export, the
// same single-purpose reveal the API key field uses. The document therefore
// carries the key in plain text, which the dialog states before saving.
func (a *App) exportProvider(id string) {
	state := &a.providerExport
	var provider model.Provider
	for _, config := range a.state.CustomProviders {
		if config.Provider.ID == id {
			provider = model.CloneProvider(config.Provider)
		}
	}
	if provider.ID == "" {
		a.notification(toastError, a.t("The provider no longer exists."))
		return
	}
	state.request++
	*state = providerExportState{open: true, providerID: id, loading: true, request: state.request}
	request := state.request
	service, ok := a.service.(contracts.ProviderKeyService)
	if !ok {
		a.finishProviderExport(provider, request)
		return
	}
	a.backgroundTask(func() func() {
		key, err := service.RevealProviderKey(a.ctx, id)
		return func() {
			if state.request != request || state.providerID != id || !state.open {
				return
			}
			if err != nil {
				state.loading, state.errorText = false, err.Error()
				return
			}
			provider.APIKey = key
			a.finishProviderExport(provider, request)
		}
	})
}

func (a *App) finishProviderExport(provider model.Provider, request uint64) {
	state := &a.providerExport
	if state.request != request {
		return
	}
	text, err := providerExportJSON(provider)
	state.loading = false
	if err != nil {
		state.errorText = err.Error()
		return
	}
	state.json = text
}

func (a *App) dismissProviderExport() {
	a.providerExport.request++
	a.providerExport = providerExportState{request: a.providerExport.request}
}

func (a *App) saveProviderExportFile() {
	state := &a.providerExport
	if state.loading || state.json == "" {
		return
	}
	content := []byte(state.json)
	name := providerExportFileName(a.providerName(state.providerID))
	window, choose, title := a.win, a.chooseProviderExportPath, a.t("Export provider as JSON")
	a.backgroundTask(func() func() {
		var path string
		var err error
		if choose != nil {
			path, err = choose()
		} else if window != nil {
			path, err = mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: window, Title: title, DefaultPath: name,
				Filters: []mygo.FileFilter{{Name: "JSON", Extensions: []string{"json"}}}})
		}
		if err == nil && path != "" {
			err = os.WriteFile(path, content, 0600)
		}
		return func() {
			if err != nil {
				state.errorText = err.Error()
			} else if path != "" {
				a.notification(toastSuccess, a.t("Provider exported"))
			}
		}
	})
}

func (a *App) providerExportDialog(c *ui.Context) {
	state := &a.providerExport
	a.settingsDialog(c, "settings-provider-export", "Export provider", &state.open, 680, func() {
		ui.Column(c.Key("provider-export-fields")).FillWidth().Gap(12).Children(func() {
			if state.loading {
				ui.Text(c, tr(c, "Reading the API key…")).TextColor(a.palette.Muted)
			} else {
				controlTextArea(c.Key("provider-export-json"), &state.json).
					FillWidth().Height(320).Label(tr(c, "Provider JSON"))
			}
			ui.Text(c, tr(c, "This JSON contains the API key in plain text. Keep the file private.")).
				TextColor(a.palette.Muted)
			if state.errorText != "" {
				ui.Text(c, state.errorText).TextColor(a.palette.Readable(a.palette.Danger))
			}
			ui.Row(c.Key("provider-export-actions")).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
				buttonWithIcon(c.Key("provider-export-file"), tr(c, "Save as file"), "floppy-disk", buttonSecondary).
					Disabled(state.loading || state.json == "").OnClick(a.saveProviderExportFile)
				ui.Spacer(c).Grow(1)
				textButton(c, tr(c, "Close")).OnClick(a.dismissProviderExport)
			})
		})
	})
	if !state.open {
		a.dismissProviderExport()
	}
}

func (a *App) providerName(id string) string {
	for _, config := range a.state.CustomProviders {
		if config.Provider.ID == id {
			return config.Provider.Name
		}
	}
	return ""
}

// providerExportFileName turns a provider name into a portable file name.
func providerExportFileName(name string) string {
	var slug strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			slug.WriteRune(r)
		default:
			slug.WriteByte('-')
		}
	}
	value := strings.Trim(strings.Join(strings.FieldsFunc(slug.String(), func(r rune) bool { return r == '-' }), "-"), "-")
	if value == "" {
		value = "provider"
	}
	return value + ".json"
}
