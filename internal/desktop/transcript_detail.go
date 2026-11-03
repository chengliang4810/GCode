package desktop

import (
	"bytes"
	"encoding/json"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

type activityDetailSource struct {
	kind, text, command, args, path, output string
	live                                    bool
}

type activityDetailState struct {
	entry  *activityDetailEntry
	list   ui.ListState
	scroll ui.ScrollState
}

func detailSource(block contracts.Block, live bool) activityDetailSource {
	source := activityDetailSource{kind: block.Kind, text: block.Text, live: live}
	if t := block.Tool; t != nil {
		source.command, source.args, source.path, source.output = t.Command, t.Args, t.Path, t.Output
	}
	return source
}

func detailLines(source activityDetailSource) []thoughtLine {
	if source.kind == "error" {
		var lines []thoughtLine
		for _, line := range strings.Split(source.text, "\n") {
			lines = append(lines, thoughtLine{{text: line}})
		}
		return lines
	}
	if source.kind == "reasoning" || source.kind == "text" {
		return thoughtLines(source.text, source.live)
	}
	invocation := source.args
	if invocation != "" {
		var pretty bytes.Buffer
		if json.Indent(&pretty, []byte(invocation), "", "  ") == nil {
			invocation = pretty.String()
		}
	} else {
		invocation = firstNonempty(source.command, source.path)
	}
	lines := []thoughtLine{}
	appendText := func(text string) {
		// Existing logical wrapping retains every character.
		for _, line := range activityInvocation(strings.TrimRight(text, "\r\n")) {
			lines = append(lines, thoughtLine{{text: line, style: thoughtStyle{code: true}}})
		}
	}
	if invocation != "" {
		appendText(invocation)
	}
	if source.output != "" {
		if len(lines) > 0 {
			lines = append(lines, nil)
		}
		appendText(source.output)
	}
	return lines
}

// One viewport policy for thoughts, folded notes, invocation and output:
// preserve all lines, grow up to 24 rows, and build only the visible rows.
func (a *App) activityTextDetail(c *ui.Context, block contracts.Block, live bool) {
	container := ui.Column(c.Key("activity-detail:"+block.ID)).FillWidth().MinWidth(0).Padding(6, 0)
	state := ui.Local(container, "detail", func() activityDetailState { return activityDetailState{} })
	source := detailSource(block, live)
	size := design.TextSize(c, design.TextBase)
	if block.Tool != nil {
		size = float32(a.state.Settings.Appearance.CodeFontSize)
	}
	codeFont := design.MonoFont(a.state.Settings.Appearance.CodeFont)
	color := a.palette.Faint
	if block.Kind == "error" {
		color = a.palette.Danger
	}
	sessionID := firstNonempty(a.linkSource, a.sessionID)
	bodyFont := design.Font(a.state.Settings.Appearance.Font)
	if bodyFont == "" {
		bodyFont = c.Theme().Font
	}
	font := activityDetailFont{body: bodyFont, code: codeFont, size: size}
	// Oversized details stay with their mounted widget only; they must not
	// reparse every frame or evict the entire bounded reopen cache.
	if state.entry == nil || state.entry.source != source || state.entry.font != font || state.entry.bytes <= activityDetailsMaxBytes {
		state.entry = a.activityDetails.get(activityDetailKey{session: sessionID, block: block.ID}, source, font)
	}
	entry := state.entry
	spans := func(line thoughtLine) []ui.Span {
		result := detailSpans(line, codeFont, size, color)
		for i := range result {
			if result[i].Font == "" {
				result[i].Font = font.body
			}
		}
		return result
	}
	measure := func(i int) {
		if _, found := entry.measured[i]; found {
			return
		}
		width, _ := c.MeasureText(0, spans(entry.lines[i])...)
		entry.measured[i] = width
		if width+16 > entry.width {
			entry.width = width + 16
			c.Invalidate()
		}
	}
	if len(entry.lines) > 0 {
		measure(entry.representative)
	}
	height := float32(min(len(entry.lines), activityDetailLines)) * size * 1.5
	container.Children(func() {
		ui.ScrollHorizontal(c.Key("activity-detail-horizontal:" + block.ID)).TrackScroll(&state.scroll).
			FillWidth().Height(height).Children(func() {
			ui.List(c.Key("activity-detail-viewport:"+block.ID), &state.list, len(entry.lines), func(i int) {
				measure(i)
				ui.RichText(c, spans(entry.lines[i])...).
					SingleLine().FillWidth().Height(size * 1.5).FontSize(size).LineHeight(1.5).
					TextColor(color).Selectable()
			}).Width(max(entry.width, 1)).Height(height).Focusable().Selectable().Label(tr(c, "Activity detail"))
		})
	})
}

func detailSpans(line thoughtLine, codeFont string, size float32, color ui.Color) []ui.Span {
	spans := make([]ui.Span, 0, max(len(line), 1))
	for _, run := range line {
		span := ui.Span{
			Text: run.text, Size: size, Color: color,
			Italic: run.style.italic, Strikethrough: run.style.strike, Underline: run.style.link,
		}
		if run.style.bold {
			span.Weight = 600
		}
		if run.style.code {
			span.Font = codeFont
		}
		spans = append(spans, span)
	}
	if len(spans) == 0 {
		spans = append(spans, ui.Span{Text: " ", Size: size})
	}
	return spans
}
