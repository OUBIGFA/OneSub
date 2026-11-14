"""Give MyGo-signed archives a monotonic build identity and exact release URL.

MyGo generates and signs the archive. Only its unsigned feed metadata changes:
the numeric fourth version component and the immutable release URL. Product
version, installer identity, signed archive bytes and signature stay intact.
"""

import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import tempfile


MANIFEST_NAME = "update-windows-amd64.json"


def finalize_update(config_path: Path, package: Path, build: str, output: Path, release_tag: str | None = None):
    """Validate the generator output and atomically replace fixed local artifacts."""
    if re.fullmatch(r"[0-9]{14}", build) is None:
        raise ValueError("Build number must use yyyyMMddHHmmss")
    config = json.loads(config_path.read_text(encoding="utf-8"))
    version, repository = config["version"], config["updates"]["github"]
    if release_tag is not None and release_tag != version:
        raise ValueError("Public release tag must equal the product version")
    archive_name = f"onesub-{version}-windows-amd64.tar.gz"
    archive = package / archive_name
    manifest = json.loads((package / MANIFEST_NAME).read_text(encoding="utf-8"))
    expected_url = f"https://github.com/{repository}/releases/download/v{version}/{archive_name}"
    if manifest["version"] != version or manifest["url"] != expected_url:
        raise ValueError("Signed archive manifest does not match the product configuration")
    if manifest.get("deltas"):
        # Same-product-version local builds do not use MyGo's product-version
        # deltas. Preserve the verified full archive rather than a wrong base.
        raise ValueError("Internal-build updates require full archives without deltas")
    signature = base64.b64decode(manifest["signature"], validate=True)
    if len(signature) != 64:
        raise ValueError("MyGo did not produce an Ed25519-signed update")
    size = archive.stat().st_size
    if size <= 0 or size != manifest["size"]:
        raise ValueError("Signed update archive size differs from its manifest")

    manifest["version"] = f"{version}.{build}"
    tag = release_tag if release_tag is not None else f"build-{build}"
    manifest["url"] = f"https://github.com/{repository}/releases/download/{tag}/{archive_name}"
    with archive.open("rb") as file:
        digest = hashlib.file_digest(file, "sha256").hexdigest()
    output.mkdir(parents=True, exist_ok=True)
    # Prepare every file before replacing earlier verified local artifacts.
    # Do not edit the package's generated manifest or read a signing secret.
    pending = []
    try:
        for name, content in (
            (archive_name, None),
            (archive_name + ".sha256", f"{digest}  {archive_name}\n".encode("utf-8")),
            (MANIFEST_NAME, (json.dumps(manifest, ensure_ascii=False, indent=2) + "\n").encode("utf-8")),
        ):
            with tempfile.NamedTemporaryFile(dir=output, prefix=name + ".", suffix=".partial", delete=False) as file:
                path = Path(file.name)
                pending.append((path, output / name))
                if content is None:
                    with archive.open("rb") as source:
                        shutil.copyfileobj(source, file)
                else:
                    file.write(content)
                file.flush()
                os.fsync(file.fileno())
        for source, destination in pending:
            os.replace(source, destination)
    finally:
        for source, _ in pending:
            source.unlink(missing_ok=True)
    return manifest


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--config", required=True, type=Path)
    parser.add_argument("--package", required=True, type=Path)
    parser.add_argument("--build", required=True)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--release-tag")
    args = parser.parse_args()
    manifest = finalize_update(args.config, args.package, args.build, args.output, args.release_tag)
    print(f"Signed update {manifest['version']}")
    print(f"Release URL: {manifest['url']}")


if __name__ == "__main__":
    main()
