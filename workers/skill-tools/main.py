"""Bounded file protocol around the unchanged subtitle-skill CLI.

The checker reports findings; it never translates, writes a project, or calls a
model. Packaging keeps its configuration at the original relative location.
Its own errors are stable E_ codes that the desktop says in its language.
"""
import contextlib
import io
import json
import os
from pathlib import Path
import sys

import check_subtitle


def main():
    request_id = ""
    try:
        raw = sys.stdin.buffer.read(1024 * 1024 + 1)
        if len(raw) > 1024 * 1024:
            raise ValueError("E_CONTROL_SIZE")
        request = json.loads(raw)
        request_id = request["requestId"]
        if request["protocol"] != 1 or request["operation"] != "check":
            raise ValueError("E_PROTOCOL")
        for key in ("inputPath", "sourcePath"):
            path = Path(request[key])
            if not path.is_file() or path.stat().st_size > 64 * 1024 * 1024:
                raise ValueError("E_INPUT")
        sys.argv = ["check_subtitle", request["inputPath"], "--source", request["sourcePath"],
                    "--lang", request["language"], "--lang-config", request["languageConfig"], "--json"]
        if request.get("strict", True):
            sys.argv.append("--strict")
        output = io.StringIO()
        with contextlib.redirect_stdout(output):
            code = check_subtitle.main()
        if code not in (0, 1):
            raise ValueError("E_RESULT")
        findings = json.loads(output.getvalue())
        with open(request["outputPath"], "x", encoding="utf-8", newline="\n") as file:
            json.dump(findings, file, ensure_ascii=False)
            file.flush()
            os.fsync(file.fileno())
        print(json.dumps({"protocol": 1, "requestId": request_id, "ok": True}))
        return 0
    except (Exception, SystemExit) as error:
        print(json.dumps({"protocol": 1, "requestId": request_id, "ok": False,
                          "error": str(error)}, ensure_ascii=False))
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
