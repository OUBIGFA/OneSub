# -*- coding: utf-8 -*-
import importlib.util
import tempfile
import unittest
from pathlib import Path

SCRIPT_PATH = Path(__file__).with_name("check_subtitle.py")
SPEC = importlib.util.spec_from_file_location("check_subtitle", SCRIPT_PATH)
CHECKER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(CHECKER)


def make_cfg(lang="zh", **overrides):
    """The cfg dict main() builds, so tests exercise the same defaults as the CLI."""
    profile = CHECKER.get_language_profile(lang)
    cfg = {
        "lang": lang,
        "profile_code": CHECKER.language_profile_resolution(lang)[1],
        "max_cps": profile["max_cps"],
        "max_width": profile["max_width"],
        "counting": profile["counting"],
        "final_punct": profile["final_punctuation"] == "none",
        "ban_exclamation": profile.get("ban_exclamation", False),
        "spacing": profile["spacing"],
        "strict": False,
        "min_duration": CHECKER.DEFAULTS["min_duration"],
        "max_duration": CHECKER.DEFAULTS["max_duration"],
        "max_lines": CHECKER.DEFAULTS["max_lines"],
        "min_split_piece": CHECKER.DEFAULTS["min_split_piece"],
        "pause_gap": CHECKER.DEFAULTS["pause_gap"],
        "min_span_cost": CHECKER.FIDELITY["min_span_cost"],
        "min_spans": CHECKER.FIDELITY["min_spans"],
        "pad_factor": CHECKER.FIDELITY["pad_factor"],
        "drop_factor": CHECKER.FIDELITY["drop_factor"],
        "max_reports": CHECKER.FIDELITY["max_reports"],
    }
    cfg.update(overrides)
    return cfg


def run_check(output, source=None, lang="zh", **overrides):
    """Write the given SRT text(s) to a temp dir and run the checker on them."""
    with tempfile.TemporaryDirectory() as tmp:
        out_path = Path(tmp) / "out.srt"
        out_path.write_text(output, encoding="utf-8")
        src_path = None
        if source is not None:
            src_path = Path(tmp) / "in.srt"
            src_path.write_text(source, encoding="utf-8")
        return CHECKER.check(
            str(out_path),
            str(src_path) if src_path else None,
            "srt",
            "srt" if src_path else None,
            make_cfg(lang, **overrides),
        )


def run_check_bytes(output, source=None, lang="zh", **overrides):
    """Run the checker while controlling the output encoding."""
    with tempfile.TemporaryDirectory() as tmp:
        out_path = Path(tmp) / "out.srt"
        out_path.write_bytes(output)
        src_path = None
        if source is not None:
            src_path = Path(tmp) / "in.srt"
            src_path.write_bytes(source)
        return CHECKER.check(
            str(out_path),
            str(src_path) if src_path else None,
            "srt",
            "srt" if src_path else None,
            make_cfg(lang, **overrides),
        )


def run_format_check(output, source, fmt, lang="zh", **overrides):
    """Run the checker for a non-SRT fixture."""
    with tempfile.TemporaryDirectory() as tmp:
        out_path = Path(tmp) / ("out." + fmt)
        src_path = Path(tmp) / ("in." + fmt)
        out_path.write_text(output, encoding="utf-8")
        src_path.write_text(source, encoding="utf-8")
        return CHECKER.check(
            str(out_path),
            str(src_path),
            fmt,
            fmt,
            make_cfg(lang, **overrides),
        )


def srt(*blocks):
    """Build SRT text from (start_seconds, end_seconds, text) tuples."""
    def stamp(t):
        ms = int(round(t * 1000))
        h, ms = divmod(ms, 3600000)
        m, ms = divmod(ms, 60000)
        s, ms = divmod(ms, 1000)
        return "%02d:%02d:%02d,%03d" % (h, m, s, ms)

    return "\n".join(
        "%d\n%s --> %s\n%s\n" % (i + 1, stamp(a), stamp(b), text)
        for i, (a, b, text) in enumerate(blocks)
    )


