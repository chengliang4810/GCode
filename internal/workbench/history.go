package workbench

import (
	"slices"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/design"

	"github.com/egoist/mygo/ui"
)

// The topological lane transition follows Zeron's history.rs layout_graph.
// Lane identities survive merges independently of their current screen column.
type historyLane struct {
	id, color int
	target    string
}
type historySegment struct {
	from, to, color int
	shape           string
}
type historyRow struct {
	column, color int
	segments      []historySegment
}

func layoutHistory(commits []contracts.Commit) ([]historyRow, int) {
	var lanes []historyLane
	nextID, nextColor, maxColumns := 0, 0, 0
	rows := make([]historyRow, 0, len(commits))
	indexOf := func(lanes []historyLane, id int) int {
		for i, l := range lanes {
			if l.id == id {
				return i
			}
		}
		return 0
	}
	for _, commit := range commits {
		before := slices.Clone(lanes)
		var incoming []int
		resolved := map[int]bool{}
		for i, l := range before {
			if l.target == commit.SHA {
				incoming = append(incoming, i)
				resolved[l.id] = true
			}
		}
		column, color := len(before), nextColor
		primaryID := -1
		if len(incoming) > 0 {
			column = incoming[0]
			color = before[column].color
			primaryID = before[column].id
		} else {
			nextColor++
		}
		next := make([]historyLane, 0, len(before)+len(commit.Parents))
		for _, l := range before {
			if !resolved[l.id] {
				next = append(next, l)
			}
		}
		var outgoing []historyLane
		if len(commit.Parents) > 0 {
			if primaryID < 0 {
				primaryID = nextID
				nextID++
			}
			l := historyLane{primaryID, color, commit.Parents[0]}
			next = slices.Insert(next, min(column, len(next)), l)
			outgoing = append(outgoing, l)
		}
		offset := 1
		for _, parent := range commit.Parents[min(1, len(commit.Parents)):] {
			found := false
			for _, l := range next {
				if l.target == parent {
					outgoing = append(outgoing, l)
					found = true
					break
				}
			}
			if found {
				continue
			}
			l := historyLane{nextID, nextColor, parent}
			nextID++
			nextColor++
			primary := min(column, len(next))
			if primaryID >= 0 {
				primary = indexOf(next, primaryID)
			}
			next = slices.Insert(next, min(primary+offset, len(next)), l)
			offset++
			outgoing = append(outgoing, l)
		}
		row := historyRow{column: column, color: color}
		for from, l := range before {
			if !resolved[l.id] {
				row.segments = append(row.segments, historySegment{from, indexOf(next, l.id), l.color, "through"})
			}
		}
		for _, from := range incoming {
			row.segments = append(row.segments, historySegment{from, column, before[from].color, "incoming"})
		}
		for _, l := range outgoing {
			row.segments = append(row.segments, historySegment{column, indexOf(next, l.id), l.color, "outgoing"})
		}
		maxColumns = max(maxColumns, len(before), len(next), column+1)
		rows = append(rows, row)
		lanes = next
	}
	return rows, maxColumns
}

func historyGraph(c *ui.Context, row historyRow, columns int, head bool, p design.Tokens) {
	colors := []ui.Color{p.Accent, p.ANSI[6], p.Success, p.Warning, p.Danger, p.ANSI[5]}
	width := max(float32(48), float32(columns)*12+18)
	ui.Box(c).Width(width).Height(36).Draw(func(painter *ui.Painter, r ui.Rect) {
		x := func(lane int) float32 { return r.X + 12 + float32(lane)*12 }
		for _, segment := range row.segments {
			y0, y1 := r.Y, r.Y+36
			if segment.shape == "incoming" {
				y1 = r.Y + 18
			}
			if segment.shape == "outgoing" {
				y0 = r.Y + 18
			}
			line := (&ui.Path{}).MoveTo(x(segment.from), y0)
			middle := (y0 + y1) / 2
			line.CubeTo(x(segment.from), middle, x(segment.to), middle, x(segment.to), y1)
			painter.StrokePath(line, 1.5, colors[segment.color%len(colors)])
		}
		cx, cy := x(row.column), r.Y+18
		painter.Fill(ui.Rect{X: cx - 3, Y: cy - 3, W: 6, H: 6}, colors[row.color%len(colors)], 3)
		if head {
			painter.Stroke(ui.Rect{X: cx - 5, Y: cy - 5, W: 10, H: 10}, colors[row.color%len(colors)], 5, 1.5)
		}
	})
}

