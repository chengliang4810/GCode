# MCP client integration

`Config.Transport` accepts `stdio`, `http`, `streamable-http`, or `sse`.
The empty transport selects stdio for a command and Streamable HTTP for a URL.
`Config.Plugin` is the owning plugin name. Configuration is copied at connection
creation; caller-owned maps and slices are never retained.
Private `Config.ProcessEnv` contains nonsecret host defaults for stdio child
processes, such as scratch directories. These values are not credential redaction
inputs and are omitted from JSON. Explicit `Config.Env` overrides them and keeps
its existing private-value redaction semantics.

`Connect` / `ConnectWithOptions` negotiate a handshake revision, offering
`2025-11-25` and accepting `2025-06-18`, `2025-03-26`, and `2024-11-05` as well.
`Options.ProtocolVersion="auto"` probes `server/discover` with `2026-07-28`
metadata before choosing an era. An explicit `ModernProtocolVersion` requires
that modern protocol and never silently sends `initialize`. A modern request
has the per-request `_meta`, `Mcp-Method`, `Mcp-Name`, and supported schema
`x-mcp-header` mirrors. New protocol revisions are not guessed from date strings.

Tool names are `mcp__<server>__<tool>` or
`mcp__plugin_<plugin>_<server>__<tool>`. Invalid characters become underscores;
normalization collisions return an explicit error. Claude-qualified names use
a 128-character namespace limit; discovery skips an overlong tool while keeping
its server's other tools, records `ToolDiagnostics()`, and emits
`gcode/tool_rejected`. The engine additionally excludes names over 64 for Chat
Completions and tells the worker which tools were excluded. Messages and
Responses retain names up to 128; the current Responses reference explicitly
documents 128 for namespace functions but does not state a separate flat-function
maximum. No truncated/hash names or `mcp_` aliases are installed.

`RemoteTool.Execute` returns text and validated image blocks to the existing
model adapter. `ToolOutput.Data` is a typed `CallResult` retaining all content
blocks, structured content, annotations and result extensions. Audio and binary
resources are retained and identified in text; they are not claimed to have
been heard or decoded by a text/image-only model adapter. Configured credentials,
injected bearer tokens, URI query secrets and credential CLI flags are scrubbed
at the MCP boundary. Credential-bearing binary content is rejected rather than
silently corrupting an image.

Resources and prompts have explicit public APIs: `Resources`,
`ResourceTemplates`, `ReadResource`, `Prompts`, `GetPrompt`. They send MCP RPCs,
never dereference a server resource URI as a local file or arbitrary web URL.
Pagination is bounded and cursor cycles are rejected.

Observe `Notifications()`, `DroppedNotifications()`, and `Done()`; refresh the
full cached projection after overflow or reconnect. Handshake stdio/HTTP+SSE
already have an open reader. For an independent HTTP notification stream the
owner runs `ListenNotifications(ctx, filter)` in a goroutine it owns. It uses a
GET stream in handshake mode and `subscriptions/listen` in modern mode. Modern
resource subscriptions use the listen filter. No notification replay or silent
reconnect is claimed. `Close` cancels owned work, terminates its session/process,
closes owned HTTP connections and the notification channel.

`AuthRequiredError` is discoverable with `errors.As`; `AuthChallenge` exposes the
resource metadata URL, scopes and HTTP status without an access token.
`Options.TokenProvider` supplies a token on a 401 challenge and can use the
owner's protected credential store. There is only one authorization retry per
HTTP request. Redirects never forward configured credentials to another origin.

The user-driven OAuth APIs are `DiscoverAuthorization`, `NewOAuthFlow`,
`OAuthFlow.Complete`, `OAuthFlow.Refresh`, `RegisterOAuthClient`, and
`NewRegisteredOAuthFlow`. They support protected-resource discovery,
OAuth/OIDC issuer discovery, PKCE S256, state/issuer/resource binding, native
public-client registration when explicitly requested, and preregistered/CIMD
client IDs. Token diagnostic/JSON methods omit credentials; the owner persists
token fields explicitly. The owner provides the browser and callback listener;
connecting does not register a client or launch an authorization flow.

Client capabilities are deliberately empty. Sampling, roots, elicitation, MRTR
input completion, task extensions, output-schema validation and arbitrary JSON
Schema dialect validation are not advertised or claimed. Server requests for
unadvertised capabilities receive an explicit unsupported-capability response;
modern input-required results return an error instead of fabricated success.

Local tests exercise actual stdio subprocesses and loopback HTTP/SSE/OAuth
fixtures. They do not install a marketplace plugin, contact its external server,
use a real provider/account, or establish a marketplace-wide availability rate.

Primary protocol references inspected on 2026-10-30:

- https://modelcontextprotocol.io/specification/2026-07-28/basic/versioning
- https://modelcontextprotocol.io/specification/2026-07-28/basic/transports/streamable-http
- https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization
- https://modelcontextprotocol.io/specification/2026-07-28/server/discover
- https://modelcontextprotocol.io/specification/2025-11-25/basic/lifecycle

Name behavior was checked against the read-only local Claude reference's MCP
normalization and naming utilities. This package is independently implemented;
reference code and external SDK source are not copied or added as dependencies.
