# Desktop data contract

Schema version: **1**. Reference: Zeron **v0.2.128**, commit
`ab189159f10cf8e63c0502be75a59bbc487e0eaf`.
The Go definitions in `internal/contracts/` are authoritative. The field inventory
below lists every exported DTO field and its JSON name and Go type. Contracts
import no MyGo, window, rendering or platform objects.

## Service and ownership

```go
type Service interface {
    Bootstrap(context.Context) (Snapshot, error)
    Snapshot(context.Context) (Snapshot, error)
    Execute(context.Context, Command) error
    Subscribe(context.Context) (<-chan Event, error)
    Close() error
}

type ResourceService interface {
    ReadResource(context.Context, string) ([]byte, error)
}
```

The application injects `Service`; `ResourceService` is an optional capability
implemented by the same in-process server. All identifiers are stable strings.
JSON names use camelCase. Times are UTC `time.Time`, encoded as RFC3339; optional
completion times are absent until known. Counts and geometry are numeric values.

Snapshots, event snapshots, retained commands and managed resource bytes are
owned copies. This includes nested pointers, slices, maps, annotation arrays,
model-picker favorites/options, theme variants and diff lines. Callers may edit
their copies without mutating backend state. `Command.Clone` freezes drafts
before they cross a queue/goroutine boundary; `Command.Validate` rejects unknown
commands, missing required identities and a payload belonging to another kind.

`Run.requestId` optionally retains the send's request identity in complete and
persisted snapshots. The latest-snapshot subscription can coalesce away the
transient event ACK; consumers must not infer creation ownership from the first
new session row. Empty/legacy request IDs remain valid. The desktop excludes
preexisting RunIDs when matching a new canvas send, including after restart.

The backend has no model API, real account, Git process or shell. Virtual file
operations never edit the project on disk. Public browser content comes from the
real page engine. Native image selection is a deliberate local-file exception:
`background.install` reads the selected image and installs an independent managed
copy. Application state is saved only to the caller-supplied independent
`Options.StatePath`; no original Zeron data directory is used by the server.

## States and content

`Snapshot.messages` is keyed by session ID. Session status is exactly `idle`,
`working`, `awaitingInput` or `errored`. `lastOutcome` describes the latest finished
turn; it can retain that outcome while a new turn is working. Check both session
status and the latest run before treating a send as finished. Run status is
`running`, `queued`, `awaitingInput`, `completed`, `failed` or `interrupted`; `finishReason` distinguishes
normal completion, stopping and cancellation. Message `streaming` is independent
of role and is cleared on stop, archive, deletion and process recovery.

`Block.kind` selects one payload:

| kind | Active payload |
|---|---|
| text, reasoning, error | text |
| code | text, language |
| tool | tool |
| question | question |
| permission | permission |
| todo | todos |
| subagent | subagent |
| attachment, image | attachment |
| math, diagram | artifact |
| annotation | annotation |

Question/permission answers are recorded once. Stopping or archiving a waiting
session records cancellation and prevents an old request from resuming it.
Permission denial settles the wait without starting another run. Browser HTML
and styles are inert annotation data, never executed as application markup.

Math and Mermaid artifacts carry source, vector resource reference, width, height
and baseline. `tools/reference-artifacts` exports the fixed scene from original
zatex and Mermaid renderer code at build time; the Go application embeds these
resources. It never launches Rust at runtime. The export provenance retains the
original generating commit and separately records compatibility verification
against the current reference. Unknown source/font/theme combinations do not
fabricate vector geometry. Full native visual comparison remains an acceptance
item outside this data contract.

## Agent turns and activity

The source rules are in `proto/src/agent.rs`, `proto/src/view.rs:770` and
`ui/src/transcript.rs:1365,2166` at the pinned reference. Ordered messages and
blocks form the timeline; there is no second presentation-event store.
`Message.turnId` identifies `Run.id`. `continuationOf` names the preceding
assistant entry in the same turn. Answering a question or granting permission
continues that turn; denying permission settles it without a second run.

`ToolCall.kind` explicitly distinguishes `exec`, `readFile`, `writeFile`,
`editFile`, `applyPatch`, `search`, `glob`, `webFetch`, `webSearch`, `todo`, `mcp`
and `unknown`. Existing name/summary/command/path/output fields remain compatible.
`args` carries readable invocation input, `language` identifies syntax, and
`isError`/optional numeric `exitCode` distinguish failure from output prose.
`diff` reuses typed `Change`: paths, old/new content, hunks and actual line counts.
Historical operation diffs remain immutable when a later edit changes the file.

Tool-start inserts a stable invocation/block ID with working state; completion
updates that ID with output, exit metadata and completion time. Reasoning,
intermediate notes, tool actions and the final Markdown retain event order.
Virtual writes and edits update both Files and Changes. Read-only/conflict
failures preserve the newer file and produce a failed tool/run result. None of
these deterministic activity records launches a shell, real agent or network tool.

`TurnActivity` extracts the working prefix before the trailing reply.
`SummarizeActivity` derives `ActivitySummary` from actual items: invocation IDs
deduplicate call/result representations; write/edit/patch paths deduplicate into
edited files; search/glob/web-search share a category. Failures use explicit
status/error/exit metadata. Subagents retain a separate count and count as other
tool invocations in compact groups. Thought and note counts come from their
recorded blocks. `Run.summary` stores this same derivation as events commit.

`ActivitySummary.Label()` follows the nested source capitalization, for example
`Thought process · Ran 2 commands · edited 1 file · 1 failed`. Empty groups have
no fabricated zero-tool label. The UI may derive the same summary for a displayed
subset through this pure helper.

`Message.durationMs` and `Run.durationMs` describe recorded work intervals.
Tool timestamps lie within their run; a settled run's duration equals finish
minus start. Playback uses deterministic logical intervals, including Instant
mode. These fields describe mock execution, not measured UI performance or
real-model latency.

Ten primary tasks have independent prompts, traces and final responses: layout,
palette, authentication, deployment, PR review, focus, documentation, composer,
reconnect and settings search. PR review links a real read-only child session;
it does not reuse an unrelated editing task as its subagent. A failed auth check
is followed by a recorded edit and passing rerun, so the successful turn retains
the failed attempt in its summary. Session PR links resolve to actual records.

`transcript` extends the rich showcase with a failed fixture check, virtual file
write, successful rerun, todos and a linked subagent. `live` plays an active
thinking/tool tail; `loading` remains the separate frozen loading case. Stop,
archive, fork and close settle pending tools and reject late worker updates.
Forks have independent message, continuation, turn and invocation identities.

Existing stored histories are preserved. Fresh scenarios use the new traces;
upgrading does not replace a user's conversation history with new fixture text.
Arbitrary-agent intelligence, Cloud setup-specific summary wording and native
visual equivalence are not established by this deterministic backend.

For a review package, the launch entry must pass `--scenario default` or
`--scenario transcript` and omit `--data-dir`. The CLI then uses an empty
backend StatePath and creates the fresh scenario in memory; framework state is
separate under `gcode-zeron-ui/acceptance/<scenario>`. Directly launching the
executable without those arguments is ordinary persistent mode and may still
show an older stored fixture transcript. A different executable filename alone
does not isolate its data.

