# Native application contract

`internal/contracts` contains the authoritative Go types. Local MyGo UI calls
`contracts.Service` directly; snapshots, commands and events stay typed Go values.
JSON is used for disk persistence, model APIs and real remote device boundaries.
No HTTP, stdio or JavaScript bridge sits between the local UI and engine.

The production implementation is `internal/engine`. Deterministic scenarios use
`internal/mock` only when explicitly launched with `--scenario`. The historical
[UI fixture contract](ui-fixture-contracts.md) describes those scenes, not live
model, tool, account or workspace capabilities.

## Ownership and events

The engine owns providers, sessions, queued input, runs, full execution history,
compacted model context, files and persistence. The UI owns drafts and presentation.
Commands are cloned before background dispatch. Snapshots and subscriber events
have independent owned nested data; no rendering contexts or elements cross into
the core. MyGo schedules updates through `Window.Update`.

A subscription begins with a complete snapshot. `message.changed` replaces one
message; `message.delta` appends text or reasoning to a message block. Other state
changes carry complete snapshots. A slow subscriber receives a fresh snapshot
instead of an undetectable event gap. Remote reconnection starts from a fresh
snapshot and then follows future events; there is no persisted event replay log.

`SendRequest.requestId` correlates a new send with its `Run`; branch creation uses
`Command.requestId` and `Session.creationRequestId`. A side chat forks a snapshot
of the parent's model history and compacted context into a new, independently
persisted session. Its visible transcript starts empty. During a parent run,
the boundary ends at the latest accepted human input and excludes subsequent
assistant/tool execution. Later parent messages do not update that snapshot.
The child inherits the model and workspace configuration, but no Goal, queued
input, active run, archive, unread or pin state. Its system instructions treat
parent history as reference, require a new side-chat request, and allow workspace
modifications only when requested there. Both sessions use the same files.
A regular fork also copies the visible conversation transcript.
Desktop sends generate a fresh random request ID for each new action, including
after relaunch or recreating a side composer. Re-delivery of that same command
retains its ID so the engine's durable deduplication still prevents a second run.
User messages sent during a run enter the queue. `steer` is consumed at a model-step
boundary, and follow-up input after the current answer, one queued row per turn in
queue order. `queue.move` reorders a row before or after another row of the same
session; `queue.send` removes a row, stops the live run and opens the next turn with
it. Removing a queued message prevents its execution.

Desktop side-chat creation lives in the right workspace's empty launcher and
its `+` menu. Each explicit action creates a new child tab and focuses its
independent composer without replacing the parent selection or draft. Session
information keeps existing-child navigation and the regular fork action, with
no side-chat creation action. See [side-chats.md](side-chats.md).

## Live capabilities

Exactly three model protocols are supported: `openai-chat`, `openai-responses`,
`anthropic-messages`. `Snapshot.customProviders` contains public configuration
with API keys removed and `hasApiKey` metadata. `provider.save` accepts private
configuration; `preserveKey` explicitly retains a saved key during edits.
An explicit reveal action reads a saved key through `ProviderKeyService`;
local desktop calls the core directly, while remote clients use the authenticated
`GET /v1/provider-key?id=...` endpoint with `Cache-Control: no-store`.
Revealed values stay in transient UI state, outside snapshots and events.
`provider.reorder` and `provider.models.reorder` accept only a typed `reorder`
payload: `sourceId`, `targetId`, and `after` move an existing item immediately
before or after another. Model moves additionally require `providerId`; model IDs
are local to that provider. The host validates both IDs against its current
configuration and persists the resulting order, preserving credentials and all
model metadata. Settings lists and the model picker use this same order.
Each configured model owns its `reasoningLevels` array. An empty array exposes
no reasoning control; a new selection defaults to the last configured entry.
Explicit unsupported levels fail validation. The former `reasoning` boolean
has been removed without legacy parsing or migration.

`Settings.subagents` contains task profiles with an optional `ModelSelection`.
An absent selection inherits the invoking run's model and reasoning together.
Profiles and provider configuration are frozen for each accepted run. Child
sessions own independent history and retain their actual model/level selection;
parent `subagent` blocks link to those sessions. Foreground children drain before
the invoking tool returns. Background children enqueue a durable completion
message (`QueuedMessage.source = subagent`) and can wake an idle parent without
displaying the notification as human input. See [subagents](subagents.md).

