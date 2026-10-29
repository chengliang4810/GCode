# Account and device settings source audit

Reference: Zeron v0.2.128, ab189159f10cf8e63c0502be75a59bbc487e0eaf.
Source is read-only. This audit tracks implementation and evidence separately.

Initial differences: Providers incorrectly contained global sign-in, plain model
labels and connect/disconnect buttons. Devices incorrectly offered transport
toggles. Cloud showed sync/storage rather than GitHub, repositories and usage.
Billing showed a dollar balance and invoices rather than plans and credit packs.

- [x] Providers device selector and inline preferences disclosure implemented.
- [x] Installed/missing, enabled/disabled, last usable agent, install/cancel.
- [x] Completion, native subagents, update policy and update/cancel/retry controls.
- [x] Embedded account identity, active account, usage/reasons, refresh/retry.
- [x] Switch/remove menu and scoped login, code, waiting, error/retry/cancel.
- [x] Cloud API key entry stays masked and never reaches persisted snapshots.
- [x] Devices this/other group, scope subtitle, presence, ID copy and rename.
- [x] Cloud loading/error/unavailable/provisioning, GitHub connection/code/cancel.
- [x] GitHub reconnect/authorization/disconnect and repository refresh/selection.
- [x] Five usage ranges and backend-owned deterministic daily credits.
- [x] Billing unavailable/loading/error, credit totals and segmented balance.
- [x] Base/Pro/Max conditions, checkout pending/dismiss and idempotent results.
- [x] Extra credit pack stepper (1–20), purchase, subscription result transitions.
- [x] Sign-out and cancellation invalidate old asynchronous completions.
- [x] Backend ownership, persistence, stale flow cancellation and behavior tests.
- [ ] Native screenshots at matching size/theme and manual source comparison.

Business authentication, installation, billing and GitHub operations use the
in-process backend. They never execute a CLI or contact those services. Native
window and clipboard operations are real. Test captures prove implementation
states only; reference-native parity remains separately pending.

## Verification at handoff

- `go test ./internal/contracts ./internal/mock` passed after the final source
  edits; the integration backend suite includes ownership, persistence, secret
  exclusion, offline targets, last enabled agent, stale login completion,
  checkout idempotence and five usage ranges.
- Seven desktop integration behavior tests passed. They cover actual clicks,
  native text entry, Enter submission, menu selection, clipboard data, installation,
  preference writes, code login, Cloud key scope, repository choice and billing.
- The existing settings page test passed at 900×600 and 1320×880.
- `go vet` passed for contracts/mock/desktop/workbench; MyGo vet passed desktop.
- Focused Windows CGO race run passed with portable GCC 16.2.0:
  `go test -race ./internal/contracts ./internal/mock ./internal/desktop -run
  'TestIntegrat|TestProvider|TestDevices|TestCloudGithub|TestBillingPlan|TestSettingsPages'
  -count=1`. Mock 1.837s, desktop 3.310s, no race report; contracts had no tests
  matching this filter. The earlier unfiltered contracts/mock race run also passed.
- An earlier concurrent whole desktop run found two composer attachment removal
  failures (`attachments_test.go:57`, `transcript_side_test.go:252`), reported to
  their owner. The final four-package rerun after integration passed:
  contracts 0.016s, mock 1.012s, desktop 4.069s, workbench 0.237s.
- 24 implementation screenshots were captured at 1320×880, light and dark, by
  `TestCaptureIntegrationSettingsAudit`. Capture setup applies the theme after
  business actions, so backend refreshes cannot relabel a light image as dark.
- Native app/CU was not launched during the handoff freeze. No source/native
  screenshot comparison or user acceptance is claimed.
- The six integration QF1003 style findings were converted to tagged switches;
  the surface predicate is used by dialog rendering and is available to the root
  Escape/stop guard. No functional scope was added during the freeze.

Recreate captures with `ZERON_INTEGRATIONS_AUDIT_DIR=<absolute-directory>` and
`go test ./internal/desktop -run '^TestCaptureIntegrationSettingsAudit$' -count=1`.
The full local set is `output/acceptance/settings-integrations/`; representative
implementation images are retained in `docs/evidence/settings-integrations/`.

## Cloud navigation follow-up (2026-10-29)

The live four-file attempt-02 uses a fresh typed `cloud.refresh` for each visit,
then enables only a Ready/available/Off result. Loading may settle later; redraws,
same-page navigation and failed actions do not retry themselves. Leaving cancels
the page request; returning without an intermediate View also replaces its
generation before the new route is installed.

