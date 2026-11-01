package mock

import (
	"fmt"
	"hash/fnv"
	"path"
	"strings"
	"time"

	"gcode/internal/contracts"
)

func toolBlock(kind contracts.ToolKind, name, summary, command, file, output string) contracts.Block {
	args := command
	if args == "" {
		args = file
	}
	return contracts.Block{Kind: "tool", Tool: &contracts.ToolCall{Kind: kind, Name: name, Summary: summary, Command: command, Path: file, Args: args, Output: output, Status: "completed", Language: "text"}}
}

func taskBlocks(task agentTask, project, before, after string) []contracts.Block {
	blocks := []contracts.Block{{Kind: "reasoning", Text: task.thought}}
	if task.index == 3 {
		blocks = append(blocks, toolBlock(contracts.ToolWebFetch, "Web fetch", "Read deployment response fields", "https://status.example.com/api/deployment", "", "revision: c42b18e\nphase: ready\nurl: https://preview.example.com/fieldnotes"))
	}
	if task.index == 1 || task.index == 8 || task.index == 9 {
		blocks = append(blocks, toolBlock(contracts.ToolSearch, "Grep", "Find the affected state transition", "rg -n 'selected|sequence|SearchFields' "+path.Dir(task.path), task.path, before))
	}
	if task.index == 0 || task.index == 6 || task.index == 9 {
		blocks = append(blocks, toolBlock(contracts.ToolGlob, "Glob", "Locate source and checks", "**/*"+path.Base(task.path), "", task.path))
	}
	if before != "" {
		read := toolBlock(contracts.ToolReadFile, "Read", "Read "+task.path, "", task.path, before)
		read.Tool.Language = task.language
		blocks = append(blocks, read)
	}
	if task.index != 5 && task.index != 6 {
		blocks = append(blocks, contracts.Block{Kind: "text", Text: task.note})
	}
	if task.failure {
		failed := toolBlock(contracts.ToolExec, "Bash", "Reproduce the redirect regression", "go test ./internal/auth -run TestRedirect -count=1", "", "--- FAIL: TestRedirect (0.00s)\n    redirect_test.go:24: got /, want /projects/fieldnotes\nFAIL\nexit status 1")
		code := 1
		failed.Tool.IsError = true
		failed.Tool.ExitCode = &code
		failed.Tool.Status = "failed"
		blocks = append(blocks, failed, contracts.Block{Kind: "reasoning", Text: "The assertion reproduces the lost destination. I’ll preserve local paths while retaining the external-URL guard."})
	}
	if task.review {
		blocks = append(blocks, toolBlock(contracts.ToolMCP, "GitHub", "Read pull request #412 discussion", "pull_request_read #412", "", "PR #412 · Refine the workspace layout\nBuild: success\nTests: success\nReview: pending\n1 existing comment"))
		blocks = append(blocks, contracts.Block{Kind: "subagent", Subagent: &contracts.Subagent{SessionID: "agent-pr-412", Name: "Explorer", Task: "Review keyboard focus when closing the right pane", Status: "completed", Summary: "Reviewed the previous-focus fallback without changing files."}})
		blocks = append(blocks, toolBlock(contracts.ToolSearch, "Grep", "Inspect comment revision and line identity", "rg -n 'revision|line|side' internal/review", task.path, before))
		return blocks
	}
	kind, name := contracts.ToolEditFile, "Edit"
	if task.write {
		kind, name = contracts.ToolWriteFile, "Write"
	}
	firstAfter := after
	if task.index == 1 {
		firstAfter = strings.Replace(after, "return -1", "return 0", 1)
	}
	edit := toolBlock(kind, name, "Update "+task.path, "", task.path, "Updated "+task.path)
	edit.Tool.Language = task.language
	added, removed, hunks := contiguousDiff(before, firstAfter)
	edit.Tool.Diff = &contracts.Change{ProjectID: project, Path: task.path, Status: "modified", OldContent: before, NewContent: firstAfter, Additions: added, Deletions: removed, Hunks: hunks}
	if before == "" {
		edit.Tool.Diff.Status = "added"
	}
	blocks = append(blocks, edit)
	if task.index == 1 && firstAfter != after {
		patch := toolBlock(contracts.ToolApplyPatch, "Apply patch", "Keep an empty result list unselected", "", task.path, "Applied empty-result sentinel")
		added, removed, hunks = contiguousDiff(firstAfter, after)
		patch.Tool.Diff = &contracts.Change{ProjectID: project, Path: task.path, Status: "modified", OldContent: firstAfter, NewContent: after, Additions: added, Deletions: removed, Hunks: hunks}
		blocks = append(blocks, patch)
	}
	if task.index == 0 || task.index == 2 || task.index == 5 || task.index == 8 {
		blocks = append(blocks, contracts.Block{Kind: "reasoning", Text: "The recorded change is in place. I’ll check the affected behavior before writing the final response."})
	}
	command := "go test ./" + path.Dir(task.path)
	if task.path == "README.md" {
		command = "git diff --check"
	}
	check := toolBlock(contracts.ToolExec, "Bash", "Run focused checks", command, "", "ok  "+project+"/"+path.Dir(task.path)+"  0.018s")
	if task.path == "README.md" {
		check.Tool.Output = ""
	}
	code := 0
	check.Tool.ExitCode = &code
	blocks = append(blocks, check)
	if task.index == 9 {
		blocks = append(blocks, toolBlock(contracts.ToolTodo, "TodoWrite", "Complete settings search checks", "", "", "Index metadata: completed\nLocate matching row: completed\nVerify search terms: completed"))
	}
	return blocks
}

