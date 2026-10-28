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
4. Cloud automatically enables after mock sign-in; the source's automatic enable
   upon entering an already signed-in, off Cloud page still needs navigation wiring.
   Remote provider metadata is isolated by target; the main composer must consume
   that device's installed/enabled flags rather than local global flags.
5. Authentication, installation, GitHub selection and billing portal outcomes are
   deterministic mock flows. They do not open real authentication/payment services.
   Exact pending/error/disabled/loading/offline visuals, focus restoration, native
   materials, DPI and macOS behavior remain unverified.
6. Source parity requires reference captures with identical data and platform.
   These implementation captures are reproducible evidence of current work only.

The checkpoint intentionally freezes these differences for the machine handoff;
it is an implementation milestone, not completion of the continuous UI parity goal.
