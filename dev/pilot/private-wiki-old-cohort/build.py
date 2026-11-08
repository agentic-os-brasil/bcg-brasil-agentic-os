#!/usr/bin/env python3
"""Build a new harness linked to one unmodified, pinned historical library."""
import argparse
import hashlib
import io
import json
import os
from pathlib import Path, PurePosixPath
import shutil
import stat
import subprocess
import tarfile

ROOT = Path(__file__).resolve().parents[3]
REVISION = "f93e9ce21edcd19246e7e5985d97489297dae18b"
HARNESS = Path("dev/pilot/private-wiki-old-cohort")
MAX_ARCHIVE = 256 << 20
MAX_FILES = 20000
MAX_SOURCE = 512 << 20


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def extract_pinned_archive(archive, destination):
    if len(archive) > MAX_ARCHIVE:
        raise ValueError("historical archive exceeds its bound")
    records = {}
    with tarfile.open(fileobj=io.BytesIO(archive), mode="r:") as source:
        members = source.getmembers()
        if source.pax_headers.get("comment") != REVISION:
            raise ValueError("archive does not identify the pinned historical commit")
        if len(members) > MAX_FILES or sum(m.size for m in members) > MAX_SOURCE:
            raise ValueError("historical source inventory exceeds its bound")
        for entry in members:
            name = entry.name.rstrip("/")
            relative = PurePosixPath(name)
            if (not name or relative.is_absolute() or "\\" in name or ":" in name or
                    any(part in {"", ".", ".."} for part in name.split("/")) or
                    not (entry.isdir() or entry.isfile())):
                raise ValueError("historical archive must contain only ordinary confined paths")
            if relative == PurePosixPath(HARNESS.as_posix()) or PurePosixPath(HARNESS.as_posix()) in relative.parents:
                raise ValueError("historical source already contains the new harness namespace")
            path = destination.joinpath(*relative.parts)
            if entry.isdir():
                path.mkdir(parents=True, exist_ok=True)
                continue
            path.parent.mkdir(parents=True, exist_ok=True)
            with source.extractfile(entry) as incoming, path.open("xb") as output:
                shutil.copyfileobj(incoming, output)
            os.chmod(path, entry.mode & 0o777)
            records[name] = {"sha256": digest(path), "archive_mode": entry.mode,
                             "extracted_mode": stat.S_IMODE(path.stat().st_mode)}
    return records


def verify_library(source, records, harness_files):
    expected = set(records) | set(harness_files)
    actual = {p.relative_to(source).as_posix() for p in source.rglob("*") if p.is_file() or p.is_symlink()}
    if actual != expected:
        raise ValueError("build changed the historical library file inventory")
    for relative, record in records.items():
        path = source / relative
        if (path.is_symlink() or digest(path) != record["sha256"] or
                stat.S_IMODE(path.stat().st_mode) != record["extracted_mode"]):
            raise ValueError("build changed historical library bytes or file mode")
    for relative, expected_sha in harness_files.items():
        path = source / relative
        if path.is_symlink() or digest(path) != expected_sha:
            raise ValueError("build changed its added harness source")


def build(output, go):
    if not output.is_absolute() or output.exists() or output.is_symlink():
        raise ValueError("use a new absolute output directory; previous proof is preserved")
    # Archive bytes must retain the pinned LF and mode seal on every host.
    archive = subprocess.run(["git", "-c", "core.autocrlf=false", "-c", "core.eol=lf",
                              "-c", "tar.umask=0002", "archive", "--format=tar", REVISION],
                             cwd=ROOT, check=True, capture_output=True).stdout
    if len(archive) > MAX_ARCHIVE:
        raise ValueError("historical archive exceeds its bound")
    output.mkdir(mode=0o700)
    (output / "historical-source.tar").write_bytes(archive)
    source = output / "source"
    source.mkdir(mode=0o700)
    library_files = extract_pinned_archive(archive, source)
    harness_files = {}
    for name in ("main.go", "main_test.go"):
        origin = Path(__file__).with_name(name)
        relative = HARNESS / name
        destination = source / relative
        destination.parent.mkdir(parents=True, exist_ok=True)
        with destination.open("xb") as file:
            file.write(origin.read_bytes())
        harness_files[relative.as_posix()] = digest(destination)
    env = dict(os.environ, GOFLAGS="", GOWORK="off", GOENV="off", GOTOOLCHAIN="local",
               GOPROXY="off", GOSUMDB="off", CGO_ENABLED="0")
    target = subprocess.run([go, "env", "GOOS", "GOARCH", "GOVERSION"], cwd=source,
                            env=env, check=True, text=True, capture_output=True).stdout.splitlines()
    if len(target) != 3:
        raise ValueError("Go did not identify one target and SDK version")
    executable = output / ("private-wiki-old-cohort-harness" + (".exe" if target[0] == "windows" else ""))
    command = [go, "build", "-mod=readonly", "-trimpath", "-buildvcs=false",
               "-ldflags", "-X=main.linkedLibraryRevision=" + REVISION,
               "-o", str(executable), "./" + HARNESS.as_posix()]
    built = subprocess.run(command, cwd=source, env=env, text=True, capture_output=True)
    (output / "build.stdout.log").write_text(built.stdout, encoding="utf-8")
    (output / "build.stderr.log").write_text(built.stderr, encoding="utf-8")
    built.check_returncode()
    verify_library(source, library_files, harness_files)
    resolved_go = shutil.which(go)
    if not resolved_go or not executable.is_file() or executable.is_symlink():
        raise ValueError("ordinary SDK and harness executables are required")
    manifest = {
        "schema_version": 1, "artifact_kind": "new_harness_unmodified_historical_library",
        "historical_release_artifact": False, "library_revision": REVISION,
        "original_archive_sha256": hashlib.sha256(archive).hexdigest(),
        "original_library_files": library_files, "harness_files": harness_files,
        "builder_sha256": digest(Path(__file__)),
        "sdk_executable_sha256": digest(Path(resolved_go).resolve()), "sdk_version": target[2],
        "target_os": target[0], "target_arch": target[1],
        "executable": str(executable), "executable_sha256": digest(executable),
        "build_command": command, "library_unchanged_after_build": True,
        "native_execution": "not_run_by_builder",
    }
    (output / "historical-build.json").write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
    return {key: value for key, value in manifest.items() if key != "original_library_files"}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("output", type=Path)
    parser.add_argument("--go", default="go")
    args = parser.parse_args()
    print(json.dumps(build(args.output, args.go)))


if __name__ == "__main__":
    main()
