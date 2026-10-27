# -*- coding: utf-8 -*-
"""Regression cases for format preservation, coverage, and CLI validation."""
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

from test_check_subtitle import CHECKER, SCRIPT_PATH, run_check, run_check_bytes, run_format_check, srt


ASS_HEADER = (
    "[Events]\n"
    "Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n"
)


class ReadingLoadRegressionTests(unittest.TestCase):
    def test_english_parentheses_are_exempt_even_when_they_are_the_entire_line(self):
        for text in ("(Blocking)", "（Pulse Modifier）", "（don't change, use Local）", "（User’s Guide）"):
            for counting in ("cjk", "raw"):
                with self.subTest(text=text, counting=counting):
                    self.assertEqual(0.0, CHECKER.display_width(text, counting))

    def test_nested_english_parentheses_are_fully_exempt(self):
        self.assertEqual(2.0, CHECKER.display_width("缩放（Set (Local) Scale）", "cjk"))

    def test_long_english_annotation_is_exempt_from_both_speed_and_scan_width(self):
        text = "缩放（Set Local Scale and Preserve Original Proportions）"
        _, errors, warnings, _ = run_check(srt((0, 1, text)))
        self.assertEqual([], errors)
        self.assertFalse(any("chars/s" in warning or "line spans" in warning
                             for warning in warnings), warnings)

    def test_chinese_parenthetical_payload_is_not_an_english_annotation(self):
        self.assertEqual(5.0, CHECKER.display_width("设置（不要改）", "cjk"))

    def test_visible_comparisons_are_not_stripped_as_markup(self):
        self.assertEqual(9.0, CHECKER.display_width("1 < 3 > 2", "raw"))

    def test_entities_are_measured_as_their_displayed_characters(self):
        self.assertEqual(5.0, CHECKER.display_width("A &amp; B", "raw"))
        self.assertEqual(1.0, CHECKER.display_width("&#x4E2D;", "cjk"))

    def test_markup_and_nbsp_only_is_empty_visible_text(self):
        errors = run_check(srt((0, 2, "<i>&nbsp;</i>")))[1]
        self.assertTrue(any("empty subtitle text" in error for error in errors), errors)