func stampBlocks(blocks []contracts.Block, prefix string, started time.Time) (time.Time, int64) {
	cursor := started
	for i := range blocks {
		block := &blocks[i]
		block.ID = fmt.Sprintf("%s-block-%d", prefix, i)
		duration := int64(650)
		if block.Tool != nil {
			duration = int64(120 + i*73)
			block.Tool.ID = fmt.Sprintf("%s-tool-%d", prefix, i)
			block.Tool.StartedAt = cursor
			end := cursor.Add(time.Duration(duration) * time.Millisecond)
			block.Tool.FinishedAt = &end
			if block.Tool.Diff != nil {
				block.Tool.Diff.ID = block.Tool.ID + "-diff"
			}
		}
		if block.Subagent != nil {
			block.Subagent.ID = prefix + "-subagent"
			duration = 1240
		}
		cursor = cursor.Add(time.Duration(duration) * time.Millisecond)
	}
	return cursor, cursor.Sub(started).Milliseconds()
}

func seedAgentTranscripts(snapshot *contracts.Snapshot) {
	for i := range snapshot.Sessions {
		session := &snapshot.Sessions[i]
		for _, project := range snapshot.Projects {
			if project.ID == session.ProjectID {
				session.DeviceID = project.DeviceID
				break
			}
		}
		task := taskDefinition(i)
		before, after := task.before, task.after
		if task.review {
			after = before
		}
		if i == 0 {
			for _, file := range snapshot.Files {
				if file.ProjectID == session.ProjectID && file.Path == task.path {
					after = file.Content
				}
			}
		}
		ensureAgentFile(snapshot, session.ProjectID, task.path, after)
		blocks := taskBlocks(task, session.ProjectID, before, after)
		turnID := session.ID + "-turn"
		started := session.UpdatedAt.Add(-2 * time.Minute)
		finished, duration := stampBlocks(blocks, turnID, started)
		finished = finished.Add(900 * time.Millisecond)
		duration += 900
		firstID, nextID, finalID := session.ID+"-assistant", session.ID+"-continued", session.ID+"-reply"
		split := min(3, len(blocks))
		model := []string{"claude-opus-4-6", "claude-sonnet-4-6", "gpt-5.4"}[i%3]
		session.ModelID = model
		if i == 0 || i == 4 {
			session.Branch = "fieldnotes/workspace"
		}
		if i == 1 {
			session.Branch = "fieldnotes/command-palette"
		}
		for pi := range snapshot.Projects {
			if snapshot.Projects[pi].ID == session.ProjectID {
				found := false
				for _, branch := range snapshot.Projects[pi].Branches {
					found = found || branch == session.Branch
				}
				if !found {
					snapshot.Projects[pi].Branches = append(snapshot.Projects[pi].Branches, session.Branch)
				}
			}
		}
		session.UpdatedAt = finished
		limit := 200000
		if model == "gpt-5.4" {
			limit = 1000000
		}
		input := len([]rune(task.prompt))/4 + 350 + i*61
		output := len([]rune(task.final))/4 + 80
		snapshot.Messages[session.ID] = []contracts.Message{
			{ID: session.ID + "-user", SessionID: session.ID, TurnID: turnID, Role: "user", CreatedAt: started, Blocks: []contracts.Block{{ID: session.ID + "-prompt", Kind: "text", Text: task.prompt}}},
			{ID: firstID, SessionID: session.ID, TurnID: turnID, Role: "assistant", CreatedAt: started, Blocks: blocks[:split]},
			{ID: nextID, ContinuationOf: firstID, SessionID: session.ID, TurnID: turnID, Role: "assistant", CreatedAt: started.Add(time.Second), Blocks: blocks[split:]},
			{ID: finalID, ContinuationOf: nextID, DurationMS: duration, SessionID: session.ID, TurnID: turnID, Role: "assistant", CreatedAt: finished, Blocks: []contracts.Block{{ID: finalID + "-text", Kind: "text", Text: task.final}}, Usage: contracts.Usage{InputTokens: input, OutputTokens: output, CachedTokens: input / 2, ContextTokens: input + output + 2400, ContextLimit: limit, Cost: float64(input+output) * 0.000003}},
		}
		summary := contracts.SummarizeActivity(contracts.TurnActivity(snapshot.Messages[session.ID], turnID))
		snapshot.Runs = append(snapshot.Runs, contracts.Run{ID: turnID, SessionID: session.ID, Status: "completed", FinishReason: "stop", StartedAt: started, FinishedAt: &finished, DurationMS: duration, Summary: summary})
		for _, block := range blocks {
			if block.Tool != nil && block.Tool.Diff != nil {
				mergeAgentChange(snapshot, *block.Tool.Diff)
			}
		}
	}
	seedReviewChild(snapshot)
}

