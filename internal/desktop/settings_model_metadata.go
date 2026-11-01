package desktop

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/model"
	"gcode/internal/modelcatalog"
	"github.com/egoist/mygo/ui"
)

func (a *App) modelMetadataFields(c *ui.Context) {
	draft := &a.modelsUI.draft
	ui.Column(c.Key("model-metadata-fields")).FillWidth().Gap(16).Children(func() {
		a.modelMetadataField(c, "Model ID", func() {
			controlTextInput(c.Key("id"), &draft.model.ID).FillWidth().MinWidth(0).
				Label(tr(c, "Model ID")).Placeholder(tr(c, "Model ID")).OnChange(a.lookupModelMetadata)
		})
		a.modelMetadataField(c, "Context window", func() {
			controlTextInput(c.Key("context"), &draft.context).FillWidth().MinWidth(0).
				Label(tr(c, "Context window")).OnChange(a.cancelModelMetadataLookup)
		})
		a.modelMetadataField(c, "Maximum output tokens", func() {
			controlTextInput(c.Key("output"), &draft.output).FillWidth().MinWidth(0).
				Label(tr(c, "Maximum output tokens")).OnChange(a.cancelModelMetadataLookup)
		})
		a.modelMetadataField(c, "Input types", func() {
			ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
				for _, item := range []struct{ id, label string }{
					{"text", "Text"}, {"image", "Image"}, {"audio", "Audio"}, {"video", "Video"}, {"pdf", "PDF"},
				} {
					checked := item.id == "text" || slices.Contains(draft.model.InputTypes, item.id)
					controlCheckboxOption(c.Key("model-input:"+item.id), &checked, tr(c, item.label)).
						Disabled(item.id == "text").OnClick(func() {
						a.cancelModelMetadataLookup()
						draft.model.InputTypes = modelMetadataSelection(draft.model.InputTypes, item.id, checked, []string{"text", "image", "audio", "video", "pdf"})
					})
				}
			})
		})
		a.modelMetadataField(c, "Reasoning levels", func() {
			ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
				for _, level := range configurableReasoningLevels {
					checked := slices.Contains(draft.model.ReasoningLevels, level)
					controlCheckboxOption(c.Key("model-reasoning:"+level), &checked, tr(c, model.ReasoningLabel(level))).
						Label(fmt.Sprintf(a.t("Model reasoning %s"), model.ReasoningLabel(level))).
						OnClick(func() {
							a.cancelModelMetadataLookup()
							draft.model.ReasoningLevels = modelMetadataSelection(draft.model.ReasoningLevels, level, checked, configurableReasoningLevels)
						})
				}
			})
		})
	})
}

func (a *App) modelMetadataField(c *ui.Context, label string, draw func()) {
	ui.Column(c).FillWidth().Gap(8).Children(func() {
		ui.Text(c, tr(c, label)).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted)
		draw()
	})
}

func modelMetadataSelection(current []string, id string, enabled bool, order []string) []string {
	result := []string{}
	for _, candidate := range order {
		if (candidate == id && enabled) || (candidate != id && slices.Contains(current, candidate)) {
			result = append(result, candidate)
		}
	}
	return result
}

func (a *App) cancelModelMetadataLookup() {
	draft := &a.modelsUI.draft
	draft.lookup++
	draft.loading = false
}

func (a *App) lookupModelMetadata() {
	draft := &a.modelsUI.draft
	draft.lookup++
	draft.loading = false
	if strings.TrimSpace(draft.model.ID) == "" {
		return
	}
	service, ok := a.service.(contracts.ModelMetadataService)
	if !ok {
		return
	}
	query := modelcatalog.Query{ID: strings.TrimSpace(draft.model.ID)}
	var protocol model.Protocol
	for _, config := range a.state.CustomProviders {
		if config.Provider.ID == a.modelsUI.providerID {
			query.BaseURL, protocol = config.Provider.BaseURL, config.Provider.Protocol
		}
	}
	revision, key := draft.lookup, draft.key
	draft.loading = true
	window, ctx := a.win, a.ctx
	a.backgroundTask(func() func() {
		if window != nil {
			timer := time.NewTimer(250 * time.Millisecond)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return func() {}
			case <-timer.C:
			}
		}
		result, err := service.LookupModelMetadata(ctx, query)
		return func() {
			current := &a.modelsUI.draft
			if !a.modelsUI.modelOpen || current.key != key || current.lookup != revision {
				return
			}
			current.loading = false
			if err == nil && result.Found {
				if result.ContextWindow > 0 {
					current.context = strconv.Itoa(result.ContextWindow)
				}
				if result.MaxOutput > 0 {
					current.output = strconv.Itoa(result.MaxOutput)
				}
				current.model.InputTypes = slices.Clone(result.InputTypes)
				current.model.ReasoningLevels = slices.Clone(result.ReasoningLevels)
				if protocol == model.Messages {
					current.model.ReasoningLevels = slices.DeleteFunc(current.model.ReasoningLevels, func(s string) bool { return s == "minimal" })
				}
			}
		}
	})
}
