"""Real Windows retranslation workflow against an isolated local SSE service."""
import argparse
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
import subprocess
import threading
import time


class Provider(BaseHTTPRequestHandler):
    requests = []

    def do_POST(self):
        request = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        assert request["model"] == "local-retranslation-test"
        payload = json.loads(request["messages"][1]["content"])
        ids = [unit["id"] for unit in payload["units"]]
        Provider.requests.append(ids)
        round_number = len(Provider.requests)
        if round_number == 4:
            self.send_error(401, "Deliberate local test failure")
            return
        texts = ["第一条译文", "第二条译文", "第三条译文"]
        prefix = {1: "原来的", 2: "新的", 3: "再次翻译的"}[round_number]
        result = {unit["id"]: prefix + texts[int(unit["id"]) - 1] for unit in payload["units"]}
        # Leave time to close and reopen the actual modal while work continues.
        if round_number == 2:
            time.sleep(2)
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
    parser.add_argument("--packaged", action="store_true")
    args = parser.parse_args()
    server = ThreadingHTTPServer(("127.0.0.1", 0), Provider)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        script = Path(__file__).with_suffix(".ps1").resolve()
        invoke = ("[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false); "
                  f"& ([scriptblock]::Create([IO.File]::ReadAllText({literal(script)}))) "
                  f"-Exe {literal(args.exe)} -Work {literal(args.work)} "
                  f"-ScreenshotTool {literal(args.screenshot_tool)} "
                  f"-Endpoint 'http://127.0.0.1:{server.server_port}/v1'")
        if args.packaged:
            invoke += " -Packaged"
        subprocess.run(["powershell", "-NoProfile", "-Command", invoke], check=True)
        assert Provider.requests == [["1", "2", "3"], ["1", "3"], ["1"], ["1", "3"]], Provider.requests
        print("PASS: real desktop select/clear, close/reopen during translation, retry/failure, replace-one, undo and stale protection; four local requests")
    finally:
        server.shutdown()
        thread.join()
        server.server_close()
