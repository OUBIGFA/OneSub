"""Build receipts must describe the exact staged source, before tools run."""

import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]


@unittest.skipUnless(os.name == "nt", "Windows build entry only")
class BuildPreflightTests(unittest.TestCase):
    def setUp(self):
        scratch = ROOT / "_temp/repository-tests"
        scratch.mkdir(parents=True, exist_ok=True)
        self.temp = tempfile.TemporaryDirectory(prefix="onesub-build-preflight-", dir=scratch)
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        (self.root / "build").mkdir()
        (self.root / "apps/desktop").mkdir(parents=True)
        shutil.copyfile(ROOT / "build/build-windows.ps1", self.root / "build/build-windows.ps1")
        shutil.copyfile(ROOT / "apps/desktop/mygo.json", self.root / "apps/desktop/mygo.json")
        (self.root / "bin").mkdir()
        (self.root / "third_party/manifests").mkdir(parents=True)
        self.prepare_bun()
        (self.root / "source.go").write_text("package fixture\n", encoding="utf-8")
        subprocess.run(["git", "init", "-q", str(self.root)], check=True, capture_output=True)
        subprocess.run(["git", "-C", str(self.root), "add", "."], check=True, capture_output=True)

    def prepare_bun(self, revision="1.3.9+fixture"):
        executable = self.root / "bin/bun.cmd"
        executable.write_text("@echo off\necho " + revision + "\n", encoding="utf-8")
        manifest = {"bun": {"executableSha256": hashlib.sha256(executable.read_bytes()).hexdigest(),
                            "cliRevision": "1.3.9+fixture"}}
        (self.root / "third_party/manifests/distribution-materials.json").write_text(
            json.dumps(manifest), encoding="utf-8",
        )

    def build(self, *arguments, signing_key=None):
        env = os.environ.copy()
        env["PATH"] = str(self.root / "bin") + os.pathsep + env.get("PATH", "")
        env.pop("MYGO_UPDATER_PRIVATE_KEY", None)
        env["GOTOOLCHAIN"] = "auto"
        if signing_key is not None:
            env["MYGO_UPDATER_PRIVATE_KEY"] = signing_key
        return subprocess.run(
            ["powershell.exe", "-NoProfile", "-NonInteractive", "-File", str(self.root / "build/build-windows.ps1"),
             "-BuildNumber", "20261114000001", *arguments],
            capture_output=True, text=True, encoding="utf-8", errors="replace", timeout=30,
            env=env, cwd=self.root, creationflags=subprocess.CREATE_NO_WINDOW,
        )

    def test_unstaged_source_is_rejected_before_building(self):
        (self.root / "source.go").write_text("package changed\n", encoding="utf-8")
        result = self.build()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Stage all release source changes before building", result.stderr)
        self.assertFalse((self.root / "build/output").exists())

    def test_untracked_source_is_rejected_before_building(self):
        (self.root / "extra.go").write_text("package fixture\n", encoding="utf-8")
        result = self.build()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Stage all release source changes before building", result.stderr)

    def test_different_product_tag_is_rejected_before_building(self):
        result = self.build("-ReleaseTag", "0.2.0")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Release tag must equal the product version", result.stderr)

    def test_staged_source_reaches_the_signing_key_check(self):
        result = self.build("-ReleaseTag", "0.1.0")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("matching update signing key is required", result.stderr)

    def test_signing_key_does_not_reach_preparation_helpers(self):
        (self.root / "tools").mkdir()
        (self.root / "tools/prepare_mygo.py").write_text(
            "import os, pathlib, sys\n"
            "pathlib.Path('exposed-key.txt').write_text(os.environ.get('MYGO_UPDATER_PRIVATE_KEY', ''), encoding='utf-8')\n"
            "sys.exit(42)\n", encoding="utf-8",
        )
        subprocess.run(["git", "-C", str(self.root), "add", "."], check=True, capture_output=True)
        result = self.build(signing_key="fixture-signing-key")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual((self.root / "exposed-key.txt").read_text("utf-8"), "")

    def test_changed_bun_is_rejected_before_preparation(self):
        (self.root / "bin/bun.cmd").write_text("@echo off\necho changed\n", encoding="utf-8")
        subprocess.run(["git", "-C", str(self.root), "add", "."], check=True, capture_output=True)
        result = self.build(signing_key="fixture-signing-key")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Pinned Bun executable checksum differs", result.stderr)

    def test_wrong_bun_revision_is_rejected_even_with_matching_checksum(self):
        self.prepare_bun("1.3.9+different")
        subprocess.run(["git", "-C", str(self.root), "add", "."], check=True, capture_output=True)
        result = self.build(signing_key="fixture-signing-key")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Pinned Bun CLI revision differs", result.stderr)

    def test_preparation_uses_only_the_local_go_toolchain(self):
        (self.root / "tools").mkdir()
        (self.root / "tools/prepare_mygo.py").write_text(
            "import os, pathlib, sys\n"
            "pathlib.Path('toolchain.txt').write_text(os.environ.get('GOTOOLCHAIN', ''), encoding='utf-8')\n"
            "sys.exit(42)\n", encoding="utf-8",
        )
        subprocess.run(["git", "-C", str(self.root), "add", "."], check=True, capture_output=True)
        result = self.build(signing_key="fixture-signing-key")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual((self.root / "toolchain.txt").read_text("utf-8"), "local")

    def test_media_version_does_not_replace_the_product_build_receipt(self):
        (self.root / "tools").mkdir()
        (self.root / "workers/style-core").mkdir(parents=True)
        (self.root / "build/output/oneasr").mkdir(parents=True)
        (self.root / ".gitignore").write_text("/build/output/\n", encoding="utf-8")
        (self.root / "tools/prepare_mygo.py").write_text("print('fixture.mod')\n", encoding="utf-8")
        media_program = self.root / "media.cs"
        media_program.write_text(
            'class FixtureMedia { static void Main() { System.Console.WriteLine("ffmpeg version 9.0.2"); '
            'for (int lineIndex = 0; lineIndex < 8; lineIndex++) { '
            'System.Threading.Thread.Sleep(40); System.Console.WriteLine("version details"); } } }',
            encoding="utf-8",
        )
        compiler = Path(os.environ["WINDIR"]) / "Microsoft.NET/Framework64/v4.0.30319/csc.exe"
        subprocess.run([str(compiler), "/nologo", "/out:" + str(self.root / "bin/ffmpeg.exe"), str(media_program)],
                       check=True, capture_output=True, creationflags=subprocess.CREATE_NO_WINDOW)
        shutil.copyfile(self.root / "bin/ffmpeg.exe", self.root / "bin/ffprobe.exe")
        (self.root / "tools/prepare_media.py").write_text(
            "from pathlib import Path\nimport shutil\n"
            "out = Path('build/output/media-tools')\nout.mkdir(parents=True, exist_ok=True)\n"
            "for name in ('ffmpeg', 'ffprobe'): shutil.copyfile(Path('bin') / (name + '.exe'), out / (name + '.exe'))\n",
            encoding="utf-8",
        )
        media = []
        for name in ("ffmpeg", "ffprobe"):
            tool = self.root / f"bin/{name}.exe"
            media.append({"file": f"{name}.exe", "sha256": hashlib.sha256(tool.read_bytes()).hexdigest()})
        runtime = self.root / "build/output/oneasr/oneasr-cli.exe"
        runtime.write_bytes(b"fixture-runtime")
        manifest = {"media": media, "oneasr": {"sha256": hashlib.sha256(runtime.read_bytes()).hexdigest()}}
        (self.root / "third_party/manifests/runtimes.json").write_text(json.dumps(manifest), encoding="utf-8")
        for name in ("build-player", "build-subtitle", "build-skill-tools", "build-oneasr", "prepare-vad", "prepare-sherpa"):
            (self.root / f"build/{name}.ps1").write_text("", encoding="utf-8")
        (self.root / "build/test.ps1").write_text("throw 'fixture-stop-after-build-info'\n", encoding="utf-8")
        subprocess.run(["git", "-C", str(self.root), "add", "."], check=True, capture_output=True)

        result = self.build("-ReleaseTag", "0.1.0", signing_key="fixture-signing-key")

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("fixture-stop-after-build-info", result.stderr)
        receipt = json.loads((self.root / "build/output/build-info.json").read_text("utf-8"))
        self.assertEqual(receipt["version"], "0.1.0")
        self.assertEqual(receipt["updateVersion"], "0.1.0.20261114000001")
        self.assertEqual(receipt["releaseTag"], "0.1.0")


if __name__ == "__main__":
    unittest.main()