The chosen compatibility policy is profile isolation rather than heuristic
replacement of schema-1 conversation content. A legacy-profile regression test
keeps old repeated text, user-added turns, renamed sessions, model preferences
and saved file edits, then verifies that a memory-only review scenario has the
new activity while leaving the normal profile bytes unchanged. Existing
histories receive additive turn/reference normalization, not replacement text.

## Model catalog and sidebar state

`Snapshot.modelCatalog` is typed `ModelCatalogState{state,error}`. States are
`ready`, `loading`, `error`, `empty`, `noAgents`; the default is ready. A cached
catalog may accompany an error. `catalog.retry` clears the error and refreshes
missing models for configured providers while retaining their authentication
and enabled settings. Zero providers produces noAgents; an unknown configured
provider with no models produces empty. UI tests can supply any bootstrap state
without adding more public scenarios.

`Settings.modelPicker.selections` maps provider ID to `ModelSelection{modelId,
reasoning}` independently of favorites and option values. Commands, snapshots
and persisted settings own those maps.

`ModelOption.choices` and `defaultChoice` retain stable option IDs. Optional
`choiceLabels` maps those IDs to display text, for example `1m` to `1M tokens`.
The UI stores and sends IDs; missing or empty labels fall back to the ID. The
optional field preserves legacy JSON, and snapshot ownership includes a deep
copy of each label map.

Sidebar defaults follow source settings.rs:1295-1305: compact, one list,
last-updated sort. Location/icon, harness, branch, PR, project filter and archived
entries are shown. Their typed preferences persist independently.

## Reference .128 additions

`AccessMode` is `supervised`, `auto-accept-edits` or `full-access`. Providers declare
`accessModes` and `supportsPlan`. Session, queued send and run retain access mode
and plan configuration. Provider capabilities normalize unsupported choices;
queued sends preserve the configuration captured when they were queued. Plan
responses describe a proposed plan without an execution response. Session update
pointer fields distinguish an omitted field from an explicit false/empty value.

`Annotation` (alias `BrowserAnnotation`) matches the browser picker protocol.
`Bounded` copies and normalizes it with UTF-8 byte limits: URL 2048, HTML 2400,
selector 480, text/title/element/role/name 240; path at most 8 entries of 240;
styles at most 24 key/value pairs of 64/240. Rect is `[x,y,width,height]`, viewport
is `[width,height]`. Truncation preserves UTF-8 boundaries and adds an ellipsis.
`Link` and `ParseAnnotationLink` use canonical `[Annotation N](zeron-annotation:<hex
JSON>)` links. Parser size limits bound decoding; link labels must match the
encoded index. Annotation-only and attachment-only sends are valid prompts.

Projects expose typed `actions`. Create/update/delete changes actual mock state;
run adds a terminal containing scripted output and updates `lastRunStatus`.
Commands are never forwarded to an operating-system process. Forking copies a
transcript with new message/block IDs; a side chat starts empty. Both reference
`parentSessionId`, inherit model/device/branch/worktree/access/plan configuration,
and begin idle with independent drafts and run ownership. Copied pending requests
receive new identities and a cancelled answer, preserving the parent wait.

Branch commands may include `requestId`; the created session exposes the same
value as `creationRequestId`. It survives saved-state restoration. Repeating the
same request for the same parent and branch kind returns the existing child;
reusing it for another parent or kind fails. Older commands without this optional
field retain their existing behavior.

The session-info **New side chat** action opens that exact child in the native
right pane, using its existing independent composer. Existing side-chat entries
reopen there, preserving the central conversation and draft. Fork opens the
copied conversation centrally. A completion arriving after navigation updates
the data without taking over the newly selected conversation.

`ThemeLibraryEntry.sourceKind` is `imported`, `linked` or `editable`.
The theme import workflow is deterministic business simulation: analyze produces
compiled light/dark palette variants, selection changes the preview, import adds
the selected variants, and remove/reload/duplicate mutate library state. Reload
failure retains the last usable variants and exposes a warning. No JSONC/package
loader or external linked-file watcher is implied by this mock contract.

## Native attachment ingestion

The optional `contracts.AttachmentImporter` is implemented by the same server:

```go
type AttachmentImporter interface {
    ImportAttachments(context.Context, AttachmentImportRequest) ([]Attachment, error)
}
```

`AttachmentImportRequest.paths` accepts regular files from a native picker, drop
or pasted file list. `clipboard` accepts encoded bytes with name and MIME type.
`Clone` owns both paths and byte slices before background dispatch. The importer
reads and validates input off the UI thread, returns typed attachment metadata,
and adds it to `Snapshot.attachments`, independently of `Snapshot.appshots`.
No service command forwards input to a model or shell.

Limits are explicit: 24 MiB per file (matching reference attachments.rs), 20
items and 96 MiB per batch. Raster image edges are limited to 16,384 pixels and
images to 32 million pixels; full decoding must succeed. PNG, JPEG, GIF, BMP,
TIFF and WebP use their actual decoded MIME/dimensions. SVG is XML-validated,
limited to 100,000 tokens and depth 128, and measured using width/height or
viewBox with the same dimension limits; SVG bytes are native resource content,
never application HTML. Clipboard image MIME must match the encoded bytes.
Names cannot inject lines; unnamed clipboard images receive a matching extension.
Opaque regular files preserve their original bytes and declared/detected MIME.

The managed resource reference is `resource://managed/attachment-<id>.<extension>`.
With `StatePath`, bytes are copied to its sibling `attachments/` directory;
without it they are held in owned memory. Catalog metadata alone is serialized
in application JSON. Source files remain untouched and can be removed after
import. `ResourceService.ReadResource` retrieves owned bytes from the known
catalog. All inputs in a batch are validated before committing; cancellation,
validation or persistence failure leaves no partial catalog and cleans prepared
files. Catalog entries retain resources for transcript/draft references; automatic
collection of discarded draft resources is not implemented.

Default imports complete local ingestion with status `ready`. The optional
`transfer` flag simulates `transferring` followed by `ready`, with events
`attachments.imported` and `attachments.ready`. Cancelling the import context
sets pending transfer status `cancelled` and emits `attachments.cancelled`.
Close/restart settles process-local pending transfers to `cancelled`, retaining
the user's managed bytes. Transfer timing uses `Options.TokenInterval` or
`Options.Instant`; no network connection is made.

## Settings, defaults and resources

Product defaults follow the reference: appearance `system`, no forced theme ID,
light/dark choices `zeron-light`/`zeron-dark`, motion preference `system`, smooth
scrolling on, pause-in-background off; Escape-to-stop off; file autosave off;
Appshot capture sound on and destination `automatic`; session sounds and agent
updates on. Scenarios may override these values explicitly.

UI font defaults to Geist 16; code to Geist Mono 12.5; terminal to Geist Mono 13;
math to enabled Fira Math at 100%; transcript width 736. Layout defaults are
sidebar 256, files 286, right pane 520 and terminal height 280. Persisted widths
normalize to sidebar 242–400, files 220–440, right pane at least 360 and terminal at
least 160. Transcript width is 560–1200, UI font 12–20, code/terminal font 8–32.
The view additionally fits geometry to the current viewport.

