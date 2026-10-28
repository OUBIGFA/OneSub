"""Check the staged Git snapshot before publishing; standard library only.

This is a small repository guard, not a complete secret scanner or Markdown parser.
It never prints matched credential contents. Application tests remain separate.
"""

import hashlib
import json
from pathlib import Path, PurePosixPath
import posixpath
import re
import subprocess
import sys
from urllib.parse import unquote, urlsplit


MAX_BYTES = 1024 * 1024
# Only the maintainer-supplied branding source may be binary. A fixed digest
# prevents this exception from becoming a general asset/secret bypass.
APPROVED_BINARY = {
    "apps/desktop/branding/logo.png": "184fe121fc27cfd702a7090e517e1c9f4d973406a8d8f78113305c0fedfa2b9f",
}
CREDENTIAL = re.compile(
    r"(?:gh[pousr]_[A-Za-z0-9]{36,}|github_pat_[A-Za-z0-9_]{50,}"
    r"|AKIA[0-9A-Z]{16}|sk-(?:proj-)?[A-Za-z0-9_-]{32,}"
    r"|-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----)"
)
MACHINE_PATH = re.compile(r"(?<![A-Za-z0-9])[A-Za-z]:[\\/](?![\\/])|/(?:Users|home)/[^/\s]+/")
LINK = re.compile(r"!?\[[^\]\n]*\]\(([^)\n]+)\)")
RESERVED = re.compile(r"^(?:con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\.|$)", re.I)


def git(*args):
    """Use Git's index and blob APIs so unstaged cleanup cannot hide staged data."""
    result = subprocess.run(["git", *args], capture_output=True)
    if result.returncode:
        raise RuntimeError("Git command failed: " + " ".join(args[:2]))
    return result.stdout


def ignored(path):
    # Ignore a developer's global excludes; project rules must work equally in CI.
    result = subprocess.run(
        ["git", "-c", "core.excludesFile=", "check-ignore", "--no-index", "-q", "--", path],
        capture_output=True,
    )
    if result.returncode not in (0, 1):
        raise RuntimeError("Git ignore check failed")
    return result.returncode == 0


def check():
    root = Path(git("rev-parse", "--show-toplevel").decode("utf-8").strip())
    # Require the root so Git ignore probes and relative paths have one meaning.
    if Path.cwd().resolve() != root.resolve():
        raise RuntimeError("Run the checker from the repository root")
    entries = []
    for record in git("ls-files", "--stage", "-z").split(b"\0"):
        if record:
            metadata, raw_path = record.split(b"\t", 1)
            mode, blob, stage = metadata.decode("ascii").split()
            entries.append((raw_path.decode("utf-8"), mode, blob, stage))
    errors = []
    texts = {}
    case_names = {}
    paths = {entry[0] for entry in entries}
    targets = set(paths)
    if not entries:
        errors.append("empty Git index: stage the intended files first")

    for path, mode, blob, stage in entries:
        parts = PurePosixPath(path).parts
        for offset, part in enumerate(parts, 1):
            prefix = "/".join(parts[:offset])
            targets.add(prefix)
            previous = case_names.setdefault(prefix.casefold(), prefix)
            if previous != prefix:
                errors.append(f"{path}: case collision with {previous}")
            if RESERVED.match(part) or part.endswith((" ", ".")) or re.search(r'[<>:"\\|?*\x00-\x1f]', part):
                errors.append(f"{path}: Windows-incompatible path")
        if mode not in ("100644", "100755") or stage != "0":
            errors.append(f"{path}: unsupported mode or unresolved index stage")
            continue
        if "_temp" in [part.casefold() for part in parts] or ignored(path):
            errors.append(f"{path}: ignored path is tracked")
        size = int(git("cat-file", "-s", blob))
        if size > MAX_BYTES:
            errors.append(f"{path}: exceeds 1 MiB file limit")
            continue
        data = git("cat-file", "blob", blob)
        if path in APPROVED_BINARY:
            if hashlib.sha256(data).hexdigest() != APPROVED_BINARY[path]:
                errors.append(f"{path}: branding asset hash mismatch")
            continue
        try:
            text = data.decode("utf-8")
        except UnicodeDecodeError:
            errors.append(f"{path}: invalid UTF-8 or binary file")
            continue
        if "\x00" in text or text.startswith("\ufeff") or "\r" in text:
            errors.append(f"{path}: expected UTF-8 without BOM, NUL or CR")
        texts[path] = text
        if CREDENTIAL.search(text):
            errors.append(f"{path}: credential pattern detected (value withheld)")
        if MACHINE_PATH.search(text):
            errors.append(f"{path}: private machine path detected")
        if path.endswith(".json"):
            try:
                json.loads(text)
            except (ValueError, RecursionError):
                errors.append(f"{path}: invalid JSON")

    # check-ignore uses the worktree. Reject divergent ignore files before trusting it.
    ignore_files_match = True
    for path in paths:
        if PurePosixPath(path).name == ".gitignore":
            if path not in texts or not (root / path).is_file() or (root / path).read_text(encoding="utf-8") != texts[path]:
                errors.append(f"{path} differs from staged .gitignore")
                ignore_files_match = False
    rules = [line.strip() for line in texts.get(".gitignore", "").splitlines() if line.strip() and not line.startswith("#")]
    if not rules or rules[-1] != "_temp/":
        errors.append("ignore boundary: the last active root rule must be _temp/")
    if ignore_files_match:
        for probe in ("_temp/probe.txt", "docs/_temp/probe.txt", "a/b/_temp/probe.txt", ".env", ".local/private.txt", "node_modules/package/index.js"):
            if not ignored(probe):
                errors.append(f"ignore boundary: {probe} is exposed")

    for path, text in texts.items():
        if not path.endswith(".md"):
            continue
        for raw in LINK.findall(text):
            destination = raw[1:raw.index(">")] if raw.startswith("<") and ">" in raw else raw.split()[0]
            url = urlsplit(destination)
            if url.scheme or url.netloc or not url.path:
                continue
            decoded = unquote(url.path)
            resolved = posixpath.normpath(posixpath.join(posixpath.dirname(path), decoded))
            if decoded.startswith("/") or resolved == ".." or resolved.startswith("../") or resolved not in targets:
                errors.append(f"{path}: missing tracked link: {destination}")

    if errors:
        print("Repository check failed:")
        for error in sorted(set(errors)):
            print("- " + error)
        return 1
    print(f"Repository check passed: {len(paths)} staged files.")
    return 0


if __name__ == "__main__":
    try:
        sys.exit(check())
    except (OSError, UnicodeError, RuntimeError, ValueError) as exc:
        print(f"Repository check could not complete: {exc}", file=sys.stderr)
        sys.exit(1)
