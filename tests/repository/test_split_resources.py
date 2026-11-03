"""Validate shipped split resources and the real deterministic DAT compiler."""

import base64
import hashlib
import importlib.util
import json
from pathlib import Path
import struct
import unittest
import zlib


ROOT = Path(__file__).resolve().parents[2]


class SplitResourceTests(unittest.TestCase):
    def test_manifest_matches_embedded_resources_and_licenses(self):
        manifest = json.loads((ROOT / "third_party/manifests/subtitle-splitting.json").read_text("utf-8"))
        dictionary, budoux = manifest["jiebaDictionary"], manifest["budoux"]
        files = [(dictionary["artifact"], dictionary["artifactSha256"], dictionary["artifactBytes"])]
        files.extend((model["artifact"], model["sha256"], model["bytes"]) for model in budoux["models"])
        files.extend((item["license"], item["licenseSha256"], None) for item in (dictionary, budoux))
        files.append((dictionary["reviewedPhrases"], dictionary["reviewedPhrasesSha256"], None))
        for name, digest, size in files:
            with self.subTest(path=name):
                data = (ROOT / name).read_bytes()
                self.assertEqual(hashlib.sha256(data).hexdigest(), digest)
                if size is not None:
                    self.assertEqual(len(data), size)
                self.assertNotIn(b"\r", data)
        header = json.loads((ROOT / dictionary["artifact"]).read_text("utf-8"))
        payload = zlib.decompress(base64.b64decode(header["data"]))
        self.assertEqual(hashlib.sha256(payload).hexdigest(), header["sha256"])
        self.assertEqual(len(payload), dictionary["runtimeArrayBytes"])
        self.assertEqual(len(payload) // 8, header["states"])
        self.assertEqual(header["words"], dictionary["words"])

    def test_actual_compiler_is_deterministic_and_preserves_utf8_words(self):
        spec = importlib.util.spec_from_file_location("split_compiler", ROOT / "tools/build_split_lexicon.py")
        compiler = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(compiler)
        words = {"研究": 1000, "研究生": 100, "生命": 500, "人工智能": 100, "人工智慧": 100, "关于": 500, "關於": 500}
        data, unknown = compiler.compile_trie(words)
        second, second_unknown = compiler.compile_trie(dict(reversed(list(words.items()))))
        self.assertEqual((data, unknown), (second, second_unknown))
        for word in words:
            state = 1
            for byte in word.encode("utf-8"):
                base = struct.unpack_from("<I", data, state * 8)[0] & 0xFFFFFF
                next_state = base + byte + 1
                self.assertEqual(struct.unpack_from("<I", data, next_state * 8 + 4)[0], state)
                state = next_state
            self.assertGreater(struct.unpack_from("<I", data, state * 8)[0] >> 24, 0)
        self.assertGreater(unknown, 0)


if __name__ == "__main__":
    unittest.main()