`Settings.modelPicker` keeps provider selection, typed favorite model/provider
pairs and option values keyed by stable `modelID/optionID`. Empty favorites and
options are valid. The native UI resolves available models against the catalog.
`General.sendBehavior` follows `enter`/`modEnter`; `AppshotSettings.includeCursor`
is retained only for saved-state compatibility and has no visible reference row.

Background image references use `resource://managed/background-<id>.<format>`;
`Appearance.background` is the current image identity, `backgroundAssetRef` is
the resolver reference, and `backgroundName` retains the original filename.
Install validates the exact bytes through a full image decode before publishing
anything. Supported decoders are PNG, JPEG, GIF, BMP, TIFF and WebP; images over
50 MiB or 64 million pixels are rejected. With `StatePath`, managed files live in
the sibling `backgrounds/` directory; without it they live in memory. Source files
can be removed after installation. Resource reads resolve the active background
and imported attachment catalog, without arbitrary filesystem access. Image settings are persisted before the
previous managed file is retired; failed installation/persistence preserves the
active image and removes prepared files. A process crash before commit can leave
an orphaned file; automatic orphan-file collection is not implemented.

`BackgroundAdjustment` uses focal coordinates 0–1 and zoom 1–4, default .5/.5/1;
nonfinite focal values normalize to .5, nonfinite zoom to 1. Only `background.*`
commands change image identity/name/resource/crop. General `settings.update`
preserves those current fields, allowing a stale settings draft to save unrelated
preferences without restoring a retired image. `background.adjust.path` must equal
the current image identity, otherwise it returns `ErrConflict`.

Appshots UI is macOS-only. `AppshotCapabilities` values are `ready`,
`permissionRequired` or `unavailable`; platform identifies their provenance.
Permission commands update simulated capability state on macOS only. A check
never grants permissions. No real screen capture or OS permission prompt occurs.

## Commands

All commands accept cancellation before work starts; long image operations check
again before commit. IDs not present in the current snapshot return errors.

| kind | Required fields and mutation |
|---|---|
| session.create | optional projectId; creates an idle session |
| session.update | session with id and optional pointer fields |
| session.rename | id, text; changes title |
| session.archive, session.pin | id, enabled; archive cancels work and queue |
| session.delete | id; removes session/messages/queue and settles owned run |
| session.fork, session.sidechat | sessionId (id shorthand), optional name and requestId; creates correlated child |
| project.create | name, path; adds an isolated virtual project |
| project.action.create, project.action.update | projectId, action; update requires action.id |
| project.action.delete, project.action.run | projectId, id |
| message.send | send: SendRequest; empty sessionId atomically creates then sends; projectId envelope fills missing send.projectId |
| message.stop, message.retry | sessionId; retry retains configuration without duplicating prompt |
| queue.remove, queue.update | id; update uses text; removes or edits queued send |
| request.answer | id, text; records once and continues or denies wait |
| file.create | projectId, relative path (name shorthand), text; enabled means directory |
| file.rename | id, name; renames descendants while retaining stable identities |
| file.delete | id; removes directory descendants after read-only preflight |
| file.save | id, text, version; stale nonzero version returns ErrConflict |
| settings.update | settings; stores complete preferences, preserves managed image fields |
| background.install | path from native file picker; validates, copies and replaces image |
| background.remove | removes current image metadata and its managed copy |
| background.adjust | path=current background identity, adjustment |
| theme.analyze | path, name=source mode (default imported); creates preview/error |
| theme.select | id=variant ID, enabled; updates preview selection |
| theme.import | optional name=source mode; adds selected variants, clears preview |
| theme.remove, theme.reload, theme.duplicate | id=library entry; mutates library |
| provider.toggle | id, enabled |
| provider.connect | id; connects simulated account |
| device.toggle | id, enabled; reconnect updates lastSeen |
| pr.comment | id, comment(body,path,line); text/path/version compatible shorthand |
| pr.merge | id; sets local status merged and updates timestamp |
| terminal.create, terminal.close | optional projectId for create; id for close |
| terminal.input | id, text; scripted command input and ANSI output |
| terminal.resize | id, terminalSize(columns,rows); version/name compatible shorthand |
| account.signin, account.signout | changes simulated account/user status |
| cloud.toggle | enabled |
| update.check | refreshes simulated release state |
| catalog.retry | clears catalog errors and refreshes configured model metadata |
| attachment.add | legacy Appshots catalog append; regular composer input uses AttachmentImporter |
| appshots.permissions.check | refreshes current simulated capabilities |
| appshots.permission | id=windowCapture or applicationText; macOS only |

Virtual file create/rename rejects collisions, invalid relative paths and missing
parents. Directory rename/delete checks read-only descendants before mutation.
Save increments the version and updates Changes text, line counts and a contiguous
context hunk; restoring the base removes a non-added change. No Git engine is
being run. Create adds a typed added change; rename carries the original path
and stable change identity; delete records the tracked base deletion. Creating
and then deleting a new file removes its change. Directory operations update
descendant file changes. Seed editor text, diff text and line counts agree.

Terminal input recognizes `pwd`, `ls`/`dir`, `git status`, `go test ./...`, `help`,
`echo ...` and `clear`; unknown commands produce a command-not-found line. Create
uses the selected project's path, or `~` for a projectless terminal. Resize stores
rows/columns. Native terminal selection, search, scrolling and ANSI rendering
belong to the UI/terminal component.

## Events, persistence and deterministic scenarios

Every event includes schemaVersion, eventId, sequence, revision, kind, optional
session/run/request identities, optional cumulative text and an owned snapshot.
Subscribe immediately sends the current snapshot. Revisions and event sequence
increase for visible mutations. Channel capacity is one: a slow consumer receives
the newest full state rather than an unbounded obsolete backlog. Consumers must
not count text events or rely on delivery of each intermediate revision.

Cancellation closes the owned subscription. Closing the backend cancels workers,
settles streaming and closes subscriptions. A worker checks its run identity
before writing; stopped, archived, deleted or superseded runs cannot append late
results. Changing the UI's selected session does not cancel legitimate background
work in another session; the UI applies events by session identity/revision.

`Options.Now` fixes UTC timestamps; generated IDs are monotonic. Default token
pacing is 30ms. `Options.Instant` accelerates asynchronous completion for tests;
wait for a completed latest run rather than assuming Execute is synchronous.
State writes serialize by revision, use temporary files, Sync and atomic rename.
Restart settles process-local running states and supplies newly introduced
settings defaults without repopulating deleted messages. Invalid JSON or an
unknown schema version is a startup error rather than silent state replacement.

Scenarios are `default`, `empty`, `loading`, `error`, `offline`, `questions`,
`permissions`, `showcase`, `transcript`, `live` and `stress`. Loading is a reproducible frozen visual
state. Showcase contains text, reasoning, code, tools, questions, permissions,
todos, subagents, attachments/images, math and diagrams. Browser-picked annotations
supply the annotation case. Stress contains 1,000 sessions and 5,000 messages.
No scenario labels or development control panels are inserted in product UI.

Tests cover defensive copies, independent subscribers, cancellation, shutdown,
create/send atomicity, queue configuration/drain, retry identity, stale worker
rejection, request cancellation, access/plan/annotation bounds and links, optimistic
file versions, read-only atomicity, pin ordering, child sessions, project actions,
theme import/reload/removal, Appshot platform guards, settings defaults and managed
image install/read/replace/adjust/remove/persistence/failure paths. Backend tests
prove these data and lifecycle properties only. Native visual parity, IME/system
behavior, macOS runtime and race instrumentation require separate recorded checks.