The engine binds the configured channel/model through `model.Client.Bind`.
The Agent consumes its own single-method `Model` interface and neutral
`model.Request`, `Delta`, and `Result` values. Three independent adapters own wire
encoding, authentication and event decoding; HTTP transport and SSE framing are
shared. Adapter-owned continuation is opaque to the engine and Agent, persisted
with messages, and replayed only for the matching channel/protocol/endpoint/
credential/model target. Switching models preserves the full history without
engine-side protocol field cleanup. See [model boundaries](model-adapters.md).

The engine binds a channel/model configuration through `model.Client.Bind`.
The Agent consumes its own small `Model` interface and uniform
`model.Request`/`Delta`/`Result` types. Requests carry no channel configuration.
Chat Completions, Responses and Anthropic Messages each own a separate adapter;
HTTP/retry and SSE framing are shared. Stop reasons and complete tool calls are
normalized before reaching the Agent. Compaction uses the same bound interface.

Adapter-owned `Message.Continuation` is opaque to the Agent and engine. It is
persisted and deeply cloned with history. Native reasoning/signature/output data
is replayed only to the matching channel ID, protocol, endpoint, credential and
model ID; other targets receive the uniform message/tool history. Model switches
do not require engine code to parse or clear private protocol fields. This is a
direct internal format update with no legacy aliases or migration path.

Core commands cover custom providers, settings, projects, sessions, model naming,
real Git refs/worktree preparation, send/stop/retry,
goal set/pause/resume/clear,
queue remove/steer, user questions, interactive terminals, lazy file reads and file edits, and data-directory
changes. Other prototype commands return a visible unsupported-operation error.
Billing and cloud accounts are not implemented. Tools execute with the host
process's OS permissions; session, send and run contracts expose no access mode
or Plan configuration.
PR pages, project actions and local account sign-in/sign-out commands were
removed. See [the cleanup record](ui-residual-cleanup.md) and
[naming/checkouts](thread-naming-worktrees.md).
The sidebar now has a repository PR web link, rather than the retired PR
workbench: `Project.pullRequestsUrl` is derived from the host's Git origin
while inspecting repository metadata. It carries no Git credentials. The
desktop uses the existing typed Git ref command/events to refresh missing
metadata and opens the resulting HTTPS page in the system browser.
Sidebar usage is a display projection of recorded message token usage,
deduplicated by the core's execution IDs across forked histories; it does
not read account, subscription or provider billing state.

Tool-start events retain original JSON arguments in `ToolCall.Args`, decode the
shell script into `Command`, and project search patterns into readable summaries.
This projection is shared by local and remote clients. `AskUserQuestion` remains in
execution history and is excluded from activity counts. Interactive questions
temporarily replace the bottom message composer; the transcript retains only a
passive one-line record. `Question` owns a request ID, `pages`, typed `answers`
and `resolved`; each `QuestionPage` contains `question`, `header`, object
`options` and `multiSelect`. The UI submits all `QuestionAnswer` values through
`Command.QuestionAnswers` once, using either selected labels or custom text per
page. Local calls remain typed Go values; only remote boundaries and model tool
results encode them. Partial or invalid answers do not resume execution.
Successful submission restores the ordinary message draft. A failed question call retains an
independent error detail, including validation failures that create no card.
Work-detail viewports retain all received text and scroll internally above
24 visible lines; they do not change the tools' execution/output budgets.
Thought previews and manual folding follow the current transcript audit.

`LinkedFileService.ReadLinkedFile(LinkedFileRequest)` reads a transcript link on
the source host. The request retains `sessionId` and the target path; relative
paths belong only to that session's CWD. Results contain a typed `File` and its
canonical host `absolutePath`. Existing owned project files retain their IDs
and editability; other files are read-only. Image results use managed resource
references read through `ResourceService`. Desktop calls the local core directly;
remote clients use the authenticated linked-file adapter. UI callbacks retain
the source session and reject stale workspace results. New browser tabs start
with an empty address. `Settings.General.OpenLinksExternally` defaults false;
explicit internal/external menu actions override that preference. See
[transcript links](transcript-links.md) for parsing and verification boundaries.

The Agent's read-only search tools are `glob` (path patterns) and `grep` (file
contents); the previous `find` tool is removed. Paths default to the working
directory. Grep defaults to matching file names, with explicit content/count
modes, file filters, context, multiline and pagination controls. Offset precedes
the page limit in every mode. The implementation uses Go standard libraries,
without shelling out to a search binary. See [search semantics](agent-search-tools.md).

