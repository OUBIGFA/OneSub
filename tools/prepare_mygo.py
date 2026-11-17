"""Prepare the pinned MyGo source patch without modifying the module cache.

Go forbids overlays below GOMODCACHE. A generated -modfile points only MyGo
at an ignored patched source copy; the checked-in go.mod/go.sum remain pinned.
"""
import hashlib
import json
import os
from pathlib import Path
import shutil
import stat
import subprocess

ROOT = Path(__file__).resolve().parents[1]
MODULE = "github.com/egoist/mygo@v0.9.3-0.20261116184738-6fbc3c535009"
BASE_HASHES = {
    "cmd/mygo/nsis.go": "1a666c780024f42d80b5659e251e519aa21a7727bfdff809c019add37b4b1adb",
    "internal/windows/menu.go": "92b12c4874b6c5795dfa2c9adc3ff82e1f314dfab9264fafd6df3c5bd6531424",
    "internal/windows/menudraw.go": "4b3e9006379538d4739dd7a5cc8189cef7718742ccc3c7054f7a234aff78f7b6",
    "internal/windows/menudraw_test.go": "bb929267efa4ac53c1e901bde6f6c5a5b753ae38feb1cd058dda583410b729d7",
    "internal/windows/surface.go": "b6c66aff316f09bc53dc9362a180ccb5d47ccc7151a01ebf2254db28a797b218",
    "internal/windows/webview_embed.go": "320bd425677a94525861fd4b16458831605078eaab01dcc72107d626020cc5ce",
    "ui/editor.go": "5b39e1ec0a91f2652d4100b0fbc90ecc1ca1569b850d3a515b845f01ff98a8f3",
    "ui/editor_layout.go": "e6bec845539d9d669d5f6a39f740953812fcecfb1c1b6d292f625a880ba33379",
    "ui/headless.go": "f0f4729da3adb3b7813d952ba70ed06b42ea8f6d82318dd9f61ab2ac7459ea40",
    "ui/sidebar.go": "e2a6fe527d0ee57bdb1ef58eecc1f014f26ee023ad39bdce908d216e2ab593d3",
    "ui/list.go": "7aaa3c3d75558cca05c06ee656729d1acb4fedc5810447967cdfb5cd728fe3a3",
    "ui/value_input.go": "8dd5db4a0af7e86e7abecffc482f3dff27295b68836ff86cbe3d2e519c1fa793",
    "ui/webview.go": "8f04072c441be417431c077a9f29fbbb5b78c2dfb35528fb8eef9d7e554a486b",
    "ui/textinput.go": "27594382dcffb32d2aab5c16de118513be397d0428987394fb34b4a690f2c542",
    "ui/tooltip.go": "5c2678202046274a90e8c4a0119e7afc6a6953eebad3ae0360cc77cee637133d",
    "ui/table.go": "e4e6987b1f7684bc8e3027bc92dd4604be3bb195a2551d4b4a22416b5be16144",
}


def mirror(source, output):
    """Copy the module's files over output, adding any that are missing.

    Only contents are copied: the module cache's read-only attributes would
    keep the copy from being updated. Nothing is deleted, so an update needs
    no removal of the previous copy.
    """
    for folder, _, files in os.walk(source):
        target = output / Path(folder).relative_to(source)
        target.mkdir(parents=True, exist_ok=True)
        for name in files:
            destination = target / name
            # Older mirrors may retain the module cache's read-only bit.
            # Change only the owned working copy, never the verified source.
            if destination.exists():
                destination.chmod(destination.stat().st_mode | stat.S_IWRITE)
            shutil.copyfile(Path(folder) / name, destination)


def cached_files(output):
    """Hash the actual mirrored inputs, including files not touched by patches."""
    result = {}
    if not output.is_dir():
        return result
    for path in sorted(output.rglob("*")):
        if path.is_symlink():
            raise RuntimeError(f"Patched source contains a link: {path}")
        if path.is_file() and path.name != ".onesub-patch-sha256":
            with path.open("rb") as stream:
                result[path.relative_to(output).as_posix()] = hashlib.file_digest(stream, "sha256").hexdigest()
    return result


def cache_matches(output, fingerprint):
    """A matching patch marker alone cannot attest to locally edited sources."""
    stamp = output / ".onesub-patch-sha256"
    try:
        recorded = json.loads(stamp.read_text(encoding="utf-8"))
    except (OSError, ValueError):
        return False
    return isinstance(recorded, dict) and recorded.get("fingerprint") == fingerprint and recorded.get("files") == cached_files(output)