## Provider, device, Cloud and billing settings

`Snapshot.integrations` is independent of `Settings`: an optimistic appearance
save cannot overwrite account or checkout responses. `observedAt` fixes fixture
time for device metadata. `workspaceScope` controls local/synced device groups.
The ten provider identities follow proto/providers.rs; mock remains absent from
the product. `ProviderDetails` is scoped by target device and provider, with
installation, enabled state, completion/delegation preferences, update policy,
account loading/error state, login options and provider-specific account notes.
Local enable/install operations synchronize the composer provider catalog;
remote settings remain isolated. Cloud exposes only Claude Code and Codex.

`ProviderAccount` carries active/switchable/removable/authentication state and
owned usage windows. Adding a local account preserves the existing active login;
switch/remove operations change the stored account selection. Hermes owns its
pool and Antigravity keeps one login. Login flows carry stable attempt IDs,
target/provider identity, browser/code kind, expiry and error state. Cancelling
or replacing a flow rejects obsolete callbacks. API keys and authorization code
text are command inputs only: neither snapshots nor state.json retain them.

Integration commands use an owned `IntegrationRequest` with `targetId`,
`providerId`, `accountId`, enum `value` and credit `packs`. This payload is
exclusive of send/session/settings/file/project payloads. Commands are:

| Area | Commands and effects |
|---|---|
| Provider catalog | `providers.refresh` transitions loading → ready |
| Installation | `provider.install`, `provider.install.cancel`; mock install updates metadata and the local available-model catalog |
| Updates | `provider.update`, `provider.update.cancel`, `provider.preference` (`updatePolicy`) |
| Preferences | `provider.enable`; `provider.preference` with `name=dollar/slash/subagents/updatePolicy` |
| Accounts | `provider.accounts.refresh`, `provider.account.switch/remove` |
| Login | `provider.login.start/submit/poll/cancel/retry`, `provider.apiKey.save` |
| Devices | `device.rename` validates a nonempty name of at most 80 characters; copying an ID uses the native clipboard |
| Cloud | `cloud.refresh/enable/delete/retry`, `cloud.authorize` |
| GitHub | `cloud.github.start/poll/cancel/disconnect`, `cloud.repositories.refresh/choose` |
| Usage | `cloud.usage` selects `7d/30d/thisMonth/lastMonth/90d` and regenerates deterministic daily values |
| Billing | `billing.refresh/subscribe/buy/manage`, `billing.checkout.poll/dismiss` |

Billing plans use integer price cents and numeric credit allowances, matching the
source Base/Pro/Max catalog. Monthly + extra equals total. Packs are bounded to
1–20. Checkout completion is idempotent by ID and only applies once; cancellation,
sign-out and shutdown invalidate pending background work. No actual payment,
CLI installation, OAuth, GitHub request, credential-store mutation or shell runs.
Browser completion/checkout outcomes are deterministic in-process transitions.

`mock.Options.IntegrationScenario` selects backend fixtures for providers
loading/error, disabled/notConnected/updateError, accounts loading/error/expired/
usageError, Cloud loading/error/unavailable, GitHub waiting/needsReconnect/
notAuthorized/failed and Billing loading/error/unavailable/noPlan/pastDue/
cancelled/pending. It is an internal scenario selector, with no extra navigation
or controls in the product. Ordinary persisted profiles retain their metadata;
fresh fixtures do not replace saved account settings. Restart discards pending
sign-ins and marks unfinished checkout cancelled instead of replaying side effects.

Behavior and implementation captures are recorded in
`docs/audit-account-settings.md`. Remaining source differences are explicit there;
compilation, regression tests and these captures do not establish visual parity.

## Composer footer resources and occupancy

`Snapshot.contextUsage` maps each session ID to a host occupancy snapshot with
optional `tokens` and `window` counters. An absent counter is unknown; reported
zero tokens remain zero. A report merges only its supplied fields, ignoring a
zero window. Occupancy may decrease after compaction or exceed capacity. This
state is independent of cumulative `Message.Usage` and model catalog limits.
The deterministic host keeps provider/target/cwd session scopes: first dispatch
to a new scope resets its occupancy, while resuming an existing scope restores
its own host state. Merely selecting another model does not reset a host report.
Snapshots, subscribers and persistence own separate copies; deleting a session
removes only its own occupancy and rejects late host updates.

`Snapshot.repoRefs` is keyed by `RepoRefScope.Key()` including project, target,
repository path and Cloud repository/default branch. `git.refs.load` and
`git.ref.switch` use `Command.git`; generations reject stale results. Refreshes
retain previous rows. Draft sends consume a `SendRequest.checkout` plan:
`current`, `reuseWorktree`, `newWorktree` or `cloud`. A new worktree is created
by the mock on send; an unspecified base means HEAD. An established session's
checkout cannot be migrated by a later draft selection.

Each fresh host dispatch captures `Session.sourceContext` from that session's
actual cwd. `Snapshot.checkoutPR` is keyed by device, nonempty checkout ID,
repository root and trimmed branch. UI matching does not add cwd/head-SHA/time
requirements absent from the source. Successful nil summaries clear a prior
PR; errors retain the previous success for the same source. Deletion prevents
late work from recreating an unowned PR resource. All Git/PR work remains
deterministic in-process state; no Git process or external service is used.

The footer and Settings use the same `Integrations.accounts` store.
`provider.accounts.refresh` accepts an empty provider ID to list all providers
on its explicit target; other provider commands require a provider ID. Account
display names fall back from email to `displayName` to `Unknown account`.
`UsageWindow.resetsAt` uses its zero time for an unreported reset moment.

## Complete DTO field inventory

Generated from the exported struct fields in `internal/contracts/*.go`; `?` marks JSON fields tagged `omitempty`.

### Account

| JSON field | Go type |
|---|---|
| `plan` | `string` |
| `cloudStatus` | `string` |
| `storageUsed` | `int64` |
| `storageLimit` | `int64` |
| `credits` | `float64` |
| `usage` | `[]UsageWindow` |
| `invoices` | `[]Invoice` |

### ActivitySummary

| JSON field | Go type |
|---|---|
| `toolCalls` | `int` |
| `commands` | `int` |
| `editedFiles` | `int` |
| `reads` | `int` |
| `searches` | `int` |
| `fetches` | `int` |
| `todos` | `int` |
| `other` | `int` |
| `failed` | `int` |
| `thoughts` | `int` |
| `notes` | `int` |
| `subagents` | `int` |

### Annotation

| JSON field | Go type |
|---|---|
| `index` | `uint32` |
| `url` | `string` |
| `title` | `string` |
| `element` | `string` |
| `selector` | `string` |
| `path` | `[]string` |
| `role` | `string` |
| `name` | `string` |
| `text` | `string` |
| `html` | `string` |
| `rect` | `[4]int` |
| `viewport` | `[2]int` |
| `styles` | `[][2]string` |

### AppearanceSettings

