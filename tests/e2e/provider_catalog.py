"""Local-only model discovery fixture for the real native desktop UI.

The script exercises model management and queue entry, not cloud translation
quality or mouse-driven OLE drag/drop. No credentials or external service.
"""
import argparse
import ctypes
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
import subprocess
import threading


class Catalog(BaseHTTPRequestHandler):
    requests = 0

    def do_GET(self):
        if self.path == "/v1/test-state":
            body = json.dumps({"requests": Catalog.requests}).encode()
            self.send_response(200)
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return
        if self.path != "/v1/models":
            self.send_error(404)
            return
        Catalog.requests += 1
        if self.headers.get("Authorization") != "Bearer onesub-local-catalog-test":
            self.send_error(401)
            return
        if Catalog.requests == 1:
            self.send_error(503)
            return
        models = [{"id": name, "max_input_tokens": 96000, "max_output_tokens": 8192,
                   "context_window": 128000}
                  for name in ("catalog-alpha", "catalog-beta", "catalog-gamma")]
        models.extend({"id": f"catalog-model-{i:02d}"} for i in range(20))
        body = json.dumps({"data": models}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *_):
        pass


def literal(value):
    return "'" + str(value).replace("'", "''") + "'"


def clear_fixture_credentials(work, endpoint):
    """Remove only this run's generated account keys, including on failure."""
    settings = Path(work) / "data" / "settings.json"
    if not settings.exists():
        return
    providers = json.loads(settings.read_text(encoding="utf-8")).get("providers", [])
    delete = ctypes.WinDLL("advapi32.dll", use_last_error=True).CredDeleteW
    delete.argtypes = [ctypes.c_wchar_p, ctypes.c_uint32, ctypes.c_uint32]
    delete.restype = ctypes.c_int
    for provider in providers:
        url = provider.get("url", "").strip().rstrip("/")
        identity = provider.get("id", "")
        if not identity.startswith("native-catalog-test-") or url != endpoint:
            continue
        scope = f"provider:{identity}:{url}"
        target = "OneSub/provider/" + hashlib.sha256(scope.encode("utf-8")).hexdigest()
        if not delete(target, 1, 0) and ctypes.get_last_error() != 1168:
            raise ctypes.WinError(ctypes.get_last_error())


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--exe", required=True)
    parser.add_argument("--work", required=True)
    parser.add_argument("--screenshot-tool", required=True)
    parser.add_argument("--packaged", action="store_true")
    parser.add_argument("--appearance", choices=("dark", "light"), default="dark")
    args = parser.parse_args()
    server = ThreadingHTTPServer(("127.0.0.1", 0), Catalog)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        script = Path(__file__).with_name("native_smoke.ps1").resolve()
        # Explicit UTF-8 source loading also works with Windows PowerShell 5.
        invocation = (f"& ([scriptblock]::Create([IO.File]::ReadAllText({literal(script)}))) "
                      f"-Exe {literal(args.exe)} -Work {literal(args.work)} "
                      f"-ScreenshotTool {literal(args.screenshot_tool)} -OpenArgument -UIAPlayback -Appearance {literal(args.appearance)} "
                      f"-ProviderCatalogEndpoint 'http://127.0.0.1:{server.server_port}/v1'")
        if args.packaged:
            invocation += " -Packaged"
        result = subprocess.run(["powershell", "-NoProfile", "-Command", invocation])
        if result.returncode:
            raise SystemExit(result.returncode)
        if Catalog.requests != 3:
            raise AssertionError(f"Expected one failed click, retry and refresh only; observed {Catalog.requests} GETs")
        print(f"PASS: desktop catalog discovery and refresh used {Catalog.requests} local GETs")
    finally:
        try:
            clear_fixture_credentials(args.work, f"http://127.0.0.1:{server.server_port}/v1")
        finally:
            server.shutdown()
            thread.join()
            server.server_close()
