"""Prepare the pinned MyGo source patch without modifying the module cache.

Go forbids overlays below GOMODCACHE. A generated -modfile points only MyGo
at an ignored patched source copy; the checked-in go.mod/go.sum remain pinned.
"""
import hashlib
import os
from pathlib import Path
import shutil
import subprocess

ROOT = Path(__file__).resolve().parents[1]
MODULE = "github.com/egoist/mygo@v0.8.3"
BASE_HASHES = {
    "internal/windows/menu.go": "355564d0d9a6d34b96557d9e71a1109fdd65a7166cc6c84929797ea922ddbf96",
    "internal/windows/menudraw.go": "4b3e9006379538d4739dd7a5cc8189cef7718742ccc3c7054f7a234aff78f7b6",
    "internal/windows/menudraw_test.go": "bb929267efa4ac53c1e901bde6f6c5a5b753ae38feb1cd058dda583410b729d7",
    "internal/windows/surface.go": "81de18fd3d2a299abb27c38414093cc39bdba979c1a95d7fbe7279d901ac0a59",
    "internal/windows/webview_embed.go": "8e8048672152b4eb7a21b60a34495ddd6ea7ccf758fb1786ebdb16981b0caf2e",
    "ui/editor.go": "89d34e1993be3fb6751a4d8ffe0ca4cebb13ec3700d9ba0323fded728765789c",
    "ui/editor_layout.go": "bc53c797b69b52fa6bf01be3b15a1fc6eec01241e9f31289a8a81c8a6b8aeeb7",
    "ui/headless.go": "a1fb07bf39700ec6be6add85bd4fad49104841f1157b66da1a3423c3b980dfe5",
    "ui/sidebar.go": "e2a6fe527d0ee57bdb1ef58eecc1f014f26ee023ad39bdce908d216e2ab593d3",
    "ui/list.go": "e10dcfe58e256a43f181650c7d0094f0b01133bc4e5f4d0712c3aaecfc042f9a",
    "ui/value_input.go": "d1aac3c4a8428821692fd3f01aeb6d12e73f5da6ecc46d973ffc0eaa9619654c",
    "ui/webview.go": "57d637898c762902d818bced327f09b23e900336380ea4c5c653b19cb2897949",
    "ui/textinput.go": "ec45c181c6fbcd08885381c0cb48f792a3db18de74f2be3ceb84b9a29dd71680",
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
            shutil.copyfile(Path(folder) / name, target / name)


def prepare():
    source = ROOT / ".local/gomodcache" / MODULE
    patches = [ROOT / "third_party/patches/mygo" / name for name in
               ("windows-native.patch", "ui-interaction.patch", "ui-layout.patch", "ui-row-click.patch", "ui-sidebar.patch")]
    relative = ".local/mygo-menu-source-v0.8.3"
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
        for name in ("onesub_interaction_test.go", "onesub_layout_test.go", "onesub_sidebar_test.go"):
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
    subprocess.run(
        ["go", "mod", "edit", "-modfile=" + str(modfile),
         "-replace=github.com/egoist/mygo=" + str(output)], cwd=ROOT / "apps/desktop", check=True,
    )
    print(modfile)


if __name__ == "__main__":
    prepare()