class MinimalChinesePunctuationTests(unittest.TestCase):
    def check_warnings(self, text):
        _, errors, warnings, _ = run_check(srt((0.0, 4.0, text)))
        self.assertEqual([], errors)
        return warnings

    def test_warns_for_semicolon_that_needs_manual_review(self):
        warnings = self.check_warnings("先选择对象；再打开面板")
        self.assertTrue(any("internal '；'" in warning for warning in warnings))

    def test_allows_speaker_colon(self):
        warnings = self.check_warnings("John：需要更多几何体")
        self.assertFalse(any("internal" in warning for warning in warnings))

    def test_allows_comma_and_dunhao(self):
        warnings = self.check_warnings("先选择对象，再打开面板、确认设置")
        self.assertFalse(any("internal" in warning or "exclamation" in warning for warning in warnings))

    def test_allows_question_mark(self):
        warnings = self.check_warnings("这样可以吗？")
        self.assertFalse(any("internal" in warning or "exclamation" in warning for warning in warnings))

    def test_allows_terminal_question_mark(self):
        warnings = self.check_warnings("这样可以吗？")
        self.assertFalse(any("line ends" in warning for warning in warnings))

    def test_warns_for_terminal_exclamation_mark(self):
        warnings = self.check_warnings("设置完成！")
        self.assertTrue(any("exclamation mark" in warning or "line ends with '！'" in warning for warning in warnings))

    def test_warns_for_internal_exclamation_mark(self):
        warnings = self.check_warnings("注意！先选择对象")
        self.assertTrue(any("exclamation mark" in warning for warning in warnings))

    def test_warns_for_halfwidth_exclamation_mark(self):
        warnings = self.check_warnings("注意! 先选择对象")
        self.assertTrue(any("exclamation mark" in warning for warning in warnings))

    def test_warns_for_internal_period_that_splits_sentences(self):
        period_warnings = self.check_warnings("先选择对象。然后打开面板")
        self.assertTrue(any("internal '。'" in warning for warning in period_warnings))

    def test_does_not_warn_for_speaker_label(self):
        label_warnings = self.check_warnings("John：请打开面板")
        self.assertFalse(any("internal" in warning for warning in label_warnings))

    def test_warns_for_dangling_tail_word_before_continuation(self):
        _, errors, warnings, _ = run_check(srt(
            (0.0, 2.0, "做一只完全动态的鸟会很有趣 看看"),
            (2.0, 4.0, "能做些什么"),
        ))
        self.assertEqual([], errors)
        self.assertTrue(any("thought-unit boundary broken — ends on dangling" in w for w in warnings))

    def test_warns_for_dangling_head_particle_stranded(self):
        _, errors, warnings, _ = run_check(srt(
            (0.0, 2.0, "这是一个非常重要"),
            (2.0, 4.0, "的设置项目"),
        ))
        self.assertEqual([], errors)
        self.assertTrue(any(
            "thought-unit boundary broken — starts with stranded particle '的'" in w
            for w in warnings
        ))

    def test_warns_for_slash_separated_alternatives(self):
        warnings = self.check_warnings("可以使用 微调/蒸馏 两种方法")
        self.assertTrue(any("slash-separated alternative" in w for w in warnings))

    def test_allows_valid_slash_usages(self):
        # CLI command, fraction, and URL should not trigger slash-alternative warning
        warnings1 = self.check_warnings("输入 /compact 命令")
        self.assertFalse(any("slash-separated alternative" in w for w in warnings1))
        warnings2 = self.check_warnings("比例设为 1/2 大小")
        self.assertFalse(any("slash-separated alternative" in w for w in warnings2))
        warnings3 = self.check_warnings("访问 https://example.com 官网")
        self.assertFalse(any("slash-separated alternative" in w for w in warnings3))


