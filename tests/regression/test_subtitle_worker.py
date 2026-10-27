"""Exercise the real libse worker through its bounded file protocol."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
WORKER = Path(os.environ.get("ONESUB_SUBTITLE_WORKER", ROOT / "build/output/subtitle-core/OneSub.SubtitleCore.exe"))


class SubtitleWorkerTests(unittest.TestCase):
    def setUp(self):
        self.work = tempfile.TemporaryDirectory(prefix="onesub-subtitles-")
        self.addCleanup(self.work.cleanup)
        self.root = Path(self.work.name)

    def request(self, operation, source, output):
        request = dict(protocol=1, requestId="test", operation=operation,
                       inputPath=str(source), outputPath=str(output))
        run = subprocess.run([str(WORKER)], input=json.dumps(request),
                             capture_output=True, text=True, encoding="utf-8", timeout=30)
        return json.loads(run.stdout)

    def read(self, name, text):
        source = self.root / name
        source.write_text(text, encoding="utf-8-sig")
        output = self.root / "document.json"
        reply = self.request("read", source, output)
        self.assertTrue(reply.get("ok"), reply)
        return json.loads(output.read_text(encoding="utf-8"))

    def test_srt_bom_chinese_and_millisecond_times(self):
        doc = self.read("课程.srt", "1\n00:00:01,250 --> 00:00:03,000\n第一句\n\n2\n00:00:04,000 --> 00:00:05,500\nsecond\n")
        self.assertEqual([(x["startMs"], x["endMs"], x["text"]) for x in doc["cues"]],
                         [(1250, 3000, "第一句"), (4000, 5500, "second")])

    def test_invalid_input_does_not_report_success(self):
        source = self.root / "invalid.srt"
        source.write_text("This is not a subtitle", encoding="utf-8")
        output = self.root / "invalid.json"
        self.assertFalse(self.request("read", source, output).get("ok"))
        self.assertFalse(output.exists())

    def test_webvtt_metadata_survives_text_edit(self):
        original = "WEBVTT\n\nNOTE keep this\nprivate note\n\nintro\n00:01.000 --> 00:02.500 align:start position:10%\nHello\n"
        doc = self.read("lesson.vtt", original)
        doc["cues"][0]["text"] = "你好"
        source = self.root / "edited.json"
        source.write_text(json.dumps(doc), encoding="utf-8")
        output = self.root / "edited.vtt"
        self.assertTrue(self.request("write", source, output).get("ok"))
        self.assertEqual(output.read_text(encoding="utf-8-sig"), original.replace("Hello", "你好"))

    def test_ass_text_edit_preserves_non_text_bytes(self):
        original = ("[Script Info]\nScriptType: v4.00+\n[Custom]\nkeep=1\n[Events]\n"
                    "Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n"
                    "Comment: 0,0:00:00.00,0:00:01.00,Default,,0,0,0,,Keep me\n"
                    "Dialogue: 2,0:00:01.00,0:00:03.00,Default,Alice,0,0,0,,{\\i1}Hello{\\i0}\\NWorld\n")
        doc = self.read("lesson.ass", original)
        self.assertEqual(len(doc["cues"]), 1)
        doc["cues"][0]["text"] = "{\\i1}你好{\\i0}\\N世界"
        source = self.root / "edited.json"
        source.write_text(json.dumps(doc), encoding="utf-8")
        output = self.root / "edited.ass"
        reply = self.request("write", source, output)
        self.assertTrue(reply.get("ok"), reply)
        self.assertEqual(output.read_text(encoding="utf-8-sig"), original.replace("Hello", "你好").replace("World", "世界"))

    def test_ass_protected_tags_cannot_be_removed(self):
        original = ("[Script Info]\nScriptType: v4.00+\n[Events]\n"
                    "Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n"
                    "Dialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,{\\i1}Hello{\\i0}\n")
        doc = self.read("tags.ass", original)
        doc["cues"][0]["text"] = "你好"
        source = self.root / "edited.json"
        source.write_text(json.dumps(doc), encoding="utf-8")
        output = self.root / "edited.ass"
        self.assertFalse(self.request("write", source, output).get("ok"))
        self.assertFalse(output.exists())


if __name__ == "__main__":
    unittest.main()
