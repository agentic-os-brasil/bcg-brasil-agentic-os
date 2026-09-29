# Native caseOS setup

The official production host is caseos.production.mcp.bcg.com. The full URL path
is externally supplied and must be verified. No URL in this package is an active
endpoint. Historical development setup guidance is stale.

Prerequisites: case-lead access grant, corporate ZPA connectivity and Okta login.
For a missing entitlement or failed TLS/MCP handshake, keep the integration
unavailable and continue local work. Never disable certificate verification.

## Codex

Project .codex/config.toml loads after project trust. After endpoint verification
and consent, merge a local [mcp_servers.caseOS] section with url set to that exact
verified URL. Keep enabled false until the approved activation step; omit
bearer_token_env_var and http_headers. Retain the owner's existing settings.
Use codex mcp login caseOS or the host's Authenticate action for OAuth, then /mcp
and live discovery for health. Restrict enabled_tools to the discovered and
reviewed read tools. Configuration is not a successful connection.

## Claude Code

After endpoint verification and consent, use Claude's native MCP flow or
claude mcp add --transport http --scope local caseOS VERIFIED_URL.
VERIFIED_URL denotes the exact reviewed value, not a literal command argument.
Authenticate through /mcp and the browser's corporate OAuth flow. Local scope
avoids sharing personal connection enrollment in project .mcp.json. A deliberately
shared project configuration requires an explicit team decision and contains no
credentials or owner-specific case enrollment.

Both hosts manage OAuth credentials. Never export host credential stores into
Maestro. Record allowlisted workspace, case and tools locally. If the host has no
temporary authenticated preflight, do not invent one or persist a speculative URL;
use the official service onboarding route to obtain verified endpoint evidence.

## Adapter boundary

The native helper's caseos-check accepts a bounded JSON object containing endpoint
on stdin. It makes a credential-free TLS/MCP initialization probe, refuses redirects
and never saves configuration. A 401 or an SSE-only response remains unverified;
the attended native host flow must complete authenticated verification.
The Go RouteCaseOS boundary requires consent, exact workspace/case, fresh discovery,
reviewed read tools and matching live JSON schema. It rejects export. It does not
provide an OAuth client or turn a host plugin into a mandatory enforcement proxy.
Native skill routing follows the same policy; hooks alone do not mediate every
possible MCP call or shell path.

Sources checked for this implementation:
- https://learn.chatgpt.com/docs/extend/mcp?surface=cli
- https://code.claude.com/docs/en/mcp
- https://learn.chatgpt.com/docs/hooks