class LengthFidelityTests(unittest.TestCase):
    """Padding and dropped payload leave the timeline intact, so they need their own check."""

    # Ten separated speech spans; each source line is ~40 raw Latin chars (cost ~20)
    # and each lean translation is ~10 full-width chars, so the file's norm is ~0.5.
    SOURCE_LINES = [
        "move the object up a little bit right now",
        "then add a bevel to the top edge here",
        "set the roughness value down to twenty",
        "drag the light closer to the model now",
        "open the material manager on the right",
        "select every polygon on the front face",
        "turn the substep count up to sixteen ok",
        "check the render preview window again",
        "delete the extra edge loop near the top",
        "save the project before you continue on",
    ]
    LEAN_LINES = [
        "把对象往上挪一点",
        "给上边加个倒角",
        "粗糙度降到 20",
        "把灯光往模型挪近",
        "打开右边材质管理器",
        "选中正面所有多边形",
        "子步调到 16",
        "再看看渲染预览窗",
        "删掉顶部多余循环边",
        "继续之前先保存工程",
    ]

    def build(self, out_lines):
        """One 4s span per line, separated by 1s of real silence."""
        blocks = lambda lines: srt(*[
            (i * 5.0, i * 5.0 + 4.0, text) for i, text in enumerate(lines)
        ])
        return blocks(out_lines), blocks(self.SOURCE_LINES)

    def run_fidelity(self, out_lines):
        output, source = self.build(out_lines)
        _, errors, warnings, notes = run_check(output, source)
        self.assertEqual([], errors)
        return warnings, notes

    def test_lean_translation_raises_no_fidelity_warning(self):
        warnings, notes = self.run_fidelity(self.LEAN_LINES)
        self.assertFalse([w for w in warnings if "padding" in w or "dropped payload" in w],
                         "a uniformly lean file must not be flagged")
        self.assertTrue(any("length fidelity: median" in n for n in notes))

    def test_padded_span_is_flagged(self):
        lines = list(self.LEAN_LINES)
        lines[3] = "接下来我们需要把这个灯光稍微向着模型的方向移动得更近一些就可以了"
        warnings, _ = self.run_fidelity(lines)
        padded = [w for w in warnings if "check for padding" in w]
        self.assertEqual(1, len(padded), "exactly the padded span should be flagged: %s" % warnings)
        self.assertIn("00:00:15,000", padded[0])

    def test_dropped_span_is_flagged(self):
        lines = list(self.LEAN_LINES)
        lines[6] = "调一下"
        warnings, _ = self.run_fidelity(lines)
        dropped = [w for w in warnings if "check for dropped payload" in w]
        self.assertEqual(1, len(dropped), "exactly the gutted span should be flagged: %s" % warnings)
        self.assertIn("00:00:30,000", dropped[0])

    def test_short_file_is_not_judged(self):
        """Below min_spans there is no reliable median, so the check stays silent."""
        out = srt((0.0, 4.0, "接下来我们需要把这个灯光稍微向着模型的方向移动得更近一些"))
        src = srt((0.0, 4.0, "move the light closer"))
        _, errors, warnings, notes = run_check(out, src)
        self.assertEqual([], errors)
        self.assertFalse([w for w in warnings if "check for padding" in w])
        self.assertFalse([n for n in notes if "length fidelity" in n])

    def test_bilingual_output_shifts_the_norm_not_the_verdict(self):
        """Every span doubles in length, so the median absorbs it and nothing is flagged."""
        lines = ["%s / %s" % (zh, en)
                 for zh, en in zip(self.LEAN_LINES, self.SOURCE_LINES)]
        warnings, _ = self.run_fidelity(lines)
        self.assertFalse([w for w in warnings if "check for padding" in w],
                         "uniform doubling is a house style, not a fidelity defect")


