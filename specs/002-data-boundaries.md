# Spec 002 - Data boundaries

Status: accepted principle; detailed schema pending.

## Distribution data

May contain code, sanitized templates, schemas, public/internal-safe guidance, runtime adapters, official skills and hooks.

## User-level local data

Includes credentials, preferences, installed versions, caches and diagnostic logs. It belongs in operating-system application directories and credential stores, not in the source repository.

## Workspace data

Includes case files, project artifacts and source code. `bcgos init` may add only minimal, documented and regenerable metadata or adapters. Its current surface is `.bcgos/workspace.json` plus a non-overwriting `brain/README.md` that explains local Markdown navigation; it does not impose content taxonomy. It must not upload or copy workspace content into the managed bundle. Private memory remains user-local application data, not workspace content.

## Prohibited data

Real secrets, client data, personally identifiable information and unsanitized work artifacts must never appear in releases, examples, tests, issues or commits.