The loop records assistant tool calls before executing effects, records results
before requesting another model step, and never automatically replays an uncertain
side effect at startup. Restarted runs become interrupted. Complete execution
history and the model context used after compaction are persisted separately.
Recoverable transport, HTTP and stream failures share one model-layer budget:
10 additional retries (11 physical attempts) per logical model request. Incomplete
display blocks are replaced before retrying; incomplete tool calls are never
committed or executed. Completed tool effects and results remain in the context.
Each physical attempt retains its own usage and trajectory entry. Cancellation
ends recovery immediately. Session.ModelRetry is a cloned runtime projection
for local and remote clients, cleared on output or settlement and omitted from
persistence. The loading slot exposes its retry counter from retry 3, without a
countdown. See [model retry](model-retry.md).

The shell tool is named `Bash` on every platform. On Windows it runs Git Bash
(`RCODE_GIT_BASH_PATH` selects the executable and Windows System32 WSL `bash.exe`
is rejected); on Linux/macOS it runs `bash`. PowerShell is not the Bash executor.
Tool definitions and the system prompt describe the actual host syntax and paths;
no script translation or second shell alias is provided. Each call starts a new
noninteractive shell in the configured directory. Hooks and skill dynamic-context
shells may explicitly select `powershell`.
New core-generated IDs use lowercase random text, including all session branches;
provider-supplied tool-call IDs remain case-sensitive and unchanged.

File reads use project-relative `os.Root`; editor saves reject an on-disk version
conflict. Core agent tools can access files and execute shells as the current OS
user. Tool write/edit changes expose actual before/after text in the transcript.
Files in a project tree are loaded on demand. Search is bounded rather than a
background filesystem index.

Agent tools return `model.ToolOutput` with separate text and image values.
`read` accepts UTF-8 text and PNG/JPEG/GIF/WebP/BMP files, bounds input pixels and
bytes, normalizes JPEG orientation, preserves the first GIF frame's logical
canvas, and resizes/encodes images within local budgets. Images are stored in
execution history; display events carry the textual description only. Each model
adapter maps tool images into its own supported message location, preserving
complete tool-result batches. See `docs/read-tool.md` for limits and validation.

The engine owns interactive shell processes through a pure Go PTY host.
`TerminalService.OpenTerminal` returns a typed `TerminalStream` carrying raw VT
bytes; local UI passes it directly to the MyGo terminal plugin. There is no
JSON conversion or local network hop. Input is forwarded without simulated echo.
Closing the stream detaches a view; `terminal.close` ends the process.
The engine closes owned shells at shutdown, and restart marks them interrupted.
Each view replays up to 8 MiB of recent output and then follows live bytes.
This bounded byte replay is not an exact terminal screen snapshot for arbitrary
full-screen programs. Remote output and input cross authenticated binary HTTP
endpoints; resizing and explicit close use the existing command interface.

New draft attachments are stored under the active data root. Imports for an
existing conversation use that conversation's bound root. Each attachment and
managed resource reference includes its storage root identity, so changing the
active root does not redirect historical reads. Live imports accept PNG/JPEG/GIF
images and UTF-8 text, with 24 MiB per item, 20 items, 96 MiB per batch and image
pixel/dimension limits. This is narrower than the historical mock media showcase.

`Snapshot.runtime` exposes the fixed configuration directory, active data root,
pending root, directory-switch errors and whether the host is remote. Switching
roots never copies existing data. `Session.storageRootId` binds conversation
history to its creation root; a conversation with an unavailable root reports
`storageError` instead of opening empty history. Local-only
`ConnectionSettingsService` stores desktop connection
preferences independently of the remote host. Neither public snapshot exposes
provider keys or the remote API token.

See [backend-v1.md](backend-v1.md) for run commands, storage behavior, MCP and current
verification boundaries. There is no legacy schema/profile import or migration.

`Session.goal` is authoritative core state, cloned with snapshots. `goal.set`
accepts `Command.Goal` (`objective`, optional positive `tokenBudget`); other goal
commands require the current goal ID as `Command.ID` to reject stale UI actions.
Only the user controls goal creation and lifecycle. `goal_read` is a read-only
Agent tool; the runtime checks completion. See [goal execution](goals.md).

The six user-level Agent settings pages share this core. `Snapshot.UserResources`
contains the public skills, commands and MCP projections; `Command.Resource`
carries typed `resource.*` mutations. MCP credential values exist only in private
configuration and submitted commands. `Settings.Hooks` and `Settings.Subagents`
are deeply cloned. `provider.test` exposes transient per-model diagnostics.
Project resources and old prompts/mcp.json formats are not loaded.
See [user settings](user-settings.md) for runtime semantics and verification limits.
