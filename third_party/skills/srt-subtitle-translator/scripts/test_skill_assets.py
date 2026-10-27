# -*- coding: utf-8 -*-
"""Asset integrity checks, not wording snapshots or model-quality benchmarks."""
import json
import re
import unittest
from pathlib import Path

from test_check_subtitle import CHECKER


ROOT = Path(__file__).resolve().parents[1]
LINK_RE = re.compile(r"\[[^\]]*\]\(([^)]+)\)")


def local_links(path):
    """Resolve maintained Markdown links relative to the containing document."""
    for target in LINK_RE.findall(path.read_text(encoding="utf-8")):
        target = target.split("#", 1)[0]
        if not target or "://" in target or target.startswith("mailto:"):
            continue
        yield (path.parent / target).resolve()


class SkillAssetTests(unittest.TestCase):
    def test_references_are_reachable_and_links_stay_inside_the_skill(self):
        pending, visited = [ROOT / "SKILL.md"], set()
        while pending:
            document = pending.pop()
            if document in visited:
                continue
            visited.add(document)
            for target in local_links(document):
                self.assertTrue(target.is_relative_to(ROOT), (document, target))
                self.assertTrue(target.is_file(), (document, target))
                if target.suffix == ".md":
                    pending.append(target)
        self.assertTrue(set((ROOT / "references").glob("*.md")).issubset(visited), visited)

    def test_documented_script_paths_exist(self):
        documents = [ROOT / "SKILL.md", *(ROOT / "references").glob("*.md")]
        mentioned = set()
        for document in documents:
            mentioned.update(re.findall(r"scripts/[A-Za-z0-9_-]+\.py", document.read_text(encoding="utf-8")))
        self.assertTrue(mentioned)
        for script in mentioned:
            self.assertTrue((ROOT / script).is_file(), script)

    def test_manifest_has_unique_cases_with_real_input_paths(self):
        manifest = json.loads((ROOT / "evals" / "evals.json").read_text(encoding="utf-8"))
        self.assertEqual("srt-subtitle-translator", manifest["skill_name"])
        self.assertTrue(manifest["evals"])
        ids, referenced = set(), set()
        for case in manifest["evals"]:
            self.assertIs(type(case["id"]), int)
            self.assertNotIn(case["id"], ids)
            ids.add(case["id"])
            for field in ("prompt", "expected_output"):
                self.assertIsInstance(case[field], str)
                self.assertTrue(case[field].strip(), (case["id"], field))
            self.assertIsInstance(case.get("files", []), list)
            for filename in case.get("files", []):
                path = (ROOT / filename).resolve()
                self.assertTrue(path.is_relative_to(ROOT), filename)
                self.assertTrue(path.is_file(), (case["id"], filename))
                referenced.add(path)
        fixtures = {path.resolve() for path in (ROOT / "evals" / "fixtures").iterdir() if path.is_file()}
        self.assertEqual(fixtures, referenced, "Remove orphan fixtures or bind them to a real case")

    def test_subtitle_fixtures_parse_as_their_declared_format(self):
        for path in (ROOT / "evals" / "fixtures").iterdir():
            with self.subTest(path=path.name):
                fmt = CHECKER.EXT_FORMATS[path.suffix]
                blocks, errors = CHECKER.parse_file(path, fmt, require_utf8=True)
                self.assertEqual([], errors)
                self.assertTrue(blocks)

    def test_language_profiles_load_using_the_production_schema(self):
        profiles = CHECKER.load_language_profiles(ROOT / "config" / "language_profiles.json")
        self.assertIn("default", profiles)
        for code, profile in profiles.items():
            self.assertEqual(profile, CHECKER.get_language_profile(code, profiles))


if __name__ == "__main__":
    unittest.main()