class LanguageProfileTests(unittest.TestCase):
    def test_language_config_contains_required_fields(self):
        profiles = CHECKER.load_language_profiles(CHECKER.DEFAULT_LANGUAGE_CONFIG)
        required = {"target_language", "counting", "final_punctuation", "max_cps", "max_width"}
        for code in ("zh", "zh-hant", "ja", "ko", "en", "es", "ru", "ar", "th", "vi"):
            self.assertTrue(required.issubset(profiles[code]), code)

    def test_language_region_code_resolves_to_base_profile(self):
        profile = CHECKER.get_language_profile("en-US")
        self.assertEqual("English", profile["target_language"])
        self.assertEqual(20.0, profile["max_cps"])

    def test_region_code_inherits_base_language_style_rules(self):
        warnings = run_check(srt((0.0, 4.0, "设置完成！")), lang="zh-CN")[2]
        self.assertTrue(any("exclamation mark" in warning for warning in warnings), warnings)

    def test_unknown_language_uses_explicit_default_profile(self):
        profile = CHECKER.get_language_profile("xx-unknown")
        self.assertEqual("default", profile["target_language"])

    def test_zh_profile_max_cps_calibrated(self):
        profile = CHECKER.get_language_profile("zh")
        self.assertEqual(4.5, profile["max_cps"])

    def test_short_duration_density_overload_warning(self):
        # 16 characters in 2.0s is 8.0 chars/s on a short block -> must flag short-duration overload
        warnings = run_check(srt((0.0, 2.0, "这极为方便也可以直接把节点拖进去")), lang="zh")[2]
        self.assertTrue(any("short-duration overload" in w for w in warnings), warnings)


