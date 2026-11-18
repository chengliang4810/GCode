module gcode

go 1.27.3

require (
	github.com/clipperhouse/uax29/v2 v2.5.0
	github.com/dlclark/regexp2 v1.12.0
	github.com/egoist/mygo v0.9.1
	github.com/fyne-io/image v0.1.1
	github.com/go-latex/latex v0.0.0-20250304174226-2790903426af
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2
	go.yaml.in/yaml/v3 v3.0.5
	golang.org/x/image v0.46.0
	golang.org/x/net v0.59.0
	golang.org/x/sys v0.48.0
)

replace github.com/egoist/mygo => ./third_party/mygo

require (
	github.com/ProtonMail/go-crypto v1.4.1 // indirect
	github.com/clipperhouse/stringish v0.1.1 // indirect
	github.com/cloudflare/circl v1.6.2 // indirect
	github.com/ebitengine/purego v0.11.1 // indirect
	github.com/go-text/typesetting v0.3.5 // indirect
	github.com/jsummers/gobmp v0.0.0-20230614200233-a9de23ed2e25 // indirect
	github.com/klauspost/compress v1.18.6 // indirect
	github.com/robfig/cron/v3 v3.0.1
	github.com/ulikunitz/xz v0.5.15 // indirect
	go.digitalxero.dev/rpm v0.3.4 // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

tool github.com/egoist/mygo/cmd/mygo