func seedReviewChild(snapshot *contracts.Snapshot) {
	var parent contracts.Run
	for _, run := range snapshot.Runs {
		if run.ID == "chat-4-turn" {
			parent = run
			break
		}
	}
	started := parent.StartedAt.Add(1832 * time.Millisecond)
	finished := started.Add(1240 * time.Millisecond)
	content := ""
	for _, file := range snapshot.Files {
		if file.ProjectID == "fieldnotes" && file.Path == "internal/focus.go" {
			content = file.Content
			break
		}
	}
	read := toolBlock(contracts.ToolReadFile, "Read", "Inspect focus fallback", "", "internal/focus.go", content)
	read.ID = "agent-pr-412-read"
	read.Tool.ID = "agent-pr-412-invocation"
	read.Tool.Language = "go"
	read.Tool.StartedAt = started.Add(650 * time.Millisecond)
	readEnd := started.Add(950 * time.Millisecond)
	read.Tool.FinishedAt = &readEnd
	snapshot.Sessions = append(snapshot.Sessions, contracts.Session{ID: "agent-pr-412", ParentSessionID: "chat-4", SideChat: true, Title: "Review keyboard navigation for PR #412", ProjectID: "fieldnotes", DeviceID: "local", ModelID: "claude-sonnet-4-6", ReasoningLevel: "High", Branch: "fieldnotes/workspace", Status: contracts.Idle, CreatedAt: started, UpdatedAt: finished, LastOutcome: "completed"})
	messages := []contracts.Message{{ID: "agent-pr-412-user", SessionID: "agent-pr-412", TurnID: "agent-pr-412-turn", Role: "user", CreatedAt: started, Blocks: []contracts.Block{{ID: "agent-pr-412-prompt", Kind: "text", Text: "Review keyboard focus for PR #412 without changing files."}}}, {ID: "agent-pr-412-reply", SessionID: "agent-pr-412", TurnID: "agent-pr-412-turn", Role: "assistant", CreatedAt: started, DurationMS: 1240, Blocks: []contracts.Block{{ID: "agent-pr-412-thought", Kind: "reasoning", Text: "I’ll inspect the current close-panel focus fallback without editing it."}, read, {ID: "agent-pr-412-final", Kind: "text", Text: "The current implementation restores the previous control and falls back to the composer. I found no additional keyboard blocker and changed no files."}}}}
	snapshot.Messages["agent-pr-412"] = messages
	snapshot.Runs = append(snapshot.Runs, contracts.Run{ID: "agent-pr-412-turn", SessionID: "agent-pr-412", Status: "completed", FinishReason: "stop", StartedAt: started, FinishedAt: &finished, DurationMS: 1240, Summary: contracts.SummarizeActivity(contracts.TurnActivity(messages, "agent-pr-412-turn"))})
}

func ensureAgentFile(snapshot *contracts.Snapshot, project, filePath, content string) {
	parent := ""
	parts := strings.Split(filePath, "/")
	for i := 1; i <= len(parts); i++ {
		current := strings.Join(parts[:i], "/")
		found := -1
		for fi, file := range snapshot.Files {
			if file.ProjectID == project && file.Path == current {
				found = fi
				break
			}
		}
		if found >= 0 {
			if i == len(parts) {
				snapshot.Files[found].Content = content
			}
			parent = snapshot.Files[found].ID
			continue
		}
		h := fnv.New64a()
		_, _ = h.Write([]byte(project + "/" + current))
		id := fmt.Sprintf("agent-file-%x", h.Sum64())
		kind := "directory"
		if i == len(parts) {
			kind = "file"
		}
		language := strings.TrimPrefix(path.Ext(current), ".")
		if language == "md" {
			language = "markdown"
		}
		file := contracts.File{ID: id, ProjectID: project, Path: current, Name: parts[i-1], Kind: kind, Language: language, ParentID: parent, Version: 1}
		if kind == "file" {
			file.Content = content
		}
		snapshot.Files = append(snapshot.Files, file)
		parent = id
	}
}

func mergeAgentChange(snapshot *contracts.Snapshot, change contracts.Change) {
	if change.OldContent == change.NewContent {
		return
	}
	for fi := range snapshot.Files {
		if snapshot.Files[fi].ProjectID == change.ProjectID && snapshot.Files[fi].Path == change.Path {
			snapshot.Files[fi].Modified = true
		}
	}
	for i, existing := range snapshot.Changes {
		if existing.ProjectID == change.ProjectID && existing.Path == change.Path {
			change.ID = existing.ID
			change.OldContent = existing.OldContent
			change.Additions, change.Deletions, change.Hunks = contiguousDiff(change.OldContent, change.NewContent)
			snapshot.Changes[i] = change
			return
		}
	}
	snapshot.Changes = append(snapshot.Changes, change)
}
