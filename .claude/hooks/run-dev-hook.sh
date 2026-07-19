#!/usr/bin/env sh
set -eu

event="${1:-}"
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/../.." && pwd)
cd "$repo_root"

if ! command -v go >/dev/null 2>&1 && [ "$event" = "session-start" ]; then
  printf '%s\n' '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"ESTADO NAO SUPORTADO: Go nao esta instalado, portanto o harness ainda nao pode proteger este clone. Nao modifique o repositorio nem invoque skills. Feche esta sessao e conclua o onboarding externo do Windows; depois abra uma nova sessao nesta pasta."}}'
  exit 0
fi

if ! command -v go >/dev/null 2>&1; then
  if [ "$event" = "prompt-expansion" ]; then
    printf '%s\n' '{"decision":"block","reason":"Go ainda nao esta instalado. Feche esta sessao e conclua o onboarding externo do Windows antes de usar skills."}'
  elif [ "$event" = "post-tool" ] || [ "$event" = "skill-used" ]; then
    printf '%s\n' '{"decision":"block","reason":"Go ainda nao esta instalado; o harness nao pode validar esta acao."}'
  else
    printf '%s\n' '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"Go ainda nao esta instalado. Feche esta sessao e conclua o onboarding externo do Windows; nada foi alterado pelo harness."}}'
  fi
  exit 0
fi

exec go run ./dev/harness claude "$event"
