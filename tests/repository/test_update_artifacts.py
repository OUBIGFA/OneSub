"""Test the real signed-manifest finalizer, not a second update generator."""

import base64
import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location("finalize_update", ROOT / "tools/finalize_update.py")
FINALIZER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(FINALIZER)


class UpdateArtifactTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.package = self.root / "package"
        self.package.mkdir()
        self.output = self.root / "release"
        self.config = self.root / "mygo.json"
        self.config.write_text(json.dumps({"version": "0.0.1", "updates": {"github": "OUBIGFA/OneSub", "deltas": 0}}), "utf-8")
        self.archive_name = "onesub-0.0.1-windows-amd64.tar.gz"
        self.archive = b"archive bytes already signed by mygo build"
        (self.package / self.archive_name).write_bytes(self.archive)
        # The finalizer preserves a signature. Cryptographic acceptance is
        # exercised by the real MyGo updater tests, with actual Ed25519 keys.
        self.manifest = {"version": "0.0.1", "date": "2026-11-05T00:00:00Z", "notes": "About and signed updates", "url": "https://github.com/OUBIGFA/OneSub/releases/download/v0.0.1/" + self.archive_name, "size": len(self.archive), "signature": base64.b64encode(bytes(64)).decode("ascii")}
        self.save_manifest()

    def save_manifest(self):
        (self.package / "update-windows-amd64.json").write_text(json.dumps(self.manifest), "utf-8")

    def finalize(self, build="20261105000001"):
        return FINALIZER.finalize_update(self.config, self.package, build, self.output)

    def test_preserves_signed_bytes_and_product_version(self):
        self.finalize()
        manifest = json.loads((self.output / "update-windows-amd64.json").read_text("utf-8"))
        self.assertEqual(manifest["version"], "0.0.1.20261105000001")
        self.assertEqual(manifest["url"], "https://github.com/OUBIGFA/OneSub/releases/download/build-20261105000001/" + self.archive_name)
        for key in ("date", "notes", "size", "signature"):
            self.assertEqual(manifest[key], self.manifest[key])
        self.assertEqual((self.output / self.archive_name).read_bytes(), self.archive)
        digest = hashlib.sha256(self.archive).hexdigest()
        self.assertEqual((self.output / (self.archive_name + ".sha256")).read_text("utf-8"), f"{digest}  {self.archive_name}\n")
        self.assertEqual(json.loads(self.config.read_text("utf-8"))["version"], "0.0.1")
        self.assertEqual(json.loads((self.package / "update-windows-amd64.json").read_text("utf-8")), self.manifest)
        self.assertFalse(list(self.output.glob("*.partial")))

    def test_replaces_only_fixed_update_artifacts_on_next_build(self):
        self.finalize()
        (self.output / "OneSub-0.0.1-Setup.exe").write_bytes(b"installer")
        self.finalize("20261105000002")
        manifest = json.loads((self.output / "update-windows-amd64.json").read_text("utf-8"))
        self.assertEqual(manifest["version"], "0.0.1.20261105000002")
        self.assertIn("/build-20261105000002/", manifest["url"])
        self.assertEqual((self.output / "OneSub-0.0.1-Setup.exe").read_bytes(), b"installer")
        self.assertEqual(len(list(self.output.iterdir())), 4)

    def test_rejects_mismatched_or_unsigned_generator_output(self):
        for field, value in (("version", "0.0.2"), ("size", 0), ("size", len(self.archive) + 1), ("signature", ""), ("signature", "invalid"), ("url", "https://other.example/app.tar.gz"), ("deltas", [{"from": "0.0.1"}])):
            with self.subTest(field=field, value=value):
                original = self.manifest.copy()
                self.manifest[field] = value
                self.save_manifest()
                with self.assertRaises(ValueError):
                    self.finalize()
                self.assertFalse(self.output.exists())
                self.manifest = original

    def test_rejects_non_numeric_build_identifiers(self):
        for build in ("native-preview", "20261105", "20261105000001/../other", "２０２６１１０５０００００１"):
            with self.subTest(build=build), self.assertRaises(ValueError):
                self.finalize(build)
        self.assertFalse(self.output.exists())

    def test_missing_signed_manifest_does_not_claim_an_update_package(self):
        (self.package / "update-windows-amd64.json").unlink()
        with self.assertRaises(FileNotFoundError):
            self.finalize()
        self.assertFalse(self.output.exists())

    def test_about_icons_match_the_registered_upstream_bytes(self):
        manifest = json.loads((ROOT / "third_party/manifests/lucide.json").read_text("utf-8"))
        files = {entry["path"]: entry for entry in manifest["files"]}
        for name in ("github", "info"):
            path = "apps/desktop/icons/" + name + ".svg"
            entry = files[path]
            data = (ROOT / path).read_bytes()
            self.assertEqual(len(data), entry["bytes"])
            self.assertEqual(hashlib.sha256(data).hexdigest(), entry["sha256"])
            self.assertIn(manifest["revision"], entry["source"])


if __name__ == "__main__":
    unittest.main()
