package main

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"
	"runtime"
	"sync"

	"gcode/internal/application"
	"gcode/internal/appmodel"
	"gcode/internal/desktop"
	"gcode/internal/sandbox"
	"github.com/egoist/mygo"
)

type host struct {
	service     *application.Service
	view        *desktop.App
	window      *mygo.Window
	tray        *mygo.Tray
	unsubscribe func()
	quitting    bool
	quitPending bool
	closeOnce   sync.Once
}

func main() {
	if runtime.GOOS == "windows" && len(os.Args) > 1 {
		switch os.Args[1] {
		case sandbox.WindowsCommandRunnerSubcommand:
			os.Exit(sandbox.RunWindowsSandboxCommandRunner(os.Args[2:], os.Stderr))
		case sandbox.WindowsSandboxSetupSubcommand:
			os.Exit(sandbox.RunWindowsSandboxSetup(os.Args[2:], os.Stderr))
		}
	}
	if !mygo.App.IsPackaged() {
		name := "GCode"
		if mygo.IsDev() {
			name += " Dev"
		}
		mygo.App.SetName(name)
	}
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	dataDir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		log.Fatal(err)
	}
	service, err := application.New(dataDir)
	if err != nil {
		log.Fatal(err)
	}
	h := &host{service: service}
	mygo.App.OnSecondInstance(func([]string, string) { h.show() })
	mygo.App.WhenReady(h.open)
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.OnActivate(func(bool) { h.show() })
	mygo.App.OnBeforeQuit(h.beforeQuit)
	mygo.App.OnQuit(func() {
		h.cleanup()
	})
	if err := mygo.App.Run(); err != nil {
		h.cleanup()
		log.Fatal(err)
	}
}

func (h *host) open() {
	if os.Getenv("GCODE_VISUAL_SAMPLE") == "1" {
		h.view = desktop.NewVisualSample()
	} else {
		snapshot, _ := h.service.Snapshot(context.Background())
		h.view = desktop.Open(h.service, desktop.OpenOptions{
			Initial:         snapshot,
			Workspace:       application.Workspace{Service: h.service},
			TerminalFactory: desktop.NativeTerminalFactory{},
		})
	}
	h.window = mygo.NewWindow(mygo.WindowOptions{
		Title: "GCode", Width: 1440, Height: 900,
		MinWidth: 900, MinHeight: 640, StateKey: "workbench",
		TitleBarStyle:        mygo.TitleBarHidden,
		TrafficLightPosition: &mygo.Point{X: 16, Y: 16},
		TitleBarHeight:       32, AutoHideMenuBar: true,
		Content: h.view.View(),
	})
	h.view.SetDispatch(h.window.Update)
	if os.Getenv("GCODE_VISUAL_SAMPLE") != "1" {
		h.unsubscribe = h.service.Subscribe(func(appmodel.Snapshot) {
			h.window.Update(func() {
				snapshot, err := h.service.Snapshot(context.Background())
				if err == nil {
					h.view.Update(snapshot)
				}
			})
		})
		go func() {
			if err := h.view.Refresh(context.Background()); err != nil {
				log.Print(err)
			}
		}()
	}
	h.installMenu()
	h.installTray()
	h.window.OnClose(func(event *mygo.CloseEvent) {
		if h.quitting {
			return
		}
		event.PreventDefault()
		if runtime.GOOS == "darwin" || h.tray != nil {
			h.window.Hide()
		} else {
			h.window.Minimize()
		}
	})
}

func (h *host) show() {
	if h.window != nil && !h.window.IsDestroyed() {
		h.window.Show()
		h.window.Focus()
	}
}

