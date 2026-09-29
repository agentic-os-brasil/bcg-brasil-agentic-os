#!/usr/bin/env python3
"""Compatibility launcher only; the sole migration authority is maestro-runtime.

Native hooks do not require Python. Old callers of this filename are delegated
to the same version/hash-verified managed binary, or receive an explicit block.
"""
import hashlib
import json
from pathlib import Path
import platform
import subprocess
import sys


def main():
    try:
        args = sys.argv[1:]
        project = Path(args[args.index("--project") + 1]).absolute()
        system = {"Darwin": "darwin", "Windows": "windows", "Linux": "linux"}[platform.system()]
        architecture = {"arm64": "arm64", "aarch64": "arm64", "x86_64": "amd64", "amd64": "amd64"}[platform.machine().lower()]
        suffix = ".exe" if system == "windows" else ""
        relative = f"runtime/{system}-{architecture}/maestro-runtime{suffix}"
        binary = project / relative
        manifest_path = project / "runtime/manifest.json"
        for item in (project, project/"runtime", binary.parent, binary, manifest_path, project/"VERSION"):
            if item.is_symlink():
                raise ValueError("managed_runtime_symlink")
        manifest = json.loads(manifest_path.read_text(encoding="utf8"))
        if type(manifest.get("schema_version")) is not int or manifest["schema_version"] != 1 or manifest.get("version") != (project/"VERSION").read_text().strip():
            raise ValueError("managed_runtime_version_mismatch")
        selected = [a for a in manifest["artifacts"] if a.get("os") == system and a.get("arch") == architecture]
        if len(selected) != 1 or selected[0].get("path") != relative:
            raise ValueError("managed_runtime_selection_invalid")
        if selected[0].get("sha256") != hashlib.sha256(binary.read_bytes()).hexdigest():
            raise ValueError("managed_runtime_checksum_mismatch")
        return subprocess.run([str(binary), "migration", *args], check=False).returncode
    except (OSError, ValueError, KeyError, IndexError, TypeError):
        print(json.dumps({"schema_version": 1, "state": "blocked", "errors": ["managed_runtime_unavailable"],
                          "restored_runtime": False, "next_action": "Verify the managed helper; original data was not changed."}))
        return 2


if __name__ == "__main__":
    sys.exit(main())
