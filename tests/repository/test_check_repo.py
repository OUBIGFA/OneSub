"""Exercise the published checker against real Git indexes, including staged data."""

import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
CHECKER = ROOT / "tools" / "check_repo.py"


class RepositoryCheckTests(unittest.TestCase):
    def setUp(self):
        # Keep all test repositories in the ignored task area for explicit cleanup.
        scratch = Path(os.environ.get("ONESUB_TEST_TEMP", ROOT / "_temp" / "repository-tests"))
        scratch.mkdir(parents=True, exist_ok=True)
        self.repo = Path(tempfile.mkdtemp(prefix="case-", dir=scratch))
        self.git("init", "-q")
        self.git("config", "core.autocrlf", "false")
        self.write(".gitignore", (ROOT / ".gitignore").read_text(encoding="utf-8"))
        self.write("README.md", "# Example\n\n[Guide](docs/guide.md)\n")
        self.write("docs/guide.md", "# Guide\n")
        self.git("add", ".")

    def git(self, *args, data=None):
        result = subprocess.run(
            ["git", "-C", str(self.repo), *args], input=data, capture_output=True,
            check=True,
        )
        return result.stdout

    def write(self, path, content):
        target = self.repo / path
        target.parent.mkdir(parents=True, exist_ok=True)
        if isinstance(content, bytes):
            target.write_bytes(content)
        else:
            target.write_text(content, encoding="utf-8", newline="\n")

    def run_check(self):
        return subprocess.run(
            [sys.executable, "-X", "utf8", str(CHECKER)], cwd=self.repo,
            capture_output=True, text=True, encoding="utf-8",
        )

    def assert_rejected(self, marker):
        result = self.run_check()
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertIn(marker, result.stdout + result.stderr)

    def test_clean_index_with_unicode_and_space_links_passes(self):
        self.write("docs/中文 名称.md", "# 字幕\n")
        self.write("README.md", "[Guide](docs/%E4%B8%AD%E6%96%87%20%E5%90%8D%E7%A7%B0.md)\n")
        self.git("add", ".")
        self.assertEqual(self.run_check().returncode, 0)

    def test_force_added_nested_scratch_is_rejected(self):
        self.write("docs/_temp/private.txt", "private\n")
        self.git("add", "-f", "docs/_temp/private.txt")
        self.assert_rejected("ignored path")

    def test_later_unignore_cannot_expose_nested_scratch(self):
        ignore = (self.repo / ".gitignore").read_text(encoding="utf-8")
        self.write(".gitignore", ignore + "!docs/_temp/\n!docs/_temp/**\n")
        self.git("add", ".gitignore")
        self.assert_rejected("ignore boundary")

    def test_unstaged_ignore_change_does_not_hide_bad_index(self):
        self.write(".gitignore", "_temp/\n")
        self.assert_rejected(".gitignore differs")

    def test_link_to_untracked_file_is_rejected(self):
        self.write("README.md", "[Missing](docs/local-only.md)\n")
        self.write("docs/local-only.md", "local\n")
        self.git("add", "README.md")
        self.assert_rejected("missing tracked link")

    def test_staged_secret_is_detected_even_after_worktree_cleanup(self):
        secret = "ghp_" + "A" * 36  # Synthetic token, assembled to avoid a literal secret fixture.
        self.write("config.txt", secret + "\n")
        self.git("add", "config.txt")
        self.write("config.txt", "redacted\n")
        self.assert_rejected("credential pattern")
        self.assertNotIn(secret, self.run_check().stdout)

    def test_invalid_utf8_and_json_are_rejected(self):
        for content, marker in [(b"\xff", "UTF-8"), (b"{bad}\n", "invalid JSON")]:
            with self.subTest(marker=marker):
                self.write("data.json", content)
                self.git("add", "data.json")
                self.assert_rejected(marker)

    def test_only_registered_branding_binary_is_allowed(self):
        logo = (ROOT / "apps/desktop/branding/logo.png").read_bytes()
        self.write("apps/desktop/branding/logo.png", logo)
        self.git("add", ".")
        self.assertEqual(self.run_check().returncode, 0)
        self.write("apps/desktop/branding/logo.png", logo + b"modified")
        self.git("add", ".")
        self.assert_rejected("branding asset hash mismatch")

    def test_large_staged_file_is_rejected(self):
        self.write("large.txt", b"x" * (1024 * 1024 + 1))
        self.git("add", "large.txt")
        self.assert_rejected("1 MiB")

    def test_private_machine_path_is_rejected(self):
        self.write("notes.md", "See " + "E:" + "\\" + "private\\media.mp4\n")
        self.git("add", "notes.md")
        self.assert_rejected("machine path")

    def test_symlink_and_case_conflict_in_index_are_rejected(self):
        blob = self.git("hash-object", "-w", "--stdin", data=b"README.md\n").decode().strip()
        self.git("update-index", "--add", "--cacheinfo", f"120000,{blob},link.md")
        self.assert_rejected("unsupported mode")
        self.git("update-index", "--add", "--cacheinfo", f"100644,{blob},readme.md")
        self.assert_rejected("case collision")


if __name__ == "__main__":
    unittest.main()
