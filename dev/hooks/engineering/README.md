# Neutral engineering hooks

This directory is a development-only source pack. It ports reusable mechanics
from mature engineering workflows into runtime-neutral hooks without carrying
client, business, owner or provider context.

## Included mechanics

| Hook | Lifecycle | Default behavior |
|---|---|---|
| `session-start-engineering.sh` | session start | pointer-only snapshot |
| `post-pr-created-reminder.sh` | post Bash action | advisory quality-loop reminder |
| `post-test-failure.sh` | post Bash action | advisory failure summary |
| `pre-pr-create-staleness.sh` | before PR creation | deterministic deny when base is stale or unknown |
| `pre-merge-guard.sh` | before merge | fail closed; defer to the repository's authenticated protected workflow |
| `pre-commit-debug-scan.sh` | before commit | deny debugger artifacts; warn on likely debug logging |
| `pre-commit-sensitive-scan.sh` | before commit | advisory scan for obvious secrets |
| `spec-check-reminder.sh` | before edit/write | advisory spec pointer |
| `session-end-reminder.sh` | stop | compact checkpoint/decision reminder |

The hooks accept the native JSON envelope on stdin and emit Claude-compatible
`hookSpecificOutput` JSON when a message is useful. Messages contain counts,
categories and bounded pointers only; matched source, output and token payloads
are never emitted. They never call a model,
wait for a worker, perform a network request or read source bodies for session
context. The pre-action hooks perform only local deterministic checks.

## Deliberately excluded

Business-specific agent routing, client-data/PII patterns, maker-checker rules,
local decision-ID allocation, and provider-specific automated review gates are
not portable contracts. A repository may add those in its own adapter or local
policy, but they must not be implied by this neutral pack.

## Wiring boundary

These files are templates for a runtime adapter; they are not active product
hooks and are not included in the signed base distribution. An adapter must
map each script to its native lifecycle event, preserve the stdin/output
contract, and record conformance evidence before promoting a capability from
`unavailable`.

The quality loop reminder points to `$pr-quality-loop`; the scripts do not
invoke skills, run tests, merge branches or mutate durable state themselves.
