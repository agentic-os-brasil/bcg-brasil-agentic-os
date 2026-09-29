#!/usr/bin/env bash
# Verified managed helper. Signature: maestro_runtime ROOT COMMAND [ARGS...].
# No installation, ambient Python, Git Bash or network is used.
maestro_runtime() {
  local root="$1"
  shift
  local platform architecture relative helper manifest expected actual version candidate
  root="$(cd "$root" 2>/dev/null && pwd -P)" || return 2
  case "$(uname -s)" in Darwin) platform=darwin ;; Linux) platform=linux ;; *) printf 'Maestro runtime unavailable: unsupported host.\n' >&2; return 2 ;; esac
  case "$(uname -m)" in arm64|aarch64) architecture=arm64 ;; x86_64|amd64) architecture=amd64 ;; *) return 2 ;; esac
  relative="runtime/$platform-$architecture/maestro-runtime"
  helper="$root/$relative"
  manifest="$root/runtime/manifest.json"
  for candidate in "$root/runtime" "$root/runtime/$platform-$architecture" "$helper" "$manifest" "$root/VERSION"; do
    if [ -L "$candidate" ]; then printf 'Maestro runtime unavailable: symbolic link in managed runtime.\n' >&2; return 2; fi
  done
  if [ ! -f "$manifest" ] || [ ! -x "$helper" ] || [ ! -f "$root/VERSION" ]; then
    printf 'Maestro runtime unavailable: managed helper or manifest missing.\n' >&2
    return 2
  fi
  version="$(tr -d '\r\n' < "$root/VERSION")"
  if [ "$platform" = darwin ]; then
    expected=$(/usr/bin/osascript -l JavaScript - "$manifest" "$platform" "$architecture" "$relative" "$version" <<'JXA'
ObjC.import('Foundation');
function run(args) {
  var raw = $.NSString.stringWithContentsOfFileEncodingError(args[0], $.NSUTF8StringEncoding, null);
  if (!raw) throw new Error('manifest unreadable');
  var m = JSON.parse(ObjC.unwrap(raw));
  if (m.schema_version !== 1 || typeof m.version !== 'string' || m.version !== args[4] || !Array.isArray(m.artifacts)) throw new Error('manifest binding invalid');
  var selected = m.artifacts.filter(function(a) { return a && a.os === args[1] && a.arch === args[2]; });
  if (selected.length !== 1) throw new Error('artifact selection ambiguous');
  var a = selected[0];
  if (a.path !== args[3] || typeof a.sha256 !== 'string' || !/^[0-9a-f]{64}$/.test(a.sha256)) throw new Error('artifact invalid');
  return a.sha256;
}
JXA
    ) || { printf 'Maestro runtime unavailable: manifest validation failed.\n' >&2; return 2; }
  elif command -v jq >/dev/null 2>&1; then
    expected=$(jq -er --arg os "$platform" --arg arch "$architecture" --arg path "$relative" --arg version "$version" '
      select(.schema_version == 1 and .version == $version and (.artifacts|type) == "array") |
      [.artifacts[] | select(.os == $os and .arch == $arch)] |
      select(length == 1) | .[0] | select(.path == $path and (.sha256|type) == "string") |
      .sha256 | select(test("^[0-9a-f]{64}$"))
    ' "$manifest") || return 2
  else
    printf 'Maestro runtime unavailable: native JSON verifier unavailable.\n' >&2
    return 2
  fi
  if command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "$helper")" || return 2
  elif command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "$helper")" || return 2
  else
    printf 'Maestro runtime unavailable: SHA256 verifier unavailable.\n' >&2
    return 2
  fi
  actual="$(printf '%s' "$actual" | cut -d ' ' -f 1)"
  if [ "$actual" != "$expected" ]; then
    printf 'Maestro runtime unavailable: managed helper checksum mismatch.\n' >&2
    return 2
  fi
  "$helper" "$@"
}
