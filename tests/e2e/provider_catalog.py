"""Local-only model discovery fixture for the real native desktop UI.

The script exercises model management and queue entry, not cloud translation
quality or mouse-driven OLE drag/drop. No credentials or external service.
"""
import argparse
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
import subprocess
import threading


class Catalog(BaseHTTPRequestHandler):
    requests = 0

    def do_GET(self):
        if self.path != "/v1/models":
            self.send_error(404)
            return
        Catalog.requests += 1
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
        if Catalog.requests < 2:
            raise AssertionError(f"Expected discovery and refresh, observed {Catalog.requests} GETs")
        print(f"PASS: desktop catalog discovery and refresh used {Catalog.requests} local GETs")
    finally:
        server.shutdown()
        thread.join()
        server.server_close()
