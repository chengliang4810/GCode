package desktop

import (
	"strings"

	"gcode/internal/contracts"
)

// transcriptRows is the presentation projection of one continuation-joined
// message. It follows rows_for_entry in the pinned GPUI source: ordinary work
// forms a tree; compact mode moves narration into that tree as progress updates.
// DTO order is authoritative, including input/error rows interleaved with work.
// A standalone row references its immutable message block rather than copying it.
type transcriptRow struct {
	ID       string
	Block    *contracts.Block
	Activity []contracts.Block
	Summary  contracts.ActivitySummary
	LiveTail bool
	Compact  bool
}

// The gaps a transcript row opens above itself, following top_gap_for in the
// pinned GPUI source: a turn starts on a larger step, a tool group gets room
// on either boundary, and sibling blocks of one reply sit closest together.
const (
	transcriptTurnGap     = float32(16)
	transcriptActivityGap = float32(12)
	transcriptBlockGap    = float32(8)
)

// conversationRow is one row of the transcript list: a user message, one
// projected block of an assistant entry, or an assistant entry that projected
// no rows. The list virtualizes rows rather than messages, the way the pinned
// GPUI source does, so a long reply costs what its visible blocks cost.
type conversationRow struct {
	// Key is what the row keeps its element state by.
	Key string
	// Message is the index of the entry in the conversation, or -1.
	Message int
	// Block is the index of the entry's row this shows, or -1.
	Block int
	// User shows a user message; Bare shows an assistant entry with no rows.
	User, Bare bool
	// Last marks the entry's last row, which carries its copy and hover.
	Last     bool
	Activity bool
	TopGap   float32
}

