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
    interrupt = False
    clear_workbench = False
    subtitle_issues = False
    unit_requests = []
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
        content = {unit["id"]: ("早上好" if unit["id"] == "2" else "你好，世界") for unit in data["units"]}
        if Provider.clear_workbench and Provider.requests:
            content = {unit["id"]: "重新翻译的结果" for unit in data["units"]}
        Provider.requests += 1
        Provider.unit_requests.append([unit["id"] for unit in data["units"]])
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.end_headers()
        # Deliberately deliver real SSE chunks slowly enough for the desktop
        # smoke test to observe progress before the checked file is published.
        interrupted = Provider.interrupt and Provider.requests == 1
        first_text = '你好，世界' + (' \U0004e7fd' if Provider.subtitle_issues else '')
        if Provider.subtitle_issues and '1' in content:
            content['1'] = first_text
        text = '{"1":' + json.dumps(first_text, ensure_ascii=False) + ',"2":"尚未完成' if interrupted else json.dumps(content, ensure_ascii=False)
        for char in text:
            chunk = {"choices": [{"index": 0, "delta": {"content": char}}]}
            self.wfile.write(("data: " + json.dumps(chunk) + "\n\n").encode())
            self.wfile.flush()
            time.sleep(2 if char == "好" else 0.15)
        if interrupted:
            return  # Deliberate EOF without a completion event.
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
    parser.add_argument("--interrupt", action="store_true")
    parser.add_argument("--clear-workbench", action="store_true")
    parser.add_argument("--skip-translation", action="store_true")
    parser.add_argument("--subtitle-issues", action="store_true", help="Check resumed preview and editable issue locations")
    parser.add_argument("--pipeline-models-root")
    parser.add_argument("--pipeline-model", default="Qwen3-ASR-1.7B-hf")
    parser.add_argument("--uia-playback", action="store_true", help="Use UI Automation on a locked desktop; does not test Space")
    args = parser.parse_args()
    if args.clear_workbench and (args.interrupt or args.pipeline_models_root):
        parser.error("--clear-workbench must run as a separate scenario")
    if args.skip_translation and (args.interrupt or args.clear_workbench or args.pipeline_models_root):
        parser.error("--skip-translation must run as a separate scenario")
    Provider.interrupt = args.interrupt or args.skip_translation
    Provider.clear_workbench = args.clear_workbench
    Provider.subtitle_issues = args.subtitle_issues
    if args.subtitle_issues and not args.interrupt:
        parser.error("--subtitle-issues requires --interrupt")
    server = ThreadingHTTPServer(("127.0.0.1", 0), Provider)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        # Windows PowerShell 5 decodes BOM-less .ps1 as the system code page.
        # Execute the repository's UTF-8 source explicitly without changing it.
        def literal(value):
            return "'" + str(value).replace("'", "''") + "'"
        script = Path(__file__).with_name("private_native_smoke.ps1").resolve()
        # Retain the source filename so the private launcher can resolve its
        # driver and screenshot helper through PowerShell's PSScriptRoot.
        invocation = (f"$tokens=$null; $errors=$null; $script=[System.Management.Automation.Language.Parser]::ParseInput("
                      f"[IO.File]::ReadAllText({literal(script)}), {literal(script)}, [ref]$tokens, [ref]$errors); "
                      f"if($errors){{throw $errors}}; & $script.GetScriptBlock() "
                      f"-Exe {literal(args.exe)} -Work {literal(args.work)} "
                      f"-TranslationEndpoint 'http://127.0.0.1:{server.server_port}/v1'")
        if args.pipeline_models_root:
            invocation += f" -PipelineModelsRoot {literal(args.pipeline_models_root)} -AsrModel {literal(args.pipeline_model)}"
        if not args.packaged:
            invocation += " -Development"
        if args.interrupt:
            invocation += " -InterruptTranslation"
        if args.clear_workbench:
            invocation += " -ClearTranslation"
        if args.skip_translation:
            invocation += " -SkipTranslation -TaskTimings"
        if args.subtitle_issues:
            invocation += " -SubtitleIssues"
        command = ["powershell", "-NoProfile", "-Command", invocation]
        result = subprocess.run(command)
        if result.returncode:
            raise SystemExit(result.returncode)
        expected = 2 if args.interrupt or args.clear_workbench or args.skip_translation else 1
        if args.pipeline_models_root:
            assert Provider.requests >= 1, Provider.requests
            expected = Provider.requests
        else:
            assert Provider.requests == expected, Provider.requests
        if args.interrupt or args.skip_translation:
            assert Provider.unit_requests == [["1", "2"], ["2"]], Provider.unit_requests
        print(f"PASS: desktop used {expected} local requests; restart/resume={args.interrupt}; no cloud quality claim")
    finally:
        server.shutdown()
        thread.join()
        server.server_close()
