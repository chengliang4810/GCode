# Port gap analysis: RCode (Rust) → GCode (Go)

Cross-repo feature audit from 2026-10-31. RCode at D:\projects\RCode is the
original Rust implementation; GCode is the Go re-implementation. This file
lists what is already at parity, what is missing, and what is intentionally
out of scope, so follow-up ports can pick from an audited list instead of
re-deriving it.

## Already at parity (verified, no action)

| Area | RCode | GCode |
|---|---|---|
| Agent loop | crates/agent/runner.rs (steering, follow-ups, stop hooks, retry, cancel) | internal/agent/agent.go (`Pending`, `Idle`, `BeforeStep`, Stop hooks, `streamResponse` retry) |
| TodoWrite + reminders + stop check + compaction retention | crates/agent/src/todo.rs, runner.rs | internal/agent/todo.go (ported 2026-10-31) |
| Compaction (full) | crates/agent/src/compact.rs `full_compact` | internal/agent/compact.go (`Compact`, 3/4 window trigger, keeps last 8 messages) |
| Goal system (host-owned store, iteration, verification) | crates/agent/src/goal.rs, engine goals.rs | internal/engine/goal.go, `goalModel`, `goalBeforeStep`, `goalIdle` |
| Subagents (spawn tool + notifications + own context) | crates/agent/src/tools/agent.rs, harness builtin_subagents | internal/engine/subagents.go (`Agent` tool); profiles: general-purpose/Explore/Plan/General ⊇ RCode's general-purpose/Explore |
| Core tools | Read/Write/Edit/Glob/Grep/Folder/Bash/Skill/AskUserQuestion | Read/Write/Edit/ListDirectory/Glob/Grep/Bash/Skill (`runtimeSkillTool`)/AskUserQuestion (`question.go`) |
| MCP client | crates/mcp | internal/mcp (client, OAuth) |
| Headless service | apps/rcode/src/daemon.rs (launchd/systemd) | cmd/gcode-server |
| Remote client protocol | crates/rpc, crates/sync (chat2 checkpoints) | internal/remote + internal/contracts (HTTP, auth, event continuation) |
| Persistence | crates/sync (SQLite) | internal/storage |
| Terminal host | crates/engine/src/terminals.rs | internal/terminalhost (+ ghostty-vt) |
| Skills | DiscoverSkills/Skill tools, harness | `Skill` tool + plugin runtime (`runtimeSkillTool`) |
| Session fork/resume | engine sessions | `session.fork` command, storage resume |
| Git worktrees | engine | internal/engine/worktree.go |
| Updater | crates/update | mygo plugins/updater (main.go) |
| Repo links (browser URLs from remotes, no credentials) | source_control partial | internal/repohost |
| Search tools without an external binary | `crates/agent/src/tools/glob.rs`、`grep.rs`：遍历目录 + `regex`/`glob` crate，明言不依赖机器上的搜索程序 | `internal/agent/glob.go`、`grep_walk.go`：进程内遍历目录树 + Go `regexp`（2026-11-01 由宿主 rg 改为原生实现，`internal/agent` 不再需要 ripgrep） |
| i18n / themes / wallpapers | various | internal/i18n, internal/design themes.json, internal/wallpaper |

## Gaps worth porting (ranked)

1. **~~WebFetch tool~~ — PORTED 2026-10-31** (`internal/agent/webfetch.go`):
   http(s) only, 2 MiB read cap enforced mid-stream, 100k char output cap,
   30 s timeout, HTML reduced to prose (scripts/styles/templates dropped,
   block tags become breaks, prose entities decoded), 4xx body reported.
   2026-11-12: the optional `prompt` argument is honoured — the turn's model
   condenses the page around it instead of returning the whole text; a
   missing model or a failed summary keeps the page and says why the prompt
   was not applied.
2. **~~WebSearch tool~~ — PORTED 2026-10-31** (`internal/agent/websearch.go`):
   Tavily-protocol `POST {endpoint}/search`, config from
   `Settings.General.WebSearch` with its own endpoint+token that never
   inherits model-provider credentials, redirects not followed, 1 MiB response
   cap, per-field truncation, HTTP errors reported without reading the body.
   Registered only when enabled.
3. **~~micro_compact~~ — PORTED 2026-10-31** (`internal/agent/compact.go`):
   tool results over 4000 chars outside the last four user turns are shortened
   in place each round (head 3/4 + tail, marker inside the budget, errored
   results untouched, second pass is a no-op). The summary path now triggers at
   95% instead of 75%, so the free reduction runs first — matching the original
   project's tuned defaults (0.75 micro / 0.95 full).
4. **~~Prompt caching (Anthropic)~~ — PORTED 2026-10-31**
   (`internal/model/messages.go` + `internal/engine/run.go`): the system prompt
   carries a `SYSTEM_PROMPT_DYNAMIC_BOUNDARY` seam between session-stable
   instructions and per-request context; the Anthropic adapter turns that into
   text blocks with `cache_control: ephemeral` on the stable prefix and strips
   the marker. OpenAI paths keep sending a plain string (prefix caching is
   automatic there).
5. **Project memory index — PORTED** (`internal/engine/memory.go`, injected in
   `internal/engine/run.go`): per-project `projects/<slug>/memory/MEMORY.md`,
   headers injected into context, bodies never loaded; 200-line / 25 KB caps.
   Gives the agent durable project knowledge across sessions.
6. **Custom commands — IMPLEMENTED** (`internal/contracts/user_resources.go`,
   `internal/engine/user_resources.go`): user-defined slash prompt commands
   persisted in `user-resources.json`, managed by
   `resource.command.save|delete|toggle`, with `$ARGUMENTS`, `$0`/`$ARGUMENTS[n]`
   and declared-parameter expansion.
7. **Source-control depth** — RCode engine source_control.rs (4.5k lines:
   remote/branch normalization, PR review threads, change requests with
   polling streams), remote_hosts.rs (1.5k lines), github crate. GCode has
   repohost links and worktrees only. Large; port on demand when the git work
   panel grows beyond status/diff.
8. **McpStatus context records** — RCode `Domain::McpStatus`: MCP server
   status recorded into execution context so the model sees transport state
   without a tool call. Small, nice-to-have.
9. **Engine-side fs_watch** — RCode crates/engine/src/fs_watch.rs (971 lines):
   workspace file watching with change streams. GCode delegates watching to
   the mygo desktop layer; the headless server currently has no equivalent.

## Intentionally out of scope

- Mobile (RCode crates/mobile Flutter visual bridge): AGENTS.md defers mobile
  to remote connections against the headless service.
- Loro/CRDT doc sync (crates/sync docs store, doc_host 7.4k lines, diff_sync):
  RCode's multi-device document sync; GCode's remote protocol covers the
  current single-user remote story.
- Zatex/veil/visual-gpu rendering internals: GCode renders with MyGo.
- daemon launchd/systemd packaging specifics: cmd/gcode-server covers the
  runtime; OS service installation is packaging work.

## Measurement notes

- Tool-count pins in tests: adding tools changes `len(request.Tools)`
  assertions (agent_test.go pins 8 after TodoWrite; expect updates when
  WebFetch/WebSearch land).
- RCode's WebSearch default endpoint is a free community service with no
  availability guarantee; the config surface (endpoint+token, never from the
  provider) must come along with the port.
