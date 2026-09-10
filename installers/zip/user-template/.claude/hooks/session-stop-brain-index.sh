#!/usr/bin/env bash
# Maestro Stop — recompila o indice do brain e avalia a saude da estrutura.
#
# Por que no Stop e nao numa rotina semanal:
#   O compilador e idempotente e leva ~1s. Rodar a cada fim de sessao significa
#   que o indice nunca fica atras do disco. Uma rotina semanal deixaria ate sete
#   dias de deriva — pasta nova sem entrar no indice de cima, pasta que passou do
#   limiar e continua sem indice proprio.
#
#   Isso tambem torna desnecessaria a regra "ao criar pasta, atualize o indice
#   mais proximo". Regra que depende de alguem lembrar falha exatamente quando o
#   trabalho aperta. A estrutura passa a ser derivada, nao mantida a mao.
#
# O que NAO e automatico:
#   Orfa, link quebrado, lacuna de contrato e trabalho parado sao diagnostico,
#   nao conserto. Cada um pode ser problema ou escolha, e so o dono decide.
#   Esses viram um marcador que o SessionStart surfaca — como erro, sempre; como
#   revisao de rotina, no maximo uma vez por semana.
#
# Fail-open e silencioso: qualquer erro sai 0 sem bloquear o encerramento.

set +e

PROJECT_DIR="${CLAUDE_PROJECT_DIR:-.}"
BRAIN_DIR="$PROJECT_DIR/brain"
TOOL="$PROJECT_DIR/bundles/base/tools/brain-index.py"
PATHS_TOOL="$PROJECT_DIR/bundles/base/tools/paths-check.py"
MACHINE="$BRAIN_DIR/.maestro"
DIAG="$MACHINE/diagnostics.json"
MARKER="$MACHINE/.health-requested"
SEEN="$MACHINE/.health-last-seen"

[ -d "$BRAIN_DIR" ] || exit 0
[ -f "$TOOL" ] || exit 0
# shellcheck source=lib/python.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib/python.sh" 2>/dev/null || true
maestro_python >/dev/null 2>&1 || exit 0

# --- compila -----------------------------------------------------------------
# O exit do compilador e testado, e a falha vira marcador visivel. Antes o
# resultado era descartado com `>/dev/null 2>&1` e o hook seguia avaliando o
# diagnostico ANTERIOR como se fosse desta compilacao: com o compilador quebrado,
# o dono via saude verde do estado de semanas atras. Falhar em silencio e pior
# que falhar.
COMPILE_ERR="$MACHINE/.index-failed"
if ( cd "$PROJECT_DIR" && PYTHONIOENCODING=utf-8 maestro_py "$TOOL" >/dev/null 2>&1 ); then
  rm -f "$COMPILE_ERR" 2>/dev/null
else
  mkdir -p "$MACHINE" 2>/dev/null
  printf '%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" > "$COMPILE_ERR" 2>/dev/null
  exit 0   # sem compilacao nova, o diagnostico em disco esta vencido: nao avaliar
fi

# --- caminhos mortos ---------------------------------------------------------
# O bug dominante deste codigo e caminho que deixou de existir com o codigo que
# aponta para ele intacto: a migracao de 01/09 deixou quatro coisas mortas em
# silencio, o refactor de 05/09 deixou oito. Nenhuma reclamou, porque nada
# verificava. Agora verifica, e o resultado entra na saude como erro — nao como
# curadoria: um caminho morto quebra a skill que o cita.
DEAD_PATHS=0
if [ -f "$PATHS_TOOL" ]; then
  DEAD_PATHS=$( cd "$PROJECT_DIR" && PYTHONIOENCODING=utf-8 maestro_py "$PATHS_TOOL" --count 2>/dev/null )
  case "$DEAD_PATHS" in ''|*[!0-9]*) DEAD_PATHS=0 ;; esac
fi

[ -f "$DIAG" ] || exit 0

# --- avalia saude ------------------------------------------------------------
# Erro (link quebrado, lacuna de contrato) surfaca sempre: quebra navegacao.
# Sinal de curadoria (orfa, parado) surfaca no maximo uma vez por semana.
PYTHONIOENCODING=utf-8 maestro_py - "$DIAG" "$MARKER" "$SEEN" "$DEAD_PATHS" <<'PY' 2>/dev/null
import json, os, sys, time

diag_p, marker_p, seen_p = sys.argv[1], sys.argv[2], sys.argv[3]
dead_paths = int(sys.argv[4]) if len(sys.argv) > 4 and sys.argv[4].isdigit() else 0
try:
    d = json.load(open(diag_p, encoding="utf-8"))
except Exception:
    sys.exit(0)

broken = len(d.get("broken_links", []))
gaps = len(d.get("contract_gaps", []))
orphans = len(d.get("orphans", []))
stale = len(d.get("stale_open_work", []))

errors = broken + gaps + dead_paths
curation = orphans + stale

now = time.time()
try:
    last = os.path.getmtime(seen_p)
except OSError:
    last = 0
week_due = (now - last) > 7 * 86400

reason = None
if errors:
    reason = "erro"
elif curation and week_due:
    reason = "revisao"

if not reason:
    # Nada a surfacar. Remove marcador antigo para nao repetir sem motivo.
    try:
        os.remove(marker_p)
    except OSError:
        pass
    sys.exit(0)

payload = {
    "reason": reason,
    "broken_links": broken,
    "contract_gaps": gaps,
    "dead_paths": dead_paths,
    "orphans": orphans,
    "stale_open_work": stale,
    "pages_indexed": d.get("pages_indexed"),
    "fingerprint": d.get("source_fingerprint"),
    "evaluated_at": d.get("generated_at"),
}
os.makedirs(os.path.dirname(marker_p), exist_ok=True)
with open(marker_p, "w", encoding="utf-8", newline="\n") as fh:
    json.dump(payload, fh, ensure_ascii=False, indent=2)
    fh.write("\n")
PY

# O aviso de migração se apaga sozinho quando a condição que o justifica deixa
# de existir: zero lacuna de contrato significa que as páginas antigas já foram
# normalizadas. Marcador que só um humano consegue limpar vira ruído permanente
# — foi assim que o `.dream-requested` acabou ligado para sempre.
NOTICE="$MACHINE/.migration-notice"
if [ -f "$NOTICE" ] && [ -f "$DIAG" ] && maestro_python >/dev/null 2>&1; then
  GAPS=$(PYTHONIOENCODING=utf-8 maestro_py -c "
import json, sys
try:
    print(len(json.load(open(sys.argv[1], encoding='utf-8')).get('contract_gaps', [])))
except Exception:
    print(-1)" "$DIAG" 2>/dev/null)
  [ "$GAPS" = "0" ] && rm -f "$NOTICE" 2>/dev/null
fi

exit 0
