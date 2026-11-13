"""The region-batch port must preserve upstream inputs and verify cache contents."""

import hashlib
import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch
import zipfile


ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location("prepare_oneasr", ROOT / "tools/prepare_oneasr.py")
PREPARER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(PREPARER)


class OneAsrPortTests(unittest.TestCase):
    def prepare_fixture(self, root):
        # Keep Git discovery inside this fixture even when TEMP is in the workspace.
        subprocess.run(["git", "init", "--quiet", str(root)], check=True)
        (root / ".gitattributes").write_text("*.rs text eol=crlf\n", encoding="utf-8")
        original, modified = b"original\n", b"patched\n"
        archive = root / ".local/downloads/source.zip"
        archive.parent.mkdir(parents=True)
        with zipfile.ZipFile(archive, "w") as bundle:
            bundle.writestr("OneAsr-test/source.rs", original)
        patch_path = root / "third_party/patches/oneasr/region-batch.patch"
        patch_path.parent.mkdir(parents=True)
        patch_path.write_text(
            "diff --git a/source.rs b/source.rs\n"
            "--- a/source.rs\n+++ b/source.rs\n@@ -1 +1 @@\n-original\n+patched\n"
            "diff --git a/batch.rs b/batch.rs\nnew file mode 100644\n"
            "--- /dev/null\n+++ b/batch.rs\n@@ -0,0 +1 @@\n+batch\n",
            encoding="utf-8", newline="\n")
        pin = {
            "revision": "test",
            "sourceArchive": {"file": archive.name, "sha256": PREPARER.sha256(archive), "url": "https://example.invalid/source.zip"},
            "patch": patch_path.relative_to(root).as_posix(),
            "patchSha256": PREPARER.sha256(patch_path),
            "sourceHashes": {"source.rs": hashlib.sha256(original).hexdigest()},
            "patchedHashes": {"source.rs": hashlib.sha256(modified).hexdigest(),
                              "batch.rs": hashlib.sha256(b"batch\n").hexdigest()},
            "addedFiles": ["batch.rs"],
        }
        manifest = root / "third_party/manifests/oneasr.json"
        manifest.parent.mkdir(parents=True)
        manifest.write_text(json.dumps(pin), encoding="utf-8")
        return patch_path

    def test_prepare_and_reuse_leave_upstream_untouched(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            self.prepare_fixture(root)
            with patch.object(PREPARER, "ROOT", root):
                first = PREPARER.prepare()
                output = Path(first["directory"]) / "source.rs"
                modified = output.stat().st_mtime_ns
                self.assertEqual(output.read_text(encoding="utf-8"), "patched\n")
                self.assertEqual(PREPARER.prepare(), first)
                self.assertEqual(output.stat().st_mtime_ns, modified)
                self.assertEqual((root / "third_party/sources/oneasr-test/source.rs").read_text(encoding="utf-8"), "original\n")

    def test_modified_patched_cache_is_recreated(self):
        for name in ("source.rs", "batch.rs"):
            with self.subTest(name=name), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                self.prepare_fixture(root)
                with patch.object(PREPARER, "ROOT", root):
                    first = PREPARER.prepare()
                    output = Path(first["directory"])
                    (output / name).write_text("changed", encoding="utf-8")
                    self.assertEqual(PREPARER.prepare(), first)
                    self.assertEqual((output / "source.rs").read_bytes(), b"patched\n")
                    self.assertEqual((output / "batch.rs").read_bytes(), b"batch\n")

    def test_modified_upstream_is_rejected_before_cache_reuse(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            self.prepare_fixture(root)
            with patch.object(PREPARER, "ROOT", root):
                PREPARER.prepare()
                (root / "third_party/sources/oneasr-test/source.rs").write_text("changed", encoding="utf-8")
                with self.assertRaisesRegex(ValueError, "Pinned OneAsr source changed"):
                    PREPARER.prepare()

    def test_unregistered_patch_is_rejected(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            patch_path = self.prepare_fixture(root)
            patch_path.write_text("changed", encoding="utf-8")
            with patch.object(PREPARER, "ROOT", root):
                with self.assertRaisesRegex(ValueError, "register the changed"):
                    PREPARER.prepare()