| JSON field | Go type |
|---|---|
| `motionPreference` | `string` |
| `smoothScrolling` | `bool` |
| `pauseInBackground` | `bool` |
| `wallpaperColors` | `bool` |
| `renderMath` | `bool` |
| `mathFont` | `string` |
| `mathScale` | `float64` |
| `mode` | `string` |
| `themeId` | `string` |
| `lightThemeId` | `string` |
| `darkThemeId` | `string` |
| `accent` | `string` |
| `surface` | `string` |
| `font` | `string` |
| `fontSize` | `float64` |
| `codeFont` | `string` |
| `codeFontSize` | `float64` |
| `terminalFont` | `string` |
| `terminalFontSize` | `float64` |
| `transcriptWidth` | `float64` |
| `background` | `string` |
| `backgroundName` | `string` |
| `backgroundAssetRef` | `string` |
| `backgroundAdjustment` | `BackgroundAdjustment` |
| `backgroundEffect` | `string` |
| `reduceMotion` | `bool` |

### AppshotCapabilities

| JSON field | Go type |
|---|---|
| `platform` | `string` |
| `globalShortcut` | `string` |
| `windowCapture` | `string` |
| `applicationText` | `string` |

### AppshotSettings

| JSON field | Go type |
|---|---|
| `enabled` | `bool` |
| `destination` | `string` |
| `shortcut` | `string` |
| `includeCursor` | `bool` |
| `sound` | `bool` |

### Artifact

| JSON field | Go type |
|---|---|
| `source` | `string` |
| `assetRef` | `string` |
| `width` | `float64` |
| `height` | `float64` |
| `baseline` | `float64` |

### Attachment

| JSON field | Go type |
|---|---|
| `id` | `string` |
| `name` | `string` |
| `mime` | `string` |
| `size` | `int64` |
| `width` | `int` |
| `height` | `int` |
| `assetRef` | `string` |
| `status` | `string` |
| `sourceApp?` | `string` |
| `windowTitle?` | `string` |

### AttachmentImportRequest

| JSON field | Go type |
|---|---|
| `paths?` | `[]string` |
| `clipboard?` | `*ClipboardAttachment` |
| `transfer` | `bool` |

### BackgroundAdjustment

| JSON field | Go type |
|---|---|
| `focalX` | `float32` |
| `focalY` | `float32` |
| `zoom` | `float32` |

### BillingCheckout

| JSON field | Go type |
|---|---|
| `id` | `string` |
| `kind` | `string` |
| `planId?` | `string` |
| `packs?` | `int` |
| `state` | `string` |
| `error?` | `string` |

### BillingCredits

| JSON field | Go type |
|---|---|
| `monthly` | `float64` |
| `monthlyTotal` | `float64` |
| `monthlyEndsAt` | `time.Time` |
| `extra` | `float64` |
| `total` | `float64` |

### BillingPack

| JSON field | Go type |
|---|---|
| `priceCents` | `int` |
| `credits` | `float64` |

### BillingPlan

| JSON field | Go type |
|---|---|
| `id` | `string` |
| `name` | `string` |
| `priceCents` | `int` |
| `monthlyCredits` | `float64` |

### BillingState

| JSON field | Go type |
|---|---|
| `loadState` | `LoadStatus` |
| `available` | `bool` |
| `error?` | `string` |
| `plans` | `[]BillingPlan` |
| `subscription?` | `*BillingSubscription` |
| `credits` | `BillingCredits` |
| `pack` | `BillingPack` |
| `hasCustomer` | `bool` |
| `checkout?` | `*BillingCheckout` |
| `notice?` | `string` |

### BillingSubscription

| JSON field | Go type |
|---|---|
| `planId` | `string` |
| `status` | `string` |
| `periodEnd` | `time.Time` |
| `cancelAtPeriodEnd` | `bool` |

### Block

| JSON field | Go type |
|---|---|
| `annotation?` | `*Annotation` |
| `id` | `string` |
| `kind` | `string` |
| `text?` | `string` |
| `language?` | `string` |
| `tool?` | `*ToolCall` |
| `question?` | `*Question` |
| `permission?` | `*Permission` |
| `todos?` | `[]Todo` |
| `subagent?` | `*Subagent` |
| `attachment?` | `*Attachment` |
| `artifact?` | `*Artifact` |

### BrowserTab

| JSON field | Go type |
|---|---|
| `id` | `string` |
| `url` | `string` |
| `title` | `string` |
| `loading` | `bool` |
| `canGoBack` | `bool` |
| `canGoForward` | `bool` |
| `error?` | `string` |

### Capabilities

| JSON field | Go type |
|---|---|
| `appshots` | `bool` |
| `cloud` | `bool` |
| `browser` | `bool` |
| `voice` | `bool` |

### Change

| JSON field | Go type |
|---|---|
| `id` | `string` |
| `projectId` | `string` |
| `path` | `string` |
| `oldPath?` | `string` |
| `status` | `string` |
| `additions` | `int` |
| `deletions` | `int` |
| `oldContent` | `string` |
| `newContent` | `string` |
| `hunks` | `[]DiffHunk` |

### ChangeRequestSummary

| JSON field | Go type |
|---|---|
| `provider` | `string` |
| `number` | `int` |
| `title` | `string` |
| `url` | `string` |
| `state` | `string` |
| `baseRef` | `string` |
| `headRef` | `string` |
| `detailId?` | `string` |

### Check

| JSON field | Go type |
|---|---|
| `name` | `string` |
| `status` | `string` |
| `detail` | `string` |

### CheckoutPRKey

| JSON field | Go type |
|---|---|
| `targetId` | `string` |
| `checkoutId` | `string` |
| `repoRoot` | `string` |
| `branch` | `string` |

### CheckoutPRStatus

| JSON field | Go type |
|---|---|
| `key` | `CheckoutPRKey` |
| `state` | `LoadStatus` |
| `summary?` | `*ChangeRequestSummary` |
| `error?` | `string` |
| `generation` | `uint64` |

### CheckoutPlan

| JSON field | Go type |
|---|---|
| `kind` | `CheckoutKind` |
| `branch?` | `string` |
| `base?` | `string` |
| `path?` | `string` |
| `worktreeId?` | `string` |

### ClipboardAttachment

| JSON field | Go type |
|---|---|
| `name` | `string` |
| `mime` | `string` |
| `data` | `[]byte` |

### CloudConnection

| JSON field | Go type |
|---|---|
| `providerId` | `string` |
| `state` | `string` |
| `account?` | `string` |
| `userCode?` | `string` |
| `url?` | `string` |
| `error?` | `string` |
| `authorized` | `bool` |

### CloudRepository

| JSON field | Go type |
|---|---|
| `id` | `string` |
| `name` | `string` |
| `private` | `bool` |
| `authorized` | `bool` |

### CloudSettingsState

| JSON field | Go type |
|---|---|
| `loadState` | `LoadStatus` |
| `available` | `bool` |
| `state` | `string` |
| `deviceId?` | `string` |
| `error?` | `string` |
| `github` | `CloudConnection` |
| `repositories` | `[]CloudRepository` |
| `repositoriesState` | `LoadStatus` |
| `usageRange` | `string` |
| `usage` | `[]CloudUsageDay` |
| `connections` | `[]CloudConnection` |

### CloudUsageDay