class ConfigurationRegressionTests(unittest.TestCase):
    def load_profile(self, **overrides):
        profile = {
            "target_language": "default", "counting": "raw",
            "final_punctuation": "standard", "max_cps": 17, "max_width": 42,
        }
        profile.update(overrides)
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "profiles.json"
            path.write_text(json.dumps({"default": profile}), encoding="utf-8")
            return CHECKER.load_language_profiles(path)["default"]

    def test_optional_booleans_have_safe_defaults(self):
        profile = self.load_profile()
        self.assertIs(False, profile["spacing"])
        self.assertIs(False, profile["ban_exclamation"])

    def test_invalid_limits_raise_value_error_instead_of_disabling_checks(self):
        for field in ("max_cps", "max_width"):
            for value in (0, -1, True, None, "17", float("nan"), float("inf")):
                with self.subTest(field=field, value=value), self.assertRaises(ValueError):
                    self.load_profile(**{field: value})

    def test_optional_booleans_reject_truthy_strings(self):
        with self.assertRaises(ValueError):
            self.load_profile(spacing="false")

    def test_script_and_region_codes_keep_the_most_specific_profile(self):
        self.assertEqual("zh-hant", CHECKER.language_profile_resolution("zh-Hant-TW")[1])
        self.assertEqual("zh-hant", CHECKER.language_profile_resolution("zh_TW")[1])

    def test_cli_rejects_invalid_overrides_and_strict_without_source(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "out.srt"
            path.write_text(srt((0, 2, "Hello")), encoding="utf-8")
            cases = (
                ["--max-cps", "0"], ["--max-width", "nan"],
                ["--min-duration", "-1"], ["--pause-gap", "-0.1"],
                ["--max-lines", "0"], ["--pad-factor", "1"],
                ["--drop-factor", "1.1"], ["--strict"],
            )
            for arguments in cases:
                with self.subTest(arguments=arguments):
                    result = subprocess.run(
                        [sys.executable, "-B", str(SCRIPT_PATH), str(path), *arguments],
                        capture_output=True, text=True, encoding="utf-8",
                    )
                    self.assertEqual(2, result.returncode, result.stdout + result.stderr)
                    self.assertNotIn("Traceback", result.stderr)


class MarkupRegressionTests(unittest.TestCase):
    def test_strict_mode_rejects_tags_moved_to_a_different_cue(self):
        source = srt((0, 2, "<i>First</i>"), (2, 4, "Second"))
        output = srt((0, 2, "第一句"), (2, 4, "<i>第二句</i>"))
        errors = run_check(output, source, strict=True)[1]
        self.assertTrue(any("markup" in error for error in errors), errors)

    def test_strict_mode_rejects_changed_tag_order(self):
        source = srt((0, 2, "<i><b>First</b></i>"))
        output = srt((0, 2, "<b><i>第一句</i></b>"))
        errors = run_check(output, source, strict=True)[1]
        self.assertTrue(any("markup" in error for error in errors), errors)

    def test_unbalanced_or_crossed_html_tags_are_rejected_without_source(self):
        for text in ("<i>你好", "<i><b>你好</i></b>", "你好</i>"):
            with self.subTest(text=text):
                errors = run_check(srt((0, 2, text)))[1]
                self.assertTrue(any("markup" in error for error in errors), errors)

    def test_escaped_entities_cannot_silently_change(self):
        source = "WEBVTT\n\n00:00.000 --> 00:02.000\nA &amp; B\n"
        output = "WEBVTT\n\n00:00.000 --> 00:02.000\n甲 & B\n"
        errors = run_format_check(output, source, "vtt")[1]
        self.assertTrue(any("markup" in error for error in errors), errors)


class FormatRegressionTests(unittest.TestCase):
    def test_missing_srt_block_separator_is_an_error_not_a_wrapped_line(self):
        output = "1\n00:00:00,000 --> 00:00:02,000\nFirst\n2\n00:00:02,000 --> 00:00:04,000\nSecond\n"
        self.assertTrue(run_check(output)[1])

    def test_invalid_webvtt_header_is_rejected(self):
        for header in ("WEBVTTbad", "WEBVTT --> bad"):
            with self.subTest(header=header):
                text = header + "\n\n00:00.000 --> 00:02.000\nHello\n"
                errors = CHECKER.parse_vtt(text)[1]
                self.assertTrue(errors)

    def test_vtt_split_keeps_identifier_only_on_first_piece(self):
        source = "WEBVTT\n\ncue-1\n00:00.000 --> 00:04.000 align:start\nFirst and second\n"
        output = (
            "WEBVTT\n\ncue-1\n00:00.000 --> 00:02.000 align:start\n第一句\n\n"
            "00:02.000 --> 00:04.000 align:start\n第二句\n"
        )
        self.assertEqual([], run_format_check(output, source, "vtt")[1])

    def test_vtt_style_after_cues_is_rejected(self):
        text = "WEBVTT\n\n00:00.000 --> 00:02.000\nHello\n\nSTYLE\n::cue {color:red;}\n"
        self.assertTrue(CHECKER.parse_vtt(text)[1])

    def test_ass_layered_events_keep_their_own_fields(self):
        source = ASS_HEADER + (
            "Dialogue: 0,0:00:00.00,0:00:05.00,Default,,0,0,0,,Speech\n"
            "Dialogue: 1,0:00:01.00,0:00:02.00,Sign,,0,0,0,,Sign\n"
        )
        output = source.replace("Speech", "对白").replace(",,Sign\n", ",,标牌\n")
        self.assertEqual([], run_format_check(output, source, "ass")[1])

    def test_ass_event_order_need_not_be_chronological(self):
        source = ASS_HEADER + (
            "Dialogue: 1,0:00:03.00,0:00:05.00,Sign,,0,0,0,,Sign\n"
            "Dialogue: 0,0:00:00.00,0:00:03.00,Default,,0,0,0,,Speech\n"
        )
        self.assertEqual([], run_format_check(source, source, "ass")[1])

    def test_ass_cannot_drop_an_overlapping_sign(self):
        dialogue = "Dialogue: 0,0:00:00.00,0:00:05.00,Default,,0,0,0,,Speech\n"
        source = ASS_HEADER + dialogue + "Dialogue: 1,0:00:01.00,0:00:02.00,Sign,,0,0,0,,Sign\n"
        self.assertTrue(run_format_check(ASS_HEADER + dialogue, source, "ass")[1])

    def test_ass_normal_mode_is_text_only_and_cannot_retime(self):
        source = ASS_HEADER + "Dialogue: 0,0:00:00.00,0:00:05.00,Default,,0,0,0,,Speech\n"
        output = source.replace("0:00:05.00", "0:00:06.00")
        errors = run_format_check(output, source, "ass")[1]
        self.assertTrue(any("non-Text" in error for error in errors), errors)

    def test_ass_format_with_nonterminal_text_is_rejected(self):
        text = "[Events]\nFormat: Start, Text, End\nDialogue: 0:00:00.00,Hello,0:00:02.00\n"
        self.assertTrue(CHECKER.parse_ass(text)[1])


class CoverageRegressionTests(unittest.TestCase):
    def test_a_cue_touching_only_the_edges_of_speech_is_still_in_the_gap(self):
        source = srt((0, 2, "Hello"), (3, 5, "World"))
        output = srt((0, 2, "你好"), (2, 3, "空档"), (3, 5, "世界"))
        errors = run_check(output, source)[1]
        self.assertTrue(any("outside source speech" in error for error in errors), errors)

    def test_a_cue_wholly_inside_a_small_source_gap_is_also_rejected(self):
        source = srt((0, 2, "Hello"), (2.2, 5, "World"))
        output = srt((0, 2, "你好"), (2.02, 2.18, "空档"), (2.2, 5, "世界"))
        errors = run_check(output, source)[1]
        self.assertTrue(any("outside source speech" in error for error in errors), errors)

    def test_crossing_a_small_source_gap_is_reported(self):
        source = srt((0, 2, "Hello"), (2.2, 5, "World"))
        errors, warnings = run_check(srt((0, 5, "你好世界")), source)[1:3]
        self.assertEqual([], errors)
        self.assertTrue(any("source gap" in warning for warning in warnings), warnings)

    def test_shortening_an_existing_gap_does_not_drop_source_speech(self):
        source = srt((0, 1, "Hello"), (1.2, 3, "World"))
        output = srt((0, 1.05, "你好"), (1.15, 3, "世界"))
        self.assertEqual([], run_check(output, source)[1])


class EncodingAndLayoutRegressionTests(unittest.TestCase):
    def test_legacy_source_encodings_are_read_without_corrupting_the_timeline(self):
        source = srt((0, 2, "原文"))
        output = srt((0, 2, "译文")).encode("utf-8")
        for encoding in ("utf-8-sig", "utf-16", "utf-16-le", "utf-16-be", "gb18030"):
            with self.subTest(encoding=encoding):
                self.assertEqual([], run_check_bytes(output, source.encode(encoding))[1])

    def test_ass_comment_cannot_move_relative_to_dialogue_events(self):
        first = "Dialogue: 0,0:00:00.00,0:00:02.00,Default,,0,0,0,,First\n"
        second = "Dialogue: 0,0:00:02.00,0:00:04.00,Default,,0,0,0,,Second\n"
        comment = "Comment: 0,0:00:02.00,0:00:04.00,Default,,0,0,0,,keep here\n"
        source = ASS_HEADER + first + comment + second
        output = ASS_HEADER + comment + first + second
        self.assertTrue(run_format_check(output, source, "ass")[1])

    def test_strict_vtt_keeps_note_placement_between_cues(self):
        first = "00:00.000 --> 00:02.000\nFirst\n\n"
        second = "00:02.000 --> 00:04.000\nSecond\n"
        source = "WEBVTT\n\n" + first + "NOTE chapter\n\n" + second
        output = "WEBVTT\n\nNOTE chapter\n\n" + first + second
        self.assertTrue(run_format_check(output, source, "vtt", strict=True)[1])


class CheckerCliIntegrationTests(unittest.TestCase):
    def test_json_cli_validates_all_formats_in_strict_mode(self):
        sources = {
            "srt": srt((0, 4, "Hello")),
            "vtt": "WEBVTT\n\ncue-1\n00:00.000 --> 00:04.000 align:start\n<i>Hello</i>\n",
            "ass": ASS_HEADER + "Dialogue: 0,0:00:00.00,0:00:04.00,Default,,0,0,0,,Hello\n",
        }
        with tempfile.TemporaryDirectory() as tmp:
            for fmt, source in sources.items():
                with self.subTest(fmt=fmt):
                    source_path, output_path = Path(tmp) / ("in." + fmt), Path(tmp) / ("out." + fmt)
                    source_path.write_text(source, encoding="utf-8")
                    output_path.write_text(source.replace("Hello", "你好"), encoding="utf-8")
                    result = subprocess.run(
                        [sys.executable, "-B", str(SCRIPT_PATH), str(output_path),
                         "--source", str(source_path), "--lang", "zh", "--strict", "--json"],
                        capture_output=True, text=True, encoding="utf-8",
                    )
                    self.assertEqual(0, result.returncode, result.stdout + result.stderr)
                    report = json.loads(result.stdout)
                    self.assertEqual(fmt, report["format"])
                    self.assertEqual(1, report["blocks"])
                    self.assertEqual([], report["errors"])
                    self.assertEqual([], report["warnings"])

    def test_exit_zero_still_exposes_density_warnings_in_json(self):
        with tempfile.TemporaryDirectory() as tmp:
            output = Path(tmp) / "out.srt"
            output.write_text(srt((0, 2, "这极为方便也可以直接把节点拖进去")), encoding="utf-8")
            result = subprocess.run(
                [sys.executable, "-B", str(SCRIPT_PATH), str(output), "--json"],
                capture_output=True, text=True, encoding="utf-8",
            )
            self.assertEqual(0, result.returncode, result.stdout + result.stderr)
            report = json.loads(result.stdout)
            self.assertEqual([], report["errors"])
            self.assertTrue(any("short-duration overload" in warning for warning in report["warnings"]))


if __name__ == "__main__":
    unittest.main()
