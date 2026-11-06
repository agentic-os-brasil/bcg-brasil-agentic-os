# Lifecycle context and filesystem preparation

The explicitly selected `mcp_lifecycle` Code plugin delivers bounded generated
memory when a session starts, a short context reminder on each prompt and a
metadata-only observation when the assistant stops. It uses the existing MCP
executable and domain stores. It runs no model, network request or worker itself.

This is a non-enforcing profile, separate from the fully qualified native
`maestro_planner` profile. It does not install specialists, pre-tool guards,
approval authority or work-completion automation. Plain `plugin` remains
MCP-only with no hooks; MCPB remains the Desktop server/resources carrier.

## Prepare the filesystem explicitly

The participant preparer calls the existing `init` command with an explicit
product, owner and workspace binding. The canonical owners materialize the
registered workspace, authored `brain/` scaffold, private owner templates,
reviewed-memory authority and fixed storage directories. Repeating preparation
preserves existing content, keys and modes. Foreign, ambiguous or partially
prepared roots remain refused rather than repaired automatically.

| Contents | Owning location |
| --- | --- |
| Participant documents and authored knowledge | Selected workspace, including its `brain/` |
| Private owner context and reviewed memory | Resolved home `brain/owner` and `brain/memory` |
| Binding, execution and lifecycle metadata | Resolved home `state/` |
| Managed executable and generated client assets | Product and exports selected by the preparer |

Hooks consume this established binding. They never initialize missing state,
create a new owner, repair files or merge the authored and private brain roots.

## Build and connect the plugin

After explicit initialization, use the normal plugin builder with
`--lifecycle-context`, retaining the same `--product-root`, `--user-home`,
`--workspace`, `--server`, `--destination` and packet `--version` inputs.
The builder writes a new destination and inventories the exact hook files and
configured profile. The selected executable and paths stay bound to that packet.
The participant preparers select this profile directly.

Install the generated marketplace/plugin through the client's normal manager
and start a new Code session in that workspace. Preserve earlier trial folders
when preparing a different kit. Existing trial state is not silently migrated.
Windows uses the documented PowerShell command-hook handler; actual client
support and execution must be observed on that Windows installation. It does
not assume WSL or Git Bash.

## What the hooks do

- `SessionStart` validates the binding and reads only eligible, committed
  generated memory through the same visibility checks as `memory.context`.
  Historical-data framing, UTF-8 boundaries, per-layer limits, an 8-KiB memory
  ceiling and a 16-KiB total context ceiling remain effective. Raw captures are
  not a fallback for missing or invalid generated views.
- `UserPromptSubmit` refreshes metadata without repeating memory bodies,
  retaining prompt text or entering the native-agent admission flow.
- `Stop` respects the host recursion flag and writes only the canonical
  confined lifecycle observation. It does not checkpoint work, save memory,
  consolidate, index the brain, compile a wiki or run recurring maintenance.

Capture approval, daily/weekly consolidation and maintenance enrollment retain
their existing explicit operations and review. Lifecycle hooks make committed
context available; they do not provide an unattended second-brain pipeline.

The packet reports configuration separately from actual client execution and
keeps enforcement unavailable. A real session must independently establish
context delivery and Stop observation. Such evidence does not qualify the full
native guard, another client/platform, production signing or automatic updates.
