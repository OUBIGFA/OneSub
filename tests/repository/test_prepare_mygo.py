"""A previously copied module must remain refreshable on Windows."""
import importlib.util
import hashlib
import json
from pathlib import Path
import stat
import tempfile
import unittest
from unittest.mock import patch
import subprocess

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location("prepare_mygo", ROOT / "tools/prepare_mygo.py")
PREPARE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(PREPARE)


class PrepareMygoTests(unittest.TestCase):
    def test_lobe_cache_recovers_changed_and_missing_files(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            subprocess.run(["git", "init", "--quiet", str(root)], check=True)
            source = root / ".local/gomodcache/example.test/kit@v1"
            source.mkdir(parents=True)
            (source / "widget.go").write_bytes(b"original\n")
            (source / "other.go").write_bytes(b"unchanged\n")
            integration = root / "integration.patch"
            integration.write_text(
                "diff --git a/widget.go b/widget.go\n--- a/widget.go\n+++ b/widget.go\n"
                "@@ -1 +1 @@\n-original\n+patched\n", encoding="utf-8", newline="\n")
            manifest = root / "third_party/manifests/lobe-mygo.json"
            manifest.parent.mkdir(parents=True)
            manifest.write_text(json.dumps({
                "module": "example.test/kit", "version": "v1", "patch": "integration.patch",
                "sourceHashes": {"widget.go": hashlib.sha256(b"original\n").hexdigest()},
                "addedFiles": [],
            }), encoding="utf-8")
            with patch.object(PREPARE, "ROOT", root):
                output = PREPARE.prepare_lobe()
                unchanged = (output / "widget.go").stat().st_mtime_ns
                self.assertEqual(PREPARE.prepare_lobe(), output)
                self.assertEqual((output / "widget.go").stat().st_mtime_ns, unchanged)
                for name, expected in (("widget.go", b"patched\n"), ("other.go", b"unchanged\n")):
                    with self.subTest(file=name):
                        (output / name).write_bytes(b"local accidental change\n")
                        PREPARE.prepare_lobe()
                        self.assertEqual((output / name).read_bytes(), expected)
                (output / "other.go").unlink()
                PREPARE.prepare_lobe()
                self.assertEqual((output / "other.go").read_bytes(), b"unchanged\n")

    def test_mirror_refreshes_readonly_copy_without_changing_source(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source, output = root / "source", root / "output"
            source.mkdir()
            output.mkdir()
            original, copied = source / "widget.go", output / "widget.go"
            original.write_text("new version", encoding="utf-8")
            copied.write_text("previous version", encoding="utf-8")
            original.chmod(stat.S_IREAD)
            copied.chmod(stat.S_IREAD)
            try:
                PREPARE.mirror(source, output)
                self.assertEqual(copied.read_text(encoding="utf-8"), "new version")
                self.assertFalse(original.stat().st_mode & stat.S_IWRITE)
                self.assertTrue(copied.stat().st_mode & stat.S_IWRITE)
            finally:
                original.chmod(stat.S_IREAD | stat.S_IWRITE)
                copied.chmod(stat.S_IREAD | stat.S_IWRITE)


if __name__ == "__main__":
    unittest.main()
