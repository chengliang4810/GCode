package engine

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/privacy"
	"gcode/third_party/magpie/pii"
)

// bindModelLocked applies host-owned policy to every model consumer, including
// naming, compaction, hook models, subagents and provider checks.
func (e *Engine) bindModelLocked(provider model.Provider, selected model.Model) (*model.Gateway, error) {
	gateway, err := e.client.Bind(provider, selected)
	if err != nil {
		return nil, err
	}
	gateway.SetPrivacy(e.state.Settings.Privacy)
	if e.state.Settings.General.RecordModelWire {
		directory, err := e.storage.Resolve(e.storage.ActiveRootID())
		if err != nil {
			return nil, fmt.Errorf("resolve model wire capture data directory: %w", err)
		}
		gateway.WireCaptureDirectory = filepath.Join(directory, "model-wire")
	}
	language := e.state.Settings.General.Language
	notify := e.state.Settings.Privacy.Notify
	gateway.PrivacyReport = func(report privacy.Report) {
		if report.Blocked > 0 || report.Warned == 0 && (!notify || report.Masked == 0) {
			return
		}
		text := fmt.Sprintf("Privacy: masked %d values; allowed %d unchanged under warning rules.", report.Masked, report.Warned)
		if language == "zh-CN" {
			text = fmt.Sprintf("脱敏提醒：已替换 %d 项敏感信息，按警告策略允许原样发送 %d 项。", report.Masked, report.Warned)
		}
		e.mu.Lock()
		defer e.mu.Unlock()
		e.publishLocked(contracts.Event{Kind: "privacy.warning", Text: text})
	}
	return gateway, nil
}

func (e *Engine) refreshPrivacyModelsLocked() {
	var statuses []privacy.ModelStatus
	cacheErr := pii.PrepareCache()
	for _, m := range pii.Models() {
		if cacheErr != nil {
			statuses = append(statuses, privacy.ModelStatus{Model: m.ID(), Size: m.Size(), Error: cacheErr.Error()})
			continue
		}
		statuses = append(statuses, m.Now())
	}
	e.state.PrivacyModels = statuses
}

func (e *Engine) privacyModelCommandLocked(command contracts.Command) error {
	if err := pii.PrepareCache(); err != nil {
		return err
	}
	m := pii.Of(command.ID)
	action := strings.TrimPrefix(command.Kind, "privacy.model.")
	if action == "cancel" {
		m.Cancel()
		return nil
	}
	if e.privacyModelJobs[command.ID] {
		return errors.New("a privacy model operation is already in progress")
	}
	if action == "remove" && e.state.Settings.Privacy.Enabled && e.state.Settings.Privacy.LocalModel &&
		pii.Of(e.state.Settings.Privacy.ModelName).ID() == command.ID {
		return errors.New("turn off the local privacy model before removing it")
	}
	if action == "download" {
		if err := m.Download(nil); err != nil {
			return err
		}
	}
	if e.privacyModelJobs == nil {
		e.privacyModelJobs = map[string]bool{}
	}
	e.privacyModelJobs[command.ID] = true
	e.refreshPrivacyModelsLocked()
	e.publishLocked(contracts.Event{Kind: "privacy.models.changed"})
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		if action == "remove" {
			err := m.Remove()
			e.mu.Lock()
			defer e.mu.Unlock()
			delete(e.privacyModelJobs, command.ID)
			e.refreshPrivacyModelsLocked()
			if !e.closed {
				text := ""
				if err != nil {
					text = err.Error()
				}
				e.publishLocked(contracts.Event{Kind: "privacy.models.changed", Text: text})
			}
			return
		}
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-e.done:
				m.Cancel()
				return
			case <-ticker.C:
				status := m.Now()
				e.mu.Lock()
				if !status.Downloading {
					delete(e.privacyModelJobs, command.ID)
				}
				if !e.closed {
					e.refreshPrivacyModelsLocked()
					e.publishLocked(contracts.Event{Kind: "privacy.models.changed", Text: status.Error})
				}
				e.mu.Unlock()
				if !status.Downloading {
					return
				}
			}
		}
	}()
	return nil
}
