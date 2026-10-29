"""Desktop transport fixture, not a translation engine or quality benchmark."""
import argparse
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
import subprocess
import threading
import time

class Provider(BaseHTTPRequestHandler):
    requests = 0
    def do_GET(self):
        if self.path != "/v1/models":
            self.send_error(404)
            return
        body = json.dumps({"data": [{"id": "local-protocol-test"}]}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):
        if self.path != "/v1/chat/completions":
            self.send_error(404)
            return
        request = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        assert request["model"] == "local-protocol-test"
        assert request["stream"] is True
        assert "# Operation: translate\n" in request["messages"][0]["content"]
        data = json.loads(request["messages"][1]["content"])
        content = {unit["id"]: "你好，世界" for unit in data["units"]}
        Provider.requests += 1
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.end_headers()
        # Deliberately deliver real SSE chunks slowly enough for the desktop
        # smoke test to observe progress before the checked file is published.
        for char in json.dumps(content, ensure_ascii=False):
            chunk = {"choices": [{"index": 0, "delta": {"content": char}}]}
            self.wfile.write(("data: " + json.dumps(chunk) + "\n\n").encode())
            self.wfile.flush()
            time.sleep(2 if char == "好" else 0.15)
        self.wfile.write(b'data: {"choices":[{"delta":{},"finish_reason":"stop"}]}\n\n')
        self.wfile.write(b'data: {"choices":[],"usage":{"prompt_tokens":1200,"completion_tokens":40}}\n\n')
        self.wfile.write(b'data: [DONE]\n\n')
        self.wfile.flush()
    def log_message(self, *args):
        pass

if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--exe", required=True)
    parser.add_argument("--work", required=True)
    parser.add_argument("--screenshot-tool", required=True)
    parser.add_argument("--packaged", action="store_true")
    args = parser.parse_args()
    server = ThreadingHTTPServer(("127.0.0.1", 0), Provider)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        # Windows PowerShell 5 decodes BOM-less .ps1 as the system code page.
        # Execute the repository's UTF-8 source explicitly without changing it.
        def literal(value):
            return "'" + str(value).replace("'", "''") + "'"
        script = Path(__file__).with_name("native_smoke.ps1").resolve()
        invocation = (f"& ([scriptblock]::Create([IO.File]::ReadAllText({literal(script)}))) "
                      f"-Exe {literal(args.exe)} -Work {literal(args.work)} "
                      f"-ScreenshotTool {literal(args.screenshot_tool)} -OpenArgument "
                      f"-TranslationEndpoint 'http://127.0.0.1:{server.server_port}/v1'")
        if args.packaged:
            invocation += " -Packaged"
        command = ["powershell", "-NoProfile", "-Command", invocation]
        result = subprocess.run(command)
        if result.returncode:
            raise SystemExit(result.returncode)
        assert Provider.requests == 1, Provider.requests
        print("PASS: desktop used exactly one explicitly configured local request; no cloud quality claim")
    finally:
        server.shutdown()
        thread.join()
        server.server_close()
