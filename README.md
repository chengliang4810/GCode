# Zeron — native Go desktop UI

**Moving to another machine:** start with [the handoff guide](docs/machine-handoff.md).
The branch contains source, pinned dependencies, acceptance evidence and replay
scripts. Current visual parity remains in progress.

A native MyGo implementation of the desktop interface in Zeron reference
`ab189159f10cf8e63c0502be75a59bbc487e0eaf`. Application controls are Go-native;
only external web page bodies use the platform web engine. Voice is excluded.

Requires Go 1.27.3. MyGo v0.6.4 is included with five reviewed patches,
locked to revision `zeron-native-ui.2`; `scripts/check-framework.go` verifies
its source and patch hashes. Windows 11 22H2+ amd64 and macOS 13+ amd64/arm64
are targets.

```powershell
go run ./cmd/zeron
go test ./...
go vet ./...
go tool mygo vet .
./scripts/build.ps1 -Targets windows/amd64,darwin/amd64,darwin/arm64,darwin/universal
./scripts/lint.ps1
```

Use `--help` for deterministic scenario, route, appearance and capture options.
`--theme` selects an exact built-in variant and `--dump-scene` exports a versioned
deterministic contract fixture. `scripts/capture-implementation.ps1 -AllThemes
-BoundarySizes` records implementation images and hashes; these do not replace
native reference comparison.
Normal application data lives in the OS user configuration directory under
`gcode-zeron-ui`, independently of Zeron. Scenarios use isolated in-memory data
unless `--data-dir` is explicitly supplied. The backend never starts a shell or
connects to a model, account, Git host or remote workspace.

See `docs/ui-parity.md`, `docs/contracts.md`, `docs/acceptance-report.md` and
`docs/implementation-plan.md` for coverage, contracts, evidence and remaining
acceptance work. Implemented behavior and verified parity are tracked separately.

Granular UI checks are recorded in `docs/audit-model-picker.md`,
`docs/audit-workbench.md`, `docs/audit-transcript.md`, `docs/audit-agent-data.md`
and `docs/audit-sidebar.md`. Use `--scenario transcript --route chat/chat-0`
for the mixed activity scene, or `--scenario live --route chat/chat-0` for a
deterministic streaming tail. These scenes use the same native components.
