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
MODULE = "github.com/egoist/mygo@v0.7.4"
BASE_HASHES = {
    "internal/windows/menu.go": "5ce380467e2c7dfea6630c48b2fe4959e6636ceb0f9ee1b5ca6b0b5723215689",
    "internal/windows/menudraw.go": "9b8fba772f929529772d14974da158e0ce9074818c687172be417a590dd5494a",
    "internal/windows/menudraw_test.go": "aed92f41e8f97b0b17668e3643d829a068f75b447eb0514e5112e7a820014edf",
    "internal/windows/surface.go": "b540e4c3108de04773101deecac90c88d758c19fef3f9e7a455f0cd357d1c2da",
    "internal/windows/webview_embed.go": "8c85adf2755182b775f16b339169c3a95c93878b0b1b7cab8040aa917ce28156",
    "ui/editor.go": "ccd47423123eec58ef45adc1e3cfdf50d031bf06f8f02b1ff3ab4a84aa94f852",
    "ui/editor_layout.go": "7d4240adf339db927a96a2deba10640e35d683f1a982d29a65fb6e9c1b82112e",
    "ui/headless.go": "f6b8e571f8a2707ea2a088c314238569d09ccf0996e2395f58f9cd931f74c4c9",
    "ui/sidebar.go": "2ca70ae13ff9c049be93c930567610f814ee05a178d05774a201b549ccd05bba",
    "ui/list.go": "29438b0610a27eac698e614d932103318bface2550c2c867d9c13a7579e53289",
    "ui/value_input.go": "c1006d34b8d8c886940fc45a494aedd0af861f8d917c146e8d955b17e85913a9",
    "ui/webview.go": "dab6a5a5acb13b73cc6a5a4735ff37b384168b0026e64974e153bfeedcaf6637",
    "ui/textinput.go": "ff198994adad8c2a820c209a8638709ec0ef40bb4c3f58c1cb45e36af93590fc",
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
    relative = ".local/mygo-menu-source"
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