| JSON field | Go type |
|---|---|
| `day` | `time.Time` |
| `credits` | `float64` |

### Command

| JSON field | Go type |
|---|---|
| `git?` | `*GitRequest` |
| `projectCreate?` | `*ProjectCreate` |
| `integration?` | `*IntegrationRequest` |
| `adjustment?` | `*BackgroundAdjustment` |
| `action?` | `*ProjectAction` |
| `terminalSize?` | `*TerminalSize` |
| `comment?` | `*CommentInput` |
| `kind` | `string` |
| `id?` | `string` |
| `sessionId?` | `string` |
| `projectId?` | `string` |
| `text?` | `string` |
| `name?` | `string` |
| `path?` | `string` |
| `enabled` | `bool` |
| `version?` | `uint64` |
| `send?` | `*SendRequest` |
| `settings?` | `*Settings` |
| `session?` | `*SessionUpdate` |
| `attachment?` | `*Attachment` |

### Comment

| JSON field | Go type |
|---|---|
| `id` | `string` |
| `author` | `string` |
| `body` | `string` |
| `path?` | `string` |
| `line?` | `int` |
| `createdAt` | `time.Time` |

### CommentInput

| JSON field | Go type |
|---|---|
| `body` | `string` |
| `path?` | `string` |
| `line?` | `int` |

### Commit

| JSON field | Go type |
|---|---|
| `sha` | `string` |
| `parents` | `[]string` |
| `subject` | `string` |
| `author` | `string` |
| `authoredAt` | `time.Time` |
| `refs` | `[]string` |

### ContentSearchMatch

| JSON field | Go type |
|---|---|
| `fileId` | `string` |
| `path` | `string` |
| `line` | `int` |
| `column` | `int` |
| `text` | `string` |
| `ranges` | `[]MatchRange` |

### ContextUsage

| JSON field | Go type |
|---|---|
| `tokens?` | `*uint64` |
| `window?` | `*uint64` |

### ConversationSourceContext

| JSON field | Go type |
|---|---|
| `checkoutId` | `string` |
| `repoRoot` | `string` |
| `cwd` | `string` |
| `branch` | `string` |
| `headSha?` | `string` |
| `observedAt` | `time.Time` |

### Device

| JSON field | Go type |
|---|---|
| `version?` | `string` |
| `id` | `string` |
| `name` | `string` |
| `platform` | `string` |
| `architecture` | `string` |
| `status` | `string` |
| `current` | `bool` |
| `lastSeen` | `time.Time` |

### DiffHunk

| JSON field | Go type |
|---|---|
| `header` | `string` |
| `oldStart` | `int` |
| `newStart` | `int` |
| `lines` | `[]DiffLine` |

### DiffLine

| JSON field | Go type |
|---|---|
| `kind` | `string` |
| `text` | `string` |
| `oldLine` | `int` |
| `newLine` | `int` |

### DirectoryEntry

| JSON field | Go type |
|---|---|
| `name` | `string` |
| `path` | `string` |
| `hidden` | `bool` |
| `project` | `bool` |

### DirectoryListing

| JSON field | Go type |
|---|---|
| `deviceId` | `string` |
| `path` | `string` |
| `parent` | `string` |
| `home` | `string` |
| `entries` | `[]DirectoryEntry` |
| `revision` | `uint64` |

### DirectoryRequest

| JSON field | Go type |
|---|---|
| `deviceId` | `string` |
| `path` | `string` |

### Event

| JSON field | Go type |
|---|---|
| `schemaVersion` | `int` |
| `eventId` | `string` |
| `sequence` | `uint64` |
| `revision` | `uint64` |
| `kind` | `string` |
| `sessionId?` | `string` |
| `runId?` | `string` |
| `requestId?` | `string` |
| `text?` | `string` |
| `snapshot?` | `*Snapshot` |

### File

| JSON field | Go type |
|---|---|
| `id` | `string` |
| `projectId` | `string` |
| `path` | `string` |
| `name` | `string` |
| `kind` | `string` |
| `language` | `string` |
| `content` | `string` |
| `version` | `uint64` |
| `readOnly` | `bool` |
| `modified` | `bool` |
| `parentId` | `string` |
| `assetRef?` | `string` |

### FileSearchMatch

| JSON field | Go type |
|---|---|
| `fileId` | `string` |
| `path` | `string` |
| `kind` | `string` |

### FileSettings

| JSON field | Go type |
|---|---|
| `autosave` | `bool` |
| `autosaveDelay` | `int` |
| `showHidden` | `bool` |
| `wordWrap` | `bool` |
| `lineNumbers` | `bool` |
| `fontSize` | `float64` |

### GeneralSettings

| JSON field | Go type |
|---|---|
| `sendBehavior` | `string` |
| `compactMode` | `bool` |
| `compactModelPicker` | `bool` |
| `stopWithEscape` | `bool` |
| `prDestination` | `string` |
| `threadNamingModel` | `string` |
| `customWorktreeLocation` | `bool` |
| `worktreePath` | `string` |
| `creditZeron` | `bool` |
| `launchAtLogin` | `bool` |
| `restoreWindows` | `bool` |

### GitRequest

| JSON field | Go type |
|---|---|
| `scope` | `RepoRefScope` |
| `ref?` | `string` |
| `force` | `bool` |
| `source?` | `*CheckoutPRKey` |

### IntegrationRequest

| JSON field | Go type |
|---|---|
| `targetId?` | `string` |
| `providerId?` | `string` |
| `accountId?` | `string` |
| `value?` | `string` |
| `packs?` | `int` |

### IntegrationState

| JSON field | Go type |
|---|---|
| `observedAt` | `time.Time` |
| `workspaceScope` | `string` |
| `providersState` | `LoadStatus` |
| `providersError?` | `string` |
| `providers` | `[]ProviderDetails` |
| `accounts` | `[]ProviderAccount` |
| `login?` | `*ProviderLogin` |
| `cloud` | `CloudSettingsState` |
| `billing` | `BillingState` |

### Invoice

| JSON field | Go type |
|---|---|
| `id` | `string` |
| `date` | `time.Time` |
| `amount` | `float64` |
| `status` | `string` |

### MatchRange

| JSON field | Go type |
|---|---|
| `start` | `int` |
| `end` | `int` |

### Message

| JSON field | Go type |
|---|---|
| `continuationOf?` | `string` |
| `durationMs?` | `int64` |
| `id` | `string` |
| `sessionId` | `string` |
| `turnId` | `string` |
| `role` | `string` |
| `createdAt` | `time.Time` |
| `blocks` | `[]Block` |
| `streaming` | `bool` |
| `usage` | `Usage` |

### Model

| JSON field | Go type |
|---|---|
| `id` | `string` |
| `providerId` | `string` |
| `label` | `string` |
| `description` | `string` |
| `reasoningLevels` | `[]string` |
| `contextLimit` | `int` |
| `options` | `[]ModelOption` |

### ModelCatalogState

| JSON field | Go type |
|---|---|
| `state` | `ModelCatalogStatus` |
| `error?` | `string` |

### ModelOption

| JSON field | Go type |
|---|---|
| `id` | `string` |
| `label` | `string` |
| `choices` | `[]string` |
| `choiceLabels?` | `map[string]string` |
| `defaultChoice` | `string` |

