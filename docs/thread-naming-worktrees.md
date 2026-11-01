# Thread naming and Git checkouts

The real Go core owns thread naming and checkout preparation for both the local
desktop and the headless host. The desktop dispatches typed commands from its
service worker; Git and model requests run outside the core state mutex.

`General.ThreadNamingModel = "first-message"` uses up to 48 characters from the
first message without another model request. Selecting a configured model key
(`provider-id/model-id`) asks that model for a title separately from Agent
execution. This adds a provider request and its associated usage cost. The
request has no tools, at most 4,000 input characters, at most 96 output tokens,
and a 30-second timeout. A failed request retains the initial title and publishes
`session.naming.failed`; it does not fail the Agent run. Explicit renaming,
deletion, naming-model changes, provider replacement/removal and host shutdown
cancel pending naming. Results cannot replace a manually supplied title.

Adding a project detects Git and discovers local/remote refs and registered
worktrees. `git.refs.load` and `git.ref.switch` publish loading/switching state
immediately, perform Git work in an owned background job, and publish success or
an actionable error. Switching runs `git switch` without force or reset. Active
project runs and checkout preparation prevent branch switching. A dirty checkout
that Git cannot safely switch remains intact.

`SendRequest.Checkout` is consumed when creating a conversation:

- `current` uses the selected project directory and verifies any selected branch.
  Plain directories are supported.
- `reuseWorktree` verifies the path, branch and optional identity against Git's
  registered worktrees in that repository.
- `newWorktree` resolves the requested base to a commit (`HEAD` when empty), then
  runs `git worktree add -b` with a unique `feat/` branch. Its path is always
  `<host-data-directory>/worktrees/<short-id>`; there is no custom worktree
  directory setting. The 12-character ID uses random lowercase letters and digits.
  Creation reserves a new directory atomically and retries directory collisions.
  The registered data root must be available and match its stored identity.
- `cloud` is rejected by the local host.

The resulting conversation owns its CWD, branch, worktree identity and source
context. Real Agent tools execute in that CWD. The main project checkout remains
on its original branch when creating another worktree. Worktrees and conversation
ownership survive a host restart. An existing conversation keeps its own checkout
when a later send carries a draft checkout choice. Duplicate accepted request IDs
do not create additional worktrees.

The configuration directory (`Store.ConfigDirectory()`, normally `~/.gcode`) owns
settings and the small conversation index. `Store.Directory()` is the active
runtime data root. Selecting another root takes effect after restart without
copying, deleting or moving existing data. Each conversation keeps its
`StorageRootID`: subsequent messages, runs and new attachments for that
conversation stay in the original root. A newly created conversation uses the
active root, including when it reuses a worktree from another root. Its CWD
remains the selected worktree's original path.
Draft attachments bind their import root before a conversation exists; the first
send retains that root instead of relocating or reinterpreting the attachment.

An independent conversation without a project owns
`<bound-data-root>/workspaces/<conversation-id>` instead of sharing a workspace.
Its ID is also a 12-character random lowercase letter/digit identifier and does
not change with its title. Side chats, forks and subagents inherit the parent's
working directory so that they can cooperate on its files; separate conversation
records do not imply separate working directories. A user-created side chat or
fork records its new history in the active root; an engine-created subagent keeps
the parent's storage root. Old roots must remain available to read referenced
history, attachments and working files; missing roots are reported as unavailable.

Git is required for checkout features. Worktree paths refer to the host's local
filesystem; a remote client cannot select a directory on its own device. Existing
worktrees are never removed by conversation deletion. Failure cleanup does not
force-delete files or branches. Before creating a
checkout the core validates the command/skill prompt, attachments, selected model
and reasoning level. If a task is not accepted, it removes only the checkout
created by that dispatch, after verifying its branch, original commit and absence
of tracked, untracked or ignored changes. Branch deletion compares the original
commit atomically. Changed checkouts remain intact and the returned error identifies
the retained path and branch. Reused checkouts are never rollback targets.

Verification uses `httptest` for all three model protocols and temporary real Git
repositories. `go test ./internal/engine -run 'Test(Naming|Worktree|GitRefs)'`
covers model selection, request limits, cancellation, fixed data-root locations,
short directory IDs, missing-root rejection, actual Agent writes in the new
checkout, restart/reuse, duplicate submissions, and dirty
switch failures. These tests do not constitute real-provider or native desktop
acceptance.
