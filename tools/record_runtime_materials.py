"""Record the exact locally tested runtimes; never downloads or approves a release.

Run deliberately when updating dependencies. build-windows checks these media
hashes rather than trusting whatever binary happens to be on PATH next time.
"""
import hashlib
import json
from pathlib import Path
import re
import shutil
import subprocess

ROOT = Path(__file__).resolve().parents[1]
LICENSES = ROOT / "third_party/licenses"


def artifact(path):
    return {"file": path.name, "bytes": path.stat().st_size,
            "sha256": hashlib.sha256(path.read_bytes()).hexdigest()}


def notice(source, name):
    text = source.read_text(encoding="utf-8-sig").replace("\r\n", "\n")
    target = LICENSES / name
    target.write_text(text, encoding="utf-8", newline="\n")
    return target.relative_to(ROOT).as_posix()


def main():
    manifest_path = ROOT / "third_party/manifests/runtimes.json"
    registered = json.loads(manifest_path.read_text(encoding="utf-8"))
    locked = re.search(r"(?m)^\s*(?:require\s+)?github\.com/egoist/mygo\s+(\S+)",
                       (ROOT / "apps/desktop/go.mod").read_text(encoding="utf-8"))
    if not locked or locked[1] != registered["mygo"]["version"]:
        raise ValueError("Review and register the locked MyGo version before refreshing runtimes")
    mygo_version = locked[1]
    LICENSES.mkdir(exist_ok=True)
    media = []
    for name in ("ffmpeg", "ffprobe"):
        path = Path(shutil.which(name) or "missing")
        version = subprocess.check_output([str(path), "-version"], text=True, encoding="utf-8")
        if "8.1-full_build-www.gyan.dev" not in version or "--enable-nonfree" in version:
            raise ValueError("unapproved FFmpeg build")
        media.append({**artifact(path), "version": version.splitlines()[0],
                      "configuration": version.splitlines()[2]})
    ffmpeg_license = notice(Path(shutil.which("ffmpeg")).parents[1] / "LICENSE", "ffmpeg.txt")
    mygo_license = notice(ROOT / f".local/gomodcache/github.com/egoist/mygo@{mygo_version}/LICENSE", "mygo.txt")
    libse_license = notice(ROOT / "third_party/sources/subtitleedit-e316223ca1cfe8861acb4da4cd0e4ef5cce634f9/src/libse/LICENSE.txt", "libse.txt")
    node = ROOT / "apps/desktop/player/node_modules"
    player = []
    reviewed_player = {entry["name"]: entry for entry in registered.get("player", [])}
    for name in ("jassub", "abslink", "lfa-ponyfill", "rvfc-polyfill", "throughput"):
        package = node / name
        meta = json.loads((package / "package.json").read_text(encoding="utf-8"))
        license_path = next((package / filename for filename in ("LICENSE", "LICENSE.md", "license", "LICENSE.txt") if (package / filename).is_file()), None)
        reviewed = reviewed_player.get(name, {})
        if reviewed.get("version") != meta["version"] or reviewed.get("spdx") != meta.get("license"):
            reviewed = {}
        if license_path:
            license_notice = notice(license_path, name + ".txt")
        else:
            license_notice = reviewed.get("notice")
            if license_notice:
                reviewed_path = ROOT / license_notice
                digest = hashlib.sha256(reviewed_path.read_bytes()).hexdigest()
                if digest != reviewed.get("noticeSha256"):
                    raise ValueError(f"Reviewed player notice changed: {name}")
        player.append({**reviewed, "name": name, "version": meta["version"], "spdx": meta.get("license"),
                       "notice": license_notice})
    manifest = {
        # This tool refreshes measured artifacts, not reviewed provenance or
        # registrations maintained by other dependency-specific build entries.
        **registered,
        "channel": "local-test",
        "publicReleaseApproved": False,
        "mygo": {**registered["mygo"], "license": mygo_license},
        "libse": {**registered.get("libse", {}), "revision": "e316223ca1cfe8861acb4da4cd0e4ef5cce634f9", "license": libse_license},
        "media": media,
        "mediaSource": "https://www.gyan.dev/ffmpeg/builds/",
        "mediaLicense": ffmpeg_license,
        "oneasr": {**registered.get("oneasr", {}), **artifact(ROOT / "build/output/oneasr/oneasr-cli.exe"), "version": "v1.3.0",
                   "archiveSha256": "e9e169e108ee43d88b5b047f4ec556819cd7fa5e2424d3f5ed957634f85d4936"},
        "player": player,
        "wasm": [artifact(path) for path in sorted((node / "jassub/dist/wasm").glob("*.wasm"))],
        "fontRedistribution": "No system font files are copied into the installer. Runtime preview uses the user's installed fonts through local capability URLs."
    }
    manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


if __name__ == "__main__":
    main()
