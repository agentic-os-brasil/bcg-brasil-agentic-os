"""Builder-only checks against the actual generic kit and ordinary binary."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import stat
import struct
import subprocess
import sys
import time
import zipfile

kit, proof = map(lambda value: Path(value).resolve(), sys.argv[1:3])
proof.mkdir(parents=True, exist_ok=False, mode=0o700)
root = proof / "participant-a"
setup = kit / "Preparar Maestro.command"
env = dict(os.environ)
assert "MAESTRO_HOME" not in env, "test refuses an inherited owner HOME override"
ledger = []
for architecture, cpu in [("arm64", 0x100000C), ("amd64", 0x1000007)]:
    native = (kit / ("bin/maestro-mcp-darwin-" + architecture)).read_bytes()
    magic, actual_cpu, _, file_type = struct.unpack("<IIII", native[:16])
    assert magic == 0xFEEDFACF and actual_cpu == cpu and file_type == 2
    assert str(Path.home()).encode() not in native, "artifact embeds the builder home"


def run(arguments, *, child_env=env, success=True):
    started = time.monotonic()
    result = subprocess.run(arguments, env=child_env, text=True, capture_output=True)
    ledger.append({"argv": list(map(str, arguments)), "exit": result.returncode,
                   "seconds": round(time.monotonic() - started, 3),
                   "stdout": result.stdout, "stderr": result.stderr})
    (proof / "commands.json").write_text(json.dumps(ledger, indent=2) + "\n")
    assert (result.returncode == 0) == success, ledger[-1]
    return result


def snapshot(path):
    result = {}
    for entry in [path, *sorted(path.rglob("*"))]:
        mode = entry.lstat().st_mode
        assert stat.S_ISDIR(mode) or stat.S_ISREG(mode), str(entry)
        result[str(entry.relative_to(path))] = {
            "mode": stat.S_IMODE(mode),
            "sha256": hashlib.sha256(entry.read_bytes()).hexdigest() if entry.is_file() else None,
        }
    return result


run(["/bin/bash", "-n", str(setup)])
run(["/bin/bash", str(setup), str(root)])
assert (root / "exports/maestro-pilot.mcpb").is_file()
assert (root / "exports/code-marketplace/plugins/maestro-product/.mcp.json").is_file()
assert (root / "COMECE-AQUI.txt").is_file()
inspection = run([str(root / "product/maestro-mcp"), "inspect", "--product-root", str(root / "product"),
                  "--user-home", str(root / "user"), "--workspace", str(root / "workspace")])
assert json.loads(inspection.stdout)["writable"] is True
plugin = root / "exports/code-marketplace/plugins/maestro-product"
packet = json.loads((plugin / "maestro-packet.json").read_text())
assert packet["schema_version"] == 1 and packet["mode"] == "mcp_only"
assert packet["target"] == {"os": "darwin", "arch": "arm64"}
assert packet["native"]["state"] == "unavailable" and packet["native"]["evidence_class"] == "not_observed"
declared = {item["path"] for item in packet["files"]}
actual = {str(path.relative_to(plugin)) for path in plugin.rglob("*") if path.is_file()}
assert actual == declared | {"maestro-packet.json"}
for item in packet["files"]:
    relative = Path(item["path"])
    assert not relative.is_absolute() and ".." not in relative.parts
    path = plugin / relative
    assert stat.S_ISREG(path.lstat().st_mode)
    assert stat.S_IMODE(path.stat().st_mode) == item["mode"]
    assert hashlib.sha256(path.read_bytes()).hexdigest() == item["sha256"]
args = ["serve", "--product-root", str(root / "product"), "--user-home", str(root / "user"), "--workspace", str(root / "workspace")]
config = json.loads((plugin / ".mcp.json").read_text())
assert config == {"mcpServers": {"maestro": {"type": "stdio", "command": "${CLAUDE_PLUGIN_ROOT}/scripts/maestro-mcp", "args": args}}}
server_sha = hashlib.sha256((root / "product/maestro-mcp").read_bytes()).hexdigest()
assert hashlib.sha256((plugin / "scripts/maestro-mcp").read_bytes()).hexdigest() == server_sha
method_ids = (Path(__file__).resolve().parents[2] / "adapters/mcp/packaging/testdata/method-ids.txt").read_text().split()
assert len(method_ids) == 92 and all((plugin / "skills" / name / "SKILL.md").is_file() for name in method_ids)
with zipfile.ZipFile(root / "exports/maestro-pilot.mcpb") as extension:
    metadata = json.loads(extension.read("maestro-packet.json"))
    assert metadata["schema_version"] == 1 and metadata["mode"] == "mcp_only"
    assert metadata["native"]["state"] == "unavailable"
    assert {item["path"] for item in metadata["files"]} | {"maestro-packet.json"} == set(extension.namelist())
    for item in metadata["files"]:
        assert hashlib.sha256(extension.read(item["path"])).hexdigest() == item["sha256"]
    manifest = json.loads(extension.read("manifest.json"))
    assert manifest["server"]["mcp_config"] == {"command": "${__dirname}/server/maestro-mcp", "args": args}
    assert manifest["version"] == "0.0.0" and manifest["compatibility"]["platforms"] == ["darwin"]
    assert hashlib.sha256(extension.read("server/maestro-mcp")).hexdigest() == server_sha
before = snapshot(root)
run(["/bin/bash", str(setup), str(root)])
assert snapshot(root) == before, "repeat setup changed existing private or workspace state"

foreign = proof / "foreign"
foreign.mkdir(mode=0o700)
(foreign / "keep.txt").write_text("synthetic foreign data must remain exact\n")
before = snapshot(foreign)
run(["/bin/bash", str(setup), str(foreign)], success=False)
assert snapshot(foreign) == before

redirected = proof / "redirected-target"
run(["/bin/bash", str(setup), str(redirected)],
    child_env={**env, "MAESTRO_HOME": str(foreign)}, success=False)
assert not redirected.exists() and snapshot(foreign) == before

alias = proof / "alias"
alias.symlink_to(foreign, target_is_directory=True)
run(["/bin/bash", str(setup), str(alias / "must-not-exist")], success=False)
assert snapshot(foreign) == before

tampered = proof / "tampered-kit"
shutil.copytree(kit, tampered)
with (tampered / "LEIA-ME.md").open("a") as target:
    target.write("\nUnexpected artifact change.\n")
tampered_target = proof / "tampered-target"
run(["/bin/bash", str(tampered / setup.name), str(tampered_target)], success=False)
assert not tampered_target.exists()

with zipfile.ZipFile(str(kit) + ".zip") as archive:
    assert archive.testzip() is None
    expected = {str(path.relative_to(kit)) for path in kit.rglob("*") if path.is_file()}
    actual = set()
    for member in archive.infolist():
        path = Path(member.filename)
        assert not path.is_absolute() and ".." not in path.parts
        assert path.parts[0] == kit.name
        if not member.is_dir():
            relative = str(Path(*path.parts[1:]))
            actual.add(relative)
            assert hashlib.sha256(archive.read(member)).digest() == hashlib.sha256((kit / relative).read_bytes()).digest()
    assert actual == expected
    assert not any(part in {"user", "workspace", "exports"} for name in actual for part in Path(name).parts)

result = {"passed": True, "fresh_setup": True, "repeat_setup_full_snapshot_preserved": True,
          "foreign_root_preserved": True, "inherited_HOME_refused_before_write": True,
          "alias_refused": True, "tampered_kit_refused_before_write": True,
          "zip_exact_inventory_and_bytes": True, "private_state_or_bound_packet_in_zip": False,
          "legacy_packet_inventory_and_exact_local_args": True, "all_92_methods_present": True,
          "participant_root": str(root), "native_clients": "not_run"}
(proof / "setup-proof.json").write_text(json.dumps(result, indent=2) + "\n")
print(json.dumps(result, indent=2))
