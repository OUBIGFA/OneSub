# -*- coding: utf-8 -*-
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

from test_check_subtitle import CHECKER, srt


SCRIPT_PATH = Path(__file__).with_name("assemble_subtitle.py")


class AssembleSubtitleTests(unittest.TestCase):
    def run_assemble(self, extension, parts, *options):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            part_paths = []
            for index, content in enumerate(parts, 1):
                path = root / (f"part{index}.{extension}")
                path.write_text(content, encoding="utf-8")
                part_paths.append(str(path))
            output = root / (f"out.{extension}")
            result = subprocess.run(
                [sys.executable, "-B", str(SCRIPT_PATH), str(output), *part_paths, *options],
                text=True,
                encoding="utf-8",
                capture_output=True,
            )
            output_text = output.read_text(encoding="utf-8") if output.exists() else ""
            return result, output_text

    def test_assembles_srt_parts_without_losing_blocks(self):
        result, output_text = self.run_assemble(
            "srt",
            [
                "1\n00:00:00,000 --> 00:00:01,000\n第一块\n\n",
                "2\n00:00:01,000 --> 00:00:02,000\n第二块\n\n",
            ],
        )
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertEqual(
            "1\n00:00:00,000 --> 00:00:01,000\n第一块\n\n"
            "2\n00:00:01,000 --> 00:00:02,000\n第二块\n\n",
            output_text,
        )

    def test_assembles_vtt_with_one_header(self):
        result, output_text = self.run_assemble(
            "vtt",
            [
                "WEBVTT\n\n00:00.000 --> 00:01.000\n第一块\n\n",
                "WEBVTT\n\n00:01.000 --> 00:02.000\n第二块\n\n",
            ],
        )
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertEqual(1, output_text.count("WEBVTT"))
        self.assertIn("第二块", output_text)


    def test_srt_numbering_is_rebuilt_across_parts(self):
        result, output = self.run_assemble("srt", [
            "1\n00:00:00,000 --> 00:00:02,000\nFirst\n",
            "1\n00:00:02,000 --> 00:00:04,000\nSecond\n",
        ])
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertIn("\n\n2\n00:00:02,000", output)

    def test_empty_or_malformed_parts_fail_without_publishing_output(self):
        for part in ("", "not a subtitle"):
            with self.subTest(part=part):
                result, output = self.run_assemble("srt", [part])
                self.assertEqual(1, result.returncode, result.stderr)
                self.assertEqual("", output)

    def test_conflicting_vtt_headers_are_not_silently_discarded(self):
        result, output = self.run_assemble("vtt", [
            "WEBVTT\nLanguage: en\n\n00:00.000 --> 00:02.000\nFirst\n",
            "WEBVTT\nLanguage: zh\n\n00:02.000 --> 00:04.000\nSecond\n",
        ])
        self.assertEqual(1, result.returncode, result.stderr)
        self.assertEqual("", output)

    def test_vtt_cue_only_parts_are_supported(self):
        result, output = self.run_assemble("vtt", [
            "WEBVTT\n\nNOTE keep\n\n00:00.000 --> 00:02.000\nFirst\n",
            "00:02.000 --> 00:04.000\nSecond\n",
        ])
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertIn("NOTE keep", output)
        self.assertIn("Second", output)

    def test_ass_events_are_inserted_before_trailing_sections(self):
        header = (
            "[Events]\n"
            "Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n"
        )
        first = "Dialogue: 0,0:00:00.00,0:00:02.00,Default,,0,0,0,,First\n"
        second = "Dialogue: 0,0:00:02.00,0:00:04.00,Default,,0,0,0,,Second\n"
        result, output = self.run_assemble("ass", [header + first + "\n[Fonts]\nfontname: test\n", second])
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertLess(output.index("Second"), output.index("[Fonts]"))

    def test_conflicting_ass_headers_fail_instead_of_losing_styles(self):
        header = (
            "[Script Info]\nPlayResX: 1920\n\n[Events]\n"
            "Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n"
        )
        result, output = self.run_assemble("ass", [
            header + "Dialogue: 0,0:00:00.00,0:00:02.00,Default,,0,0,0,,First\n",
            header.replace("1920", "1280")
            + "Dialogue: 0,0:00:02.00,0:00:04.00,Default,,0,0,0,,Second\n",
        ])
        self.assertEqual(1, result.returncode, result.stderr)
        self.assertEqual("", output)

    def test_ass_unrecognized_part_content_is_not_silently_dropped(self):
        first = (
            "[Events]\n"
            "Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n"
            "Dialogue: 0,0:00:00.00,0:00:02.00,Default,,0,0,0,,First\n"
        )
        result, output = self.run_assemble("ass", [first, "unrecognized important content\n"])
        self.assertEqual(1, result.returncode, result.stderr)
        self.assertEqual("", output)

    def test_input_and_existing_output_are_protected(self):
        with tempfile.TemporaryDirectory() as tmp:
            source = Path(tmp) / "part.srt"
            original = "1\n00:00:00,000 --> 00:00:02,000\nFirst\n\n"
            source.write_text(original, encoding="utf-8")
            output = Path(tmp) / "out.srt"
            output.write_text("keep this file", encoding="utf-8")
            for target, options in ((source, []), (source, ["--overwrite"]), (output, [])):
                with self.subTest(target=target, options=options):
                    result = subprocess.run(
                        [sys.executable, "-B", str(SCRIPT_PATH), str(target), str(source), *options],
                        capture_output=True, text=True, encoding="utf-8",
                    )
                    self.assertEqual(1, result.returncode, result.stderr)
                    self.assertEqual(original, source.read_text(encoding="utf-8"))
                    self.assertEqual("keep this file", output.read_text(encoding="utf-8"))
            result = subprocess.run(
                [sys.executable, "-B", str(SCRIPT_PATH), str(output), str(source), "--overwrite"],
                capture_output=True, text=True, encoding="utf-8",
            )
            self.assertEqual(0, result.returncode, result.stderr)
            self.assertEqual(original, output.read_text(encoding="utf-8"))


    def test_assembles_300_cues_from_independently_numbered_parts(self):
        parts = [srt(*[(index * 2, index * 2 + 2, "cue %d" % (index + 1))
                       for index in range(start, start + 50)])
                 for start in range(0, 300, 50)]
        result, output = self.run_assemble("srt", parts)
        self.assertEqual(0, result.returncode, result.stderr)
        blocks, errors = CHECKER.parse_srt(output)
        self.assertEqual([], errors)
        self.assertEqual(list(range(1, 301)), [block["index"] for block in blocks])
        self.assertEqual(600.0, blocks[-1]["end"])
        self.assertEqual(["cue %d" % index for index in range(1, 301)],
                         [" ".join(block["lines"]) for block in blocks])

    def test_ssa_full_parts_keep_one_header_and_all_events(self):
        header = (
            "[Script Info]\nScriptType: v4.00\n\n[Events]\n"
            "Format: Marked, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n"
        )
        result, output = self.run_assemble("ssa", [
            header + "Dialogue: Marked=0,0:00:00.00,0:00:02.00,Default,,0,0,0,,First\n",
            header + "Comment: Marked=0,0:00:02.00,0:00:04.00,Default,,0,0,0,,keep\n"
            + "Dialogue: Marked=0,0:00:02.00,0:00:04.00,Default,,0,0,0,,Second\n",
        ])
        self.assertEqual(0, result.returncode, result.stderr)
        self.assertEqual(1, output.count("[Script Info]"))
        self.assertEqual(2, output.count("Dialogue:"))
        self.assertIn("Comment:", output)


if __name__ == "__main__":
    unittest.main()
