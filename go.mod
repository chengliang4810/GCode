module gcode

go 1.27.3

require (
	github.com/egoist/mygo v0.6.4
	golang.org/x/image v0.46.0
)

replace github.com/egoist/mygo => ./third_party/mygo

require (
	github.com/ebitengine/purego v0.11.1 // indirect
	github.com/go-text/typesetting v0.3.5 // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

tool github.com/egoist/mygo/cmd/mygo
