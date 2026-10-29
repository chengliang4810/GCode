# Zeron — native Go desktop UI

**Moving to another machine:** start with [the handoff guide](docs/machine-handoff.md).
The branch contains source, pinned dependencies, acceptance evidence and replay
scripts. UI optimization was paused by the user on October 29, 2026; the current
UI is the baseline for upcoming real backend work. Full visual parity remains
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

A native MyGo implementation of the desktop interface in Zeron reference
`ab189159f10cf8e63c0502be75a59bbc487e0eaf`. Application controls are Go-native;
only external web page bodies use the platform web engine. Voice is excluded.

The interface defaults to Simplified Chinese. Choose **设置 → 通用 → 语言**
to switch to English without restarting; the choice is saved. Older saved state
without a language also opens in Chinese. Conversation content, code, file names
and model/provider names retain their original text. See [i18n notes](docs/i18n.md).

Requires Go 1.27.3. MyGo v0.6.4 is included with fourteen local patches,
locked to the current `zeron-native-ui.9` source; `scripts/check-framework.go` verifies
its source and patch hashes. Windows 11 22H2+ amd64 and macOS 13+ amd64/arm64
are targets.

R17 now includes the reviewed V5 glyph-placement opt-in on the existing native
tooltip leaf, preserving its actual AX content names. The isolated V5 cost matrix
and native fixture regression are evidence for that change. Fresh `.9` pin and
five gates passed through retained attempts01–03, and fresh canonical four-target
package builds and verification passed in their separate attempt02. Visual acceptance,
wrapping parity and the historical C4 versus R16 size increase remain open.

```powershell
go run ./cmd/zeron
go test ./...
go vet ./...
go tool mygo vet .
./scripts/build.ps1 -Targets windows/amd64,darwin/amd64,darwin/arm64,darwin/universal
./scripts/lint.ps1
```

Use `--help` for deterministic scenario, route, appearance and capture options.
`--language zh-CN` / `--language en` overrides the saved interface language.
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
