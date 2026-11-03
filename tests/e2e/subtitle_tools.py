"""Local provider and packaged desktop checks; not an AI quality evaluation."""
import argparse
import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
import subprocess
import threading


class Provider(BaseHTTPRequestHandler):
    requests = 0

    def do_POST(self):
        request = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        units = json.loads(request["messages"][1]["content"])["units"]
        Provider.requests += 1
        texts = ["首先说明字幕处理的完整步骤。接下来仔细检查字幕内容，然后继续完成全部操作，最后保存字幕结果。", "字幕。", "最后检查所有输入，然后保存全部字幕结果。"]
        result = {u["id"]: texts[i % len(texts)] for i, u in enumerate(units)}
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.end_headers()
        data = {"choices": [{"delta": {"content": json.dumps(result, ensure_ascii=False)}}]}
        self.wfile.write(("data: " + json.dumps(data) + "\n\n").encode())
        self.wfile.write(b'data: {"choices":[{"delta":{},"finish_reason":"stop"}]}\n\ndata: [DONE]\n\n')
        self.wfile.flush()

    def log_message(self, *_):
        pass


def literal(value):
    return "'" + str(value).replace("'", "''") + "'"


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--exe", required=True)
    parser.add_argument("--work", required=True)
    parser.add_argument("--screenshot-tool", required=True)
    parser.add_argument("--appearance", choices=("dark", "light"), default="dark")
    args = parser.parse_args()
    server = ThreadingHTTPServer(("127.0.0.1", 0), Provider)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        script = Path(__file__).with_suffix(".ps1").resolve()
        invoke = (f"& ([scriptblock]::Create([IO.File]::ReadAllText({literal(script)}))) "
                  f"-Exe {literal(args.exe)} -Work {literal(args.work)} "
                  f"-ScreenshotTool {literal(args.screenshot_tool)} -NativeHelpersPath {literal(Path(__file__).with_name('native_smoke.ps1').resolve())} -Appearance {literal(args.appearance)} "
                  f"-Endpoint 'http://127.0.0.1:{server.server_port}/v1'")
        subprocess.run(["powershell", "-NoProfile", "-Command", invoke], check=True)
        assert Provider.requests == 1, Provider.requests
        print("PASS: one local translation request; split/replace/punctuation made no network calls")
    finally:
        server.shutdown()
        thread.join()
        server.server_close()