func (s *State) historyView(c *ui.Context) {
	p := s.palette()
	rows, columns := layoutHistory(s.snapshot.Commits)
	graphWidth := max(float32(48), float32(columns)*12+18)
	ui.Row(c).Height(36).Padding(0, 12).AlignItems(ui.Center).Gap(8).Children(func() {
		ui.Text(c, "History").FontWeight(500)
		ui.Spacer(c)
		ui.TextInputBase(c, &s.historySearch).Label("Search commits").Placeholder("Search commits").Width(196).Padding(5, 8).Radius(6).Background(p.Input)
	})
	ui.Row(c).Height(28).Padding(0, 8).TextColor(p.Muted).FontSize(design.UISize(c, 11)).BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
		ui.Box(c).Width(graphWidth)
		ui.Text(c, "Commit").Grow(1)
		ui.Text(c, "Author").Width(88)
		ui.Text(c, "Date").Width(88)
		ui.Text(c, "SHA").Width(74)
	})
	ui.Scroll(c).Grow(1).FillWidth().Padding(0, 8).Children(func() {
		for i, commit := range s.snapshot.Commits {
			if !strings.Contains(strings.ToLower(commit.SHA+commit.Subject+commit.Author), strings.ToLower(s.historySearch)) {
				continue
			}
			c.Key(commit.SHA)
			ui.ButtonBase(c).Label(commit.Subject).FillWidth().Height(36).OnClick(func() { s.commitID = commit.SHA; s.commitOpen = true }).Children(func() {
				historyGraph(c, rows[i], columns, i == 0, p)
				ui.Row(c).Grow(1).Gap(5).AlignItems(ui.Center).Children(func() {
					for _, ref := range commit.Refs {
						ui.Text(c, ref).FontSize(design.UISize(c, 10.5)).Padding(2, 5).Radius(4).Background(p.Active).TextColor(p.Accent).SingleLine().MaxWidth(112)
					}
					ui.Text(c, commit.Subject).Grow(1).SingleLine()
				})
				ui.Text(c, commit.Author).Width(88).SingleLine().TextColor(p.Muted).FontSize(design.UISize(c, 11))
				ui.Text(c, commit.AuthoredAt.Format("Jan 2")).Width(88).SingleLine().TextColor(p.Muted).FontSize(design.UISize(c, 11))
				ui.Text(c, shortSHA(commit.SHA)).Width(74).Font("Geist Mono").FontSize(design.UISize(c, 10.5)).TextColor(p.Muted)
			}).ContextMenu(func(m *ui.Menu) {
				if m.Item("Copy commit SHA").Chosen() {
					c.Services().WriteClipboard(commit.SHA)
				}
			})
		}
	})
	ui.Modal(c, &s.commitOpen, func() {
		for _, commit := range s.snapshot.Commits {
			if commit.SHA != s.commitID {
				continue
			}
			ui.Text(c, commit.Subject).FontSize(design.UISize(c, 18)).FontWeight(600)
			ui.Text(c, commit.SHA).Font("Geist Mono").FontSize(design.UISize(c, 11)).Selectable()
			ui.Text(c, commit.Author+" · "+commit.AuthoredAt.Format("Jan 2, 2006 15:04"))
			ui.Text(c, "Parents: "+strings.Join(commit.Parents, ", ")).FontSize(design.UISize(c, 11)).Selectable()
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				ui.Button(c, "Copy SHA").OnClick(func() { c.Services().WriteClipboard(commit.SHA) })
				ui.Button(c, "Close").OnClick(func() { s.commitOpen = false })
			})
		}
	})
}
