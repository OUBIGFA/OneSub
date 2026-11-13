"""Recreate the pinned region-batch port without editing the upstream source cache."""

import hashlib
import json
from pathlib import Path, PurePosixPath
import shutil
import subprocess
import zipfile


ROOT = Path(__file__).resolve().parents[1]


def sha256(path):
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def prepare():
    pin = json.loads((ROOT / "third_party/manifests/oneasr.json").read_text(encoding="utf-8"))
    patch = ROOT / pin["patch"]
    if sha256(patch) != pin["patchSha256"]:
        raise ValueError("Review and register the changed OneAsr integration patch")
    archive = ROOT / ".local/downloads" / pin["sourceArchive"]["file"]
    if not archive.is_file() or sha256(archive) != pin["sourceArchive"]["sha256"]:
        raise ValueError("Provide the verified source archive: " + pin["sourceArchive"]["url"])
    source = ROOT / "third_party/sources" / ("oneasr-" + pin["revision"])
    if not source.is_dir():
        with zipfile.ZipFile(archive) as bundle:
            for member in bundle.infolist():
                parts = PurePosixPath(member.filename).parts[1:]
                if not parts:
                    continue
                if ".." in parts:
                    raise ValueError("Source archive contains an unsafe path")
                target = source.joinpath(*parts)
                if member.is_dir():
                    target.mkdir(parents=True, exist_ok=True)
                else:
                    target.parent.mkdir(parents=True, exist_ok=True)
                    target.write_bytes(bundle.read(member))
    for name, expected in pin["sourceHashes"].items():
        if sha256(source / name) != expected:
            raise ValueError("Pinned OneAsr source changed: " + name)
    output = ROOT / ".local" / ("oneasr-batch-" + pin["revision"])
    matches = all((output / name).is_file() and sha256(output / name) == expected
                  for name, expected in pin["patchedHashes"].items())
    if not matches:
        shutil.copytree(source, output, dirs_exist_ok=True,
                        ignore=shutil.ignore_patterns("AGENTS.md", ".github", "docs"))
        # Added files have no upstream copy to overwrite during cache recovery.
        for name in pin["addedFiles"]:
            (output / name).unlink(missing_ok=True)
        # Apply from the workspace root: Git otherwise skips paths outside
        # the current subdirectory even when `git apply` reports success.
        command = ["git", "-c", "core.autocrlf=false", "apply",
                   "--directory=" + output.relative_to(ROOT).as_posix()]
        subprocess.run([*command, "--check", str(patch)], cwd=ROOT, check=True)
        subprocess.run([*command, str(patch)], cwd=ROOT, check=True)
        # Upstream checks out Rust as CRLF. Keep the registered port in LF
        # regardless of those attributes or the developer's Git configuration.
        for name in pin["patchedHashes"]:
            target = output / name
            target.write_bytes(target.read_bytes().replace(b"\r\n", b"\n"))
    for name, expected in pin["patchedHashes"].items():
        if sha256(output / name) != expected:
            raise ValueError("Prepared OneAsr patch differs from its registration: " + name)
    fingerprint = hashlib.sha256(json.dumps(pin, sort_keys=True).encode("utf-8")).hexdigest()
    return {"directory": str(output), "fingerprint": fingerprint}


if __name__ == "__main__":
    print(json.dumps(prepare()))
