package engine

import (
	"context"
	"errors"
	"fmt"
	"log"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gcode/internal/agent"
	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/reorder"
)

func (e *Engine) Execute(ctx context.Context, command contracts.Command) (resultErr error) {
	command = command.Clone()
	defer func() {
		if resultErr != nil && command.Kind == "project.create" && command.ProjectCreate != nil {
			log.Printf("project creation failed: path=%q: %v", command.ProjectCreate.Path, resultErr)
		}
	}()
	if err := command.Validate(); err != nil {
		return err
	}
	if command.Kind == "git.refs.load" || command.Kind == "git.ref.switch" {
		return e.gitCommand(ctx, command)
	}
	if command.Kind == "terminal.input" || command.Kind == "terminal.resize" || command.Kind == "terminal.clear" {
		return e.terminalCommand(ctx, command, true)
	}
	if command.Kind == "message.send" && command.Send != nil && command.Send.SessionID == "" && command.Send.Checkout != nil {
		return e.checkoutSend(ctx, command)
	}
	return e.executeCommand(ctx, command, nil)
}

func (e *Engine) executeCommand(ctx context.Context, command contracts.Command, checkout *checkoutResult) (resultErr error) {
	e.mu.Lock()
	var outsideLock []func()
	defer func() {
		// Joined workers may call back into the Engine.
		e.evictIdleSessionsLocked()
		e.mu.Unlock()
		for _, cleanup := range outsideLock {
			cleanup()
		}
	}()
	if err := e.checkLocked(ctx); err != nil {
		return err
	}
	if strings.HasPrefix(command.Kind, "plugins.") {
		return e.startPluginOperationLocked(command)
	}
	if checkout != nil {
		found := false
		for _, project := range e.state.Projects {
			if project.ID == checkout.projectID && project.Path == checkout.projectPath {
				found = true
			}
		}
		if !found {
			return errors.New("the project changed while preparing the checkout")
		}
	}
	affectedSession := command.SessionID
	if command.Send != nil && command.Send.SessionID != "" {
		affectedSession = command.Send.SessionID
	}
	if command.Session != nil {
		affectedSession = command.Session.ID
	}
	if affectedSession == "" && strings.HasPrefix(command.Kind, "session.") && e.sessionIndex(command.ID) >= 0 && command.Kind != "session.delete" {
		affectedSession = command.ID
	}
	if affectedSession == "" && strings.HasPrefix(command.Kind, "queue.") {
		for _, queued := range e.state.Queue {
			if queued.ID == command.ID {
				affectedSession = queued.SessionID
				break
			}
		}
	}
	// Load before taking the transaction snapshot. Failed validation must not
	// leave a loaded flag paired with rolled-back, empty execution history.
	if affectedSession != "" {
		if err := e.ensureSessionDataLocked(affectedSession); err != nil {
			e.publishLocked(contracts.Event{Kind: "storage.unavailable", SessionID: affectedSession})
			return err
		}
	}
	before := contracts.CloneSnapshot(e.state)
	providers := slices.Clone(e.providers)
	taskModels := maps.Clone(e.taskModels)
	history, executionContext := maps.Clone(e.history), maps.Clone(e.context)
	hashes, records, loaded := maps.Clone(e.sessionHashes), maps.Clone(e.sessionRecords), maps.Clone(e.loadedSessions)
	dirty := maps.Clone(e.sessionDirty)
	var answerChannel chan []contracts.QuestionAnswer
	var answerOwner *activeRun
	// A deleted conversation keeps its stored body until the index commit
	// succeeds, so a failed delete leaves the conversation intact.
	var deletedSession contracts.Session
	var deletedRecord string
	requiredSessions := []string{affectedSession}
	defer func() {
		if resultErr != nil {
			for _, session := range e.state.Sessions {
				if !loaded[session.ID] && e.loadedSessions[session.ID] && !slices.ContainsFunc(before.Sessions, func(previous contracts.Session) bool { return previous.ID == session.ID }) {
					e.releaseEmptySessionDirectories(session)
				}
			}
			storageErrors := map[string]string{}
			for _, session := range e.state.Sessions {
				if session.StorageError != "" {
					storageErrors[session.ID] = session.StorageError
				}
			}
			e.state, e.providers, e.history, e.context = before, providers, history, executionContext
			e.taskModels = taskModels
			e.sessionHashes, e.sessionRecords, e.loadedSessions = hashes, records, loaded
			e.sessionDirty = dirty
			for i := range e.state.Sessions {
				if message := storageErrors[e.state.Sessions[i].ID]; message != "" {
					e.state.Sessions[i].StorageError = message
				}
			}
			if len(storageErrors) > 0 {
				e.publishLocked(contracts.Event{Kind: "storage.unavailable", SessionID: affectedSession})
			}
		}
	}()
	switch command.Kind {
	case "provider.test":
		return e.startModelCheckLocked(command.ID)
	case "provider.models.fetch":
		return e.startModelFetchLocked(command.ID)
	case "resource.refresh", "resource.skill.save", "resource.skill.import", "resource.skill.delete", "resource.skill.toggle",
		"resource.command.save", "resource.command.delete", "resource.command.toggle",
		"resource.mcp.save", "resource.mcp.delete", "resource.mcp.toggle", "resource.mcp.test":
		return e.resourceCommandLocked(command)
	case "goal.set", "goal.replace", "goal.edit", "goal.pause", "goal.resume", "goal.clear":
		return e.goalCommandLocked(command)
	case "terminal.create":
		return e.createTerminalLocked(command)
	case "terminal.close":
		cleanup, err := e.closeTerminalLocked(command.ID)
		if cleanup != nil {
			e.wg.Add(1)
			outsideLock = append(outsideLock, func() {
				defer e.wg.Done()
				cleanup()
			})
		}
		return err
	case "message.send":
		if command.Send == nil {
			return errors.New("send payload is required")
		}
		send := *command.Send
		if send.ProjectID == "" {
			send.ProjectID = command.ProjectID
		}
		return e.sendWithCheckoutLocked(send, checkout)
	case "message.stop":
		if run := e.runs[command.SessionID]; run != nil {
			if goal := e.goalLocked(run.goal); run.goal.sessionID == command.SessionID && goal != nil &&
				(goal.Status == contracts.GoalActive || goal.Status == contracts.GoalVerifying) {
				return e.goalCommandLocked(contracts.Command{
					Kind: "goal.pause", SessionID: command.SessionID, ID: goal.ID,
				})
			}
			run.cancel()
		}
		return nil
	case "message.retry":
		if e.runs[command.SessionID] != nil {
			return errors.New("session is already running")
		}
		index := e.sessionIndex(command.SessionID)
		if index < 0 {
			return errors.New("session not found")
		}
		if e.state.Sessions[index].SubagentType != "" {
			return errors.New("subagent sessions are read-only")
		}
		history := e.history[command.SessionID]
		if len(history) == 0 || (e.state.Sessions[index].SideChat && !slices.ContainsFunc(history, func(message model.Message) bool {
			return message.UserInput
		})) {
			return errors.New("no execution history to retry")
		}
		return e.startLocked(e.state.Sessions[index], "")
	case "request.answer":
		for _, run := range e.runs {
			if answer := run.answers[command.ID]; answer != nil {
				answerChannel = answer
				answerOwner = run
				break
			}
		}
		if answerChannel == nil || len(answerChannel) != 0 {
			return errors.New("question is no longer waiting for an answer")
		}
		foundQuestion := false
		for id, messages := range e.state.Messages {
			for i := range messages {
				for j := range messages[i].Blocks {
					question := messages[i].Blocks[j].Question
					if question != nil && question.ID == command.ID {
						if command.SessionID != "" && command.SessionID != id {
							return errors.New("question belongs to another session")
						}
						if question.Resolved {
							return errors.New("question is no longer waiting for an answer")
						}
						if err := contracts.ValidateQuestionAnswers(question.Pages, command.QuestionAnswers); err != nil {
							return err
						}
						question.Answers = contracts.CloneQuestionAnswers(command.QuestionAnswers)
						question.Resolved = true
						affectedSession = id
						foundQuestion = true
					}
				}
			}
			e.state.Messages[id] = messages
		}
		if !foundQuestion {
			return errors.New("question request not found")
		}
	case "settings.update":
		if command.Settings == nil {
			return errors.New("settings are required")
		}
		if err := contracts.ValidateHookRules(command.Settings.Hooks); err != nil {
			return err
		}
		e.state.Settings = *command.Settings
	case "file.open", "file.save", "file.create", "file.rename", "file.delete":
		if err := e.fileCommandLocked(command); err != nil {
			return err
		}
	case "provider.save":
		config := model.CloneProvider(command.Provider.Provider)
		index := -1
		for i, p := range e.providers {
			if p.ID == config.ID {
				index = i
				break
			}
		}
		if index >= 0 && command.Provider.PreserveKey {
			config.APIKey = e.providers[index].APIKey
		}
		if index < 0 {
			e.providers = append(e.providers, config)
		} else {
			e.providers[index] = config
		}
		e.refreshProvidersLocked()
	case "provider.reorder", "provider.models.reorder":
		if err := e.reorderProvidersLocked(command); err != nil {
			return err
		}
	case "group.reorder":
		groups, err := reorder.Move(e.state.SessionGroups, command.Reorder.SourceID, command.Reorder.TargetID, command.Reorder.After,
			func(group contracts.SessionGroup) string { return group.ID })
		if err != nil {
			return err
		}
		for i := range groups {
			groups[i].Order = i
		}
		e.state.SessionGroups = groups
	case "session.reorder":
		if err := e.reorderSessionLocked(command); err != nil {
			return err
		}
		for _, session := range e.groupMembersLocked(command.Reorder.GroupID) {
			requiredSessions = append(requiredSessions, session.ID)
		}
	case "session.project.reorder":
		ids, err := e.reorderProjectSessionLocked(command.Reorder)
		if err != nil {
			return err
		}
		requiredSessions = append(requiredSessions, ids...)
	case "provider.delete":
		for _, session := range e.state.Sessions {
			if strings.HasPrefix(session.ModelID, command.ID+"/") && e.runs[session.ID] != nil {
				return errors.New("stop this provider's active sessions before removing it")
			}
		}
		e.providers = slices.DeleteFunc(e.providers, func(p model.Provider) bool { return p.ID == command.ID })
		e.refreshProvidersLocked()
	case "storage.change":
		if err := e.storage.RequestDirectory(command.Path); err != nil {
			return err
		}
		e.refreshStorageLocked()
		e.publishLocked(contracts.Event{Kind: "storage.changed"})
		return nil
	case "session.storage.retry":
		if err := e.ensureSessionDataLocked(command.SessionID); err != nil {
			return err
		}
	case "project.create":
		if command.ProjectCreate == nil {
			return errors.New("project is required")
		}
		create := command.ProjectCreate
		path, err := filepath.Abs(create.Path)
		if err != nil {
			return err
		}
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return errors.New("choose a project directory")
		}
		for _, project := range e.state.Projects {
			if project.Path == path {
				return errors.New("project already exists")
			}
		}
		name := strings.TrimSpace(create.Name)
		if name == "" {
			name = filepath.Base(path)
		}
		e.state.Projects = append(e.state.Projects, contracts.Project{ID: id("project"), Name: name, Path: path, DeviceID: "local", Icon: "folder", FilesState: contracts.LoadLoading})
	case "project.delete":
		e.state.Projects = slices.DeleteFunc(e.state.Projects, func(p contracts.Project) bool { return p.ID == command.ID })
		e.state.Files = slices.DeleteFunc(e.state.Files, func(f contracts.File) bool { return f.ProjectID == command.ID })
	case "project.files.refresh":
		if !slices.ContainsFunc(e.state.Projects, func(p contracts.Project) bool { return p.ID == command.ID }) {
			return errors.New("project not found")
		}
	case "session.create":
		for _, session := range e.state.Sessions {
			if command.RequestID != "" && session.CreationRequestID == command.RequestID {
				return nil
			}
		}
		session, err := e.newSessionLocked(command.ProjectID, command.Name, "")
		if err != nil {
			return err
		}
		e.state.Sessions[e.sessionIndex(session.ID)].CreationRequestID = command.RequestID
	case "session.sidechat", "session.fork":
		parentID := command.SessionID
		if parentID == "" {
			parentID = command.ID
		}
		index := e.sessionIndex(parentID)
		if index < 0 {
			return errors.New("parent session not found")
		}
		for _, session := range e.state.Sessions {
			if command.RequestID != "" && session.CreationRequestID == command.RequestID {
				return nil
			}
		}
		parent := e.state.Sessions[index]
		if err := e.ensureSessionDataLocked(parentID); err != nil {
			return err
		}
		sessionID, _, err := e.reserveSessionLocked(e.storage.ActiveRootID())
		if err != nil {
			return err
		}
		session := parent
		session.ID = sessionID
		session.StorageRootID = e.storage.ActiveRootID()
		session.StorageError = ""
		e.loadedSessions[session.ID] = true
		session.ParentSessionID = parentID
		session.SubagentType = ""
		session.RunningSubagents = 0
		session.Goal = nil
		session.CreationRequestID = command.RequestID
		session.SideChat = command.Kind == "session.sidechat"
		session.Title = command.Name
		session.Status = contracts.Idle
		session.Pinned = false
		session.PinOrder = 0
		session.Archived = false
		session.Unread = false
		session.LastOutcome = ""
		session.CreatedAt = time.Now()
		session.UpdatedAt = session.CreatedAt
		if session.Title == "" {
			session.Title = "Side chat"
		}
		e.state.Sessions = append([]contracts.Session{session}, e.state.Sessions...)
		e.prompts[session.ID] = model.ClonePrompt(e.prompts[parent.ID])
		if e.contextMemories == nil {
			e.contextMemories = map[string]agent.ContextMemory{}
		}
		e.contextMemories[session.ID] = agent.CloneContextMemory(e.contextMemories[parent.ID])
		if session.SideChat {
			e.history[session.ID], e.context[session.ID] = e.sideChatSnapshotLocked(parent.ID)
			e.contextMemories[session.ID] = agent.ContextMemoryForView(e.contextMemories[parent.ID], e.context[session.ID])
			e.state.Messages[session.ID] = []contracts.Message{}
		} else {
			e.history[session.ID] = model.CloneMessages(e.history[parent.ID])
			e.context[session.ID] = model.CloneMessages(e.context[parent.ID])
			messages := contracts.CloneSnapshot(contracts.Snapshot{Messages: map[string][]contracts.Message{parent.ID: e.state.Messages[parent.ID]}}).Messages[parent.ID]
			for i := range messages {
				messages[i].ID = id("message")
				messages[i].SessionID = session.ID
				messages[i].Streaming = false
			}
			e.state.Messages[session.ID] = messages
		}
		e.markSessionDirtyLocked(session.ID)
	case "session.update":
		index := e.sessionIndex(command.Session.ID)
		if index < 0 {
			return errors.New("session not found")
		}
		session := &e.state.Sessions[index]
		change := command.Session
		if session.SubagentType != "" && (change.ModelID != nil || change.ReasoningLevel != nil) {
			return errors.New("subagent execution settings are read-only")
		}
		if change.Title != nil {
			e.cancelNamingLocked(session.ID)
			session.Title = *change.Title
		}
		if change.Pinned != nil {
			session.Pinned = *change.Pinned
		}
		if change.Archived != nil {
			session.Archived = *change.Archived
			if *change.Archived {
				e.state.Queue = slices.DeleteFunc(e.state.Queue, func(q contracts.QueuedMessage) bool { return q.SessionID == session.ID })
			}
		}
		if change.Unread != nil {
			session.Unread = *change.Unread
		}
		if change.PinOrder != nil {
			session.PinOrder = *change.PinOrder
		}
		if change.GroupID != nil {
			if *change.GroupID != "" && e.sessionGroupIndex(*change.GroupID) < 0 {
				return errors.New("session group not found")
			}
			session.GroupID = *change.GroupID
		}
		if change.GroupOrder != nil {
			session.GroupOrder = *change.GroupOrder
		}
		if change.ModelID != nil || change.ReasoningLevel != nil {
			if err := e.setSessionModelLocked(session, change.ModelID, change.ReasoningLevel); err != nil {
				return err
			}
		}
	case "session.archive":
		index := e.sessionIndex(command.ID)
		if index < 0 {
			return errors.New("session not found")
		}
		e.state.Sessions[index].Archived = command.Enabled
		if command.Enabled {
			e.state.Queue = slices.DeleteFunc(e.state.Queue, func(q contracts.QueuedMessage) bool { return q.SessionID == command.ID })
		}
	case "group.create":
		title := strings.TrimSpace(command.Text)
		if title == "" {
			return errors.New("group title is required")
		}
		order := 0
		for _, group := range e.state.SessionGroups {
			order = max(order, group.Order+1)
		}
		e.state.SessionGroups = append(e.state.SessionGroups, contracts.SessionGroup{ID: id("group"), Title: title, Order: order})
	case "group.rename":
		index := e.sessionGroupIndex(command.ID)
		if index < 0 {
			return errors.New("session group not found")
		}
		title := strings.TrimSpace(command.Text)
		if title == "" {
			return errors.New("group title is required")
		}
		e.state.SessionGroups[index].Title = title
	case "group.delete":
		index := e.sessionGroupIndex(command.ID)
		if index < 0 {
			return errors.New("session group not found")
		}
		removed := e.state.SessionGroups[index].ID
		memberIDs := make([]string, 0)
		for _, session := range e.state.Sessions {
			if session.GroupID == removed {
				memberIDs = append(memberIDs, session.ID)
			}
		}
		memberSet := make(map[string]struct{}, len(memberIDs))
		for _, sessionID := range memberIDs {
			memberSet[sessionID] = struct{}{}
			if err := e.ensureSessionDataLocked(sessionID); err != nil {
				return err
			}
		}
		requiredSessions = append(requiredSessions, memberIDs...)
		e.state.SessionGroups = slices.Delete(e.state.SessionGroups, index, index+1)
		for i := range e.state.Sessions {
			if _, ok := memberSet[e.state.Sessions[i].ID]; ok {
				e.state.Sessions[i].GroupID = ""
				e.state.Sessions[i].GroupOrder = 0
				e.markSessionDirtyLocked(e.state.Sessions[i].ID)
			}
		}
	case "session.delete":
		if run := e.runs[command.ID]; run != nil {
			return errors.New("stop the session before deleting it")
		}
		for _, run := range e.runs {
			if run.subagent != nil && run.subagent.parentSessionID == command.ID {
				return errors.New("stop active subagents before deleting their parent session")
			}
		}
		e.cancelNamingLocked(command.ID)
		if index := e.sessionIndex(command.ID); index >= 0 {
			deletedSession = e.state.Sessions[index]
			deletedRecord = e.sessionRecords[command.ID]
			e.removeTrajectory(deletedSession)
		}
		e.state.Sessions = slices.DeleteFunc(e.state.Sessions, func(s contracts.Session) bool { return s.ID == command.ID })
		delete(e.state.Messages, command.ID)
		e.state.ModelRequests = slices.DeleteFunc(e.state.ModelRequests, func(request contracts.ModelRequest) bool { return request.SessionID == command.ID })
		e.state.Runs = slices.DeleteFunc(e.state.Runs, func(run contracts.Run) bool { return run.SessionID == command.ID })
		e.state.Changes = slices.DeleteFunc(e.state.Changes, func(change contracts.Change) bool { return change.SessionID == command.ID })
		delete(e.history, command.ID)
		delete(e.context, command.ID)
		delete(e.contextMemories, command.ID)
		delete(e.prompts, command.ID)
		delete(e.taskModels, command.ID)
		e.state.Queue = slices.DeleteFunc(e.state.Queue, func(q contracts.QueuedMessage) bool { return q.SessionID == command.ID })
	case "queue.remove":
		e.state.Queue = slices.DeleteFunc(e.state.Queue, func(q contracts.QueuedMessage) bool { return q.ID == command.ID })
	case "queue.send":
		return e.sendQueuedNowLocked(command.ID)
	case "queue.move":
		return e.moveQueuedLocked(*command.Reorder)
	case "queue.steer":
		found := false
		for i := range e.state.Queue {
			if e.state.Queue[i].ID == command.ID {
				e.state.Queue[i].Delivery = "steer"
				found = true
			}
		}
		if !found {
			return errors.New("queued message not found")
		}
	case "providers.refresh", "catalog.retry":
		e.refreshProvidersLocked()
	case "device.rename":
		for i := range e.state.Devices {
			if e.state.Devices[i].ID == command.ID {
				e.state.Devices[i].Name = command.Name
			}
		}
	default:
		return fmt.Errorf("operation %q is not implemented by the real backend", command.Kind)
	}
	if err := e.persistLocked(requiredSessions...); err != nil {
		return err
	}
	if command.Kind == "project.create" {
		e.startProjectFilesLocked(e.state.Projects[len(e.state.Projects)-1])
	} else if command.Kind == "project.delete" {
		if scan := e.projectScans[command.ID]; scan != nil {
			scan.cancel()
			delete(e.projectScans, command.ID)
		}
	} else if command.Kind == "project.files.refresh" {
		for _, project := range e.state.Projects {
			if project.ID == command.ID {
				e.startProjectFilesLocked(project)
				break
			}
		}
	}
	if command.Kind == "session.delete" {
		if deletedRecord != "" {
			e.removeSessionRecord(deletedSession.StorageRootID, deletedSession.ID, deletedRecord)
			e.releaseEmptySessionDirectories(deletedSession)
		}
		delete(e.loadedSessions, command.ID)
		if owner := e.sessionTools[command.ID]; owner != nil {
			delete(e.sessionTools, command.ID)
			e.wg.Add(1)
			outsideLock = append(outsideLock, func() {
				defer e.wg.Done()
				owner.cleanup()
			})
		}
		delete(e.sessionHookOnce, command.ID)
		delete(e.sessionStarted, command.ID)
	}
	if command.Kind == "provider.save" {
		e.cancelProviderNamingLocked(command.Provider.Provider.ID)
		e.invalidateModelChecksLocked(command.Provider.Provider.ID)
		e.invalidateModelFetchesLocked(command.Provider.Provider.ID)
		e.refreshProvidersLocked()
	} else if command.Kind == "provider.delete" {
		e.cancelProviderNamingLocked(command.ID)
		e.invalidateModelChecksLocked(command.ID)
		e.invalidateModelFetchesLocked(command.ID)
		e.refreshProvidersLocked()
	}
	if command.Kind == "settings.update" && before.Settings.General.ThreadNamingModel != e.state.Settings.General.ThreadNamingModel {
		for sessionID := range e.naming {
			e.cancelNamingLocked(sessionID)
		}
	}
	if answerChannel != nil {
		delete(answerOwner.answers, command.ID)
		answerChannel <- contracts.CloneQuestionAnswers(command.QuestionAnswers)
	}
	for _, session := range e.state.Sessions {
		if session.Archived {
			if run := e.runs[session.ID]; run != nil {
				run.cancel()
			}
		}
	}
	e.publishLocked(contracts.Event{Kind: "state.changed"})
	return nil
}

