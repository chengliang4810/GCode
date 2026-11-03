package desktop

import (
	"context"
	"errors"
	"strings"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/providerlogo"
	"github.com/egoist/mygo/ui"
)

func (a *App) openProviderLogo() {
	a.dismissProviderLogo()
	state := &a.modelsUI
	state.logoOpen, state.logoProvider = true, state.inlineID
}

func (a *App) dismissProviderLogo() {
	state := &a.modelsUI
	if state.logoCancel != nil {
		state.logoCancel()
	}
	state.logoCancel = nil
	state.logoOpen, state.logoLoading = false, false
	state.logoProvider, state.logoError = "", ""
	state.logoRequest++
}

func (a *App) chooseProviderLogo(logo string) {
	a.runtimeUI.provider.Logo = logo
	a.markInlineProviderDirty()
	a.saveInlineProvider()
	a.dismissProviderLogo()
}

func (a *App) fetchProviderLogo() {
	state := &a.modelsUI
	service, ok := a.service.(contracts.ProviderLogoService)
	if !ok {
		state.logoError = a.t("Website logo fetching is unavailable.")
		return
	}
	baseURL := strings.TrimSpace(a.runtimeUI.provider.BaseURL)
	previousLogo := a.runtimeUI.provider.Logo
	providerID := state.inlineID
	state.logoRequest++
	request := state.logoRequest
	state.logoLoading, state.logoError = true, ""
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	state.logoCancel = cancel
	a.backgroundTask(func() func() {
		defer cancel()
		logo, err := service.FetchProviderLogo(ctx, baseURL)
		// Decode away from painting before replacing the current selection.
		if err == nil {
			image := loadMedia(logo)
			if image.svg == nil && image.bitmap == nil {
				err = errors.New("The website logo cannot be displayed.")
			}
		}
		return func() {
			current := &a.modelsUI
			if !current.logoOpen || current.logoRequest != request ||
				current.inlineID != providerID || current.selected != providerID {
				return
			}
			if a.route != "settings" || a.section != "Model settings" {
				a.dismissProviderLogo()
				return
			}
			current.logoLoading, current.logoCancel = false, nil
			if strings.TrimSpace(a.runtimeUI.provider.BaseURL) != baseURL || a.runtimeUI.provider.Logo != previousLogo {
				current.logoError = a.t("The provider changed while fetching its logo. Please retry.")
				return
			}
			if err != nil {
				current.logoError = a.t(err.Error())
				return
			}
			a.chooseProviderLogo(logo)
		}
	})
}

func (a *App) providerLogoDialog(c *ui.Context) {
	state := &a.modelsUI
	if !state.logoOpen {
		if state.logoCancel != nil {
			a.dismissProviderLogo()
		}
		return
	}
	if state.logoProvider != state.inlineID || state.selected != state.inlineID {
		a.dismissProviderLogo()
		return
	}
	a.settingsDialogWithFooter(c, "provider-logo-dialog", "Provider logo", &state.logoOpen, 480, func() {
		ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
			for _, brand := range providerlogo.Brands() {
				controlProviderLogoChoice(c.Key("provider-logo:"+brand.ID), brand.Name, a.runtimeUI.provider.Logo == brand.ID).
					Disabled(state.logoLoading || a.modelSettingsBusy()).
					Children(func() {
						providerLogo(c, brand.ID, 20)
					}).OnClick(func() { a.chooseProviderLogo(brand.ID) })
			}
		})
		if state.logoLoading {
			ui.Text(c.Key("provider-logo-loading"), tr(c, "Fetching website logo…")).FillWidth()
		}
		if state.logoError != "" {
			ui.Text(c.Key("provider-logo-error"), state.logoError).FillWidth().TextColor(a.palette.Danger)
		}
	}, func() {
		ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
			secondaryButton(c.Key("provider-logo-fetch"), tr(c, "Fetch from website")).
				Disabled(state.logoLoading || a.modelSettingsBusy() || strings.TrimSpace(a.runtimeUI.provider.BaseURL) == "").
				OnClick(a.fetchProviderLogo)
			textButton(c.Key("provider-logo-default"), tr(c, "Use default icon")).
				Disabled(state.logoLoading || a.modelSettingsBusy()).OnClick(func() { a.chooseProviderLogo("") })
			textButton(c.Key("provider-logo-cancel"), tr(c, "Cancel")).OnClick(a.dismissProviderLogo)
		})
	})
	if !state.logoOpen {
		a.dismissProviderLogo()
	}
}
