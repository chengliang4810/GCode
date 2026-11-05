package contracts

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"gcode/internal/model"
)

// Clone freezes a command before it crosses a goroutine/queue boundary.
func (c Command) Clone() Command {
	c.QuestionAnswers = CloneQuestionAnswers(c.QuestionAnswers)
	c.Plugin = c.Plugin.Clone()
	c.Resource = c.Resource.Clone()
	c.Goal = copyPointer(c.Goal)
	c.GoalEdit = copyPointer(c.GoalEdit)
	if c.Goal != nil {
		c.Goal.TokenBudget = copyPointer(c.Goal.TokenBudget)
		c.Goal.References = slices.Clone(c.Goal.References)
	}
	c.Provider = copyPointer(c.Provider)
	c.Reorder = copyPointer(c.Reorder)
	if c.Provider != nil {
		c.Provider.Provider = model.CloneProvider(c.Provider.Provider)
	}
	c.ProjectCreate = copyPointer(c.ProjectCreate)
	c.Integration = copyPointer(c.Integration)
	c.Git = copyPointer(c.Git)
	c.Adjustment = copyPointer(c.Adjustment)
	if c.Settings != nil {
		v := *c.Settings
		v.Shortcuts = slices.Clone(v.Shortcuts)
		v.Subagents = CloneSubagentProfiles(v.Subagents)
		v.Hooks = CloneHookRules(v.Hooks)
		v.ModelPicker.Options = maps.Clone(v.ModelPicker.Options)
		v.ModelPicker.Selections = maps.Clone(v.ModelPicker.Selections)
		c.Settings = &v
	}
	if c.Send != nil {
		v := *c.Send
		v.Checkout = copyPointer(v.Checkout)
		v.Annotations = slices.Clone(v.Annotations)
		for i := range v.Annotations {
			v.Annotations[i] = v.Annotations[i].Bounded()
		}
		v.Attachments = slices.Clone(v.Attachments)
		c.Send = &v
	}
	if c.Session != nil {
		v := *c.Session
		v.Title = copyPointer(v.Title)
		v.Pinned = copyPointer(v.Pinned)
		v.Archived = copyPointer(v.Archived)
		v.Unread = copyPointer(v.Unread)
		v.PinOrder = copyPointer(v.PinOrder)
		v.GroupID = copyPointer(v.GroupID)
		v.GroupOrder = copyPointer(v.GroupOrder)
		v.ModelID = copyPointer(v.ModelID)
		v.ReasoningLevel = copyPointer(v.ReasoningLevel)
		v.Branch = copyPointer(v.Branch)
		v.DeviceID = copyPointer(v.DeviceID)
		v.WorktreeID = copyPointer(v.WorktreeID)
		c.Session = &v
	}
	c.Attachment = copyPointer(c.Attachment)
	c.TerminalSize = copyPointer(c.TerminalSize)
	return c
}
func copyPointer[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

// Validate rejects ambiguous tagged payloads before any state is mutated.
func (c Command) Validate() error {
	if c.QuestionAnswers != nil && (c.Kind != "request.answer" || c.Text != "") {
		return errors.New("question answers require a request.answer command without a text payload")
	}
	if err := c.validatePlugins(); err != nil {
		return err
	}
	if strings.HasPrefix(c.Kind, "plugins.") {
		return nil
	}
	isReorder := IsReorderCommand(c.Kind)
	if c.Reorder != nil && !isReorder {
		return errors.New("reorder payload does not match command")
	}
	if isReorder {
		if c.Reorder == nil {
			return errors.New("reorder payload is required")
		}
		if c.Resource != nil || c.Goal != nil || c.GoalEdit != nil || c.Provider != nil || c.ProjectCreate != nil ||
			c.Integration != nil || c.Git != nil || c.Adjustment != nil ||
			c.Settings != nil || c.Send != nil || c.Session != nil || c.Attachment != nil ||
			c.TerminalSize != nil {
			return errors.New("reorder command requires only its typed reorder payload")
		}
		return c.Reorder.Validate(c.Kind)
	}
	if err := c.validateResource(); err != nil {
		return err
	}
	if strings.HasPrefix(c.Kind, "resource.") {
		return nil
	}
	if c.Goal != nil && c.Kind != "goal.set" && c.Kind != "goal.replace" {
		return errors.New("goal payload does not match command")
	}
	if c.GoalEdit != nil && c.Kind != "goal.edit" {
		return errors.New("goal edit payload does not match command")
	}
	switch c.Kind {
	case "goal.edit":
		if c.SessionID == "" || c.ID == "" || c.GoalEdit == nil {
			return errors.New("a session, goal ID and edit payload are required")
		}
		return c.GoalEdit.Validate()
	case "goal.set", "goal.replace":
		if c.SessionID == "" || c.Goal == nil {
			return errors.New("a session and goal payload are required")
		}
		return c.Goal.Validate()
	case "goal.pause", "goal.resume", "goal.clear":
		if c.SessionID == "" || c.ID == "" {
			return errors.New("a session and goal ID are required")
		}
		return nil
	}
	if c.Provider != nil && c.Kind != "provider.save" {
		return errors.New("provider configuration does not match command")
	}
	switch c.Kind {
	case "provider.save":
		if c.Provider == nil {
			return errors.New("provider configuration is required")
		}
		return c.Provider.Provider.Validate()
	case "provider.delete", "provider.test", "provider.models.fetch":
		if c.ID == "" {
			return errors.New("provider ID is required")
		}
		return nil
	case "storage.change":
		if c.Path == "" {
			return errors.New("data directory is required")
		}
		return nil
	}
	if c.RequestID != "" && c.Kind != "session.sidechat" && c.Kind != "session.fork" && c.Kind != "session.create" {
		return errors.New("conversation request ID does not match command")
	}
	if c.Git != nil && !IsComposerGitCommand(c.Kind) {
		return errors.New("git payload does not match command")
	}
	if IsComposerGitCommand(c.Kind) {
		if c.Git == nil || c.Integration != nil || c.Send != nil || c.Session != nil || c.Settings != nil || c.Attachment != nil || c.ProjectCreate != nil || c.Adjustment != nil || c.TerminalSize != nil {
			return errors.New("git command requires only its typed git payload")
		}
		return c.Git.Validate(c.Kind)
	}
	if c.Send != nil && c.Send.Checkout != nil {
		if err := c.Send.Checkout.Validate(); err != nil {
			return err
		}
	}
	if c.ProjectCreate != nil && c.Kind != "project.create" {
		return errors.New("project create payload does not match command")
	}
	if IsIntegrationCommand(c.Kind) {
		return c.validateIntegration()
	}
	if c.Integration != nil {
		return errors.New("integration payload does not match command")
	}
	if c.Adjustment != nil && c.Kind != "background.adjust" {
		return errors.New("background adjustment does not match command")
	}
	if c.Send != nil && c.Kind != "message.send" {
		return errors.New("send payload does not match command")
	}
	if c.Settings != nil && c.Kind != "settings.update" {
		return errors.New("settings payload does not match command")
	}
	if c.Session != nil && c.Kind != "session.update" {
		return errors.New("session payload does not match command")
	}
	if c.Attachment != nil && c.Kind != "attachment.add" {
		return errors.New("attachment payload does not match command")
	}
	if c.TerminalSize != nil && c.Kind != "terminal.resize" {
		return errors.New("terminal size payload does not match command")
	}
	switch c.Kind {
	case "project.files.refresh", "project.delete":
		if c.ID == "" {
			return errors.New("project ID is required")
		}
		return nil
	case "catalog.retry":
		return nil
	case "background.install":
		if c.Path == "" {
			return errors.New("image path is required")
		}
		return nil
	case "background.remove":
		return nil
	case "background.adjust":
		if c.Path == "" || c.Adjustment == nil {
			return errors.New("current background and adjustment are required")
		}
		return nil
	case "appshots.permissions.check":
		return nil
	case "appshots.permission":
		if c.ID != "windowCapture" && c.ID != "applicationText" {
			return errors.New("unknown Appshots permission")
		}
		return nil
	case "session.fork", "session.sidechat":
		if c.SessionID == "" && c.ID == "" {
			return errors.New("parent session ID is required")
		}
		return nil
	case "theme.analyze", "theme.import":
		return nil
	case "theme.select", "theme.remove", "theme.reload", "theme.duplicate":
		if c.ID == "" {
			return errors.New("theme ID is required")
		}
		return nil
	case "message.send", "session.create", "project.create", "terminal.create", "cloud.toggle", "update.check", "file.create":
		return nil
	case "message.stop", "message.retry", "session.storage.retry":
		if c.SessionID == "" {
			return errors.New("session ID is required")
		}
	case "session.update":
		if c.Session == nil || c.Session.ID == "" {
			return errors.New("session payload with ID is required")
		}
	case "settings.update":
		if c.Settings == nil {
			return errors.New("settings payload is required")
		}
		if err := ValidateHookRules(c.Settings.Hooks); err != nil {
			return err
		}
		return ValidateSubagentProfiles(c.Settings.Subagents)
	case "attachment.add":
		if c.Attachment == nil || c.Attachment.ID == "" {
			return errors.New("attachment with ID is required")
		}
	case "session.archive", "session.pin", "session.rename", "session.delete", "queue.remove", "queue.update", "queue.steer", "queue.send", "request.answer", "file.open", "file.save", "file.rename", "file.delete", "provider.toggle", "provider.connect", "device.toggle", "terminal.input", "terminal.close", "terminal.resize", "terminal.clear":
		if c.ID == "" {
			return errors.New("object ID is required")
		}
	case "group.create":
		if strings.TrimSpace(c.Text) == "" {
			return errors.New("group title is required")
		}
	case "group.rename":
		if c.ID == "" {
			return errors.New("group ID is required")
		}
		if strings.TrimSpace(c.Text) == "" {
			return errors.New("group title is required")
		}
	case "group.delete":
		if c.ID == "" {
			return errors.New("group ID is required")
		}
	default:
		return fmt.Errorf("unsupported command %q", c.Kind)
	}
	return nil
}