func (e *Engine) newSessionLocked(projectID, title, rootID string) (contracts.Session, error) {
	if title == "" {
		title = "New conversation"
	}
	if rootID == "" {
		rootID = e.storage.ActiveRootID()
	}
	cwd := ""
	if projectID != "" {
		for _, project := range e.state.Projects {
			if project.ID == projectID {
				cwd = project.Path
				break
			}
		}
		if cwd == "" {
			return contracts.Session{}, errors.New("project not found")
		}
	}
	sessionID, root, err := e.reserveSessionLocked(rootID)
	if err != nil {
		return contracts.Session{}, err
	}
	attached := false
	defer func() {
		if !attached {
			e.releaseEmptySessionDirectories(contracts.Session{ID: sessionID, StorageRootID: rootID, CWD: cwd})
		}
	}()
	if projectID == "" {
		handle, err := os.OpenRoot(root)
		if err != nil {
			return contracts.Session{}, err
		}
		defer handle.Close()
		if err := handle.Mkdir("workspaces", 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return contracts.Session{}, err
		}
		workspace, err := openRealDirectory(handle, "workspaces")
		if err != nil {
			return contracts.Session{}, err
		}
		defer workspace.Close()
		cwd = filepath.Join(root, "workspaces", sessionID)
		if err := workspace.Mkdir(sessionID, 0700); err != nil {
			_ = handle.Remove(filepath.Join("sessions", sessionID))
			return contracts.Session{}, err
		}
	}
	now := time.Now()
	session := contracts.Session{ID: sessionID, StorageRootID: rootID, Title: title, ProjectID: projectID, CWD: cwd, DeviceID: "local", Status: contracts.Idle, CreatedAt: now, UpdatedAt: now}
	if len(e.state.Models) > 0 {
		session.ModelID = e.state.Models[0].ID
	}
	e.state.Sessions = append([]contracts.Session{session}, e.state.Sessions...)
	e.loadedSessions[session.ID] = true
	e.state.Messages[session.ID] = []contracts.Message{}
	e.history[session.ID], e.context[session.ID] = []model.Message{}, []model.Message{}
	e.markSessionDirtyLocked(session.ID)
	attached = true
	return session, nil
}
