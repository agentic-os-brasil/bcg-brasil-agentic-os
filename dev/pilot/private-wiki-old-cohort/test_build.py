"""Validate sealed historical extraction without compiling or invoking the engine."""
import importlib.util
import io
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile
import unittest
from unittest import mock

SPEC = importlib.util.spec_from_file_location("old_cohort_build", Path(__file__).with_name("build.py"))
builder = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(builder)


def archive(name="internal/memory/engine.go", kind=tarfile.REGTYPE, revision=builder.REVISION):
    stream = io.BytesIO()
    with tarfile.open(fileobj=stream, mode="w", format=tarfile.PAX_FORMAT,
                      pax_headers={"comment": revision}) as source:
        entry = tarfile.TarInfo(name)
        entry.type = kind
        entry.size = 8 if kind == tarfile.REGTYPE else 0
        entry.mode = 0o644
        source.addfile(entry, io.BytesIO(b"original") if entry.size else None)
    return stream.getvalue()


class PinnedHistoricalSource(unittest.TestCase):
    def test_archive_ignores_local_eol_and_umask_without_changing_config(self):
        with tempfile.TemporaryDirectory(dir=os.environ.get("MAESTRO_TEST_TMP")) as directory:
            root = Path(directory)
            repository = root / "repository"
            repository.mkdir()

            def git(*arguments):
                return subprocess.run(["git", "-c", "core.hooksPath=" + str(root / "no-hooks"), *arguments], cwd=repository, check=True,
                                      capture_output=True).stdout

            git("init", "--quiet")
            (repository / ".gitattributes").write_bytes(b"sample.txt text\n")
            (repository / "sample.txt").write_bytes(b"original\nline endings\n")
            git("-c", "core.autocrlf=false", "add", ".")
            git("-c", "user.name=Archive Fixture", "-c", "user.email=fixture@example.invalid",
                "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "Fixture")
            revision = git("rev-parse", "HEAD").decode().strip()
            expected = git("-c", "core.autocrlf=false", "-c", "core.eol=lf",
                           "-c", "tar.umask=0002", "archive", "--format=tar", revision)
            git("config", "core.autocrlf", "true")
            git("config", "core.eol", "crlf")
            git("config", "tar.umask", "0077")
            prior_config = (repository / ".git/config").read_bytes()
            hostile = git("archive", "--format=tar", revision)
            self.assertNotEqual(hostile, expected, "fixture must reproduce inherited archive conversion")
            with tarfile.open(fileobj=io.BytesIO(hostile), mode="r:") as source:
                self.assertIn(b"\r\n", source.extractfile("sample.txt").read())
            output = root / "output"
            with mock.patch.object(builder, "ROOT", repository), mock.patch.object(builder, "REVISION", revision):
                with self.assertRaises(FileNotFoundError):
                    builder.build(output, str(root / "sdk-must-not-run"))
            actual = (output / "historical-source.tar").read_bytes()
            self.assertEqual(actual, expected)
            with tarfile.open(fileobj=io.BytesIO(actual), mode="r:") as source:
                self.assertEqual(source.extractfile("sample.txt").read(), b"original\nline endings\n")
            self.assertEqual((repository / ".git/config").read_bytes(), prior_config)

    def test_original_bytes_survive_and_mutation_is_detected(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            records = builder.extract_pinned_archive(archive(), root)
            builder.verify_library(root, records, {})
            (root / "internal/memory/engine.go").write_bytes(b"modified")
            with self.assertRaisesRegex(ValueError, "library bytes"):
                builder.verify_library(root, records, {})

    def test_wrong_revision_alias_paths_and_harness_collision_refuse(self):
        cases = [archive(revision="b" * 40), archive("../escape"), archive("C:/escape"),
                 archive("directory\\escape"), archive("link", tarfile.SYMTYPE),
                 archive("dev/pilot/private-wiki-old-cohort/main.go")]
        for data in cases:
            with self.subTest(size=len(data)), tempfile.TemporaryDirectory() as directory:
                with self.assertRaises(ValueError):
                    builder.extract_pinned_archive(data, Path(directory))

    def test_build_refuses_existing_output_before_archive_or_sdk(self):
        with tempfile.TemporaryDirectory() as directory, mock.patch.object(builder.subprocess, "run") as run:
            with self.assertRaises(ValueError):
                builder.build(Path(directory), "not-executed")
            run.assert_not_called()

    def test_added_library_file_is_not_silently_accepted(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            records = builder.extract_pinned_archive(archive(), root)
            (root / "extra.go").write_bytes(b"not historical")
            with self.assertRaisesRegex(ValueError, "inventory"):
                builder.verify_library(root, records, {})


if __name__ == "__main__":
    unittest.main()
