"""Vendor the authorized subtitle skill from an exact archive and Git tree.

Only this skill subtree is copied. Git blob hashes are verified before each file
is written; no executable, symlink or unrelated skill is imported.
"""
import argparse
import hashlib
import json
from pathlib import Path, PurePosixPath
import zipfile

REVISION = "d37492b88383c63ccf923c26483709122940de20"
ARCHIVE_SHA256 = "5c056fbd11e013d03bc52d6a2b910096e677be299b6014dc02f1868aecc2dd7c"


def vendor(archive: Path, tree_path: Path, destination: Path):
    if hashlib.sha256(archive.read_bytes()).hexdigest() != ARCHIVE_SHA256:
        raise ValueError("Source archive checksum differs")
    tree = json.loads(tree_path.read_text(encoding="utf-8"))
    expected = {row["path"]: row["sha"] for row in tree["tree"]
                if row["type"] == "blob" and row["path"].startswith("srt-subtitle-translator/")}
    if tree.get("truncated") or not expected:
        raise ValueError("Incomplete source tree")
    files = []
    with zipfile.ZipFile(archive) as source:
        prefix = f"skills-{REVISION}/"
        for path, blob in expected.items():
            relative = PurePosixPath(path).relative_to("srt-subtitle-translator")
            if relative.is_absolute() or ".." in relative.parts:
                raise ValueError("Invalid source path")
            data = source.read(prefix + path)
            actual = hashlib.sha1(b"blob " + str(len(data)).encode() + b"\0" + data).hexdigest()
            if actual != blob:
                raise ValueError(f"Git blob mismatch: {path}")
            target = destination.joinpath(*relative.parts)
            target.parent.mkdir(parents=True, exist_ok=True)
            if target.exists() and target.read_bytes() != data:
                raise ValueError(f"Refusing to overwrite changed source: {target}")
            target.write_bytes(data)
            files.append({"path": path, "destination": target.as_posix(), "gitBlob": blob,
                          "sha256": hashlib.sha256(data).hexdigest(), "bytes": len(data)})
    return {"repository": "https://github.com/OUBIGFA/skills", "revision": REVISION,
            "archiveSha256": ARCHIVE_SHA256, "authorization": "third_party/ports/subtitle-skill/AUTHORIZATION.md", "files": files}


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("archive", type=Path)
    parser.add_argument("tree", type=Path)
    parser.add_argument("destination", type=Path)
    parser.add_argument("manifest", type=Path)
    args = parser.parse_args()
    result = vendor(args.archive, args.tree, args.destination)
    args.manifest.parent.mkdir(parents=True, exist_ok=True)
    args.manifest.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n", encoding="utf-8", newline="\n")
    print(f"Verified and imported {len(result['files'])} skill files")