class StructuralContractTests(unittest.TestCase):
    def test_non_utf8_output_is_rejected(self):
        output = srt((0.0, 2.0, "你好")).encode("gb18030")
        _, errors, _, _ = run_check_bytes(output)
        self.assertTrue(any("UTF-8" in error for error in errors), errors)

    def test_invalid_srt_index_is_rejected(self):
        output = "x\n00:00:00,000 --> 00:00:02,000\nhello\n"
        _, errors, _, _ = run_check(output)
        self.assertTrue(any("index" in error for error in errors), errors)

    def test_invalid_srt_clock_is_rejected(self):
        output = "1\n00:60:00,000 --> 00:00:02,000\nhello\n"
        _, errors, _, _ = run_check(output)
        self.assertTrue(any("timestamp" in error for error in errors), errors)

    def test_out_of_order_blocks_are_rejected(self):
        output = srt((3.0, 4.0, "later"), (0.0, 1.0, "earlier"))
        _, errors, _, _ = run_check(output)
        self.assertTrue(any("order" in error for error in errors), errors)

    def test_strict_mode_rejects_merging_blocks(self):
        source = srt((0.0, 2.0, "a"), (2.0, 4.0, "b"))
        output = srt((0.0, 4.0, "ab"))
        _, errors, _, _ = run_check(output, source, strict=True)
        self.assertTrue(any("strict" in error for error in errors), errors)

    def test_strict_mode_rejects_changed_edges(self):
        source = srt((0.0, 2.0, "a"), (2.0, 4.0, "b"))
        output = srt((0.0, 1.5, "a"), (1.5, 4.0, "b"))
        _, errors, _, _ = run_check(output, source, strict=True)
        self.assertTrue(any("strict" in error for error in errors), errors)

    def test_new_gap_inside_continuous_source_span_is_rejected(self):
        source = srt((0.0, 2.0, "a"), (2.0, 4.0, "b"))
        output = srt((0.0, 1.0, "a"), (1.2, 4.0, "b"))
        _, errors, _, _ = run_check(output, source)
        self.assertTrue(any("uncovered" in error for error in errors), errors)

    def test_vtt_static_structure_and_settings_are_required(self):
        source = (
            "WEBVTT\n\n"
            "NOTE keep this note\n\n"
            "cue-1\n"
            "00:00.000 --> 00:02.000 position:50% align:center\n"
            "Hello\n"
        )
        output = "WEBVTT\n\ncue-1\n00:00.000 --> 00:02.000\n你好\n"
        _, errors, _, _ = run_format_check(output, source, "vtt")
        self.assertTrue(any("VTT" in error or "structure" in error for error in errors), errors)

    def test_vtt_duplicate_cue_identifier_is_rejected(self):
        source = (
            "WEBVTT\n\n"
            "cue-1\n00:00.000 --> 00:02.000\nHello\n\n"
            "cue-2\n00:02.000 --> 00:04.000\nAgain\n"
        )
        output = (
            "WEBVTT\n\n"
            "cue-1\n00:00.000 --> 00:02.000\n你好\n\n"
            "cue-1\n00:02.000 --> 00:04.000\n再来一次\n"
        )
        _, errors, _, _ = run_format_check(output, source, "vtt")
        self.assertTrue(any("identifier" in error for error in errors), errors)

    def test_vtt_merged_cue_must_keep_first_settings(self):
        source = (
            "WEBVTT\n\n"
            "cue-1\n00:00.000 --> 00:02.000 position:10%\nFirst\n\n"
            "cue-2\n00:02.000 --> 00:04.000 position:90%\nSecond\n"
        )
        output = "WEBVTT\n\ncue-1\n00:00.000 --> 00:04.000 position:90%\n合并\n"
        _, errors, _, _ = run_format_check(output, source, "vtt")
        self.assertTrue(any("settings" in error for error in errors), errors)

    def test_ass_non_dialogue_structure_and_markup_are_required(self):
        source = (
            "[Script Info]\nScriptType: v4.00+\n\n"
            "[V4+ Styles]\n"
            "Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\n"
            "Style: Default,Arial,20,&H00FFFFFF,&H000000FF,&H00000000,&H00000000,0,0,0,0,100,100,0,0,1,2,2,2,10,10,10,1\n\n"
            "[Events]\n"
            "Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n"
            "Dialogue: 0,0:00:00.00,0:00:02.00,Default,,0,0,0,,{\\i1}Hello{\\i0}\n"
        )
        output = (
            "[Script Info]\nScriptType: v4.00+\n\n"
            "[Events]\n"
            "Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n"
            "Dialogue: 0,0:00:00.00,0:00:02.00,Default,,0,0,0,,你好\n"
        )
        _, errors, _, _ = run_format_check(output, source, "ass")
        self.assertTrue(any("ASS" in error or "structure" in error for error in errors), errors)

    def test_ass_changed_non_text_field_is_rejected_in_normal_mode(self):
        source = (
            "[Events]\n"
            "Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n"
            "Dialogue: 0,0:00:00.00,0:00:02.00,Default,,0,0,0,,Hello\n"
        )
        output = (
            "[Events]\n"
            "Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n"
            "Dialogue: 0,0:00:00.00,0:00:02.00,Other,,0,0,0,,你好\n"
        )
        _, errors, _, _ = run_format_check(output, source, "ass")
        self.assertTrue(any("non-Text" in error for error in errors), errors)

    def test_ass_merged_event_must_keep_first_non_text_fields(self):
        source = (
            "[Events]\n"
            "Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n"
            "Dialogue: 0,0:00:00.00,0:00:02.00,Default,,0,0,0,,First\n"
            "Dialogue: 1,0:00:02.00,0:00:04.00,Other,,1,2,3,fx,Second\n"
        )
        output = (
            "[Events]\n"
            "Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n"
            "Dialogue: 1,0:00:00.00,0:00:04.00,Other,,1,2,3,fx,合并\n"
        )
        _, errors, _, _ = run_format_check(output, source, "ass")
        self.assertTrue(any("non-Text" in error for error in errors), errors)


