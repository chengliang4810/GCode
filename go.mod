module gcode

go 1.27.3

require (
	github.com/dlclark/regexp2 v1.12.0
	github.com/egoist/mygo v0.7.4
	github.com/go-latex/latex v0.0.0-20250304174226-2790903426af
	go.yaml.in/yaml/v3 v3.0.5
	golang.org/x/image v0.46.0
	golang.org/x/sys v0.48.0
)

replace github.com/egoist/mygo => ./third_party/mygo

require (
	github.com/ebitengine/purego v0.11.1 // indirect
	github.com/go-text/typesetting v0.3.5 // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

tool github.com/egoist/mygo/cmd/mygo
