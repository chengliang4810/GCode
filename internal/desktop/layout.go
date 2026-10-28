package desktop

import "gcode/internal/design"

type panelAllocation struct {
	Sidebar, Right, Files bool
	SidebarLimit          float32
}

type panelWidths struct {
	Sidebar, Chat, Right, Files float32
}

// allocatePanels ports the settled files_panel_layout/right_target allocation.
// Fitting may squeeze a preferred width, but never overwrites that preference.
func allocatePanels(viewport, sidebar, files, right float32, fit panelAllocation, expanded bool) panelWidths {
	var width panelWidths
	if fit.Sidebar {
		width.Sidebar = min(max(sidebar, design.SidebarMin), min(design.SidebarMax, fit.SidebarLimit))
	}
	available := max(0, viewport-width.Sidebar)
	if fit.Files {
		floor := design.ChatMin
		if fit.Right {
			floor += design.RightMin
		}
		width.Files = min(max(files, design.FilesMin), min(design.FilesMax, max(0, available-floor)))
	}
	available -= width.Files
	if fit.Right {
		if expanded {
			width.Right = available
		} else {
			width.Right = min(max(right, design.RightMin), max(0, available-design.ChatMin))
		}
	}
	width.Chat = max(0, available-width.Right)
	return width
}

// fitPanels ports horizontal_panel_fit from the frozen GPUI implementation.
// Visibility never changes the user's open flags; fit-hidden panes return later.
func fitPanels(viewport, sidebarWidth float32, open [3]bool, opened [3]uint64, expanded bool) panelAllocation {
	visible := open
	narrow := expanded && open[1] && viewport < design.ChatMin+design.RightMin
	if narrow {
		visible[0] = false
		visible[2] = false
	}
	for {
		chat := design.ChatMin
		if visible[1] && narrow {
			chat = 0
		}
		minimum := chat
		if visible[0] {
			minimum += design.SidebarMin
		}
		if visible[1] {
			minimum += design.RightMin
		}
		if visible[2] {
			minimum += design.FilesMin
		}
		if minimum <= max(viewport, 0) {
			available := viewport - chat
			if visible[1] {
				available -= design.RightMin
			}
			if visible[2] {
				available -= design.FilesMin
			}
			return panelAllocation{visible[0], visible[1], visible[2], min(sidebarWidth, available)}
		}
		victim := -1
		for i, v := range visible {
			if !v {
				continue
			}
			if victim < 0 || (expanded && victim == 1 && i != 1) || ((!expanded || i != 1) && (opened[i] < opened[victim])) {
				victim = i
			}
		}
		if victim < 0 {
			return panelAllocation{}
		}
		visible[victim] = false
	}
}
