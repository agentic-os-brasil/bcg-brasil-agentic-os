#!/usr/bin/env python3
"""Build a genuine compatible producer using one retained description-only overlay."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess

ROOT = Path(__file__).resolve().parents[3]
COMPILER = Path("internal/privatewiki/compiler.go")
ORIGINAL = b'commonFrontmatter(m, "Memory Index", "Memory navigation", in.CommittedAt)'
VARIANT = b'commonFrontmatter(m, "Memory Index", "Compatible producer-only description", in.CommittedAt)'


def overlay_source(source):
    if source.count(ORIGINAL) != 1 or VARIANT in source:
        raise ValueError("expected exactly one unchanged index-description seam")
    return source.replace(ORIGINAL, VARIANT, 1)


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def build(output, source_revision, go):
    if not output.is_absolute() or output.exists() or output.is_symlink():
        raise ValueError("use a new absolute evidence directory; previous artifacts are preserved")
    if not re.fullmatch(r"[0-9a-f]{40}", source_revision):
        raise ValueError("a full source revision is required")
    source_state = "caller_sealed_source_archive"
    if (ROOT / ".git").exists():
        actual = subprocess.run(["git", "rev-parse", "HEAD"], cwd=ROOT, check=True,
                                text=True, capture_output=True).stdout.strip()
        if actual != source_revision:
            raise ValueError("declared source revision differs from the checkout HEAD")
        source_state = "local_git_head_plus_sealed_witness_inputs"
    source = (ROOT / COMPILER).read_bytes()
    variant = overlay_source(source)
    output.mkdir(mode=0o700)
    overlay = output / "compiler.go"
    overlay.write_bytes(variant)
    overlay_map = output / "overlay.json"
    overlay_map.write_text(json.dumps({"Replace": {str(ROOT / COMPILER): str(overlay)}}) + "\n", encoding="utf-8")
    env = dict(os.environ, GOTOOLCHAIN="local", GOPROXY="off", GOSUMDB="off", GOENV="off", GOWORK="off")
    target = subprocess.run([go, "env", "GOOS", "GOARCH"], cwd=ROOT, env=env,
                            check=True, text=True, capture_output=True).stdout.splitlines()
    if len(target) != 2:
        raise ValueError("Go did not identify exactly one target OS and architecture")
    executable = output / ("private-wiki-compatible-producer" + (".exe" if target[0] == "windows" else ""))
    command = [go, "build", "-trimpath", "-buildvcs=false", "-overlay", str(overlay_map),
               "-o", str(executable), "./dev/pilot/private-wiki-producer"]
    result = subprocess.run(command, cwd=ROOT, env=env, text=True, capture_output=True)
    (output / "build.stdout.log").write_text(result.stdout, encoding="utf-8")
    (output / "build.stderr.log").write_text(result.stderr, encoding="utf-8")
    result.check_returncode()
    if not executable.is_file() or executable.is_symlink():
        raise ValueError("build did not produce an ordinary executable")
    manifest = {
        "schema_version": 1,
        "kind": "compatible-description-variant-not-historical-release",
        "source_revision": source_revision,
        "source_state": source_state,
        "target_os": target[0], "target_arch": target[1],
        "compiler_path": COMPILER.as_posix(),
        "original_compiler_sha256": hashlib.sha256(source).hexdigest(),
        "overlay_compiler_sha256": digest(overlay),
        "overlay_json_sha256": digest(overlay_map),
        "helper_main_sha256": digest(Path(__file__).with_name("main.go")),
        "builder_sha256": digest(Path(__file__)),
        "executable": str(executable), "executable_sha256": digest(executable),
        "build_command": command,
        "native_execution": "not_run_by_builder",
    }
    (output / "producer-build.json").write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
    return manifest


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("output", type=Path)
    parser.add_argument("--source-revision", required=True)
    parser.add_argument("--go", default="go")
    args = parser.parse_args()
    print(json.dumps(build(args.output, args.source_revision, args.go)))


if __name__ == "__main__":
    main()
