package desktop

import (
	"fmt"
	"strings"

	"gcode/internal/contracts"
)

// transcriptRows is the presentation projection of one continuation-joined
// message. It follows rows_for_entry in the pinned GPUI source: ordinary work
// forms a tree; compact mode moves narration into that same tree as Wrote rows.
// DTO order is authoritative, including input/error rows interleaved with work.
type transcriptRow struct {
	ID       string
	Block    contracts.Block
	Activity []contracts.Block
	AutoOpen bool
	Compact  bool
}

func transcriptEntries(messages []contracts.Message) []contracts.Message {
	entries := make([]contracts.Message, 0, len(messages))
	previousID := ""
	for _, message := range messages {
		if len(entries) > 0 && message.Role == "assistant" {
			last := &entries[len(entries)-1]
			if last.Role == "assistant" && ((message.TurnID != "" && message.TurnID == last.TurnID) || (message.ContinuationOf != "" && message.ContinuationOf == previousID)) {
				// Appending to a snapshot-owned backing array would corrupt a later
				// event. A joined presentation entry owns its block slice.
				blocks := make([]contracts.Block, 0, len(last.Blocks)+len(message.Blocks))
				blocks = append(blocks, last.Blocks...)
				last.Blocks = append(blocks, message.Blocks...)
				last.Streaming = message.Streaming
				last.DurationMS = max(last.DurationMS, message.DurationMS)
				last.Usage = message.Usage
				previousID = message.ID
				continue
			}
		}
		entries = append(entries, message)
		previousID = message.ID
	}
	return entries
}

func transcriptRows(message contracts.Message, compact bool) []transcriptRow {
	blocks := resolvedActivity(message.Blocks)
	rows := make([]transcriptRow, 0, len(blocks))
	var work []contracts.Block
	groupPosition, groupLast := -1, -1
	flush := func() {
		if len(work) == 0 {
			return
		}
		rows = append(rows, transcriptRow{
			ID: message.ID + ":activity:" + work[0].ID, Activity: work,
			AutoOpen: !compact && message.Streaming && groupLast == len(blocks)-1,
			Compact:  compact,
		})
		work = nil
	}
	// This deliberately uses the source's trailing text-run boundary, not the
	// last tool position: a generated image may follow a final text reply.
	replyStart := -1
	if compact && !message.Streaming {
		for i := len(blocks) - 1; i >= 0; i-- {
			if blocks[i].Kind == "text" && strings.TrimSpace(blocks[i].Text) != "" {
				replyStart = i
				for replyStart > 0 && blocks[replyStart-1].Kind == "text" && strings.TrimSpace(blocks[replyStart-1].Text) != "" {
					replyStart--
				}
				break
			}
		}
	}
	for i, block := range blocks {
		ordinary := block.Tool != nil || block.Kind == "reasoning"
		note := compact && block.Kind == "text" && (replyStart < 0 || i < replyStart)
		spawn := compact && block.Subagent != nil
		if ordinary || note || spawn {
			if (block.Kind == "text" || block.Kind == "reasoning") && strings.TrimSpace(block.Text) == "" {
				continue
			}
			if compact && groupPosition < 0 {
				groupPosition = len(rows)
			}
			work = append(work, block)
			groupLast = i
			continue
		}
		if !compact {
			flush()
		}
		if (block.Kind == "text" || block.Kind == "reasoning") && strings.TrimSpace(block.Text) == "" {
			continue
		}
		rows = append(rows, transcriptRow{ID: message.ID + ":" + block.ID, Block: block})
	}
	if compact && len(work) > 0 {
		group := transcriptRow{ID: message.ID + ":activity:" + work[0].ID, Activity: work, Compact: true}
		rows = append(rows, transcriptRow{})
		copy(rows[groupPosition+1:], rows[groupPosition:])
		rows[groupPosition] = group
	} else {
		flush()
	}
	return rows
}

// Some adapters emit a separate result block rather than updating their call.
// Retain the first timeline position while rendering the latest typed payload.
func resolvedActivity(blocks []contracts.Block) []contracts.Block {
	result := make([]contracts.Block, 0, len(blocks))
	positions := make(map[string]int)
	for _, block := range blocks {
		if block.Tool != nil && block.Tool.ID != "" {
			if i, exists := positions[block.Tool.ID]; exists {
				block.ID = result[i].ID
				result[i] = block
				continue
			}
			positions[block.Tool.ID] = len(result)
		}
		result = append(result, block)
	}
	return result
}

func activityTitle(block contracts.Block) (label, detail, glyph string) {
	if block.Kind == "reasoning" {
		return "Thought process", "", "chat-round-line"
	}
	if block.Kind == "text" {
		lines, _ := thoughtLines(block.Text, false)
		for _, line := range lines {
			if text := strings.TrimSpace(thoughtPlain(line)); text != "" {
				return "Wrote", text, "pen"
			}
		}
		return "Wrote", "", "pen"
	}
	if block.Subagent != nil {
		return "Agent", block.Subagent.Name, "bot"
	}
	if block.Tool == nil {
		return "Tool", "", "widget"
	}
	t := block.Tool
	switch t.Category() {
	case contracts.ToolExec:
		return "Run", firstNonempty(t.Command, t.Args, t.Summary), "terminal"
	case contracts.ToolReadFile:
		return "Read", t.Path, "document"
	case contracts.ToolWriteFile:
		return "Write", t.Path, "document-add"
	case contracts.ToolEditFile:
		return "Edit", t.Path, "pen"
	case contracts.ToolApplyPatch:
		return "Patch", firstNonempty(t.Path, "workspace"), "document"
	case contracts.ToolSearch:
		return "Search", firstNonempty(t.Summary, t.Args, t.Path), "magnifer"
	case contracts.ToolGlob:
		return "Glob", firstNonempty(t.Summary, t.Args, t.Path), "folder-with-files"
	case contracts.ToolWebFetch:
		return "Fetch", firstNonempty(t.Summary, t.Args), "global"
	case contracts.ToolWebSearch:
		return "Web", firstNonempty(t.Summary, t.Args), "global"
	case contracts.ToolTodo:
		return "Todo", firstNonempty(t.Summary, t.Args), "checklist"
	default:
		return "Tool", firstNonempty(t.Name, t.Summary), "widget"
	}
}

func firstNonempty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func activityBasename(path string) string {
	parts := strings.FieldsFunc(strings.TrimRight(path, "/\\"), func(r rune) bool { return r == '/' || r == '\\' })
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return path
}

func activityFile(block contracts.Block) bool {
	if block.Tool == nil || block.Tool.Path == "" {
		return false
	}
	switch block.Tool.Category() {
	case contracts.ToolReadFile, contracts.ToolWriteFile, contracts.ToolEditFile, contracts.ToolApplyPatch:
		return true
	}
	return false
}

func workedFor(milliseconds int64) string {
	secs := max(milliseconds/1000, 0)
	var duration string
	switch {
	case secs < 60:
		duration = fmt.Sprintf("%ds", secs)
	case secs < 3600:
		duration = fmt.Sprintf("%dm %ds", secs/60, secs%60)
	case secs < 86400:
		duration = fmt.Sprintf("%dh %dm", secs/3600, secs%3600/60)
	default:
		duration = fmt.Sprintf("%dd %dh", secs/86400, secs%86400/3600)
	}
	return "Worked for " + duration
}
