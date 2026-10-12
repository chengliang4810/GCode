package desktop

import (
	"math"
	"path/filepath"
	"strconv"

	"gcode/internal/appmodel"
	"github.com/egoist/mygo/ui"
)

func (a *App) rightPanel(c *ui.Context, p palette, data appmodel.Snapshot, workspace WorkspaceSnapshot, workspaceRoot, workspacePath, activeID string, selected rightTab, terminal TerminalSurface, terminalLoading bool) {
	session, ok := activeSession(data.Sessions, activeID)
	if selected == rightTerminal && ok {
		a.ensureTerminal(session.Cwd)
	}
	bar := c.TitleBar()
	right := bar.Right + 12
	if right < 12 {
		right = 12
	}
	height := desktopHeaderHeight(c)
	ui.Column(c).Fill().MinWidth(rightPanelMinimumWidth).Background(p.surfaceAlt).Children(func() {
		ui.Row(c).Height(height).Padding(0, right, 0, 12).Gap(10).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(p.border).Children(func() {
			selectedIndex := int(selected)
			tabs := ui.Tabs(c.Key("right-tabs"), &selectedIndex, "文件", "Diff", "终端").Label("资源面板").Grow(1).Shrink(1).MinWidth(0)
			if tabs.Changed() {
				a.setRightTab(rightTab(selectedIndex))
			}
			if selected == rightDiff {
				a.diffHeaderSummary(c, p, workspace)
			}
			if iconButton(c, p, "x", "收起资源面板").Size(30, 30).Radius(7).Clicked() {
				a.toggleRightPanel(true, false)
			}
		})
		switch selected {
		case rightDiff:
			a.diffView(c, p, workspace)
		case rightTerminal:
			a.terminalView(c, p, session, ok, terminal, terminalLoading)
		default:
			a.filesView(c, p, workspace, workspaceRoot, workspacePath)
		}
	})
}

func (a *App) filesView(c *ui.Context, p palette, workspace WorkspaceSnapshot, workspaceRoot, workspacePath string) {
	ui.Column(c).Grow(1).Padding(12).Gap(8).Children(func() {
		ui.Row(c).Height(26).AlignItems(ui.Center).Children(func() {
			if workspaceRoot != "" && workspacePath != "" && filepath.Clean(workspaceRoot) != filepath.Clean(workspacePath) {
				if iconButton(c, p, "chevron-left", "返回上级目录").Size(24, 24).Radius(6).Clicked() {
					a.parentDirectory()
				}
			}
			sectionLabel(c, p, "工作区文件")
			ui.Spacer(c)
			if iconButton(c, p, "refresh", "刷新工作区").Size(24, 24).Radius(6).Clicked() {
				a.refreshWorkspace()
			}
			ui.Text(c, stringCount(len(workspace.Files))).FontSize(11).TextColor(p.muted)
		})
		if workspacePath != "" {
			pathLabel := workspacePath
			if workspaceRoot != "" {
				if relative, err := filepath.Rel(workspaceRoot, workspacePath); err == nil {
					pathLabel = relative
				}
			}
			if pathLabel == "." {
				pathLabel = "项目根目录"
			}
			ui.Text(c, filepath.ToSlash(pathLabel)).Font("SFMono-Regular, Menlo, Consolas, monospace").FontSize(10.5).TextColor(p.muted).SingleLine()
		}
		if workspace.Error != "" {
			errorRow(c, p, workspace.Error, func() { a.UpdateWorkspace(WorkspaceSnapshot{}) })
			return
		}
		ui.Scroll(c.Key("files-scroll")).Grow(1).Gap(2).Children(func() {
			if len(workspace.Files) == 0 {
				emptyPanel(c, p, "folder", "暂无文件数据", "打开项目后，可以在这里浏览当前会话的文件。")
			}
			for _, file := range workspace.Files {
				a.fileRow(c, p, file)
			}
		})
		if workspace.Preview != nil {
			a.preview(c, p, *workspace.Preview)
		}
	})
}