### ModelPickerSettings

| JSON field | Go type |
|---|---|
| `favorites` | `[]ModelRef` |
| `providerId` | `string` |
| `options` | `map[string]string` |
| `selections` | `map[string]ModelSelection` |

### ModelRef

| JSON field | Go type |
|---|---|
| `providerId` | `string` |
| `modelId` | `string` |

### ModelSelection

| JSON field | Go type |
|---|---|
| `modelId` | `string` |
| `reasoning` | `string` |

### NotificationSettings

| JSON field | Go type |
|---|---|
| `agentUpdates` | `bool` |
| `errors` | `bool` |
| `enabled` | `bool` |
| `completion` | `bool` |
| `questions` | `bool` |
| `sound` | `bool` |
| `whenFocused` | `bool` |

### Permission

| JSON field | Go type |
|---|---|
| `id` | `string` |
| `title` | `string` |
| `description` | `string` |
| `command` | `string` |
| `answer` | `string` |

### Project

| JSON field | Go type |
|---|---|
| `gitDetected` | `bool` |
| `cloudRepository?` | `string` |
| `defaultBranch?` | `string` |
| `actions` | `[]ProjectAction` |
| `id` | `string` |
| `name` | `string` |
| `path` | `string` |
| `deviceId` | `string` |
| `icon` | `string` |
| `branch` | `string` |
| `branches` | `[]string` |
| `worktrees` | `[]Worktree` |

### ProjectAction

| JSON field | Go type |
|---|---|
| `id` | `string` |
| `name` | `string` |
| `command` | `string` |
| `icon` | `string` |
| `enabled` | `bool` |
| `runOnNewWorktree` | `bool` |
| `lastRunStatus` | `string` |

### ProjectCreate

| JSON field | Go type |
|---|---|
| `deviceId` | `string` |
| `name` | `string` |
| `path` | `string` |

### Provider

| JSON field | Go type |
|---|---|
| `accessModes` | `[]AccessMode` |
| `supportsPlan` | `bool` |
| `id` | `string` |
| `name` | `string` |
| `icon` | `string` |
| `status` | `string` |
| `account` | `string` |
| `enabled` | `bool` |
| `models` | `[]string` |

### ProviderAccount

| JSON field | Go type |
|---|---|
| `switchable` | `bool` |
| `reportsUsage` | `bool` |
| `organization?` | `string` |
| `loginProvider?` | `string` |
| `id` | `string` |
| `providerId` | `string` |
| `targetId` | `string` |
| `email` | `string` |
| `displayName?` | `string` |
| `plan` | `string` |
| `inUse` | `bool` |
| `authStatus` | `string` |
| `usageState` | `LoadStatus` |
| `usageError?` | `string` |
| `usage` | `[]UsageWindow` |
| `canRemove` | `bool` |

### ProviderDetails

| JSON field | Go type |
|---|---|
| `loginOptions` | `[]ProviderLoginOption` |
| `accountNote?` | `string` |
| `providerId` | `string` |
| `targetId` | `string` |
| `installed` | `bool` |
| `canInstall` | `bool` |
| `enabled` | `bool` |
| `installState` | `string` |
| `error?` | `string` |
| `version` | `string` |
| `latestVersion` | `string` |
| `updateState` | `string` |
| `updatePolicy` | `string` |
| `dollarSkills` | `bool` |
| `separateSlash` | `bool` |
| `nativeSubagents` | `bool` |
| `canSwitchSubagents` | `bool` |
| `accountsState` | `LoadStatus` |
| `accountsError?` | `string` |
| `warning?` | `string` |
| `loginKind` | `string` |
| `singleLogin` | `bool` |

### ProviderLogin

| JSON field | Go type |
|---|---|
| `loginProvider?` | `string` |
| `id` | `string` |
| `providerId` | `string` |
| `targetId` | `string` |
| `kind` | `string` |
| `state` | `string` |
| `url` | `string` |
| `userCode?` | `string` |
| `error?` | `string` |
| `expiresAt` | `time.Time` |

### ProviderLoginOption

| JSON field | Go type |
|---|---|
| `id` | `string` |
| `label` | `string` |
| `kind` | `string` |

### PullRequest

| JSON field | Go type |
|---|---|
| `id` | `string` |
| `number` | `int` |
| `repository` | `string` |
| `title` | `string` |
| `author` | `string` |
| `status` | `string` |
| `draft` | `bool` |
| `baseBranch` | `string` |
| `headBranch` | `string` |
| `body` | `string` |
| `checks` | `[]Check` |
| `comments` | `[]Comment` |
| `changeIds` | `[]string` |
| `updatedAt` | `time.Time` |

### Question

| JSON field | Go type |
|---|---|
| `id` | `string` |
| `title` | `string` |
| `options` | `[]string` |
| `multiSelect` | `bool` |
| `answer` | `string` |

### QueuedMessage

| JSON field | Go type |
|---|---|
| `accessMode` | `AccessMode` |
| `plan` | `bool` |
| `annotations?` | `[]Annotation` |
| `modelId?` | `string` |
| `reasoningLevel?` | `string` |
| `deviceId?` | `string` |
| `branch?` | `string` |
| `worktreeId?` | `string` |
| `id` | `string` |
| `sessionId` | `string` |
| `text` | `string` |
| `attachments` | `[]Attachment` |
| `createdAt` | `time.Time` |

### RepoRef

| JSON field | Go type |
|---|---|
| `name` | `string` |
| `current` | `bool` |
| `worktreePath?` | `string` |

### RepoRefScope

| JSON field | Go type |
|---|---|
| `projectId` | `string` |
| `targetId` | `string` |
| `repoPath` | `string` |
| `cloudRepository?` | `string` |
| `defaultBranch?` | `string` |

### RepoRefs

| JSON field | Go type |
|---|---|
| `scope` | `RepoRefScope` |
| `state` | `LoadStatus` |
| `rows` | `[]RepoRef` |
| `error?` | `string` |
| `generation` | `uint64` |
| `switching?` | `string` |
| `switchError?` | `string` |

### Run

| JSON field | Go type |
|---|---|
| `requestId?` | `string` |
| `durationMs` | `int64` |
| `summary` | `ActivitySummary` |
| `accessMode` | `AccessMode` |
| `plan` | `bool` |
| `id` | `string` |
| `sessionId` | `string` |
| `status` | `string` |
| `finishReason` | `string` |
| `error?` | `string` |
| `startedAt` | `time.Time` |
| `finishedAt?` | `*time.Time` |

### SendRequest

| JSON field | Go type |
|---|---|
| `checkout?` | `*CheckoutPlan` |
| `accessMode` | `AccessMode` |
| `plan` | `bool` |
| `annotations?` | `[]Annotation` |
| `deviceId?` | `string` |
| `branch?` | `string` |
| `worktreeId?` | `string` |
| `projectId?` | `string` |
| `sessionId` | `string` |
| `text` | `string` |
| `modelId` | `string` |
| `reasoningLevel` | `string` |
| `attachments` | `[]Attachment` |
| `requestId` | `string` |

### Session

