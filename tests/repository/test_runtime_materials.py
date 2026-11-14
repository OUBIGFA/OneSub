"""Runtime refresh must retain reviewed provenance and dependency registrations."""

import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch


ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location("record_runtime_materials", ROOT / "tools/record_runtime_materials.py")
RECORDER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(RECORDER)


class RuntimeMaterialsTests(unittest.TestCase):
    def test_refresh_preserves_registered_dependencies(self):
        scratch = ROOT / "_temp/repository-tests"
        scratch.mkdir(parents=True, exist_ok=True)
        with tempfile.TemporaryDirectory(dir=scratch) as temporary:
            root = Path(temporary)

            def put(name, text):
                path = root / name
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text(text, encoding="utf-8")
                return path

            manifest_path = put("third_party/manifests/runtimes.json", json.dumps({
                "mygo": {"version": "v0.8.3", "revision": "reviewed-revision", "archiveSha256": "reviewed-hash", "modifications": "reviewed patches"},
                "sherpaOnnx": {"version": "v2.0.1", "files": [{"file": "runtime.dll", "sha256": "reviewed-runtime-hash"}]},
                "mcpGoSdk": {"version": "v1.8.0"},
                "remainingMaterials": ["reviewed outstanding notices"],
            }))
            put("apps/desktop/go.mod", "module test\nrequire github.com/egoist/mygo v0.8.3\n")
            put(".local/gomodcache/github.com/egoist/mygo@v0.8.3/LICENSE", "Current MyGo license")
            put(".local/gomodcache/github.com/egoist/mygo@v0.6.4/LICENSE", "Obsolete MyGo license")
            put("third_party/sources/subtitleedit-e316223ca1cfe8861acb4da4cd0e4ef5cce634f9/src/libse/LICENSE.txt", "libse license")
            put("media/LICENSE", "FFmpeg license")
            executables = {name: str(put(f"media/bin/{name}.exe", name)) for name in ("ffmpeg", "ffprobe")}
            put("build/output/oneasr/oneasr-cli.exe", "OneAsr fixture")
            for name in ("jassub", "abslink", "lfa-ponyfill", "rvfc-polyfill", "throughput"):
                put(f"apps/desktop/player/node_modules/{name}/package.json", json.dumps({"version": "1", "license": "MIT"}))
                if name != "abslink":
                    put(f"apps/desktop/player/node_modules/{name}/LICENSE", "Player license")
            reviewed = put("third_party/licenses/abslink.txt", "Verified published license")
            registered = json.loads(manifest_path.read_text(encoding="utf-8"))
            registered["player"] = [{
                "name": "abslink", "version": "1", "spdx": "MIT",
                "notice": "third_party/licenses/abslink.txt",
                "noticeSha256": hashlib.sha256(reviewed.read_bytes()).hexdigest(),
                "source": "https://registry.npmjs.org/abslink/1",
            }]
            manifest_path.write_text(json.dumps(registered), encoding="utf-8")
            before = json.loads(manifest_path.read_text(encoding="utf-8"))
            version = "ffmpeg version 8.1-full_build-www.gyan.dev\ncompiler\nconfiguration: --enable-gpl"
            with patch.object(RECORDER, "ROOT", root), patch.object(RECORDER, "LICENSES", root / "third_party/licenses"), \
                    patch.object(RECORDER.shutil, "which", side_effect=executables.get), \
                    patch.object(RECORDER.subprocess, "check_output", return_value=version):
                RECORDER.main()
            after = json.loads(manifest_path.read_text(encoding="utf-8"))
            for key in ("sherpaOnnx", "mcpGoSdk", "remainingMaterials"):
                self.assertEqual(after.get(key), before[key], key)
            for key, value in before["mygo"].items():
                self.assertEqual(after["mygo"].get(key), value, key)
            self.assertEqual((root / "third_party/licenses/mygo.txt").read_text(encoding="utf-8"), "Current MyGo license")
            self.assertEqual(after["media"][0]["bytes"], len("ffmpeg"))
            recorded = next(entry for entry in after["player"] if entry["name"] == "abslink")
            self.assertEqual(recorded["notice"], "third_party/licenses/abslink.txt")
            self.assertEqual(recorded["noticeSha256"], registered["player"][0]["noticeSha256"])
            self.assertEqual(recorded["source"], "https://registry.npmjs.org/abslink/1")


if __name__ == "__main__":
    unittest.main()
