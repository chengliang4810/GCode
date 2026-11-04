package desktop

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

// A rail tick belongs to a user entry and targets its block-list row, rather
// than its message index. The cache rebuilds it with the row projection.
type messageRailTick struct {
	ID            string
	Row           int
	Prompt, Reply string
	handle        ui.Handle
}

func messageRailTicks(entries []contracts.Message, rows []conversationRow) []messageRailTick {
	userRows := make(map[int]int)
	for index, row := range rows {
		if row.User {
			userRows[row.Message] = index
		}
	}
	var ticks []messageRailTick
	current := -1
	for index, entry := range entries {
		if entry.Role == "user" {
			row, found := userRows[index]
			current = -1
			if !found {
				continue
			}
			var text, attachments []string
			for _, block := range entry.Blocks {
				if block.Kind == "text" && strings.TrimSpace(block.Text) != "" {
					text = append(text, block.Text)
				}
				if block.Attachment != nil && block.Attachment.Name != "" {
					attachments = append(attachments, block.Attachment.Name)
				}
			}
			if len(text) == 0 {
				text = attachments
			}
			ticks = append(ticks, messageRailTick{
				ID: entry.ID, Row: row, Prompt: railPreviewText(strings.Join(text, "\n\n"), 160),
			})
			current = len(ticks) - 1
		} else if entry.Role == "assistant" && current >= 0 && ticks[current].Reply == "" {
			for _, block := range entry.Blocks {
				if block.Kind == "text" && strings.TrimSpace(block.Text) != "" {
					ticks[current].Reply = railPreviewText(block.Text, 200)
					break
				}
			}
		}
	}
	return ticks
}

// Normalize free text while reading only the bounded preview prefix, including
// for a large or still-streaming response.
func railPreviewText(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := make([]rune, 0, min(len(text), limit+1))
	space := false
	for _, r := range text {
		if unicode.IsSpace(r) {
			space = len(runes) > 0
			continue
		}
		if space {
			runes = append(runes, ' ')
			space = false
		}
		runes = append(runes, r)
		if len(runes) > limit {
			return strings.TrimSpace(string(runes[:limit-1])) + "…"
		}
	}
	return string(runes)
}

type messageRailBucket struct{ Start, End int }

func messageRailBuckets(count int, height float32) []messageRailBucket {
	if count == 0 {
		return nil
	}
	s := design.MessageRail()
	capacity := max(1, int((max(s.SlotHeight, height-2*s.VerticalMargin)+s.Gap)/(s.SlotHeight+s.Gap)))
	capacity = min(capacity, s.MaxMarks, count)
	buckets := make([]messageRailBucket, capacity)
	for index := range buckets {
		buckets[index] = messageRailBucket{Start: index * count / capacity, End: (index + 1) * count / capacity}
	}
	return buckets
}

func activeMessageRailTick(ticks []messageRailTick, topRow int) int {
	return max(0, sort.Search(len(ticks), func(i int) bool { return ticks[i].Row > topRow })-1)
}

func (cache *transcriptCache) bindMessageRailRow(row ui.Element, index int) {
	mark := sort.Search(len(cache.rail), func(i int) bool { return cache.rail[i].Row >= index })
	if mark < len(cache.rail) && cache.rail[mark].Row == index {
		row.Bind(&cache.rail[mark].handle)
	}
}

func (a *App) messageRail(c *ui.Context, ticks []messageRailTick) {
	s, viewport := design.MessageRail(), a.messages.Bounds(c)
	reserve, _ := a.infoVisibility()
	if len(ticks) < 2 || viewport.W-reserve < s.MinContainerWidth || viewport.H <= 0 {
		return
	}
	first, _ := a.messages.Visible()
	active := activeMessageRailTick(ticks, first)
	// ScrollTo aligns a row with the list's content edge. The preceding row
	// can still touch the raw clip edge, so credit prompts at the padded
	// reading line, just as the reference credits the line below its chrome.
	readingLine := viewport.Y + transcriptListTopPadding + .5
	for index := active + 1; index < len(ticks); index++ {
		bounds := ticks[index].handle.Bounds(c)
		if bounds.W <= 0 || bounds.H <= 0 || bounds.Y > readingLine {
			break
		}
		if bounds.Y+bounds.H > viewport.Y {
			active = index
		}
	}
	ui.Column(c.Key("message-rail")).Absolute().Left(s.Left).Top(0).Width(s.Width).Height(viewport.H).
		Justify(ui.Center).AlignItems(ui.Start).Gap(s.Gap).Role(ui.RoleGroup).Label(tr(c, "Conversation outline")).Children(func() {
		var previewAnchor ui.Element
		var previewPrompt, previewReply, previewCount string
		hovered := false
		for index, bucket := range messageRailBuckets(len(ticks), viewport.H) {
			representative := bucket.Start
			selected := active >= bucket.Start && active < bucket.End
			if selected {
				representative = active
			}
			tick := ticks[representative]
			prompt := tick.Prompt
			if prompt == "" {
				prompt = tr(c, "Attachments")
			}
			label := fmt.Sprintf(tr(c, "Go to message %d: %s"), representative+1, prompt)
			button := controlMessageRailTick(c.Key(fmt.Sprintf("message-rail-tick:%d", index)), label, selected).
				Description(prompt + "\n" + tick.Reply)
			button.OnClick(func() { a.messages.ScrollTo(tick.Row, ui.Start) })
			if button.Hovered() || (!hovered && button.FocusVisible()) {
				previewAnchor, previewPrompt, previewReply = button, prompt, tick.Reply
				previewCount = ""
				if bucket.End-bucket.Start > 1 {
					previewCount = fmt.Sprintf(tr(c, "%d prompts"), bucket.End-bucket.Start)
				}
				hovered = hovered || button.Hovered()
			}
		}
		if previewAnchor.Valid() {
			// The source preview follows the hovered mark immediately.
			// Pointer hover takes precedence over a different focused mark.
			ui.Overlay(c, func() {
				ui.Box(c.Key("message-rail-tooltip")).Absolute().PassThrough().Role(ui.RoleTooltip).
					AttachTo(previewAnchor, ui.AnchorEnd, ui.AnchorStart).MarginStart(s.PreviewGap).SnapToWindow(8).
					Children(func() {
						controlMessageRailPreview(c.Key("message-rail-preview"), previewPrompt, previewReply, previewCount)
					})
			})
		}
	})
}
