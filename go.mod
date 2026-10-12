module gcode

go 1.27.2

tool github.com/egoist/mygo/cmd/mygo

require (
	github.com/Microsoft/go-winio v0.6.2
	github.com/alecthomas/chroma/v2 v2.27.0
	github.com/aymanbagabas/go-udiff v0.4.1
	github.com/coder/websocket v1.8.15
	github.com/egoist/mygo v0.4.3-0.20261011132240-65154ea8777c
	github.com/ledongthuc/pdf v0.0.0-20250511090121-5959a4027728
	github.com/yuin/goldmark/v2 v2.1.6
	golang.org/x/image v0.46.0
	golang.org/x/sys v0.48.0
	mvdan.cc/sh/v3 v3.13.1
)

replace github.com/egoist/mygo => ./third_party/mygo

require (
	github.com/dlclark/regexp2/v2 v2.5.0 // indirect
	github.com/ebitengine/purego v0.11.1 // indirect
	github.com/go-quicktest/qt v1.102.0 // indirect
	github.com/go-text/typesetting v0.3.5 // indirect
	github.com/rogpeppe/go-internal v1.15.0 // indirect
)
