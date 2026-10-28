package contracts

import (
	"errors"
	"fmt"
	"maps"
	"slices"
)

// Clone freezes a command before it crosses a goroutine/queue boundary.
func (c Command) Clone() Command {
	c.ProjectCreate = copyPointer(c.ProjectCreate)
	c.Integration = copyPointer(c.Integration)
	c.Action = copyPointer(c.Action)
	c.Adjustment = copyPointer(c.Adjustment)
	if c.Settings != nil {
		v := *c.Settings
		v.Shortcuts = slices.Clone(v.Shortcuts)
		v.ModelPicker.Favorites = slices.Clone(v.ModelPicker.Favorites)
		v.ModelPicker.Options = maps.Clone(v.ModelPicker.Options)
		v.ModelPicker.Selections = maps.Clone(v.ModelPicker.Selections)
		c.Settings = &v
	}
	if c.Send != nil {
		v := *c.Send
		v.Annotations = slices.Clone(v.Annotations)
		for i := range v.Annotations {
			v.Annotations[i] = v.Annotations[i].Bounded()
		}
		v.Attachments = slices.Clone(v.Attachments)
		c.Send = &v
	}
	if c.Session != nil {
		v := *c.Session
		v.AccessMode = copyPointer(v.AccessMode)
		v.Plan = copyPointer(v.Plan)
		v.Title = copyPointer(v.Title)
		v.Pinned = copyPointer(v.Pinned)
		v.Archived = copyPointer(v.Archived)
		v.Unread = copyPointer(v.Unread)
		v.PinOrder = copyPointer(v.PinOrder)
		v.ModelID = copyPointer(v.ModelID)
		v.ReasoningLevel = copyPointer(v.ReasoningLevel)
		v.Branch = copyPointer(v.Branch)
		v.DeviceID = copyPointer(v.DeviceID)
		v.WorktreeID = copyPointer(v.WorktreeID)
		c.Session = &v
	}
	c.Attachment = copyPointer(c.Attachment)
	c.TerminalSize = copyPointer(c.TerminalSize)
	c.Comment = copyPointer(c.Comment)
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
	if c.Action != nil && c.Kind != "project.action.create" && c.Kind != "project.action.update" {
		return errors.New("project action payload does not match command")
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
	if c.Comment != nil && c.Kind != "pr.comment" {
		return errors.New("comment payload does not match command")
	}
	if c.TerminalSize != nil && c.Kind != "terminal.resize" {
		return errors.New("terminal size payload does not match command")
	}
	switch c.Kind {
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
	case "project.action.create", "project.action.update":
		if c.ProjectID == "" || c.Action == nil {
			return errors.New("project and action payload are required")
		}
		return nil
	case "project.action.delete", "project.action.run":
		if c.ProjectID == "" || c.ID == "" {
			return errors.New("project and action ID are required")
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
	case "message.send", "session.create", "project.create", "terminal.create", "account.signin", "account.signout", "cloud.toggle", "update.check", "file.create":
		return nil
	case "message.stop", "message.retry":
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
	case "attachment.add":
		if c.Attachment == nil || c.Attachment.ID == "" {
			return errors.New("attachment with ID is required")
		}
	case "session.archive", "session.pin", "session.rename", "session.delete", "queue.remove", "queue.update", "request.answer", "file.save", "file.rename", "file.delete", "provider.toggle", "provider.connect", "device.toggle", "pr.comment", "pr.merge", "terminal.input", "terminal.close", "terminal.resize":
		if c.ID == "" {
			return errors.New("object ID is required")
		}
	default:
		return fmt.Errorf("unsupported command %q", c.Kind)
	}
	return nil
}
