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
MODULE = "github.com/egoist/mygo@v0.9.1-0.20261114153143-203930fb2d43"
BASE_HASHES = {
    "internal/windows/menu.go": "4d88c3541ec923c6ff9c29cac4043964f2c51f53482679b5302d8b3d1d318401",
    "internal/windows/menudraw.go": "4b3e9006379538d4739dd7a5cc8189cef7718742ccc3c7054f7a234aff78f7b6",
    "internal/windows/menudraw_test.go": "bb929267efa4ac53c1e901bde6f6c5a5b753ae38feb1cd058dda583410b729d7",
    "internal/windows/surface.go": "534109a150e60a40e2be21db42a7c03ee1b8a1df1a600fef647a62c40ccd2bc0",
    "internal/windows/webview_embed.go": "06085e032031434790128240cc3fd1f9967aa9c8812744266eaa712aa85fb310",
    "ui/editor.go": "e7c4b6284820e956b96866f555ea7cede753fd4a7714e52f6b410e65f66d0a28",
    "ui/editor_layout.go": "e6bec845539d9d669d5f6a39f740953812fcecfb1c1b6d292f625a880ba33379",
    "ui/headless.go": "01ebb3eb94580b964af67e1fccab3021a27e83b4580d934873b2e71eee86946b",
    "ui/sidebar.go": "e2a6fe527d0ee57bdb1ef58eecc1f014f26ee023ad39bdce908d216e2ab593d3",
    "ui/list.go": "ac4d0e964325895493aff0d1354c092cbb93f5e3e87db7b3776919135b261aee",
    "ui/value_input.go": "8dd5db4a0af7e86e7abecffc482f3dff27295b68836ff86cbe3d2e519c1fa793",
    "ui/webview.go": "4d7861d4631df4b9488fc8aa950dff80633009bd8eb647f5dc6c572b446eed55",
    "ui/textinput.go": "27594382dcffb32d2aab5c16de118513be397d0428987394fb34b4a690f2c542",
    "ui/tooltip.go": "5c2678202046274a90e8c4a0119e7afc6a6953eebad3ae0360cc77cee637133d",
    "ui/table.go": "3f1cc10f9e900e2ae8ad60d096b0118fe4595611d7b5d303e4a20bc19b4fcf0e",
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
    relative = ".local/mygo-menu-source-v0.9.1-0.20261114153143-203930fb2d43"
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