| JSON field | Go type |
|---|---|
| `cwd?` | `string` |
| `sourceContext?` | `*ConversationSourceContext` |
| `accessMode` | `AccessMode` |
| `plan` | `bool` |
| `parentSessionId?` | `string` |
| `sideChat` | `bool` |
| `id` | `string` |
| `title` | `string` |
| `projectId` | `string` |
| `deviceId` | `string` |
| `modelId` | `string` |
| `reasoningLevel` | `string` |
| `branch` | `string` |
| `worktreeId` | `string` |
| `status` | `SessionStatus` |
| `unread` | `bool` |
| `pinned` | `bool` |
| `archived` | `bool` |
| `pinOrder` | `int` |
| `createdAt` | `time.Time` |
| `updatedAt` | `time.Time` |
| `lastOutcome` | `string` |
| `runningSubagents` | `int` |
| `pullRequestNumber` | `int` |

### SessionUpdate

| JSON field | Go type |
|---|---|
| `accessMode?` | `*AccessMode` |
| `plan?` | `*bool` |
| `reasoningLevel?` | `*string` |
| `branch?` | `*string` |
| `deviceId?` | `*string` |
| `worktreeId?` | `*string` |
| `id` | `string` |
| `title?` | `*string` |
| `pinned?` | `*bool` |
| `archived?` | `*bool` |
| `unread?` | `*bool` |
| `pinOrder?` | `*int` |
| `modelId?` | `*string` |

### Settings

| JSON field | Go type |
|---|---|
| `modelPicker` | `ModelPickerSettings` |
| `appearance` | `AppearanceSettings` |
| `general` | `GeneralSettings` |
| `notifications` | `NotificationSettings` |
| `files` | `FileSettings` |
| `appshots` | `AppshotSettings` |
| `shortcuts` | `[]Shortcut` |
| `sidebarWidth` | `float64` |
| `filesWidth` | `float64` |
| `rightPaneWidth` | `float64` |
| `terminalHeight` | `float64` |
| `sidebarCompact` | `bool` |
| `sidebarShowHarness` | `bool` |
| `sidebarShowBranch` | `bool` |
| `sidebarShowPullRequest` | `bool` |
| `sidebarShowProjectFilter` | `bool` |
| `sidebarShowArchived` | `bool` |
| `sidebarGrouping` | `string` |
| `sidebarSort` | `string` |
| `showProjectLabels` | `bool` |
| `showIcons` | `bool` |

### Shortcut

| JSON field | Go type |
|---|---|
| `id` | `string` |
| `label` | `string` |
| `keys` | `string` |
| `group` | `string` |

### Snapshot

| JSON field | Go type |
|---|---|
| `integrations` | `IntegrationState` |
| `modelCatalog` | `ModelCatalogState` |
| `appshotCapabilities` | `AppshotCapabilities` |
| `themeLibrary` | `[]ThemeLibraryEntry` |
| `themeImport?` | `*ThemeImportPreview` |
| `schemaVersion` | `int` |
| `revision` | `uint64` |
| `user` | `User` |
| `capabilities` | `Capabilities` |
| `projects` | `[]Project` |
| `devices` | `[]Device` |
| `sessions` | `[]Session` |
| `messages` | `map[string][]Message` |
| `models` | `[]Model` |
| `providers` | `[]Provider` |
| `settings` | `Settings` |
| `files` | `[]File` |
| `changes` | `[]Change` |
| `commits` | `[]Commit` |
| `pullRequests` | `[]PullRequest` |
| `repoRefs?` | `map[string]RepoRefs` |
| `checkoutPR?` | `map[string]CheckoutPRStatus` |
| `terminals` | `[]Terminal` |
| `queue` | `[]QueuedMessage` |
| `runs` | `[]Run` |
| `account` | `Account` |
| `appshots` | `[]Attachment` |
| `attachments` | `[]Attachment` |
| `update` | `Update` |
| `contextUsage` | `map[string]ContextUsage` |

### Subagent

| JSON field | Go type |
|---|---|
| `id` | `string` |
| `sessionId` | `string` |
| `name` | `string` |
| `task` | `string` |
| `status` | `string` |
| `summary` | `string` |

### Terminal

| JSON field | Go type |
|---|---|
| `id` | `string` |
| `projectId` | `string` |
| `title` | `string` |
| `cwd` | `string` |
| `columns` | `int` |
| `rows` | `int` |
| `status` | `string` |
| `output` | `string` |

### TerminalSize

| JSON field | Go type |
|---|---|
| `columns` | `int` |
| `rows` | `int` |

### ThemeImportPreview

| JSON field | Go type |
|---|---|
| `path` | `string` |
| `mode` | `string` |
| `variants` | `[]ThemeImportVariant` |
| `warnings` | `[]string` |
| `error` | `string` |
| `ready` | `bool` |

### ThemeImportVariant

| JSON field | Go type |
|---|---|
| `id` | `string` |
| `name` | `string` |
| `appearance` | `string` |
| `selected` | `bool` |

### ThemeLibraryEntry

| JSON field | Go type |
|---|---|
| `id` | `string` |
| `name` | `string` |
| `sourceKind` | `string` |
| `path` | `string` |
| `variantIds` | `[]string` |
| `warning` | `string` |

### Todo

| JSON field | Go type |
|---|---|
| `id` | `string` |
| `text` | `string` |
| `status` | `string` |

### ToolCall

| JSON field | Go type |
|---|---|
| `kind?` | `ToolKind` |
| `args?` | `string` |
| `language?` | `string` |
| `isError` | `bool` |
| `exitCode?` | `*int` |
| `diff?` | `*Change` |
| `id` | `string` |
| `name` | `string` |
| `summary` | `string` |
| `command` | `string` |
| `path` | `string` |
| `status` | `string` |
| `output` | `string` |
| `startedAt` | `time.Time` |
| `finishedAt?` | `*time.Time` |

### Update

| JSON field | Go type |
|---|---|
| `currentVersion` | `string` |
| `availableVersion` | `string` |
| `status` | `string` |
| `progress` | `float64` |

### Usage

| JSON field | Go type |
|---|---|
| `inputTokens` | `int` |
| `outputTokens` | `int` |
| `cachedTokens` | `int` |
| `contextTokens` | `int` |
| `contextLimit` | `int` |
| `cost` | `float64` |

### UsageWindow

| JSON field | Go type |
|---|---|
| `label` | `string` |
| `usedPercent` | `float64` |
| `resetsAt` | `time.Time` |

### User

| JSON field | Go type |
|---|---|
| `id` | `string` |
| `name` | `string` |
| `email` | `string` |
| `initials` | `string` |
| `signedIn` | `bool` |

### WorkspaceSearchRequest

| JSON field | Go type |
|---|---|
| `projectId` | `string` |
| `query` | `string` |
| `fileLimit` | `int` |
| `contentLimit` | `int` |
| `matchesPerFile` | `int` |
| `recent` | `bool` |
| `includeHidden` | `bool` |
| `namesOnly` | `bool` |

### WorkspaceSearchResult

| JSON field | Go type |
|---|---|
| `files` | `[]FileSearchMatch` |
| `content` | `[]ContentSearchMatch` |
| `revision` | `uint64` |

### Worktree

| JSON field | Go type |
|---|---|
| `baseRef?` | `string` |
| `id` | `string` |
| `name` | `string` |
| `path` | `string` |
| `branch` | `string` |
