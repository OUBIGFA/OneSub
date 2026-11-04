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
        output = self.root / (name + "-document.json")
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

    def test_insert_delete_preserves_ass_and_vtt_metadata(self):
        samples = {
            "ass": ("[Script Info]\nScriptType: v4.00+\n[Custom]\nkeep=1\n[Events]\n"
                    "Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n"
                    "Comment: 0,0:00:00.00,0:00:01.00,Default,,0,0,0,,Keep me\n"
                    "Dialogue: 2,0:00:01.00,0:00:03.00,Default,Alice,0,0,0,,First\n"
                    "Dialogue: 2,0:00:04.00,0:00:06.00,Default,Alice,0,0,0,,Second\n"),
            "vtt": "WEBVTT\n\nNOTE keep this\nmetadata\n\nfirst-id\n00:01.000 --> 00:03.000 align:start\nFirst\n\nsecond-id\n00:04.000 --> 00:06.000 position:20%\nSecond\n",
        }
        for fmt, original in samples.items():
            with self.subTest(fmt=fmt):
                source=self.root / ("source."+fmt)
                source.write_text(original,encoding="utf-8")
                parsed=self.root / (fmt+"-parsed.json")
                self.assertTrue(self.request("read",source,parsed).get("ok"))
                doc=json.loads(parsed.read_text(encoding="utf-8"))
                inserted=dict(id="manual-new",position=0,startMs=500,endMs=900,text="Inserted",originalText="Inserted",readOnly=False)
                doc["cues"]=[inserted,doc["cues"][1]]
                doc["cues"][1]["position"]=1
                edited=self.root / (fmt+"-edited.json")
                edited.write_text(json.dumps(doc),encoding="utf-8")
                output=self.root / ("edited."+fmt)
                reply=self.request("write",edited,output)
                self.assertTrue(reply.get("ok"),reply)
                saved=output.read_text(encoding="utf-8")
                self.assertIn("Inserted",saved)
                self.assertIn("Second",saved)
                self.assertNotIn("First",saved)
                if fmt=="ass":
                    self.assertIn("[Custom]\nkeep=1",saved)
                    self.assertIn("Comment: 0,0:00:00.00,0:00:01.00,Default,,0,0,0,,Keep me",saved)
                else:
                    self.assertIn("NOTE keep this\nmetadata",saved)
                    self.assertIn("second-id",saved)
                    self.assertIn("position:20%",saved)
                    self.assertNotIn("first-id",saved)

    def test_manual_merge_preserves_metadata_and_renumbers_output(self):
        samples = {
            "srt": "1\n00:00:01,000 --> 00:00:02,000\nFirst.\n\n2\n00:00:02,000 --> 00:00:03,000\nSecond.\n\n3\n00:00:04,000 --> 00:00:05,000\nLast\n",
            "vtt": "WEBVTT\n\nNOTE keep this\nmetadata\n\nfirst-id\n00:01.000 --> 00:02.000 align:start\nFirst.\n\nsecond-id\n00:02.000 --> 00:03.000 align:start\nSecond.\n\nlast-id\n00:04.000 --> 00:05.000\nLast\n",
            "ass": ("[Script Info]\nScriptType: v4.00+\n[Custom]\nkeep=1\n[Events]\n"
                    "Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n"
                    "Comment: 0,0:00:00.00,0:00:01.00,Default,,0,0,0,,Keep me\n"
                    "Dialogue: 2,0:00:01.00,0:00:02.00,Default,Alice,0,0,0,,First.\n"
                    "Dialogue: 2,0:00:02.00,0:00:03.00,Default,Alice,0,0,0,,Second.\n"
                    "Dialogue: 2,0:00:04.00,0:00:05.00,Default,Alice,0,0,0,,Last\n"),
        }
        for fmt, original in samples.items():
            with self.subTest(fmt=fmt):
                doc = self.read("merge." + fmt, original)
                merged = dict(doc["cues"][0], id="manual-merge", endMs=3000,
                              text="First, Second.", mergedCueIds=["c1", "c2"])
                doc["cues"] = [merged, doc["cues"][2]]
                doc["cues"][1]["position"] = 1
                request = self.root / (fmt + "-merge.json")
                request.write_text(json.dumps(doc), encoding="utf-8")
                output = self.root / ("merged." + fmt)
                reply = self.request("write", request, output)
                self.assertTrue(reply.get("ok"), reply)
                saved = output.read_text(encoding="utf-8")
                back = self.read("reread." + fmt, saved)
                self.assertEqual([(c["position"], c["startMs"], c["endMs"], c["text"]) for c in back["cues"]],
                                 [(0, 1000, 3000, "First, Second."), (1, 4000, 5000, "Last")])
                if fmt == "srt":
                    self.assertIn("\n\n2\n00:00:04,000", saved)
                elif fmt == "vtt":
                    self.assertIn("NOTE keep this\nmetadata", saved)
                    self.assertIn("first-id", saved)
                    self.assertIn("align:start", saved)
                    self.assertNotIn("second-id", saved)
                else:
                    self.assertIn("[Custom]\nkeep=1", saved)
                    self.assertIn("Comment: 0,0:00:00.00,0:00:01.00,Default,,0,0,0,,Keep me", saved)
                    self.assertIn("Dialogue: 2,0:00:01.00,0:00:03.00,Default,Alice,0,0,0,,First, Second.", saved)

    def test_manual_merge_rejects_incompatible_ass_events_and_vtt_settings(self):
        for fmt, original in {
            "vtt": "WEBVTT\n\n00:01.000 --> 00:02.000 align:start\nFirst\n\n00:02.000 --> 00:03.000 align:end\nSecond\n",
            "ass": ("[Script Info]\nScriptType: v4.00+\n[Events]\n"
                    "Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n"
                    "Dialogue: 2,0:00:01.00,0:00:02.00,Default,Alice,0,0,0,,First\n"
                    "Dialogue: 3,0:00:02.00,0:00:03.00,Other,Bob,0,0,0,,Second\n"),
        }.items():
            doc = self.read("incompatible." + fmt, original)
            doc["cues"] = [dict(doc["cues"][0], id="manual-merge", endMs=3000,
                                text="First, Second", mergedCueIds=["c1", "c2"])]
            request = self.root / (fmt + "-badmerge.json")
            request.write_text(json.dumps(doc), encoding="utf-8")
            output = self.root / ("badmerged." + fmt)
            reply = self.request("write", request, output)
            self.assertFalse(reply.get("ok"))
            self.assertEqual(reply["error"], "E_MERGE_METADATA")
            self.assertFalse(output.exists())

    def test_ass_merge_keeps_tags_and_rejects_timed_effects(self):
        prefix = ("[Script Info]\nScriptType: v4.00+\n[Events]\n"
                  "Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n")
        # Each case needs its own protocol destination: the worker deliberately
        # refuses to overwrite an existing parsed document.
        for name, original_text, merged_text, error in (
                ("static-tags", r"{\i1}First.{\i0}", r"{\i1}First{\i0}, {\r}Second.", None),
                ("removed-tags", r"{\i1}First.{\i0}", "First, Second.", "E_ASS_TAGS"),
                ("timed-tags", r"{\k50}First", r"{\k50}First, {\r}Second.", "E_MERGE_METADATA")):
            with self.subTest(case=name):
                original = prefix + f"Dialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,{original_text}\nDialogue: 0,0:00:02.00,0:00:03.00,Default,,0,0,0,,Second.\n"
                doc = self.read("effect-" + name + ".ass", original)
                doc["cues"] = [dict(doc["cues"][0], id="manual-merge", endMs=3000,
                                    text=merged_text, mergedCueIds=["c1", "c2"])]
                request = self.root / ("merge-effect-" + name + ".json")
                request.write_text(json.dumps(doc), encoding="utf-8")
                output = self.root / ("effect-" + name + "-output.ass")
                reply = self.request("write", request, output)
                if error:
                    self.assertFalse(reply.get("ok"))
                    self.assertEqual(reply["error"], error)
                    self.assertFalse(output.exists())
                else:
                    self.assertTrue(reply.get("ok"), reply)
                    self.assertIn(merged_text, output.read_text(encoding="utf-8"))

    def test_single_line_export_preserves_times_and_format_metadata(self):
        samples = {
            "srt": "1\n00:00:01,250 --> 00:00:03,000\n<i>Upper\n下行</i>\n\n2\n00:00:04,000 --> 00:00:05,000\nSingle\n",
            "vtt": "WEBVTT\n\nNOTE keep this\nmetadata\n\nintro\n00:01.250 --> 00:03.000 align:start position:10%\n<i>Upper\n下行</i>\n\nlast\n00:04.000 --> 00:05.000\nSingle\n",
            "ass": ("[Script Info]\nScriptType: v4.00+\n[Custom]\nkeep=1\n[Events]\n"
                    "Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n"
                    "Comment: 0,0:00:00.00,0:00:01.00,Default,,0,0,0,,Keep me\n"
                    "Dialogue: 2,0:00:01.25,0:00:03.00,Default,Alice,0,0,0,,{\\i1}Upper\\N下行{\\i0}\n"
                    "Dialogue: 2,0:00:04.00,0:00:05.00,Default,Alice,0,0,0,,Single\n"),
        }
        for fmt, original in samples.items():
            doc = self.read("bilingual." + fmt, original)
            for line, kept, removed in (("upper", "Upper", "下行"), ("lower", "下行", "Upper")):
                with self.subTest(fmt=fmt, line=line):
                    source = self.root / (fmt + line + ".json")
                    source.write_text(json.dumps(dict(document=doc, line=line)), encoding="utf-8")
                    selected = self.root / (fmt + line + "-selected.json")
                    reply = self.request("select-lines", source, selected)
                    self.assertTrue(reply.get("ok"), reply)
                    projected = json.loads(selected.read_text(encoding="utf-8"))
                    self.assertEqual([(x["startMs"], x["endMs"]) for x in projected["cues"]], [(1250, 3000), (4000, 5000)])
                    self.assertIn(kept, projected["cues"][0]["text"])
                    self.assertNotIn(removed, projected["cues"][0]["text"])
                    self.assertEqual(projected["cues"][1]["text"], "Single")
                    output = self.root / (fmt + line + "." + fmt)
                    self.assertTrue(self.request("write", selected, output).get("ok"))
                    saved = output.read_text(encoding="utf-8")
                    separator = "\\N" if fmt == "ass" else "\n"
                    expected = original.replace("Upper" + separator + "下行", kept)
                    self.assertEqual(saved.strip(), expected.strip())
            # The read document and input stay intact after either projection.
            self.assertIn("Upper", doc["cues"][0]["text"])
            self.assertIn("下行", doc["cues"][0]["text"])
            self.assertIn("Upper", (self.root / ("bilingual." + fmt)).read_text(encoding="utf-8-sig"))

    def test_single_line_export_cannot_bypass_ass_protection(self):
        doc = self.read("protected.ass", "[Script Info]\nScriptType: v4.00+\n[Events]\n"
                        "Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n"
                        "Dialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,{\\i1}Upper\\NLower{\\i0}\n")
        doc["cues"][0]["text"] = "Upper\\NLower"
        source = self.root / "bad-projection.json"
        source.write_text(json.dumps(dict(document=doc, line="upper")), encoding="utf-8")
        output = self.root / "bad-projection-result.json"
        self.assertFalse(self.request("select-lines", source, output).get("ok"))
        self.assertFalse(output.exists())

    def test_single_line_export_rejects_invalid_choice(self):
        doc = self.read("one.srt", "1\n00:00:01,000 --> 00:00:02,000\nOne\n")
        source = self.root / "invalid-line.json"
        source.write_text(json.dumps(dict(document=doc, line="unknown")), encoding="utf-8")
        reply = self.request("select-lines", source, self.root / "invalid-line-result.json")
        self.assertFalse(reply.get("ok"))
        self.assertEqual(reply["error"], "E_LINE_SELECTION")

    def test_txt_export_keeps_visible_text_without_subtitle_metadata(self):
        srt_literal = r"Use file.srt at 1.25. a@b.com {keep} \notes <T> &lt;i&gt;literal&lt;/i&gt;"
        samples = {
            "srt": ("1\n00:00:01,250 --> 00:00:03,000\n<i>First &amp; second</i>\n下行 &#x4E16;&#x754C;\n\n"
                    "2\n00:00:04,000 --> 00:00:05,000\n" + srt_literal + "\n",
                    "First & second\n下行 世界\n" + srt_literal.replace("&lt;", "<").replace("&gt;", ">") + "\n"),
            "vtt": ("WEBVTT\n\nNOTE keep this\nmetadata\n\nintro\n"
                    "00:01.250 --> 00:03.000 align:start position:10%\n"
                    "<v Alice><lang en><c.yellow><b>First &amp; second</b></c></lang></v>"
                    "<00:00:01.750>\n<ruby>字<rt>じ</rt></ruby>\n\nlast\n"
                    "00:04.000 --> 00:05.000\nMeet at 00:00:01.750 &lt;i&gt;literal&lt;/i&gt;\n",
                    "First & second\n字じ\nMeet at 00:00:01.750 <i>literal</i>\n"),
        }
        # ASS comments and drawings are not transcript text. Mixed events keep
        # the visible words on either side of every drawing span.
        for fmt, script, fields, layer in (
                ("ass", "v4.00+", "Layer", "0"),
                ("ssa", "v4.00", "Marked", "Marked=0")):
            original = (f"[Script Info]\nScriptType: {script}\n[Events]\n"
                        f"Format: {fields}, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n"
                        f"Comment: {layer},0:00:00.00,0:00:01.00,Default,,0,0,0,,Keep me\n"
                        f"Dialogue: {layer},0:00:01.25,0:00:03.00,Default,Alice,0,0,0,,"
                        r"{\i1}First\hsecond{\i0}{comment}\N下行\n世界" + "\n"
                        f"Dialogue: {layer},0:00:04.00,0:00:05.00,Default,,0,0,0,,"
                        r"{\p1}m 0 0 l 20 20{\p0}" + "\n"
                        f"Dialogue: {layer},0:00:06.00,0:00:07.00,Default,,0,0,0,,"
                        r"前{\p1}m 0 0 l 20 20{\p0\fs20}中{\p2}m 0 0 l 30 30{\p0}后 <i> &amp;" + "\n")
            samples[fmt] = (original, "First second\n下行\n世界\n前中后 <i> &amp;\n")
        for fmt, (original, expected) in samples.items():
            with self.subTest(fmt=fmt):
                doc = self.read("transcript." + fmt, original)
                source_file = self.root / ("transcript." + fmt)
                before = source_file.read_bytes()
                doc["targetFormat"] = "txt"
                source = self.root / (fmt + "-txt.json")
                source.write_text(json.dumps(doc), encoding="utf-8")
                output = self.root / (fmt + ".txt")
                reply = self.request("write", source, output)
                self.assertTrue(reply.get("ok"), reply)
                self.assertEqual(output.read_bytes(), expected.encode("utf-8"))
                self.assertEqual(source_file.read_bytes(), before)

    def test_txt_line_selection_needs_no_timeline(self):
        doc = dict(format="srt", targetFormat="txt", cues=[
            dict(id="c1", position=0, startMs=-1, endMs=-2,
                 text="<i>Upper\n下行</i>", originalText="Evidence"),
            dict(id="c2", position=1, startMs=0, endMs=0,
                 text="Single", originalText="Single")])
        for line, expected in (("all", "Upper\n下行\nSingle\n"),
                               ("upper", "Upper\nSingle\n"),
                               ("lower", "下行\nSingle\n")):
            with self.subTest(line=line):
                source = self.root / (line + "-txt-input.json")
                source.write_text(json.dumps(doc if line == "all" else
                                             dict(document=doc, line=line)), encoding="utf-8")
                if line != "all":
                    selected = self.root / (line + "-txt-selected.json")
                    reply = self.request("select-lines", source, selected)
                    self.assertTrue(reply.get("ok"), reply)
                    source = selected
                output = self.root / (line + "-untimed.txt")
                reply = self.request("write", source, output)
                self.assertTrue(reply.get("ok"), reply)
                self.assertEqual(output.read_bytes(), expected.encode("utf-8"))
        # Removing times from an export must not weaken timed subtitle writes.
        doc["targetFormat"] = "srt"
        source = self.root / "invalid-timed.json"
        source.write_text(json.dumps(doc), encoding="utf-8")
        output = self.root / "invalid-timed.srt"
        reply = self.request("write", source, output)
        self.assertEqual(reply.get("error"), "E_TIMING")
        self.assertFalse(output.exists())

    def test_empty_txt_export_has_no_placeholder_or_bom(self):
        for name, cues in (("empty", []), ("styling", [dict(text="<i></i>", startMs=0, endMs=1000)])):
            source = self.root / (name + "-txt.json")
            source.write_text(json.dumps(dict(format="srt", targetFormat="txt", cues=cues)), encoding="utf-8")
            output = self.root / (name + ".txt")
            reply = self.request("write", source, output)
            self.assertTrue(reply.get("ok"), reply)
            self.assertEqual(output.read_bytes(), b"")

    def test_last_srt_subtitle_can_be_deleted(self):
        doc=self.read("last.srt","1\n00:00:00,000 --> 00:00:01,000\nLast\n")
        doc["cues"]=[]
        source=self.root / "empty.json"
        source.write_text(json.dumps(doc),encoding="utf-8")
        output=self.root / "empty.srt"
        self.assertTrue(self.request("write",source,output).get("ok"))
        self.assertEqual(output.read_text(encoding="utf-8"),"")


if __name__ == "__main__":
    unittest.main()
