package desktop

import (
	"context"
	"fmt"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

type cloudSettingsPageState struct {
	active        bool
	waitingStatus bool
	generation    uint64
	busy          string
	error         string
	cancel        context.CancelFunc
}

func (a *App) cloudSettingsVisible() bool {
	return !a.closed && a.route == "settings" && a.section == "Cloud" && a.state.Capabilities.Cloud && a.state.User.SignedIn
}

// The source mounts Cloud once per visit, revalidates its status, and drops
// its pending page work on leaving. Rebuilds and re-picking Cloud keep it.
func (a *App) syncCloudSettingsPage() {
	page := &a.cloudPage
	if !a.cloudSettingsVisible() {
		if page.active {
			if page.cancel != nil {
				page.cancel()
			}
			*page = cloudSettingsPageState{generation: page.generation + 1}
		}
		return
	}
	if page.active {
		a.continueCloudSettingsLoad()
		return
	}
	page.active = true
	page.generation++
	a.runCloudSettingsCommand("cloud.refresh")
}

func (a *App) runCloudSettingsCommand(kind string) {
	page := &a.cloudPage
	if !a.cloudSettingsVisible() || !page.active || page.busy != "" {
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	page.cancel = cancel
	page.busy, page.error = kind, ""
	page.waitingStatus = false
	generation := page.generation
	service := a.service
	command := contracts.Command{Kind: kind}
	finish := func(snapshot contracts.Snapshot, err error) {
		defer cancel()
		if a.closed || ctx.Err() != nil || !a.cloudSettingsVisible() || !a.cloudPage.active || a.cloudPage.generation != generation {
			return
		}
		page.busy, page.cancel = "", nil
		if err != nil {
			if kind == "cloud.refresh" {
				page.error = "Couldn't check Cloud: " + err.Error()
			} else {
				page.error = fmt.Sprintf("Turning on Cloud failed: %v", err)
			}
			return
		}
		a.apply(snapshot)
		if !a.cloudSettingsVisible() {
			a.syncCloudSettingsPage()
			return
		}
		// Only the page's status read may start automatic enablement. An
		// unexpected successful Enable reply that is still Off must settle
		// this attempt rather than recursively issuing another mutation.
		if kind == "cloud.refresh" {
			page.waitingStatus = true
			a.continueCloudSettingsLoad()
		}
	}
	if a.win == nil {
		snapshot, err := cloudSettingsRequest(ctx, service, command)
		finish(snapshot, err)
		return
	}
	win := a.win
	go func() {
		snapshot, err := cloudSettingsRequest(ctx, service, command)
		if ctx.Err() != nil {
			cancel()
			return
		}
		win.Update(func() { finish(snapshot, err) })
	}()
}

// A typed status read may still be loading. Follow that read to its first
// ready/error result; a failed action never arms this continuation again.
func (a *App) continueCloudSettingsLoad() {
	page := &a.cloudPage
	if !page.waitingStatus || page.busy != "" {
		return
	}
	status := a.state.Integrations.Cloud
	if status.LoadState == contracts.LoadLoading {
		return
	}
	page.waitingStatus = false
	if status.LoadState == contracts.LoadReady && status.Available && status.State == "off" {
		a.runCloudSettingsCommand("cloud.enable")
	}
}

func cloudSettingsRequest(ctx context.Context, service contracts.Service, command contracts.Command) (contracts.Snapshot, error) {
	if err := service.Execute(ctx, command); err != nil {
		return contracts.Snapshot{}, err
	}
	return service.Snapshot(ctx)
}

func (a *App) cloudSettingsError(c *ui.Context, message, kind string) {
	ui.Row(c).FillWidth().Padding(12, 16).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Text(c, tr(c, message)).Grow(1).FontSize(design.UISize(c, 12)).TextColor(a.palette.Danger)
		textButton(c, tr(c, "Retry")).Disabled(a.cloudPage.busy != "").OnClick(func() { a.runCloudSettingsCommand(kind) })
	})
}