class CrossGapMergeTests(unittest.TestCase):
    """A source gap is a pause proxy, not proof of audible silence. Merging across
    one is the translator's judgement call: the checker reports the crossing as a
    warning instead of rejecting it, while a block floating in no-speech territory
    is still an error."""

    def test_merge_across_source_gap_is_a_warning_not_an_error(self):
        source = srt((0.0, 2.0, "hello"), (3.0, 5.0, "world"))  # 1s gap -> two spans
        output = srt((0.0, 5.0, "合并后的整句"))
        _, errors, warnings, _ = run_check(output, source)
        self.assertEqual([], errors)
        self.assertTrue(any("crosses a source gap of 1.00s" in w for w in warnings), warnings)

    def test_block_entirely_inside_source_gap_is_an_error(self):
        source = srt((0.0, 2.0, "hello"), (3.0, 5.0, "world"))
        output = srt((0.0, 2.0, "你好"), (2.2, 2.8, "孤块"), (3.0, 5.0, "世界"))
        _, errors, _, _ = run_check(output, source)
        self.assertTrue(any("outside source speech" in e for e in errors), errors)

    def test_block_lingering_before_source_speech_warns(self):
        source = srt((2.0, 4.0, "hello"))
        output = srt((0.0, 4.0, "提前出现的字幕"))
        _, errors, warnings, _ = run_check(output, source)
        self.assertEqual([], errors)
        self.assertTrue(any("lingers 2.00s before source speech" in w for w in warnings), warnings)

    def test_cross_gap_merge_over_max_duration_still_warns(self):
        source = srt((0.0, 3.0, "hello"), (10.0, 12.0, "world"))  # 7s gap
        output = srt((0.0, 12.0, "横跨很长间隔的合并"))
        _, errors, warnings, _ = run_check(output, source)
        self.assertEqual([], errors)
        self.assertTrue(any("crosses a source gap of 7.00s" in w for w in warnings), warnings)
        self.assertTrue(any("longer than 7.0s" in w for w in warnings), warnings)

    def test_bridging_merge_is_not_double_counted_by_fidelity(self):
        """A block merged across two spans is credited proportionally, so neither
        span looks padded by text that belongs to its neighbour."""
        lean = LengthFidelityTests.LEAN_LINES
        source = srt(*[
            (i * 5.0, i * 5.0 + 4.0, text)
            for i, text in enumerate(LengthFidelityTests.SOURCE_LINES)
        ])
        blocks = [
            (i * 5.0, i * 5.0 + 4.0, text) for i, text in enumerate(lean)
        ]
        # replace the blocks for spans 3 and 4 with one bridging merge (15.0 -> 24.0)
        blocks = blocks[:3] + [(15.0, 24.0, lean[3] + lean[4])] + blocks[5:]
        output = srt(*blocks)
        _, errors, warnings, _ = run_check(output, source)
        self.assertEqual([], errors)
        self.assertFalse([w for w in warnings if "check for padding" in w], warnings)
        self.assertFalse([w for w in warnings if "check for dropped payload" in w], warnings)


    def test_bracketed_original_annotation_excluded_from_budget(self):
        """Bracketed dual-language term on first appearance (e.g. 基础形体（Blocking）)
        is an auxiliary reference note and must not count toward the reading budget."""
        # 4 CJK chars (4 DU), bracketed (Blocking) costs 0 DU
        w1 = CHECKER.display_width("基础形体（Blocking）", "cjk")
        self.assertEqual(4.0, w1)
        w2 = CHECKER.display_width("基础形体(Blocking)", "cjk")
        self.assertEqual(4.0, w2)
        # 5 CJK chars (5 DU), bracketed (Pulse Modifier) costs 0 DU
        w3 = CHECKER.display_width("脉冲修改器（Pulse Modifier）", "cjk")
        self.assertEqual(5.0, w3)

    def test_bracketed_original_annotation_no_false_short_overload(self):
        """A 1.8s cue with 8 Chinese chars + bracketed English original must not
        trigger a short-duration overload warning."""
        output = srt((0.0, 1.8, "我们先做基础形体（Blocking）"))
        # Chinese core is "我们先做基础形体" (8 DU), 8 DU / 1.8s = 4.44 cps <= 4.5 cps
        _, errors, warnings, _ = run_check(output, lang="zh")
        self.assertEqual([], errors)
        self.assertFalse(any("short-duration overload" in w for w in warnings), warnings)
        self.assertFalse(any("chars/s" in w for w in warnings), warnings)


if __name__ == "__main__":
    unittest.main()
