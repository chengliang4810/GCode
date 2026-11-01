# Agent transcript data audit

2026-10-30: Model favorites and their DTO field have been removed; the favorites
checks below retain their historical audit scope. Current model-picker settings
contain provider/model selections and option values.

Reference: Zeron v0.2.128, `ab189159f10cf8e63c0502be75a59bbc487e0eaf`.
Scope: typed contracts and deterministic in-process backend. Native rendering is
verified separately; this audit cannot establish visual parity.

## Checks and exit evidence

- [x] Read source transcript grouping, tool classification and agent-message protocol.
- [x] Document user turn, run identity and ordered message/block relationships.
- [x] Preserve existing JSON fields and existing public service methods.
- [x] Add explicit tool categories instead of inferring every action from prose.
- [x] Carry command arguments, language, exit code and error state separately.
- [x] Carry typed diff paths, old/new content, hunks and real line counts.
- [x] Carry message continuation identity and elapsed duration.
- [x] Populate a corresponding run for every seeded completed or active turn.
- [x] Give every default session an independent task, prompt and final response.
- [x] Give every default session a distinct ordered execution trajectory.
- [x] Remove repeated generic layout answers from unrelated seeded sessions.
- [x] Include reasoning before, between and after tool actions where appropriate.
- [x] Include command execution, read, write, edit/apply-patch and search activity.
- [x] Include successful and failed tools, with matching output and exit status.
- [x] Include recovery after a failed command without falsely failing the whole run.
- [x] Include changed-file diffs that agree with the virtual editor and Changes state.
- [x] Include subagent work linked to a real child session.
- [x] Include permission/question waits with stable request IDs and resumable state.
- [x] Include intermediate assistant prose and a separate final Markdown response.
- [x] Include a dedicated transcript showcase and a deterministic live-tail playback scenario.
- [x] Make new sends stream a task-specific activity sequence before their final text.
- [x] Remove the obsolete Plan branch; sends use the same task execution path.
- [x] Derive summary counts from actual typed activity, never fixture display labels.
- [x] Count commands/reads/searches by actual unique tool identity.
- [x] Deduplicate written/edited paths in the summary while retaining each operation.
- [x] Count failed operations from explicit failure/exit metadata.
- [x] Derive elapsed duration from recorded operation/run timing.
- [x] Reject stale worker updates after stop, archive, deletion and replacement.
- [x] Settle pending tools and requests when a run is cancelled or the app closes.
- [x] Preserve queued send configuration and avoid mixing turns across sessions.
- [x] Preserve event sequence/revision ordering and owned snapshot deliveries.
- [x] Deep-copy new pointer/slice/diff fields at every service/queue boundary.
- [x] Reopen persisted transcripts without losing task/continuation/run data.
- [x] Maintain 1,000-session/5,000-message stress-scene bounds.
- [x] Add invariants for unique IDs, reference ownership and chronological timing.
- [x] Add counter assertions tied to recorded tools, including repeated edit paths.
- [x] Exercise streaming order, failure recovery, waits and stop/race behavior.
- [x] Run focused tests, vet and available Windows race instrumentation.
- [x] Update `docs/contracts.md` with the implemented semantics and remaining limits.

## Initial findings

The existing default seed gives nine sessions the same generic `response` result;
only the first session contains a tool trajectory. Seed message turn IDs have no
corresponding runs. New sends stream a single text block, so there is no ordered
execution activity from which truthful summaries can be derived. Current tool
metadata lacks explicit operation kind, arguments, exit code and diff payload.

Status: backend implementation and focused verification complete. Native
transcript acceptance remains a separate check.

## Verification evidence

- Ten primary tasks plus a real read-only PR-review child have distinct final
  responses and operation sequences. Summary classification, duplicate edit
  paths, intermediate notes and source capitalization have explicit tests.
- Ownership/timing invariants pass for default, transcript, showcase, loading,
  error, questions, permissions, stress, and forked histories.
- Tests cover typed playback, virtual writes, same-turn wait/resume, rejection
  of late updates, active-tool stop/archive/fork, diff/exit-code ownership and
  persistence. Context cancellation and close paths retain their owners.
- Focused tests passed for contracts/mock over 20 repetitions, and vet passed.
- Windows CGO race instrumentation passed for contracts/mock using portable
  GCC 16.2.0. Final source after catalog/preferences/legacy-profile checks passed:
  contracts 1.030s, mock 2.251s, exit code 0, no race report.
- Catalog ready/loading/error/empty/noAgents semantics and retry recovery are
  typed and tested without changing provider authentication or enabled flags.
- Per-provider model selections, options and favorites retain owned copies and
  survive persistence. Sidebar flags/defaults match settings.rs:1295-1305.
- The concurrent desktop run initially found two fixed-ten-session assumptions
  and a test assuming project grouping instead of the corrected one-list
  default. Desktop agents own those test updates; this audit does not mark
  that initial desktop run as passing. Workbench tests passed.

## Remaining acceptance limits

The review package uses the explicit independent profile option: its launch
entry must pass `--scenario default` or `--scenario transcript`, with no
`--data-dir`. This uses fresh in-memory business data and separate framework
state then under `gcode-zeron-ui/acceptance/<scenario>` (current scenario profiles
use the isolated MyGo profile; normal startup uses `~/.gcode`). Launching the bare executable
without arguments remains ordinary persistent mode; renaming it alone does not
make a new profile.

`TestLegacyProfileHistoryAndEditsStayIsolatedFromFreshReviewScenario` opens a
schema-1 file containing old repeated reply text, later user turns, a renamed
conversation, appearance/model preferences and a versioned file edit. These are
preserved. Starting and closing the fresh review scenario then leaves the normal
profile bytes unchanged. No heuristic text replacement or data wipe is used.

Existing stored histories are preserved. Fresh scenarios contain the new data;
older fixture state files require fresh capture data to show the new trajectories.
These are deterministic mock business records. No real model, shell, source
workspace process or tool-network execution was introduced. Arbitrary task
intelligence, Cloud-setup-specific summary wording and native visual equivalence
are not established by backend tests. UI/IME/macOS/native acceptance is separate.

## Integration correction

The independent UI review identified all-disabled Providers being treated as a
ready catalog. Integration now classifies ready/empty/noAgents from enabled
providers and their matching models on restore, retry and provider changes.
Catalog tests distinguish an enabled provider with an empty directory from
disabled providers. Historical cached models remain available to existing
conversation presentation; a new conversation cannot silently select a disabled
provider or send an unresolved empty model ID.
