"""Provision two finite real-binary wiki fixtures; never changes product source."""
import argparse
import hashlib
import io
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import tarfile

LEGACY_REVISION = "d1e8f18dd13d680f81a552567c869bcc4f57ac73"
DESCRIPTION = "Compatible producer-only description"
ORIGINAL_DESCRIPTION = "Derived routes over the current authorized committed rollups."


def digest(data):
    return hashlib.sha256(data).hexdigest()


def git(root, *args):
    return subprocess.check_output(["git", "-C", str(root), *args])


def archive(root, revision, target):
    data = git(root, "archive", "--format=tar", revision)
    target.mkdir()
    with tarfile.open(fileobj=io.BytesIO(data)) as source:
        source.extractall(target, filter="data")
    return digest(data)


def tree_digest(root):
    records = []
    for name in sorted(path for path in root.rglob("*") if path.is_file()):
        records.append([name.relative_to(root).as_posix(), name.stat().st_mode & 0o777, digest(name.read_bytes())])
    return digest(json.dumps(records, separators=(",", ":")).encode())


def provision(source_root, legacy_root, output):
    source_root, legacy_root = source_root.resolve(strict=True), legacy_root.resolve(strict=True)
    if git(source_root, "status", "--porcelain").strip():
        raise RuntimeError("freeze a clean source before provisioning actual binaries")
    source_revision = git(source_root, "rev-parse", "HEAD").decode().strip()
    if git(legacy_root, "rev-parse", LEGACY_REVISION + "^{commit}").decode().strip() != LEGACY_REVISION:
        raise RuntimeError("the exact historical source is unavailable")
    output.mkdir(parents=True, mode=0o700, exist_ok=False)
    output = output.resolve(strict=True)
    stages, binaries, raw = output / "stages", output / "bin", output / "raw"
    for directory in [stages, binaries, raw, output / "evidence"]:
        directory.mkdir(mode=0o700)
    old_stage, producer_stage = stages / "legacy", stages / "producer"
    old_archive = archive(legacy_root, LEGACY_REVISION, old_stage)
    current_archive = archive(source_root, source_revision, producer_stage)
    helpers = {}
    environment = os.environ.copy()
    environment.update(GOWORK="off", GOFLAGS="", GOTOOLCHAIN="local", GOOS={"Darwin": "darwin", "Linux": "linux"}[platform.system()], GOARCH={"arm64": "arm64", "aarch64": "arm64", "x86_64": "amd64", "AMD64": "amd64"}[platform.machine()])
    go = shutil.which("go")
    if not go:
        raise RuntimeError("the selected Go toolchain is unavailable")
    compiler = producer_stage / "internal/privatewiki/compiler.go"
    original = compiler.read_bytes()
    if original.count(ORIGINAL_DESCRIPTION.encode()) != 1:
        raise RuntimeError("fixture-only compiler description anchor changed")
    changed = original.replace(ORIGINAL_DESCRIPTION.encode(), DESCRIPTION.encode())
    compiler.write_bytes(changed)
    (raw / "compiler.original.go").write_bytes(original)
    (raw / "compiler.fixture.go").write_bytes(changed)
    for kind, relative, stage, revision, source_hash in [
        ("old_writer", "acceptance/fixtures/private-wiki-old-writer/main.go", old_stage, LEGACY_REVISION, old_archive),
        ("producer", "acceptance/fixtures/private-wiki-producer/main.go", producer_stage, source_revision, current_archive),
    ]:
        body = (producer_stage / relative).read_bytes()
        selected = stage / relative
        selected.parent.mkdir(parents=True, exist_ok=True)
        selected.write_bytes(body)
        (raw / (kind + ".go")).write_bytes(body)
        fixture_hash = digest(body)
        binary = binaries / kind
        flags = f"-X main.sourceRevision={revision} -X main.sourceArchiveSHA256={source_hash} -X main.fixtureSHA256={fixture_hash}"
        command = [go, "build", "-mod=readonly", "-buildvcs=false", "-trimpath", "-ldflags", flags, "-o", str(binary), "./" + str(Path(relative).parent)]
        selected_module = subprocess.check_output([go, "env", "GOMOD"], cwd=stage, env=environment).decode().strip()
        if Path(selected_module).resolve() != (stage / "go.mod").resolve():
            raise RuntimeError("fixture build escaped its exact source module")
        result = subprocess.run(command, cwd=stage, env=environment, capture_output=True)
        (raw / (kind + ".build.stdout.log")).write_bytes(result.stdout)
        (raw / (kind + ".build.stderr.log")).write_bytes(result.stderr)
        if result.returncode:
            raise RuntimeError(f"{kind} build failed: {result.stderr.decode()}")
        helpers[kind] = {"binary": str(binary), "source_revision": revision, "source_archive_sha256": source_hash, "fixture_sha256": fixture_hash, "executable_sha256": digest(binary.read_bytes()), "staged_tree_sha256": tree_digest(stage), "build_command": command}
    manifest = {"schema_version": 1, "source_revision": source_revision, "legacy_revision": LEGACY_REVISION, "legacy_tree": git(legacy_root, "rev-parse", LEGACY_REVISION + "^{tree}").decode().strip(), "compiler_original_sha256": digest(original), "compiler_fixture_sha256": digest(changed), "fixture_only_description": DESCRIPTION, "helpers": helpers}
    (output / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
    return manifest


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source-root", type=Path, required=True)
    parser.add_argument("--legacy-root", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    arguments = parser.parse_args()
    result = provision(arguments.source_root, arguments.legacy_root, arguments.output)
    print(json.dumps(result, indent=2))
