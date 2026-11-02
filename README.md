# GCode — native Go desktop UI

Naming and retained upstream attribution are documented in [branding](docs/branding.md).

**Moving to another machine:** start with [the handoff guide](docs/machine-handoff.md).
The branch contains source, pinned dependencies, acceptance evidence and replay
scripts. UI optimization was paused by the user on October 29, 2026; the current
UI is the baseline for the native Go backend. See [backend usage](docs/backend-v1.md). Full visual parity remains
unaccepted. See [the UI checkpoint](docs/ui-checkpoint.md).

The latest sealed [R16 correction](docs/audit-tabstrip-file-tooltip.md) dims inactive
file images and removes title tooltips from tabs without source details.
Timed native captures retain the remaining tooltip placement/style differences.
A same-device GPU control reproduces the light color difference; the actual UI
adapter binding and full visual acceptance remain pending. R16 release packages
pass content verification; Mac native acceptance remains pending.

[R17 work in progress](docs/audit-tabstrip-tooltip-primitives.md) adds source-timed
tooltip state and point placement. Its native observations, remaining text
differences and unused-feature size work are separate from release acceptance.

GCode is a native MyGo implementation of the desktop interface in the Zeron reference
`ab189159f10cf8e63c0502be75a59bbc487e0eaf`. Application controls are Go-native;
only external web page bodies use the platform web engine. Voice is excluded.

The interface defaults to Simplified Chinese. Choose **设置 → 通用 → 语言**
to switch to English without restarting; the choice is saved. Conversation content, code, file names
and model/provider names retain their original text. See [i18n notes](docs/i18n.md).

Requires Go 1.27.3. MyGo v0.7.4 is included with local patches,
locked to the current `rcode-native-ui.23` source; `scripts/check-framework.go` verifies
its source and patch hashes. Windows 11 22H2+ amd64 and macOS 13+ amd64/arm64
are targets.

Desktop updates use MyGo's native updater and signed GitHub Releases from
`chengliang4810/GCode`. Open **设置 → 通用 → 软件更新** to check.
Release builds require the signing key; see [update and release setup](docs/updates.md).
The GitHub repository must be public before distributing these builds.

The historical R17 `.9` phase includes the reviewed V5 glyph-placement opt-in on the existing native
tooltip leaf, preserving its actual AX content names. The isolated V5 cost matrix
and native fixture regression are evidence for that change. Fresh `.9` pin and
five gates passed through retained attempts01–03, and fresh canonical four-target
package builds and verification passed in their separate attempt02. Visual acceptance,
wrapping parity and the historical C4 versus R16 size increase remain open.

```powershell
go run ./cmd/rcode
go test ./...
go vet ./...
go tool mygo vet .
./scripts/build.ps1 -Targets windows/amd64,darwin/amd64,darwin/arm64,darwin/universal
./scripts/lint.ps1
go run ./scripts/desktop-e2e
```

[Desktop testing](docs/desktop-testing.md) defines the native Automation plugin
workflow, isolated test data, evidence and the remaining OS verification scope.
`go run ./scripts/desktop-e2e` is shared by Windows, Linux and macOS and requires
a graphical session. The plugin is included only in `e2e` builds.

Use `--help` for deterministic scenario, route, appearance and capture options.
`--language zh-CN` / `--language en` overrides the saved interface language.
`--theme` selects an exact built-in variant and `--dump-scene` exports a versioned
deterministic contract fixture. `scripts/capture-implementation.ps1 -AllThemes
-BoundarySizes` records implementation images and hashes; these do not replace
native reference comparison.
Normal startup uses the real in-process Go engine. Configure custom models in
**设置 → 模型设置**; the only protocols are OpenAI Chat Completions, OpenAI
Responses and Anthropic Messages. Application data defaults to `~/.gcode`.
Change it in **设置 → 通用 → 数据保存目录**; the selected directory is registered
and takes effect after restart, with no data copied. Existing conversations stay
in their original directories and the original directory is retained. No old
application profiles are imported.

Run `go run ./cmd/gcode-server` for the authenticated headless HTTP/SSE service.
Its dependency graph excludes MyGo and desktop/window packages. Desktop connection
settings are under **设置 → Devices**, or use `--connect` with `--token-file`.
[Backend usage](docs/backend-v1.md) covers TLS, remote ownership and limitations.
`--scenario` always uses isolated deterministic mocks; `--data-dir` only selects
an explicit profile/state directory. Normal startup without `--scenario` uses the
real in-process Go engine.

[The October 30 cleanup](docs/ui-residual-cleanup.md) removes unused account,
PR and project-action controls and connects model naming, real Git worktrees,
native notifications, wallpaper colors and six native math fonts.

See `docs/ui-parity.md`, `docs/contracts.md`, `docs/acceptance-report.md` and
`docs/implementation-plan.md` for coverage, contracts, evidence and remaining
acceptance work. Implemented behavior and verified parity are tracked separately.

Granular UI checks are recorded in `docs/audit-model-picker.md`,
`docs/audit-workbench.md`, `docs/audit-transcript.md`, `docs/audit-agent-data.md`
and `docs/audit-sidebar.md`. Use `--scenario transcript --route chat/chat-0`
for the mixed activity scene, or `--scenario live --route chat/chat-0` for a
deterministic streaming tail. These scenes use the same native components.