func (a *App) fileRow(c *ui.Context, p palette, file appmodel.FileEntry) {
	row := ui.ButtonBase(c.Key("file-"+file.Path)).Height(32).Padding(0, 7).Gap(7).Radius(7).AlignItems(ui.Center).
		Label("打开文件 " + file.Name).TextColor(p.text)
	if row.Hovered() {
		row = row.Background(p.surfaceAlt)
	}
	row.Children(func() {
		name := file.Name
		if name == "" {
			name = file.Path
		}
		iconName := "file"
		if file.IsDir {
			iconName = "folder"
		}
		ui.Icon(c, icon(iconName)).FontSize(15).TextColor(p.muted)
		ui.Text(c, name).FontSize(12.5).SingleLine().Grow(1)
		if !file.IsDir && file.Size > 0 {
			ui.Text(c, formatBytes(file.Size)).FontSize(10.5).TextColor(p.muted).SingleLine()
		}
	})
	if row.Clicked() {
		if file.IsDir {
			a.enterDirectory(file.Path)
		} else {
			a.readWorkspaceFile(file.Path)
		}
	}
}

func (a *App) preview(c *ui.Context, p palette, file appmodel.FilePreview) {
	ui.Column(c).Height(230).Background(p.code).Padding(10).Gap(6).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon("file")).FontSize(14).TextColor(p.codeText)
			ui.Text(c, file.Path).Font("SFMono-Regular, Menlo, Consolas, monospace").FontSize(11.5).TextColor(p.codeText).SingleLine()
			ui.Spacer(c)
			if compactAction(c, p, "external-link", "编辑").Height(26).Clicked() {
				a.openWorkspaceFile(file.Path)
			}
		})
		if file.Binary {
			ui.Text(c, "二进制文件不提供预览").FontSize(12).TextColor(p.muted)
			return
		}
		ui.Scroll(c.Key("file-preview")).Grow(1).Children(func() {
			ui.Text(c, file.Text).Font("SFMono-Regular, Menlo, Consolas, monospace").FontSize(11.5).LineHeight(1.35).TextColor(p.codeText)
		})
		if file.Truncated {
			ui.Text(c, "预览已截断").FontSize(10.5).TextColor(p.warning)
		}
	})
}

func (a *App) terminalView(c *ui.Context, p palette, session appmodel.Session, ok bool, terminal TerminalSurface, loading bool) {
	if !ok {
		emptyPanel(c, p, "square-terminal", "没有活动会话", "选择会话后，终端会使用该会话的工作目录。")
		return
	}
	if loading {
		ui.Column(c).Grow(1).Center().Gap(8).Children(func() {
			ui.Spinner(c).FontSize(22).TextColor(p.accent)
			ui.Text(c, "正在打开终端").FontSize(13).TextColor(p.muted)
		})
		return
	}
	if terminal == nil {
		emptyPanel(c, p, "square-terminal", "终端未连接", "终端工厂尚未配置，或当前工作目录不可用。")
		return
	}
	ui.Column(c).Grow(1).Background(p.code).Padding(8).Children(func() {
		ui.Row(c).Height(26).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon("square-terminal")).FontSize(14).TextColor(p.codeText)
			ui.Text(c, truncateMiddle(session.Cwd, 42)).Font("SFMono-Regular, Menlo, Consolas, monospace").FontSize(11).TextColor(p.codeText).SingleLine()
		})
		terminal.View(c).Grow(1)
	})
}

func emptyPanel(c *ui.Context, p palette, iconName, title, detail string) {
	ui.Column(c).Grow(1).Center().Gap(8).Padding(20).Children(func() {
		ui.Icon(c, icon(iconName)).FontSize(28).TextColor(p.muted)
		ui.Text(c, title).FontSize(14).FontWeight(600).TextAlign(ui.Center)
		ui.Text(c, detail).FontSize(12).LineHeight(1.4).TextColor(p.muted).TextAlign(ui.Center).MaxWidth(250)
	})
}

func formatBytes(value int64) string {
	switch {
	case value >= 1024*1024:
		return formatFloat(float64(value)/(1024*1024), "MB")
	case value >= 1024:
		return formatFloat(float64(value)/1024, "KB")
	default:
		return formatFloat(float64(value), "B")
	}
}

func formatFloat(value float64, suffix string) string {
	digits := 1
	if value >= 10 || math.Trunc(value) == value {
		digits = 0
	}
	return strconv.FormatFloat(value, 'f', digits, 64) + suffix
}

func (a *App) diffListState(key string) *ui.ListState {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.diffLists == nil {
		a.diffLists = make(map[string]*ui.ListState)
	}
	if state := a.diffLists[key]; state != nil {
		return state
	}
	state := &ui.ListState{}
	a.diffLists[key] = state
	return state
}