func transcriptEntries(messages []contracts.Message) []contracts.Message {
	entries := make([]contracts.Message, 0, len(messages))
	previousID := ""
	// owned tells that the last entry's blocks are a copy this projection may
	// grow. The first join copies them away from the snapshot; later joins
	// append in place, so a turn of n messages copies its blocks once instead
	// of once per message.
	owned := false
	for _, message := range messages {
		if len(entries) > 0 && message.Role == "assistant" {
			last := &entries[len(entries)-1]
			if last.Role == "assistant" && ((message.TurnID != "" && message.TurnID == last.TurnID) || (message.ContinuationOf != "" && message.ContinuationOf == previousID)) {
				if !owned {
					blocks := make([]contracts.Block, len(last.Blocks), len(last.Blocks)+len(message.Blocks))
					copy(blocks, last.Blocks)
					last.Blocks = blocks
					owned = true
				}
				// Appending to a snapshot-owned backing array would corrupt a later
				// event; owned is that copy.
				last.Blocks = append(last.Blocks, message.Blocks...)
				last.Streaming = message.Streaming
				last.DurationMS = max(last.DurationMS, message.DurationMS)
				last.Usage.InputTokens += message.Usage.InputTokens
				last.Usage.OutputTokens += message.Usage.OutputTokens
				last.Usage.CachedTokens += message.Usage.CachedTokens
				last.Usage.Cost += message.Usage.Cost
				last.Usage.ContextTokens = message.Usage.ContextTokens
				last.Usage.ContextLimit = message.Usage.ContextLimit
				previousID = message.ID
				continue
			}
		}
		entries = append(entries, message)
		owned = false
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
			ID: message.ID + ":activity:" + work[0].ID, Activity: work, Summary: contracts.SummarizeActivity(work),
			LiveTail: message.Streaming && groupLast == len(blocks)-1,
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
	for i := range blocks {
		block := &blocks[i]
		// Checklists belong to the selected chat's composer tray. The tool
		// record remains in the activity tree, including its historical output.
		if block.Kind == "todo" || block.Kind == "todos" {
			continue
		}
		if block.Tool != nil && block.Tool.IsQuestion() {
			if !block.Tool.Failed() {
				continue
			}
			block = &contracts.Block{ID: block.ID, Kind: "error", Text: block.Tool.Output}
		}
		ordinary := block.Tool != nil || block.Hook != nil || block.Kind == "reasoning"
		note := compact && block.Kind == "text" && (replyStart < 0 || i < replyStart)
		spawn := compact && block.Subagent != nil
		if ordinary || note || spawn {
			if (block.Kind == "text" || block.Kind == "reasoning") && strings.TrimSpace(block.Text) == "" {
				continue
			}
			if compact && groupPosition < 0 {
				groupPosition = len(rows)
			}
			work = append(work, *block)
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
		group := transcriptRow{ID: message.ID + ":activity:" + work[0].ID, Activity: work, Summary: contracts.SummarizeActivity(work), Compact: true}
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
	var result []contracts.Block
	var positions map[string]int
	for index, block := range blocks {
		if block.Tool != nil && block.Tool.ID != "" {
			if i, exists := positions[block.Tool.ID]; exists {
				if result == nil {
					result = make([]contracts.Block, index, len(blocks)-1)
					copy(result, blocks[:index])
				}
				block.ID = result[i].ID
				result[i] = block
				continue
			}
			if positions == nil {
				positions = make(map[string]int)
			}
			position := index
			if result != nil {
				position = len(result)
			}
			positions[block.Tool.ID] = position
		}
		if result != nil {
			result = append(result, block)
		}
	}
	if result == nil {
		return blocks
	}
	return result
}

// transcriptCache projects immutable message snapshots. A content delta that
// leaves the last entry's row structure intact can update it in place.
type transcriptCache struct {
	revision uint64
	session  string
	prefix   string
	compact  bool
	source   []contracts.Message
	entries  []contracts.Message
	// Only the newest model round's trailing text can still receive deltas.
	// Streaming on the joined entry instead describes the entire active turn.
	streamingText string
	rows          map[string]transcriptProjection
	tail          transcriptProjection
	flat          []conversationRow
	rail          []messageRailTick
}

// transcriptProjection is an entry's block rows and the fingerprint of the
// content they were made from.
type transcriptProjection struct {
	id    string
	stamp uint64
	rows  []transcriptRow
}

const transcriptCacheLimit = 512

// fnvMix hashes a string without converting it to a byte slice.
func fnvMix(hash uint64, text string) uint64 {
	const prime = 1099511628211
	for i := 0; i < len(text); i++ {
		hash ^= uint64(text[i])
		hash *= prime
	}
	return hash
}

// fnvMixInt hashes an integer without formatting it.
func fnvMixInt(hash uint64, value int) uint64 {
	const prime = 1099511628211
	hash ^= uint64(value)
	hash *= prime
	hash ^= hash >> 32
	return hash
}

// transcriptStamp hashes what a message's row projection reads, without
// reading long text or tool output bodies.
func transcriptStamp(stamp uint64, message contracts.Message, compact bool) uint64 {
	hash := fnvMix(stamp, message.ID)
	if compact {
		hash ^= 1
	}
	if message.Streaming {
		hash ^= 2
	}
	hash = fnvMixInt(hash, int(message.DurationMS))
	hash = fnvMixInt(hash, len(message.Blocks))
	for i := range message.Blocks {
		block := &message.Blocks[i]
		hash = fnvMix(hash, block.ID)
		hash = fnvMix(hash, block.Kind)
		hash = fnvMixInt(hash, len(block.Text))
		if hook := block.Hook; hook != nil {
			hash = fnvMix(hash, hook.Name)
			hash = fnvMix(hash, string(hook.Event))
			hash = fnvMix(hash, hook.Status)
			hash = fnvMixInt(hash, len(hook.Details))
		}
		if tool := block.Tool; tool != nil {
			hash = fnvMix(hash, tool.ID)
			hash = fnvMix(hash, string(tool.Kind))
			hash = fnvMix(hash, tool.Name)
			if tool.Failed() {
				hash = fnvMixInt(hash, 1)
			}
			if tool.Diff != nil {
				hash = fnvMix(hash, tool.Diff.Path)
			}
			hash = fnvMix(hash, tool.Status)
			hash = fnvMix(hash, tool.Summary)
			hash = fnvMix(hash, tool.Command)
			hash = fnvMix(hash, tool.Path)
			hash = fnvMix(hash, tool.TaskID)
			hash = fnvMix(hash, tool.TaskStatus)
			hash = fnvMixInt(hash, len(tool.Output))
			hash = fnvMixInt(hash, len(tool.Args))
		}
		if block.Subagent != nil {
			hash = fnvMix(hash, block.Subagent.ID)
		}
		if question := block.Question; question != nil {
			hash = fnvMix(hash, question.ID)
			if question.Resolved {
				hash = fnvMixInt(hash, 1)
			}
			if len(question.Pages) > 0 {
				hash = fnvMix(hash, question.Pages[0].Header)
			}
		}
	}
	return hash
}

func (cache *transcriptCache) entriesFor(revision uint64, session string, messages []contracts.Message, compact bool) []contracts.Message {
	if cache.entries != nil && cache.revision == revision && cache.session == session &&
		cache.compact == compact && sameTranscriptSource(cache.source, messages) {
		return cache.entries
	}
	if cache.session != session {
		cache.rows = nil
	}
	cache.revision, cache.session, cache.source, cache.compact = revision, session, messages, compact
	cache.entries, cache.flat, cache.rail = transcriptEntries(messages), nil, nil
	if cache.prefix != "" {
		for i := range cache.entries {
			cache.entries[i].ID = cache.prefix + cache.entries[i].ID
		}
	}
	cache.tail = transcriptProjection{}
	cache.updateStreamingText(messages)
	return cache.entries
}

func sameTranscriptSource(a, b []contracts.Message) bool {
	return len(a) == len(b) && (len(a) == 0 || &a[0] == &b[0])
}

func (cache *transcriptCache) updateStreamingText(messages []contracts.Message) {
	cache.streamingText = ""
	for i := len(messages) - 1; i >= 0; i-- {
		message := &messages[i]
		if message.Role != "assistant" {
			continue
		}
		if message.Streaming && len(message.Blocks) > 0 {
			last := &message.Blocks[len(message.Blocks)-1]
			if last.Kind == "text" {
				cache.streamingText = last.ID
			}
		}
		break
	}
}

// updateTail accepts only a change to the final assistant message of the
// exact source we rendered. A snapshot replacement, optimistic message
// overlay, or change to the row topology must use the normal projection.
func (cache *transcriptCache) updateTail(revision uint64, before, after []contracts.Message) {
	if len(after) == 0 || len(cache.entries) == 0 || cache.flat == nil || !sameTranscriptSource(cache.source, before) {
		return
	}
	last := len(after) - 1
	entry := after[last]
	if entry.Role != "assistant" {
		return
	}
	tailIndex := len(cache.entries) - 1
	tailID := cache.entries[tailIndex].ID
	rawID := strings.TrimPrefix(tailID, cache.prefix)
	if entry.ID != rawID {
		first := last - 1
		for first >= 0 && after[first].ID != rawID {
			first--
		}
		if first < 0 {
			return
		}
		joined := transcriptEntries(after[first:])
		if len(joined) != 1 {
			return
		}
		entry = joined[0]
	}
	entry.ID = tailID
	rows := transcriptRows(entry, cache.compact)
	if cache.tail.id != tailID || len(rows) != len(cache.tail.rows) {
		return
	}
	for i, row := range rows {
		if old := cache.tail.rows[i]; row.ID != old.ID || (len(row.Activity) > 0) != (len(old.Activity) > 0) {
			return
		}
	}
	cache.entries[tailIndex] = entry
	cache.tail = transcriptProjection{id: tailID, stamp: transcriptStamp(0, entry, cache.compact), rows: rows}
	cache.source, cache.revision = after, revision
	cache.updateStreamingText(after)
	if n := len(cache.rail); n > 0 {
		tick := &cache.rail[n-1]
		tick.Reply = ""
		for _, reply := range cache.entries[cache.flat[tick.Row].Message+1:] {
			if reply.Role == "assistant" {
				tick.Reply = railReplyPreview(reply.Blocks)
				if tick.Reply != "" {
					break
				}
			}
		}
	}
}

// rowsFor returns the entry's block rows, projecting them again only when
// its content changed since they were made, and keeps them for the row builder
// to read by ID.
func (cache *transcriptCache) rowsFor(entry contracts.Message, compact bool) []transcriptRow {
	stamp := transcriptStamp(0, entry, compact)
	if n := len(cache.entries); n > 0 && cache.entries[n-1].ID == entry.ID {
		if cache.tail.id == entry.ID && cache.tail.stamp == stamp {
			return cache.tail.rows
		}
		rows := transcriptRows(entry, compact)
		cache.tail = transcriptProjection{id: entry.ID, stamp: stamp, rows: rows}
		return rows
	}
	if cache.rows == nil {
		cache.rows = map[string]transcriptProjection{}
	}
	if cached, ok := cache.rows[entry.ID]; ok && cached.stamp == stamp {
		return cached.rows
	}
	rows := transcriptRows(entry, compact)
	if _, known := cache.rows[entry.ID]; known || len(cache.rows) < transcriptCacheLimit {
		cache.rows[entry.ID] = transcriptProjection{stamp: stamp, rows: rows}
	}
	return rows
}

// rowsOf returns the rows the last projection made for the entry.
func (cache *transcriptCache) rowsOf(entryID string) []transcriptRow {
	if cache.tail.id == entryID {
		return cache.tail.rows
	}
	return cache.rows[entryID].rows
}

// conversationFor returns the rows the entries flatten to, kept until the
// conversation's content changes.
func (cache *transcriptCache) conversationFor(entries []contracts.Message, compact bool) []conversationRow {
	if cache.flat != nil {
		return cache.flat
	}
	cache.flat = conversationRows(entries, func(entry contracts.Message) []transcriptRow {
		return cache.rowsFor(entry, compact)
	})
	cache.rail = messageRailTicks(entries, cache.flat)
	return cache.flat
}

// conversationRows flattens the entries into the rows a transcript list shows:
// one row per user message, one per block of an assistant message, and one for
// an assistant message whose projection has no rows of its own.
func conversationRows(entries []contracts.Message, rowsOf func(contracts.Message) []transcriptRow) []conversationRow {
	rows := make([]conversationRow, 0, len(entries))
	for index, entry := range entries {
		if entry.Role != "assistant" {
			rows = append(rows, conversationRow{Key: entry.ID, Message: index, Block: -1, User: true, Last: true})
			continue
		}
		blocks := rowsOf(entry)
		if len(blocks) == 0 {
			rows = append(rows, conversationRow{Key: entry.ID, Message: index, Block: -1, Bare: true, Last: true})
			continue
		}
		for i := range blocks {
			rows = append(rows, conversationRow{
				Key: blocks[i].ID, Message: index, Block: i,
				Last: i == len(blocks)-1, Activity: len(blocks[i].Activity) > 0,
			})
		}
	}
	for i := 1; i < len(rows); i++ {
		previous := rows[i-1]
		switch {
		case previous.Message != rows[i].Message:
			rows[i].TopGap = transcriptTurnGap
		case previous.Activity || rows[i].Activity:
			rows[i].TopGap = transcriptActivityGap
		default:
			rows[i].TopGap = transcriptBlockGap
		}
	}
	return rows
}

func activityTitle(block contracts.Block) (label, detail, glyph string) {
	if block.Hook != nil {
		return "Hook", block.Hook.Name, "widget"
	}
	if block.Kind == "reasoning" {
		return "Thought process", "", "chat-round-line"
	}
	if block.Kind == "text" {
		lines := thoughtLines(block.Text, false)
		for _, line := range lines {
			if text := strings.TrimSpace(thoughtPlain(line)); text != "" {
				return "Progress update", text, "pen"
			}
		}
		return "Progress update", "", "pen"
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
		return "Search", firstNonempty(t.Summary, t.Path), "magnifer"
	case contracts.ToolGlob:
		return "Glob", firstNonempty(t.Summary, t.Path), "folder-with-files"
	case contracts.ToolDirectory:
		return "List directory", firstNonempty(t.Summary, t.Path), "folder-with-files"
	case contracts.ToolWebFetch:
		return "Read web page", firstNonempty(t.Summary, t.Path), "global"
	case contracts.ToolWebSearch:
		return "Search web", t.Summary, "global"
	case contracts.ToolTodo:
		return "Todo", firstNonempty(t.Summary, t.Args), "checklist"
	case contracts.ToolTaskOutput:
		// 行里读给用户看的是任务名或命令行；任务编号是数据，不是描述。
		return "Read task output", firstNonempty(t.Summary, t.Command), "terminal"
	case contracts.ToolTaskStop:
		return "Stop task", t.Summary, "terminal"
	case contracts.ToolSkill:
		return "Use skill", t.Summary, "widget"
	case contracts.ToolGoalRead:
		return "Read goal", t.Summary, "checklist"
	case contracts.ToolWorkflow:
		return workflowToolTitle(t), t.Summary, "git-branch"
	case contracts.ToolAgent:
		return "Start agent", t.Summary, "bot"
	case contracts.ToolMCP:
		return "Call tool", t.Summary, "widget"
	default:
		switch t.Name {
		case "ContextStatus":
			return "View context status", "", "widget"
		case "CompressContext":
			return "Compress context", "", "widget"
		case "SearchContext":
			return "Search context history", "", "widget"
		case "DecompressContext":
			return "Read archived context", "", "widget"
		}
		return "Tool", firstNonempty(t.Name, t.Summary), "widget"
	}
}

// workflowToolTitle 给工作流工具行一个短标题。工具行说明这一次工具动作，
// 不复制运行图：阶段线与状态属于运行卡和详情。
func workflowToolTitle(tool *contracts.ToolCall) string {
	switch tool.Name {
	case "CreateWorkflow":
		return "Start workflow"
	case "AmendWorkflow":
		return "Amend workflow"
	case "SaveWorkflow":
		return "Save workflow"
	case "ListSavedWorkflows":
		return "List saved workflows"
	case "EvalWorkflowSnippet":
		return "Check workflow script"
	case "ListWorkflowRuns":
		return "List workflow runs"
	case "GetWorkflowRun":
		return "Read workflow run"
	case "ResumeWorkflowRun":
		return "Resume workflow run"
	case "ResolveWorkflowQuestion":
		return "Answer workflow question"
	default:
		return "Fill workflow hole"
	}
}

// reasoningLead 是流式思考行可以稳定引用的名字：第一段已经写完（以换行
// 收尾）的文字。首段还没写完时返回空串，行只报动作词，不念块编号，也不
// 拿还在追加的半个句子当名字。
func reasoningLead(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if i == len(lines)-1 && !strings.HasSuffix(text, "\n") {
			return ""
		}
		return strings.Join(strings.Fields(strings.ReplaceAll(line, "**", "")), " ")
	}
	return ""
}

func reasoningPreview(text string, live bool) string {
	preview := ""
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		preview = strings.Join(strings.Fields(strings.ReplaceAll(line, "**", "")), " ")
		if !live {
			break
		}
	}
	return preview
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