Root's actual offline headless overlay logs retain the red/green sequence:
attempt-01 production with the new regression fails four session/annotation/history
scenarios; the NewConversation routed-exit control already passes. Attempt-02
passes all nine top-level Cloud tests (five named re-entry cases included), and
the whole `go test ./...` overlay exits0. Evidence is under
`output/tabstrip-r17-cloud-navigation-overlay-test/{attempt-02-red,attempt-03-green,attempt-04-full}/result.json`.
The four live files are byte-exact to that tested proposal and gofmt-l is empty:
`output/tabstrip-r17-cloud-navigation-adopted/attempt-01/result.json`
(0b3a17e9…b090a). The independent delta review is
`output/tabstrip-r17-cloud-navigation-independent-review/attempt-02/result.json`.
This closes CLOUD-VISIT-01; its old report remains frozen, including the separately
corrected overgeneralization about NewConversation, which already used OpenRoute.

A separate normal native-worker run is now sealed at
`output/tabstrip-r17-cloud-navigation-native/attempt-01/result.json` (a5981b8b…63b1c).
The 2933-input Windows development main (manifest02e9e2e6…04684, EXE69a43719…f0090)
used an independent offline profile: signed-in/off rev2 became provisioning rev4
then ready/cloud-1 rev5; picking Cloud again retained rev5, Devices showed Cloud,
and actual settings search re-entry performed one fresh refresh to rev6. PID50440
closed normally via AltF4, exit0/stderr0; source inputs and the four original
windows stayed unchanged. Among 12 Sky states and nine original images, the first
wrong-window image was rejected and explicit target activation recovered later
images. This exercises the real normal worker path, not all failure/cancel/rapid
route cases, race/OS-thread tracing, exact WGC production, reference visual parity
or real Cloud services. It is a development build, not a new canonical package.

Live pin/tagged/Go-vet/MyGo-vet passed. The first lint launch failed because
GOPROXYoff blocked deprecation metadata (7dd9ccae…93e9); only lint was repeated
using the existing local file proxy and exited0 with 0 issues (8d0df8f4…3b6b1).
Both attempts remain at `output/tabstrip-r17-cloud-navigation-live-gates/`; this
is not a first-attempt all-green or a fresh rerun of every gate.
The DTO still lacks the source's `failed_action`; the existing `cloud.retry`
action-error path is retained without claiming complete failed-action parity.
The typed adaptation also deliberately prevents recursive Enable when a
successful Enable reply is unexpectedly still Off: only a fresh status read
can arm it. Frozen `settings/cloud.rs:356,365,249–253` clears busy and reapplies
an Off reply, which can Enable again. This is an explicit implementation/source
difference, not a protection already present in the reference. Other handoff
observations below retain their historical scope.

## Remaining source differences and next work

1. Provider device options still use a native MenuButton: the source selector's
   Devices heading, device glyphs, You/presence details and exact dimensions need
   transplanting. Update/install actions also need their final position before
   the source disclosure caret. These are visual differences, not passed parity.
2. Account switching is available from the source-named actions menu; source's
   click-anywhere account-row action is not wired yet. Usage reset formatting,
   uncommon provider login copy/device-code states and all account flags need
   complete source comparison across the ten agents.
3. `Auto when idle` is persisted and shown, but automatic update scheduling behind
   the policy is not implemented. Explicit Update/Cancel/Retry transitions work.
4. Cloud navigation's automatic enable is now wired and adopted, as recorded
   in the follow-up above. This closes the previously recorded navigation gap,
   while failed-action parity and the untested native failure/cancel/rapid-route
   matrix remain open.
   Remote provider metadata is isolated by target. The main/side composer now
   consumes that device's installed/enabled flags; the resumed-machine repair
   and regressions are recorded in `audit-composer-device.md`.
   Its current settled layout is recorded separately in
   [audit-composer-layout.md](audit-composer-layout.md): hero120–304 versus
   dock104–304, with a 24px side footer footprint. Account/context rings and PR
   footer integration remain pending; the source side footer can conditionally
   show context usage, while the implementation currently reserves an empty slot.
5. Authentication, installation, GitHub selection and billing portal outcomes are
   deterministic mock flows. They do not open real authentication/payment services.
   Exact pending/error/disabled/loading/offline visuals, focus restoration, native
   materials, DPI and macOS behavior remain unverified.
6. Source parity requires reference captures with identical data and platform.
   These implementation captures are reproducible evidence of current work only.

The checkpoint intentionally freezes these differences for the machine handoff;
it is an implementation milestone, not completion of the continuous UI parity goal.