func (h *host) installMenu() {
	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		{Label: "GCode", Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleAbout}, mygo.Separator(),
			{Role: mygo.RoleHide}, {Role: mygo.RoleHideOthers},
			mygo.Separator(), {Label: "退出 GCode", Accelerator: "CmdOrCtrl+Q", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
		}},
		{Label: "项目", Submenu: []*mygo.MenuItem{
			{Label: "打开项目…", Accelerator: "CmdOrCtrl+O", Click: func(*mygo.MenuItem, *mygo.Window) { h.openProject() }},
			{Label: "显示工作台", Click: func(*mygo.MenuItem, *mygo.Window) { h.show() }},
		}},
		{Role: mygo.RoleEditMenu}, {Role: mygo.RoleWindowMenu},
	}))
}

func (h *host) openProject() {
	go func() {
		result, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: h.window, Title: "打开项目", ButtonLabel: "打开", Directory: true,
		})
		if err == nil && len(result) > 0 {
			err = h.service.AddProject(context.Background(), result[0])
		}
		if err != nil {
			_, _ = mygo.Dialog.Message(mygo.MessageOptions{Parent: h.window, Type: mygo.MessageError, Message: "打开项目失败", Detail: err.Error()})
		}
	}()
}

func (h *host) installTray() {
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Label: "显示 GCode", Click: func(*mygo.MenuItem, *mygo.Window) { h.show() }},
		mygo.Separator(),
		{Label: "退出 GCode", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	})
	tray, err := mygo.NewTray(mygo.TrayOptions{Icon: trayIcon(), ToolTip: "GCode", Menu: menu})
	if err != nil {
		log.Printf("创建托盘失败：%v", err)
		return
	}
	h.tray = tray
}

func (h *host) beforeQuit(event *mygo.QuitEvent) {
	if h.quitting {
		return
	}
	event.PreventDefault()
	if h.quitPending {
		return
	}
	h.quitPending = true
	snapshot, _ := h.service.Snapshot(context.Background())
	active := false
	for _, session := range snapshot.Sessions {
		if session.ActiveRun != nil {
			active = true
			break
		}
	}
	if active {
		h.show()
	}
	go func() {
		if active {
			result, err := mygo.Dialog.Message(mygo.MessageOptions{
				Parent: h.window, Type: mygo.MessageWarning,
				Message: "停止任务并退出 GCode？",
				Detail:  "正在运行的任务将停止。会话历史、待执行消息和工作目录会保留。",
				Buttons: []string{"取消", "停止并退出"}, DefaultButton: 0, CancelButton: 0,
			})
			if err != nil || result.Button != 1 {
				mygo.RunOnMain(func() { h.quitPending = false })
				return
			}
		}
		h.cleanup()
		mygo.RunOnMain(func() {
			h.quitting = true
			mygo.App.Quit()
		})
	}()
}

func (h *host) cleanup() {
	h.closeOnce.Do(h.closeResources)
}

func (h *host) closeResources() {
	if h.unsubscribe != nil {
		h.unsubscribe()
		h.unsubscribe = nil
	}
	if err := h.service.Close(context.Background()); err != nil {
		log.Printf("停止运行资源失败：%v", err)
	}
	if h.view != nil {
		if err := h.view.Close(); err != nil {
			log.Printf("关闭终端失败：%v", err)
		}
	}
}

func trayIcon() []byte {
	icon := image.NewRGBA(image.Rect(0, 0, 32, 32))
	draw.Draw(icon, icon.Bounds(), &image.Uniform{C: color.RGBA{R: 45, G: 111, B: 222, A: 255}}, image.Point{}, draw.Src)
	ink := &image.Uniform{C: color.White}
	for _, rect := range []image.Rectangle{
		image.Rect(7, 8, 10, 24), image.Rect(10, 8, 14, 11), image.Rect(10, 21, 14, 24),
		image.Rect(22, 8, 25, 24), image.Rect(18, 8, 22, 11), image.Rect(18, 21, 22, 24),
	} {
		draw.Draw(icon, rect, ink, image.Point{}, draw.Src)
	}
	var buffer bytes.Buffer
	_ = png.Encode(&buffer, icon)
	return buffer.Bytes()
}
