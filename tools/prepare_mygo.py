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
MODULE = "github.com/egoist/mygo@v0.8.4-0.20261109165344-5a2f9c65b92c"
BASE_HASHES = {
    "internal/windows/menu.go": "355564d0d9a6d34b96557d9e71a1109fdd65a7166cc6c84929797ea922ddbf96",
    "internal/windows/menudraw.go": "4b3e9006379538d4739dd7a5cc8189cef7718742ccc3c7054f7a234aff78f7b6",
    "internal/windows/menudraw_test.go": "bb929267efa4ac53c1e901bde6f6c5a5b753ae38feb1cd058dda583410b729d7",
    "internal/windows/surface.go": "1a91de2a2c0ece1bc0ca5a4a8a4f32beb9ab1467ef3ad777673e83fb9bfdb0d7",
    "internal/windows/webview_embed.go": "8e8048672152b4eb7a21b60a34495ddd6ea7ccf758fb1786ebdb16981b0caf2e",
    "ui/editor.go": "89d34e1993be3fb6751a4d8ffe0ca4cebb13ec3700d9ba0323fded728765789c",
    "ui/editor_layout.go": "84574fea7e59d9aade29d342f110b3bd7e09048bcf8ae23d9b125682e60b0a41",
    "ui/headless.go": "400e22307715f659c28fab7cdbe92e24e2253e88dcbaa2eca8f0a58c10a1a8d0",
    "ui/sidebar.go": "e2a6fe527d0ee57bdb1ef58eecc1f014f26ee023ad39bdce908d216e2ab593d3",
    "ui/list.go": "98192a780acba4067aac30c4b2fac3726d7bb74d18cd5690c874c376885e8bb3",
    "ui/value_input.go": "d1aac3c4a8428821692fd3f01aeb6d12e73f5da6ecc46d973ffc0eaa9619654c",
    "ui/webview.go": "4d7861d4631df4b9488fc8aa950dff80633009bd8eb647f5dc6c572b446eed55",
    "ui/textinput.go": "f2e45b5eae24f2f863786db2a06fb6c0729cba5e54a488d51e75abfb2058792f",
    "ui/tooltip.go": "5c2678202046274a90e8c4a0119e7afc6a6953eebad3ae0360cc77cee637133d",
    "ui/table.go": "614f977c2601570ade931811c222e75cfec3ced46699b149b38c7564a7705b6d",
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
    stamp = output / ".onesub-patch-sha256"
    if not stamp.exists() or stamp.read_text() != fingerprint:
        mirror(source, output)
        for name in manifest["addedFiles"]:
            (output / name).unlink(missing_ok=True)
        subprocess.run(["git", "apply", "--whitespace=nowarn", "--directory=" + relative, str(patch)], cwd=ROOT, check=True)
        stamp.write_text(fingerprint, encoding="ascii")
    return output


def prepare():
    source = ROOT / ".local/gomodcache" / MODULE
    patches = [ROOT / "third_party/patches/mygo" / name for name in
               ("windows-native.patch", "ui-interaction.patch", "ui-layout.patch", "ui-row-click.patch", "ui-sidebar.patch", "ui-table-elements.patch")]
    relative = ".local/mygo-menu-source-v0.8.4-0.20261109165344-5a2f9c65b92c"
    output = ROOT / relative
    for name, expected in BASE_HASHES.items():
        data = (source / name).read_bytes()
        if hashlib.sha256(data).hexdigest() != expected:
            raise RuntimeError(f"MyGo source changed; review native patches: {name}")
    stamp = output / ".onesub-patch-sha256"
    # A new upstream version must refresh the mirror even when its patch text
    # is unchanged; the locked module and patches together identify the output.
    fingerprint = hashlib.sha256(MODULE.encode("ascii") + b"\0" + b"".join(p.read_bytes() for p in patches)).hexdigest()
    if not stamp.exists() or stamp.read_text() != fingerprint:
        # The pristine module, then the current patch on top of it.
        mirror(source, output)
        # Upstream files are reset by mirror. Reset this patch-added test too,
        # so a changed or interrupted patch can be applied again safely.
        for name in ("onesub_interaction_test.go", "onesub_layout_test.go", "onesub_menu_tester.go", "onesub_sidebar_test.go"):
            (output / "ui" / name).unlink(missing_ok=True)
        for patch in patches:
            subprocess.run(
                ["git", "apply", "--whitespace=nowarn", "--directory=" + relative, str(patch)],
                cwd=ROOT, check=True,
            )
        stamp.write_text(fingerprint, encoding="ascii")
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