def record_cache(source, output, added, fingerprint):
    files = cached_files(output)
    expected = {path.relative_to(source).as_posix() for path in source.rglob("*") if path.is_file()} | set(added)
    # Do not bless or delete an unexpected source file left in a working copy.
    if set(files) != expected:
        raise RuntimeError("Patched source file list differs from upstream and registered additions")
    (output / ".onesub-patch-sha256").write_text(
        json.dumps({"fingerprint": fingerprint, "files": files}, sort_keys=True), encoding="utf-8")


def prepare_lobe():
    """Apply the kit's limited native integration without editing its cache."""
    manifest = json.loads((ROOT / "third_party/manifests/lobe-mygo.json").read_text(encoding="utf-8"))
    module = manifest["module"] + "@" + manifest["version"]
    source = ROOT / ".local/gomodcache" / module
    relative = ".local/lobe-mygo-source-" + manifest["version"]
    output = ROOT / relative
    for name, expected in manifest["sourceHashes"].items():
        if hashlib.sha256((source / name).read_bytes()).hexdigest() != expected:
            raise RuntimeError(f"Lobe MyGo source changed; review integration patch: {name}")
    patch = ROOT / manifest["patch"]
    fingerprint = hashlib.sha256(module.encode("ascii") + b"\0" + patch.read_bytes()).hexdigest()
    if not cache_matches(output, fingerprint):
        mirror(source, output)
        for name in manifest["addedFiles"]:
            (output / name).unlink(missing_ok=True)
        subprocess.run(["git", "-c", "core.autocrlf=false", "apply", "--whitespace=nowarn", "--directory=" + relative, str(patch)], cwd=ROOT, check=True)
        record_cache(source, output, manifest["addedFiles"], fingerprint)
    return output


def prepare():
    source = ROOT / ".local/gomodcache" / MODULE
    patches = [ROOT / "third_party/patches/mygo" / name for name in
               ("windows-native.patch", "ui-interaction.patch", "ui-layout.patch", "ui-row-click.patch", "ui-sidebar.patch", "ui-table-elements.patch", "installer-license.patch")]
    relative = ".local/mygo-menu-source-" + MODULE.rsplit("@", 1)[1]
    output = ROOT / relative
    for name, expected in BASE_HASHES.items():
        data = (source / name).read_bytes()
        if hashlib.sha256(data).hexdigest() != expected:
            raise RuntimeError(f"MyGo source changed; review native patches: {name}")
    # A new upstream version must refresh the mirror even when its patch text
    # is unchanged; the locked module and patches together identify the output.
    fingerprint = hashlib.sha256(MODULE.encode("ascii") + b"\0" + b"".join(p.read_bytes() for p in patches)).hexdigest()
    added = ["ui/" + name for name in ("onesub_interaction_test.go", "onesub_layout_test.go", "onesub_menu_tester.go", "onesub_sidebar_test.go")]
    if not cache_matches(output, fingerprint):
        # The pristine module, then the current patch on top of it.
        mirror(source, output)
        # Upstream files are reset by mirror. Reset this patch-added test too,
        # so a changed or interrupted patch can be applied again safely.
        for name in added:
            (output / name).unlink(missing_ok=True)
        for patch in patches:
            subprocess.run(
                ["git", "-c", "core.autocrlf=false", "apply", "--whitespace=nowarn", "--directory=" + relative, str(patch)],
                cwd=ROOT, check=True,
            )
        record_cache(source, output, added, fingerprint)
    modfile = ROOT / ".local/onesub-patched.mod"
    modfile.write_bytes((ROOT / "apps/desktop/go.mod").read_bytes())
    modfile.with_suffix(".sum").write_bytes((ROOT / "apps/desktop/go.sum").read_bytes())
    lobe = prepare_lobe()
    subprocess.run(
        ["go", "mod", "edit", "-modfile=" + str(modfile),
         "-replace=github.com/egoist/mygo=" + str(output),
         "-replace=github.com/lobehub/lobe-mygo=" + str(lobe)], cwd=ROOT / "apps/desktop", check=True,
    )
    print(modfile)


if __name__ == "__main__":
    prepare()
