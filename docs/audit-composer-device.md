# Composer provider availability by device

2026-10-28 resumed-machine implementation check. Reference: Zeron v0.2.128,
`ab189159f10cf8e63c0502be75a59bbc487e0eaf`, read-only source at
`output/reference-sources/zeron` on this machine.

## Source and defect

`crates/ui/src/pickers.rs:6792` describes `offered_harnesses`: a composer uses
the catalog device's provider switches and requires both enabled and installed.
There is no fallback to an installed or enabled provider on another device.
`offered_harnesses_require_an_installed_cli` at line 11239 also covers the
enabled-but-uninstalled case.

`rail_descriptors` at line 2728 only force-includes an existing conversation's
selected provider when it is still installed. The source regression
`missing_selected_harness_is_not_reintroduced_into_the_rail` at line 7736
explicitly rejects a selected but uninstalled provider.

The shared Go composer read `Snapshot.Providers.Enabled` for its empty state,
model restoration and provider rows. Those fields are also updated by local
provider settings; they cannot describe a remote or Cloud device. The aggregate
`ModelCatalog.State == noAgents` likewise overrode a usable remote target.
Changing the new-thread device did not resolve its selected model again.

## Implemented behavior

- The main composer uses the selected `App.deviceID`, which is initialized from
  the selected session and is also the device carried by its typed send request.
  Side composers use their own session's `DeviceID`; they never borrow the
  central device or the Providers settings page's selected target.
- Availability matches `(TargetID, ProviderID)` in
  `Snapshot.Integrations.Providers` and requires `Installed && Enabled`. Missing
  target metadata offers no agents; global provider flags are not a fallback.
  Global provider descriptors still supply names, icons and capabilities.
- A usable target overrides the aggregate `noAgents` display status. Existing
  loading/error behavior remains intact.
- Switching the new-thread device resolves a model from that device's usable
  providers. Returning from an existing conversation to the new-thread composer
  also restores an available selection. Remembered provider/model preferences
  are considered only when that provider is usable on the target.
- Both picker modes use the shared filtering. Provider and model activation
  recheck availability so stale picker actions cannot select an unavailable agent.
- Existing conversations retain their selected provider's rows when it is
  disabled but still installed, preserving the committed-model behavior
  already covered by `TestAllDisabledProvidersHaveNoAgentsButKeepExistingChatModel`.
  An uninstalled selected provider or one missing target metadata is not
  reintroduced into the rail or model rows, and stale actions cannot activate
  its models. This exception does not add it to another/new conversation.

## Verification

The first run of the three new device regressions against the prior implementation
failed: wrong local/Cloud selections, remote `noAgents`, missing-target fallback,
a stale model after device choice and local/side provider leakage. The same
tests pass after the changes.

Independent source review also identified that the existing-session exception
must still require installation. Its new main/side regression first failed for
all six uninstalled/missing-metadata cases, then passed after narrowing the
exception to disabled-but-installed providers.

| Check | Result |
| --- | --- |
| `TestComposerModelAvailabilityUsesTargetDevice` | Local, remote, Cloud, absent target, uninstalled agent and contradictory global flags pass |
| `TestComposerDeviceChoiceResolvesRemoteModelAndSends` | Native MyGo test clicks device/model controls, checks remote rows despite global `noAgents`, types and sends the remote model/device |
| `TestSideComposerProviderFlagsUseItsSessionDevice` | Side picker uses its remote session, ignores the Settings target, leaves main controls local and filters disabled/uninstalled providers |
| `TestComposerRejectsProviderSelectionAfterDeviceFlagsChange` | Previously captured provider/model actions cannot select an agent disabled after list construction |
| `TestExistingComposerRetainsOnlyInstalledSelectedProvider` | Main/side existing-session rows retain disabled-but-installed providers, exclude uninstalled/missing metadata and reject their stale actions |
| Existing all-disabled, catalog loading/error/retry and shared-composer regressions | Passed |
| `go test ./internal/desktop -count=1` | Passed after the independent-review correction, 2.933s |
| `go vet ./internal/desktop` | Passed |
| `go tool mygo vet ./internal/desktop` | Passed |
| `git diff --check` | Passed |

All five device/provider behavior regressions were included in the final
whole-package run. Toolchain: Go 1.27.3, Windows/amd64, pinned local MyGo v0.6.4.

## Evidence limits and older documentation

These are deterministic in-process backend and headless MyGo behavior checks.
No native application window, Computer Use, reference screenshot comparison,
remote engine, authentication or external service was operated for this change.
It establishes no visual parity, macOS behavior or complete-replica acceptance.
No worker/shared-state ownership changed; a race run was not added for this
UI-thread filtering change.

The earlier handoff versions of `audit-composer.md`, `audit-account-settings.md`
and `machine-handoff.md` listed device filtering as pending. Their current entries
point to this audit for the completed repair; their remaining layout/settings/
visual gaps remain open. Side working-trailer integration is recorded separately
in `machine-resume.md`.
